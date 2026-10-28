package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/config"
)

type stubChecker struct {
	results map[string]error
}

func (s stubChecker) Check(context.Context) map[string]error { return s.results }

func newTestServer(results map[string]error) *Server {
	return New(config.Config{}, nil, nil, stubChecker{results: results}, "test-version")
}

func TestHealthOK(t *testing.T) {
	s := newTestServer(map[string]error{"mysql": nil, "qdrant": nil})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", rec.Code)
	}
	var body struct {
		Status  string            `json:"status"`
		Version string            `json:"version"`
		Time    string            `json:"time"`
		Deps    map[string]string `json:"deps"`
		LLM     struct {
			Enabled bool `json:"enabled"`
		} `json:"llm"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON: %v", err)
	}
	if body.Status != "ok" || body.Version != "test-version" {
		t.Fatalf("status/version 不符合预期: %+v", body)
	}
	if _, err := time.Parse(time.RFC3339, body.Time); err != nil {
		t.Fatalf("time 不是 RFC3339: %q", body.Time)
	}
	if body.Deps["mysql"] != "ok" || body.Deps["qdrant"] != "ok" {
		t.Fatalf("deps 不符合预期: %+v", body.Deps)
	}
	if body.LLM.Enabled {
		t.Fatal("未配置密钥时 llm.enabled 应为 false")
	}
	low := strings.ToLower(rec.Body.String())
	for _, banned := range []string{"password", "secret", "api_key"} {
		if strings.Contains(low, banned) {
			t.Fatalf("响应体不得包含 %q: %s", banned, low)
		}
	}
}

func TestHealthDegraded(t *testing.T) {
	s := newTestServer(map[string]error{"mysql": context.DeadlineExceeded, "qdrant": nil})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("依赖不可用应返回 503，得到 %d", rec.Code)
	}
	var body struct {
		Status string            `json:"status"`
		Deps   map[string]string `json:"deps"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Status != "degraded" || body.Deps["mysql"] == "ok" || body.Deps["qdrant"] != "ok" {
		t.Fatalf("降级细节不符合预期: %s", rec.Body.String())
	}
}

func TestRequestIDAndAccessLog(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(old)

	s := newTestServer(map[string]error{"mysql": nil, "qdrant": nil})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "peer-123")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Request-ID"); got != "peer-123" {
		t.Fatalf("合法 request ID 应透传，得到 %q", got)
	}
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("访问日志不是单行 JSON: %v (%q)", err, buf.String())
	}
	if entry["request_id"] != "peer-123" || entry["method"] != "GET" ||
		entry["path"] != "/health" || entry["status"] != float64(200) {
		t.Fatalf("访问日志字段不符合 R10.1: %v", entry)
	}
	if _, ok := entry["duration_ms"].(float64); !ok {
		t.Fatalf("duration_ms 应为数值: %v", entry["duration_ms"])
	}
}

func TestInvalidRequestIDIsReplaced(t *testing.T) {
	s := newTestServer(map[string]error{"mysql": nil, "qdrant": nil})

	for _, bad := range []string{strings.Repeat("x", 200), "bad\x7fid"} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("X-Request-ID", bad)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		got := rec.Header().Get("X-Request-ID")
		if got == bad || len(got) != 32 {
			t.Fatalf("非法 request ID 应被替换为 32 位十六进制，输入 %q 得到 %q", bad, got)
		}
	}
}
