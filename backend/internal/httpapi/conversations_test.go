package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
	"github.com/Corday-nkusky/team_project/backend/internal/qa"
)

func historyRequest(t *testing.T, query string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/conversations"+query, nil)
	ctx := context.WithValue(req.Context(), claimsKey,
		auth.Claims{UserID: 7, ClassID: 3, Role: "student", Name: "学生"})
	return req.WithContext(ctx)
}

func historyServer(rec *stubRec) *Server { return &Server{rec: rec} }

func lastSSEEvent(t *testing.T, evs []sseEvent, name string) sseEvent {
	t.Helper()
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].name == name {
			return evs[i]
		}
	}
	t.Fatalf("没有 %s 事件：%s", name, eventNames(evs))
	return sseEvent{}
}

func TestListConversationsReturnsOwnClassHistory(t *testing.T) {
	rec := &stubRec{listJSON: json.RawMessage(`[
		{"id":9,"question":"酶是什么","answer":"酶是蛋白质","citations":[{"index":1}],"created_at":"2026-11-10T10:00:00Z"},
		{"id":8,"question":"有丝分裂","answer":"分四期","citations":[],"created_at":"2026-11-10T09:00:00Z"}
	]`)}
	s := historyServer(rec)
	rr := httptest.NewRecorder()
	s.handleListConversations(rr, historyRequest(t, "?limit=5"))

	if rr.Code != http.StatusOK {
		t.Fatalf("期望 200，得到 %d: %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON: %v", err)
	}
	if len(body.Items) != 2 || body.Items[0]["id"] != float64(9) || body.Items[0]["answer"] != "酶是蛋白质" {
		t.Fatalf("应原样返回两条完整问答，得到 %+v", body.Items)
	}
	if rec.lastListUser != 7 || rec.lastListClass != 3 || rec.lastListLimit != 5 {
		t.Fatalf("隔离条件或 limit 未传到底层：user=%d class=%d limit=%d",
			rec.lastListUser, rec.lastListClass, rec.lastListLimit)
	}
}

func TestListConversationsLimitDefaultAndCap(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"", defaultConversationLimit},
		{"?limit=100", maxConversationLimit},
		{"?limit=3", 3},
	} {
		rec := &stubRec{listJSON: json.RawMessage("[]")}
		s := historyServer(rec)
		rr := httptest.NewRecorder()
		s.handleListConversations(rr, historyRequest(t, tc.query))
		if rr.Code != http.StatusOK || rec.lastListLimit != tc.want {
			t.Fatalf("query=%q: status=%d limit=%d，期望 %d", tc.query, rr.Code, rec.lastListLimit, tc.want)
		}
	}
}

func TestListConversationsRejectsBadLimit(t *testing.T) {
	for _, q := range []string{"?limit=abc", "?limit=0", "?limit=-1", "?limit=1.5"} {
		rec := &stubRec{listJSON: json.RawMessage("[]")}
		s := historyServer(rec)
		rr := httptest.NewRecorder()
		s.handleListConversations(rr, historyRequest(t, q))
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("query=%q 期望 400，得到 %d", q, rr.Code)
		}
		if rec.listCalls != 0 {
			t.Fatalf("query=%q 非法参数不应查库，实际调用 %d 次", q, rec.listCalls)
		}
	}
}

func TestListConversationsQueryFailureIsNotDisguisedAsEmpty(t *testing.T) {
	rec := &stubRec{listErr: errors.New("mysql gone")}
	s := historyServer(rec)
	rr := httptest.NewRecorder()
	s.handleListConversations(rr, historyRequest(t, ""))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("期望 500，得到 %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), `"items":[]`) {
		t.Fatal("查询失败不得伪装成空历史")
	}
	if !strings.Contains(rr.Body.String(), `"error"`) {
		t.Fatalf("应使用统一错误体: %s", rr.Body.String())
	}
}

func TestListConversationsEmptyIsArray(t *testing.T) {
	rec := &stubRec{} // listJSON 为 nil：store 异常时也必须返回 []
	s := historyServer(rec)
	rr := httptest.NewRecorder()
	s.handleListConversations(rr, historyRequest(t, ""))

	if rr.Code != http.StatusOK || strings.TrimSpace(rr.Body.String()) != `{"items":[]}` {
		t.Fatalf("空历史应返回 items:[]，得到 %d %s", rr.Code, rr.Body.String())
	}
}

func TestListConversationsRequiresAuth(t *testing.T) {
	s := newTestServer(map[string]error{"mysql": nil, "qdrant": nil})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/conversations", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("未认证应返回 401，得到 %d", rr.Code)
	}
}

func TestAskDoneCarriesConversationID(t *testing.T) {
	q := &stubQA{
		cites:   []qa.Citation{{Index: 1, MaterialTitle: "讲义", Snippet: "原文"}},
		enabled: true,
		events:  []qa.StreamEvent{{Delta: "回答[1]"}, {Done: true}},
	}
	rec := &stubRec{}
	s := newAskServer(q, rec)
	rr := httptest.NewRecorder()
	s.handleAsk(rr, askRequest(t, "酶是什么"))

	evs := parseSSE(t, rr.Body.String())
	done := lastSSEEvent(t, evs, "done")
	if done.data["conversation_id"] != float64(1) {
		t.Fatalf("done 应带落库记录 id=1，得到 %v", done.data["conversation_id"])
	}
	if rec.saved != 1 {
		t.Fatalf("完整回答应落库一次，实际 %d", rec.saved)
	}
}

func TestAskDoneIsNullWhenSaveFails(t *testing.T) {
	q := &stubQA{
		cites:   []qa.Citation{{Index: 1, MaterialTitle: "讲义", Snippet: "原文"}},
		enabled: true,
		events:  []qa.StreamEvent{{Delta: "回答[1]"}, {Done: true}},
	}
	rec := &stubRec{saveErr: errors.New("db down")}
	s := newAskServer(q, rec)
	rr := httptest.NewRecorder()
	s.handleAsk(rr, askRequest(t, "酶是什么"))

	evs := parseSSE(t, rr.Body.String())
	if got := eventNames(evs); !strings.HasSuffix(got, ",done") {
		t.Fatalf("落库失败仍应发 done，事件序列：%s", got)
	}
	done := lastSSEEvent(t, evs, "done")
	if done.data["conversation_id"] != nil {
		t.Fatalf("落库失败应回执 null，得到 %v", done.data["conversation_id"])
	}
}

func TestAskNoEvidenceHasNoConversationID(t *testing.T) {
	rec := &stubRec{}
	s := newAskServer(&stubQA{cites: nil, enabled: true}, rec)
	rr := httptest.NewRecorder()
	s.handleAsk(rr, askRequest(t, "无关问题"))

	done := lastSSEEvent(t, parseSSE(t, rr.Body.String()), "done")
	if done.data["conversation_id"] != nil || rec.saved != 0 {
		t.Fatalf("未命中不应归档：id=%v saved=%d", done.data["conversation_id"], rec.saved)
	}
}
