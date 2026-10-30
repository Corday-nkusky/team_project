package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serve(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	s := newTestServer(map[string]error{"mysql": nil, "qdrant": nil})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	return rr
}

func errorCode(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("错误体不是 JSON: %v (%q)", err, rr.Body.String())
	}
	return body.Error.Code
}

func TestMethodNotAllowedReturnsJSONAndAllow(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		rr := serve(t, httptest.NewRequest(method, "/api/ask", nil))
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s /api/ask 期望 405，得到 %d", method, rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("405 应为 JSON，得到 %q", ct)
		}
		if got := rr.Header().Get("Allow"); got != "POST" {
			t.Fatalf("Allow 应只含 POST，得到 %q", got)
		}
		if code := errorCode(t, rr); code != "method_not_allowed" {
			t.Fatalf("错误码不符：%q", code)
		}
	}
}

func TestUnknownAPIPathReturnsJSON404(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		rr := serve(t, httptest.NewRequest(method, "/api/not-a-route", nil))
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s /api/not-a-route 期望 404，得到 %d", method, rr.Code)
		}
		if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("404 应为 JSON，得到 %q", ct)
		}
		if code := errorCode(t, rr); code != "not_found" {
			t.Fatalf("错误码不符：%q", code)
		}
		if strings.Contains(rr.Body.String(), "<html") {
			t.Fatal("未知 API 路径不得回落 SPA")
		}
	}
}

func TestLoginRejectsWrongContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(`{"username":"a","password":"b"}`))
	req.Header.Set("Content-Type", "text/plain")
	rr := serve(t, req)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("期望 415，得到 %d", rr.Code)
	}
	if code := errorCode(t, rr); code != "unsupported_media_type" {
		t.Fatalf("错误码不符：%q", code)
	}
}

func TestLoginRejectsOversizedBody(t *testing.T) {
	body := `{"username":"` + strings.Repeat("x", maxJSONBody) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := serve(t, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("期望 413，得到 %d", rr.Code)
	}
	if code := errorCode(t, rr); code != "payload_too_large" {
		t.Fatalf("错误码不符：%q", code)
	}
}

func TestLoginRejectsTrailingJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/login",
		strings.NewReader(`{"username":"a","password":"b"}{"x":1}`))
	req.Header.Set("Content-Type", "application/json")
	rr := serve(t, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("期望 400，得到 %d", rr.Code)
	}
	if code := errorCode(t, rr); code != "bad_request" {
		t.Fatalf("错误码不符：%q", code)
	}
}
