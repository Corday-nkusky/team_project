package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
)

// materialRequest 构造带教师身份的 POST /api/materials 请求。
func materialRequest(contentType string, body *bytes.Buffer) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/materials", body)
	req.Header.Set("Content-Type", contentType)
	return req.WithContext(context.WithValue(req.Context(), claimsKey,
		auth.Claims{UserID: 1, ClassID: 2, Role: "teacher", Name: "教师"}))
}

func multipartMaterial(t *testing.T, filename string, data []byte) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	w := multipart.NewWriter(buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("构造 multipart 失败: %v", err)
	}
	if _, err := fw.Write(data); err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("关闭 multipart 失败: %v", err)
	}
	return buf, w.FormDataContentType()
}

// TestCreateMaterialRejectsLongTitle 对应 R3.4：标题按 rune 计数，超过 255 拒绝且不落库。
// 这些用例都在落库前返回，因此 Server 不需要 store。
func TestCreateMaterialRejectsLongTitle(t *testing.T) {
	body, _ := json.Marshal(map[string]string{
		"title":   strings.Repeat("长", 255+1),
		"content": "边界测试正文",
	})
	s := &Server{}
	rr := httptest.NewRecorder()
	s.handleCreateMaterial(rr, materialRequest("application/json", bytes.NewBuffer(body)))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("256 字标题期望 400，得到 %d：%s", rr.Code, rr.Body.String())
	}
	if code := errorCode(t, rr); code != "title_too_long" {
		t.Fatalf("错误码不符：%q", code)
	}
}

// TestCreateMaterialRejectsInvalidUTF8 对应 R3.4：JSON 请求体必须是合法 UTF-8。
func TestCreateMaterialRejectsInvalidUTF8(t *testing.T) {
	raw := append([]byte(`{"title":"坏编码","content":"`), 0xff, 0xfe, 'x', '"', '}')
	s := &Server{}
	rr := httptest.NewRecorder()
	s.handleCreateMaterial(rr, materialRequest("application/json", bytes.NewBuffer(raw)))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("非法 UTF-8 期望 400，得到 %d：%s", rr.Code, rr.Body.String())
	}
	if code := errorCode(t, rr); code != "invalid_encoding" {
		t.Fatalf("错误码不符：%q", code)
	}
}

// TestCreateMaterialRejectsUnsupportedFileType 对应 R3.4：扩展名白名单。
func TestCreateMaterialRejectsUnsupportedFileType(t *testing.T) {
	body, ct := multipartMaterial(t, "note.docx", []byte("PK\x03\x04fake"))
	s := &Server{}
	rr := httptest.NewRecorder()
	s.handleCreateMaterial(rr, materialRequest(ct, body))

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf(".docx 期望 415，得到 %d：%s", rr.Code, rr.Body.String())
	}
	if code := errorCode(t, rr); code != "unsupported_file_type" {
		t.Fatalf("错误码不符：%q", code)
	}
}

// TestCreateMaterialRejectsOversizedUpload 对应 R3.4：正文超过 32 MiB 返回 413。
func TestCreateMaterialRejectsOversizedUpload(t *testing.T) {
	body, ct := multipartMaterial(t, "big.txt", bytes.Repeat([]byte("a"), maxUploadBytes+1))
	s := &Server{}
	rr := httptest.NewRecorder()
	s.handleCreateMaterial(rr, materialRequest(ct, body))

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("33 MiB 期望 413，得到 %d", rr.Code)
	}
	if code := errorCode(t, rr); code != "payload_too_large" {
		t.Fatalf("错误码不符：%q", code)
	}
}
