package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

// stubSkillRW 只保留本班 id=1 的一条技能，用于验证跨班 404。
type stubSkillRW struct{}

func (stubSkillRW) ListSkills(ctx context.Context, classID int64) ([]store.Skill, error) {
	return []store.Skill{{ID: 1, ClassID: classID, Name: "解题引导", Enabled: true,
		Instruction: "先给思路", Kind: "solve_hint", Version: 1}}, nil
}
func (stubSkillRW) ListEnabledSkills(ctx context.Context, classID int64) ([]store.Skill, error) {
	return stubSkillRW{}.ListSkills(ctx, classID)
}
func (stubSkillRW) GetSkillByID(ctx context.Context, classID, id int64) (store.Skill, error) {
	if id == 1 && classID == 1 {
		return store.Skill{ID: 1, ClassID: 1, Name: "解题引导"}, nil
	}
	return store.Skill{}, store.ErrNotFound
}
func (stubSkillRW) UpsertSkill(ctx context.Context, in store.Skill) (store.Skill, error) {
	in.ID, in.Version = 1, 1
	return in, nil
}
func (stubSkillRW) DeleteSkill(ctx context.Context, classID, id int64) error {
	if classID == 1 && id == 1 {
		return nil
	}
	return store.ErrNotFound
}

type stubAudit struct{}

func (stubAudit) Audit(context.Context, *int64, string, *int64, string, string, string) error {
	return nil
}
func (stubAudit) SaveConversation(context.Context, int64, int64, string, string, []byte) (int64, error) {
	return 1, nil
}
func (stubAudit) ListConversationsJSON(context.Context, int64, int64, int) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}

const testJWTSecret = "test-secret-for-unit-tests-only-0123456789abcdef"

func skillTestServer() *Server {
	s := newTestServer(map[string]error{"mysql": nil, "qdrant": nil})
	s.cfg.JWTSecret = testJWTSecret
	s.skillsRW = stubSkillRW{}
	s.mcp = stubMCPStore{}
	s.rec = stubAudit{}
	return s
}

// withBearer 生成真实 access token 并加到请求头，用于走完整路由（含 requireAuth/requireRole）。
func withBearer(t *testing.T, s *Server, req *http.Request, c auth.Claims) *http.Request {
	t.Helper()
	tok, err := auth.SignAccess(s.cfg.JWTSecret, c, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return req
}

func withClaims(req *http.Request, c auth.Claims) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), claimsKey, c))
}

func TestListSkillsStudentOK(t *testing.T) {
	s := skillTestServer()
	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/skills", nil),
		auth.Claims{UserID: 2, ClassID: 1, Role: "student"})
	rec := httptest.NewRecorder()
	s.handleListSkills(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "解题引导") {
		t.Fatalf("响应缺少技能: %s", rec.Body.String())
	}
}

func TestUpsertSkillStudentForbidden(t *testing.T) {
	s := skillTestServer()
	body := `{"name":"解题引导","instruction":"先给思路"}`
	req := withBearer(t, s, httptest.NewRequest(http.MethodPost, "/api/skills", strings.NewReader(body)),
		auth.Claims{UserID: 2, ClassID: 1, Role: "student"})
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	// 走路由以触发 requireRole。
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("学生写技能应被拒(403)，得到 %d: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteSkillCrossClass404(t *testing.T) {
	s := skillTestServer()
	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/skills/1", nil),
		auth.Claims{UserID: 9, ClassID: 2, Role: "teacher"})
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()
	s.handleDeleteSkill(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("跨班删除应 404，得到 %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpsertSkillRejectsBadToolRef(t *testing.T) {
	s := skillTestServer()
	body := `{"name":"解题引导","instruction":"先给思路","tool_refs":["bad_tool@campus-tools"]}`
	req := withClaims(httptest.NewRequest(http.MethodPost, "/api/skills", strings.NewReader(body)),
		auth.Claims{UserID: 1, ClassID: 1, Role: "teacher"})
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleUpsertSkill(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("非法工具引用应 400，得到 %d: %s", rec.Code, rec.Body.String())
	}
}
