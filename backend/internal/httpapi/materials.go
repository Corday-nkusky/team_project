package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"

	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

const (
	// maxUploadBytes 是材料正文的硬上限（32 MiB，spec R3.4）。
	maxUploadBytes = 32 << 20
	// maxUploadRequest 预留 JSON 转义与 multipart 边界开销，避免 32 MiB 正文被误伤。
	maxUploadRequest = maxUploadBytes + (1 << 20)
)

type materialView struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	IndexStatus string    `json:"index_status"`
	IndexError  string    `json:"index_error,omitempty"`
	OwnerID     int64     `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func materialViewOf(m store.Material) materialView {
	return materialView{
		ID: m.ID, Title: m.Title, IndexStatus: m.IndexStatus,
		IndexError: m.IndexError, OwnerID: m.OwnerID, CreatedAt: m.CreatedAt,
	}
}

// materialDetailView 是单篇材料读取的响应；正文只在这里返回，列表不携带。
type materialDetailView struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	IndexStatus string    `json:"index_status"`
	IndexError  string    `json:"index_error,omitempty"`
	OwnerID     int64     `json:"owner_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func materialDetailOf(m store.Material) materialDetailView {
	return materialDetailView{
		ID: m.ID, Title: m.Title, Content: m.Content, IndexStatus: m.IndexStatus,
		IndexError: m.IndexError, OwnerID: m.OwnerID, CreatedAt: m.CreatedAt,
	}
}

// handleCreateMaterial 处理 POST /api/materials（仅教师，R3.1 / R3.2 / R2.3）。
//
// 支持两种提交：
//   - multipart/form-data：字段 `file`（txt / md / markdown / pdf）与可选 `title`
//   - application/json：{"title":..., "content":...}
//
// 所有输入边界都在落库前校验，失败不产生材料与任务（spec R3.4）。
// 落库与任务入队在同一个事务里完成；建索引由独立 worker 执行，请求内不调用 qa.Index。
func (s *Server) handleCreateMaterial(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	ctx := r.Context()

	title, content, err := readMaterialInput(r)
	if err != nil {
		writeDecodeErr(w, err, "上传内容不合法")
		return
	}
	title = strings.TrimSpace(title)
	if title == "" {
		title = "未命名材料"
	}
	if utf8.RuneCountInString(title) > 255 {
		writeErr(w, http.StatusBadRequest, "title_too_long", "标题不能超过 255 个字符")
		return
	}
	// 空内容（或仅空白）一律拒绝且不落库（R3.2）。
	if strings.TrimSpace(content) == "" {
		writeErr(w, http.StatusBadRequest, "empty_content", "材料内容不能为空")
		return
	}
	if len(content) > maxUploadBytes {
		writeErr(w, http.StatusRequestEntityTooLarge, "payload_too_large", "材料正文不能超过 32 MiB")
		return
	}

	// 事务性发件箱：材料与 pending 任务要么一起提交，要么一起回滚。
	id, taskID, err := s.st.CreateMaterialWithTask(ctx, c.ClassID, c.UserID, title, content,
		requestIDFrom(r.Context()))
	if err != nil {
		logServerError(r, "create_material_with_task", err)
		writeErr(w, http.StatusInternalServerError, "internal", "提交失败")
		return
	}

	detail := "材料:" + title + ";task:" + taskID
	if err := s.st.Audit(ctx, &c.UserID, c.Name, &c.ClassID, "upload", "ok", detail); err != nil {
		logServerError(r, "audit_upload", err)
	}
	writeJSON(w, http.StatusAccepted, s.acceptedView(r, c.ClassID, taskID, id))
}

// handleListMaterials 处理 GET /api/materials：只列本班材料（R2.1）。
func (s *Server) handleListMaterials(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	items, err := s.st.ListMaterials(r.Context(), c.ClassID)
	if err != nil {
		logServerError(r, "list_materials", err)
		writeErr(w, http.StatusInternalServerError, "internal", "查询失败")
		return
	}
	out := make([]materialView, 0, len(items))
	for _, m := range items {
		out = append(out, materialViewOf(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"materials": out})
}

// handleGetMaterial 处理 GET /api/materials/{id}（MR1）：
// 本班教师与学生都可读取原文；跨班与不存在统一 404，避免泄漏目标存在性。
func (s *Server) handleGetMaterial(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "材料 id 不合法")
		return
	}
	m, err := s.st.GetMaterial(r.Context(), id, c.ClassID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "材料不存在")
		return
	}
	if err != nil {
		logServerError(r, "get_material_detail", err)
		writeErr(w, http.StatusInternalServerError, "internal", "查询失败")
		return
	}
	writeJSON(w, http.StatusOK, materialDetailOf(m))
}

// handleReindex 处理 POST /api/materials/{id}/reindex（仅教师，R3.3）。
// 与上传共用同一任务框架：入队或重放终态任务后返回 202，不在请求内建索引。
func (s *Server) handleReindex(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "材料 id 不合法")
		return
	}
	if _, err := s.st.GetMaterial(r.Context(), id, c.ClassID); err != nil { // 跨班视为不存在（R2.1）
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not_found", "材料不存在")
			return
		}
		logServerError(r, "get_material", err)
		writeErr(w, http.StatusInternalServerError, "internal", "查询失败")
		return
	}

	taskID, err := s.st.EnqueueReindex(r.Context(), id, c.ClassID, requestIDFrom(r.Context()))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "材料不存在")
		return
	}
	if err != nil {
		logServerError(r, "enqueue_reindex", err)
		writeErr(w, http.StatusInternalServerError, "internal", "提交失败")
		return
	}
	writeJSON(w, http.StatusAccepted, s.acceptedView(r, c.ClassID, taskID, id))
}

// readMaterialInput 从 multipart 或 JSON 请求中取出标题与正文，必要时抽取 PDF 文本。
// 返回的 error 均为 *statusError，由 writeDecodeErr 映射为 4xx。
func readMaterialInput(r *http.Request) (title, content string, err error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		return readMultipartMaterial(r)
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := decodeUploadJSON(r, &req); err != nil {
		return "", "", err
	}
	if len(req.Content) > maxUploadBytes {
		return "", "", payloadTooLarge("材料正文不能超过 32 MiB")
	}
	return req.Title, req.Content, nil
}

// decodeUploadJSON 与 decodeJSON 规则一致，但使用上传体积上限，
// 并在解码前拒绝非 UTF-8 的请求体（spec R3.4 invalid_encoding）。
func decodeUploadJSON(r *http.Request, v any) error {
	if err := requireJSONContentType(r); err != nil {
		return err
	}
	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxUploadRequest))
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return payloadTooLarge("上传内容超过 32 MiB")
		}
		return badRequest("bad_request", "读取请求体失败")
	}
	if !utf8.Valid(body) {
		return badRequest("invalid_encoding", "请求体必须是 UTF-8 编码")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(v); err != nil {
		return normalizeDecodeError(err)
	}
	if _, err := dec.Token(); err == nil {
		return errTrailingJSON
	} else if !errors.Is(err, io.EOF) {
		return errTrailingJSON
	}
	return nil
}

// readMultipartMaterial 解析表单并抽取文本；体积、类型、编码均在此校验。
func readMultipartMaterial(r *http.Request) (title, content string, err error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxUploadRequest)
	// 8 MiB 内存阈值：更大的文件落临时盘，最终仍受总请求上限约束。
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return "", "", payloadTooLarge("上传内容超过 32 MiB")
		}
		return "", "", badRequest("bad_request", "解析上传表单失败")
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	title = r.FormValue("title")
	file, header, ferr := r.FormFile("file")
	if ferr != nil {
		return "", "", badRequest("bad_request", "缺少 file 字段")
	}
	defer file.Close()

	data, rerr := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if rerr != nil {
		var maxErr *http.MaxBytesError
		if errors.As(rerr, &maxErr) {
			return "", "", payloadTooLarge("上传内容超过 32 MiB")
		}
		return "", "", badRequest("bad_request", "读取上传文件失败")
	}
	if len(data) > maxUploadBytes {
		return "", "", payloadTooLarge("文件不能超过 32 MiB")
	}
	if header == nil || header.Filename == "" {
		return "", "", badRequest("bad_request", "缺少文件名")
	}
	if strings.TrimSpace(title) == "" {
		title = strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	}
	text, terr := extractText(header.Filename, data)
	if terr != nil {
		return "", "", terr
	}
	return title, text, nil
}

// extractText 按扩展名抽取文本；PDF 用纯 Go 解析，其余按 UTF-8 文本处理。
func extractText(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", badRequest("bad_request", "PDF 解析失败（可能为扫描件或加密文档）")
		}
		pr, err := reader.GetPlainText()
		if err != nil {
			return "", badRequest("bad_request", "PDF 文本抽取失败")
		}
		b, err := io.ReadAll(pr)
		if err != nil {
			return "", badRequest("bad_request", "PDF 文本读取失败")
		}
		text := string(b)
		if strings.TrimSpace(text) == "" {
			return "", badRequest("empty_content", "PDF 未包含可提取文本（扫描件需先 OCR）")
		}
		if !utf8.ValidString(text) {
			return "", badRequest("invalid_encoding", "PDF 抽取文本不是合法 UTF-8")
		}
		return text, nil
	case ".txt", ".md", ".markdown":
		if !utf8.Valid(data) {
			return "", badRequest("invalid_encoding", "材料正文必须是 UTF-8 编码")
		}
		return string(data), nil
	default:
		return "", unsupportedFileType("仅支持 .txt / .md / .markdown / .pdf 文件")
	}
}

// unsupportedFileType 使用独立错误码区分「内容类型不符」与「文件类型不支持」（spec R3.4）。
func unsupportedFileType(message string) error {
	return &statusError{
		status:  http.StatusUnsupportedMediaType,
		code:    "unsupported_file_type",
		message: message,
	}
}
