package httpapi

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"

	"github.com/Corday-nkusky/team_project/backend/internal/store"
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

// handleCreateMaterial 处理 POST /api/materials（仅教师，R3.1 / R3.2 / R2.3）。
//
// 支持两种提交：
//   - multipart/form-data：字段 `file`（txt / md / pdf）与可选 `title`
//   - application/json：{"title":..., "content":...}
func (s *Server) handleCreateMaterial(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	ctx := r.Context()

	title, content, err := readMaterialInput(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	// 空内容（或仅空白）一律拒绝且不落库（R3.2）。
	if strings.TrimSpace(content) == "" {
		writeErr(w, http.StatusBadRequest, "empty_content", "材料内容不能为空")
		return
	}
	if strings.TrimSpace(title) == "" {
		title = "未命名材料"
	}

	id, err := s.st.CreateMaterial(ctx, c.ClassID, c.UserID, title, content)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "落库失败")
		return
	}

	// 建索引；失败则标记 failed 而非静默成功（R3.3）。
	status, indexErr := "ready", ""
	if err := s.qa.Index(ctx, id, c.ClassID, content); err != nil {
		status, indexErr = "failed", err.Error()
	}
	_ = s.st.SetMaterialStatus(ctx, id, status, indexErr)

	_ = s.st.Audit(ctx, &c.UserID, c.Name, &c.ClassID, "upload", resultOf(status), "材料:"+title)
	writeJSON(w, http.StatusCreated, materialView{
		ID: id, Title: title, IndexStatus: status, IndexError: indexErr, OwnerID: c.UserID, CreatedAt: time.Now(),
	})
}

func resultOf(status string) string {
	if status == "ready" {
		return "ok"
	}
	return "fail"
}

// handleListMaterials 处理 GET /api/materials：只列本班材料（R2.1）。
func (s *Server) handleListMaterials(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	items, err := s.st.ListMaterials(r.Context(), c.ClassID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "查询失败")
		return
	}
	out := make([]materialView, 0, len(items))
	for _, m := range items {
		out = append(out, materialViewOf(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"materials": out})
}

// handleReindex 处理 POST /api/materials/{id}/reindex（仅教师，R3.3）。
func (s *Server) handleReindex(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "材料 id 不合法")
		return
	}
	m, err := s.st.GetMaterial(r.Context(), id, c.ClassID) // 跨班视为不存在（R2.1）
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "材料不存在")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "查询失败")
		return
	}

	status, indexErr := "ready", ""
	if err := s.qa.Index(r.Context(), m.ID, m.ClassID, m.Content); err != nil {
		status, indexErr = "failed", err.Error()
	}
	_ = s.st.SetMaterialStatus(r.Context(), m.ID, status, indexErr)
	writeJSON(w, http.StatusOK, materialView{
		ID: m.ID, Title: m.Title, IndexStatus: status, IndexError: indexErr, OwnerID: m.OwnerID, CreatedAt: m.CreatedAt,
	})
}

// readMaterialInput 从 multipart 或 JSON 请求中取出标题与正文，必要时抽取 PDF 文本。
func readMaterialInput(r *http.Request) (title, content string, err error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return "", "", errors.New("解析上传表单失败")
		}
		title = r.FormValue("title")
		file, header, ferr := r.FormFile("file")
		if ferr != nil {
			return "", "", errors.New("缺少 file 字段")
		}
		defer file.Close()
		data, rerr := io.ReadAll(io.LimitReader(file, 32<<20))
		if rerr != nil {
			return "", "", errors.New("读取上传文件失败")
		}
		if title == "" && header != nil {
			title = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
		}
		text, perr := extractText(header.Filename, data)
		if perr != nil {
			return "", "", perr
		}
		return title, text, nil
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &req); err != nil {
		return "", "", errors.New("请求体格式不正确")
	}
	return req.Title, req.Content, nil
}

// extractText 按扩展名抽取文本；PDF 用纯 Go 解析，其余按 UTF-8 文本处理。
func extractText(filename string, data []byte) (string, error) {
	if strings.EqualFold(filepath.Ext(filename), ".pdf") {
		reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", errors.New("PDF 解析失败（可能为扫描件或加密文档）")
		}
		pr, err := reader.GetPlainText()
		if err != nil {
			return "", errors.New("PDF 文本抽取失败")
		}
		b, err := io.ReadAll(pr)
		if err != nil {
			return "", errors.New("PDF 文本读取失败")
		}
		return string(b), nil
	}
	return string(data), nil
}
