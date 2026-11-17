package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

// stubMCPStore 本班有一台已连接服务器，令牌明文为 SECRET_TOKEN（只应出现在写入路径）。
type stubMCPStore struct{}

func (stubMCPStore) ListMCPServers(ctx context.Context, classID int64) ([]store.MCPServer, error) {
	return []store.MCPServer{{
		ID: 1, ClassID: classID, Name: "campus-tools", BaseURL: "http://mcp-server:8090",
		TokenConfigured: true, Connected: true,
		Tools: []store.MCPTool{{Name: "formula_hint", Description: "给线索", InputSchema: map[string]any{"type": "object"}}},
	}}, nil
}
func (stubMCPStore) GetMCPServerByID(ctx context.Context, classID, id int64) (store.MCPServer, error) {
	if classID == 1 && id == 1 {
		list, _ := stubMCPStore{}.ListMCPServers(ctx, 1)
		return list[0], nil
	}
	return store.MCPServer{}, store.ErrNotFound
}
func (stubMCPStore) GetMCPAuthToken(ctx context.Context, classID, id int64) (string, error) {
	return "SECRET_TOKEN", nil
}
func (stubMCPStore) UpsertMCPServer(ctx context.Context, in store.MCPServer) (store.MCPServer, error) {
	in.ID = 1
	in.TokenConfigured = in.AuthToken != ""
	return in, nil
}
func (stubMCPStore) SetMCPConnected(context.Context, int64, int64, []store.MCPTool) error { return nil }
func (stubMCPStore) SetMCPDisconnected(context.Context, int64, int64) error               { return nil }
func (stubMCPStore) SetMCPLastError(context.Context, int64, int64, string) error          { return nil }
func (stubMCPStore) DeleteMCPServer(ctx context.Context, classID, id int64) error {
	if classID == 1 && id == 1 {
		return nil
	}
	return store.ErrNotFound
}
func (stubMCPStore) RecordMCPCall(context.Context, store.MCPCall) error { return nil }
func (stubMCPStore) ListMCPCalls(ctx context.Context, classID int64, limit int) ([]store.MCPCall, error) {
	return []store.MCPCall{{
		ID: 1, ClassID: classID, ToolRef: "formula_hint@campus-tools", Result: "ok",
		ActorName: "王老师", Round: 1, CreatedAt: "2026-11-17T00:00:00.000Z",
	}}, nil
}

func mcpTestServer() *Server {
	s := newTestServer(map[string]error{"mysql": nil, "qdrant": nil})
	s.cfg.JWTSecret = testJWTSecret
	s.mcp = stubMCPStore{}
	s.rec = stubAudit{}
	return s
}

func TestListMCPServersNeverReturnsToken(t *testing.T) {
	s := mcpTestServer()
	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/mcp/servers", nil),
		auth.Claims{UserID: 1, ClassID: 1, Role: "teacher"})
	rec := httptest.NewRecorder()
	s.handleListMCPServers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "SECRET_TOKEN") || strings.Contains(rec.Body.String(), "auth_token") {
		t.Fatalf("列表响应不得出现令牌: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"token_configured":true`) {
		t.Fatalf("响应应有 token_configured 布尔标记: %s", rec.Body.String())
	}
}

func TestMCPToolsCrossClass404(t *testing.T) {
	s := mcpTestServer()
	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/mcp/servers/1/tools", nil),
		auth.Claims{UserID: 9, ClassID: 2, Role: "teacher"})
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	s.handleMCPTools(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("跨班应 404，得到 %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListMCPCallsInvalidLimit(t *testing.T) {
	s := mcpTestServer()
	for _, q := range []string{"limit=abc", "limit=0", "limit=-1"} {
		req := withClaims(httptest.NewRequest(http.MethodGet, "/api/mcp/calls?"+q, nil),
			auth.Claims{UserID: 1, ClassID: 1, Role: "teacher"})
		rec := httptest.NewRecorder()
		s.handleListMCPCalls(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s 应 400，得到 %d", q, rec.Code)
		}
	}
}

func TestStudentCannotListMCPCalls(t *testing.T) {
	s := mcpTestServer()
	req := withBearer(t, s, httptest.NewRequest(http.MethodGet, "/api/mcp/calls", nil),
		auth.Claims{UserID: 2, ClassID: 1, Role: "student"})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("学生读调用记录应 403，得到 %d: %s", rec.Code, rec.Body.String())
	}
}

func TestMCPCallsHaveFiveFields(t *testing.T) {
	s := mcpTestServer()
	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/mcp/calls", nil),
		auth.Claims{UserID: 1, ClassID: 1, Role: "teacher"})
	rec := httptest.NewRecorder()
	s.handleListMCPCalls(rec, req)
	for _, field := range []string{"tool_ref", "result", "actor_name", "round", "created_at"} {
		if !strings.Contains(rec.Body.String(), `"`+field+`"`) {
			t.Errorf("调用记录缺少字段 %s: %s", field, rec.Body.String())
		}
	}
}
