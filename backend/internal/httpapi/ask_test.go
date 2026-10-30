package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
	"github.com/Corday-nkusky/team_project/backend/internal/config"
	"github.com/Corday-nkusky/team_project/backend/internal/qa"
)

// stubQA 是 qaService 的可控替身，用于覆盖流式分支而不依赖数据库或模型。
type stubQA struct {
	cites       []qa.Citation
	retrieveErr error
	enabled     bool
	streamErr   error
	events      []qa.StreamEvent
	// retrieveCalls 记录检索调用次数，用于断言「请求在解码阶段即被拒绝」。
	retrieveCalls int
}

func (s *stubQA) Retrieve(context.Context, int64, string) ([]qa.Citation, error) {
	s.retrieveCalls++
	return s.cites, s.retrieveErr
}

func (s *stubQA) LLMEnabled() bool { return s.enabled }

func (s *stubQA) ChatStream(context.Context, string, string) (<-chan qa.StreamEvent, error) {
	if s.streamErr != nil {
		return nil, s.streamErr
	}
	ch := make(chan qa.StreamEvent, len(s.events))
	for _, ev := range s.events {
		ch <- ev
	}
	close(ch)
	return ch, nil
}

func (s *stubQA) Index(context.Context, int64, int64, string) error { return nil }

// stubRec 记录问答落库与审计调用。
type stubRec struct {
	saved      int
	lastAnswer string
	audits     []string
	saveErr    error
}

func (r *stubRec) Audit(_ context.Context, _ *int64, _ string, _ *int64, _ string, result, detail string) error {
	r.audits = append(r.audits, result+":"+detail)
	return nil
}

func (r *stubRec) SaveConversation(_ context.Context, _, _ int64, _, answer string, _ []byte) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved++
	r.lastAnswer = answer
	return nil
}

type sseEvent struct {
	name string
	data map[string]any
}

func parseSSE(t *testing.T, raw string) []sseEvent {
	t.Helper()
	var out []sseEvent
	for _, block := range strings.Split(strings.TrimSpace(raw), "\n\n") {
		if strings.TrimSpace(block) == "" {
			continue
		}
		var ev sseEvent
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev.data); err != nil {
					t.Fatalf("SSE data 不是 JSON: %v (%q)", err, line)
				}
			}
		}
		out = append(out, ev)
	}
	return out
}

func newAskServer(q *stubQA, r *stubRec) *Server {
	return &Server{cfg: config.Config{}, qa: q, rec: r}
}

func askRequest(t *testing.T, question string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/ask",
		strings.NewReader(`{"question":`+strconvQuote(question)+`}`))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), claimsKey,
		auth.Claims{UserID: 1, ClassID: 2, Role: "student", Name: "学生"})
	return req.WithContext(ctx)
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func eventNames(evs []sseEvent) string {
	names := make([]string, len(evs))
	for i, ev := range evs {
		names[i] = ev.name
	}
	return strings.Join(names, ",")
}

func TestAskNoEvidence(t *testing.T) {
	q := &stubQA{cites: nil, enabled: true}
	rec := &stubRec{}
	s := newAskServer(q, rec)

	rr := httptest.NewRecorder()
	s.handleAsk(rr, askRequest(t, "完全没有的问题"))

	evs := parseSSE(t, rr.Body.String())
	if got := eventNames(evs); got != "meta,delta,done" {
		t.Fatalf("事件序列不符：%s", got)
	}
	if ne, _ := evs[0].data["no_evidence"].(bool); !ne {
		t.Fatal("meta 应标记 no_evidence")
	}
	if evs[1].data["text"] != qa.NoEvidenceText {
		t.Fatalf("未命中应返回固定文案，得到 %v", evs[1].data["text"])
	}
	if rec.saved != 0 || len(rec.audits) != 1 || rec.audits[0] != "fail:no_evidence" {
		t.Fatalf("未命中不应落库，应留痕：saved=%d audits=%v", rec.saved, rec.audits)
	}
}

func TestAskStreamInterruptedAfterDelta(t *testing.T) {
	q := &stubQA{
		cites:   []qa.Citation{{Index: 1, MaterialTitle: "讲义", Snippet: "原文"}},
		enabled: true,
		events: []qa.StreamEvent{
			{Delta: "部分回答"},
			{Err: errors.New("上游断开")},
		},
	}
	rec := &stubRec{}
	s := newAskServer(q, rec)

	rr := httptest.NewRecorder()
	s.handleAsk(rr, askRequest(t, "问题"))

	evs := parseSSE(t, rr.Body.String())
	if got := eventNames(evs); got != "meta,delta,error" {
		t.Fatalf("中断后不应发送 done：%s", got)
	}
	if evs[2].data["code"] != "stream_incomplete" {
		t.Fatalf("错误码不符：%v", evs[2].data)
	}
	if rec.saved != 0 {
		t.Fatalf("中断不得落库，实际保存 %d 次", rec.saved)
	}
	if len(rec.audits) == 0 || rec.audits[len(rec.audits)-1] != "fail:生成中断" {
		t.Fatalf("应审计生成中断：%v", rec.audits)
	}
}

func TestAskDegradesBeforeFirstDelta(t *testing.T) {
	q := &stubQA{
		cites:     []qa.Citation{{Index: 1, MaterialTitle: "讲义", Snippet: "原文"}},
		enabled:   true,
		streamErr: errors.New("模型不可用"),
	}
	rec := &stubRec{}
	s := newAskServer(q, rec)

	rr := httptest.NewRecorder()
	s.handleAsk(rr, askRequest(t, "问题"))

	evs := parseSSE(t, rr.Body.String())
	if got := eventNames(evs); !strings.HasPrefix(got, "meta,degraded,delta") || !strings.HasSuffix(got, ",done") {
		t.Fatalf("首字前失败应降级后正常结束：%s", got)
	}
	if evs[1].data["reason"] != "upstream_error" {
		t.Fatalf("degraded 原因不符：%v", evs[1].data)
	}
	if rec.saved != 1 || !strings.Contains(rec.lastAnswer, "原文") {
		t.Fatalf("降级回答应完整落库：saved=%d answer=%q", rec.saved, rec.lastAnswer)
	}
}

func TestAskEmptyStreamIsError(t *testing.T) {
	q := &stubQA{
		cites:   []qa.Citation{{Index: 1, MaterialTitle: "讲义", Snippet: "原文"}},
		enabled: true,
		events:  []qa.StreamEvent{{Done: true}},
	}
	rec := &stubRec{}
	s := newAskServer(q, rec)

	rr := httptest.NewRecorder()
	s.handleAsk(rr, askRequest(t, "问题"))

	evs := parseSSE(t, rr.Body.String())
	if got := eventNames(evs); got != "meta,error" {
		t.Fatalf("空流应以 error 结束：%s", got)
	}
	if evs[1].data["code"] != "empty_answer" {
		t.Fatalf("错误码不符：%v", evs[1].data)
	}
	if rec.saved != 0 {
		t.Fatal("空回答不得落库")
	}
}

func TestAskFiltersOutOfRangeCitations(t *testing.T) {
	// 越界标记被拆成「[」「9」「]」三个增量：流式过滤必须跨增量识别（R7.4）。
	q := &stubQA{
		cites:   []qa.Citation{{Index: 1, MaterialTitle: "讲义", Snippet: "原文"}},
		enabled: true,
		events: []qa.StreamEvent{
			{Delta: "结论["},
			{Delta: "9"},
			{Delta: "]，另有[1]。"},
			{Done: true},
		},
	}
	rec := &stubRec{}
	s := newAskServer(q, rec)

	rr := httptest.NewRecorder()
	s.handleAsk(rr, askRequest(t, "问题"))

	evs := parseSSE(t, rr.Body.String())
	// 中间增量只含缓冲中的编号，不产生输出，因此是 2 个 delta 而非 3 个。
	if got := eventNames(evs); got != "meta,delta,delta,done" {
		t.Fatalf("事件序列不符：%s", got)
	}
	var shown strings.Builder
	for _, ev := range evs {
		if ev.name == "delta" {
			shown.WriteString(ev.data["text"].(string))
		}
	}
	if shown.String() != "结论，另有[1]。" {
		t.Fatalf("越界引用应被丢弃：%q", shown.String())
	}
	if strings.Contains(shown.String(), "9") {
		t.Fatalf("展示文本不得残留越界编号：%q", shown.String())
	}
	if rec.lastAnswer != "结论，另有[1]。" {
		t.Fatalf("落库文本应与展示一致：%q", rec.lastAnswer)
	}
}

func TestLogServerErrorOnRetrieve(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(old)

	q := &stubQA{retrieveErr: errors.New("db down")}
	s := newAskServer(q, &stubRec{})
	req := askRequest(t, "问题")
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey, "rid-500"))

	rr := httptest.NewRecorder()
	s.handleAsk(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("检索失败应返回 500，得到 %d", rr.Code)
	}
	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("错误日志不是 JSON: %v (%q)", err, buf.String())
	}
	if entry["level"] != "ERROR" || entry["request_id"] != "rid-500" ||
		entry["operation"] != "retrieve" || !strings.Contains(entry["error"].(string), "db down") {
		t.Fatalf("R10.6 错误日志字段不符：%v", entry)
	}
	if strings.Contains(buf.String(), "问题") {
		t.Fatal("错误日志不得包含请求体内容")
	}
}

// TestAskStrictJSONRejectsBeforeRetrieve 覆盖 R11.3 / R11.4：
// 非 JSON 内容类型、超限体积与尾随内容都必须在检索前拒绝。
func TestAskStrictJSONRejectsBeforeRetrieve(t *testing.T) {
	oversized := `{"question":"` + strings.Repeat("x", 1100*1024) + `"}`
	cases := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
		wantCode    string
	}{
		{"内容类型不符", "text/plain", `{"question":"问题"}`, http.StatusUnsupportedMediaType, "unsupported_media_type"},
		{"请求体超过 1 MiB", "application/json", oversized, http.StatusRequestEntityTooLarge, "payload_too_large"},
		{"尾随 JSON", "application/json", `{"question":"问题"}{"x":1}`, http.StatusBadRequest, "bad_request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &stubQA{enabled: true}
			s := newAskServer(q, &stubRec{})
			req := httptest.NewRequest(http.MethodPost, "/api/ask", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			req = req.WithContext(context.WithValue(req.Context(), claimsKey,
				auth.Claims{UserID: 1, ClassID: 2, Role: "student", Name: "学生"}))
			rr := httptest.NewRecorder()
			s.handleAsk(rr, req)
			if rr.Code != tc.wantStatus {
				t.Fatalf("期望 %d，得到 %d：%s", tc.wantStatus, rr.Code, rr.Body.String())
			}
			if code := errorCode(t, rr); code != tc.wantCode {
				t.Fatalf("错误码期望 %q，得到 %q", tc.wantCode, code)
			}
			if q.retrieveCalls != 0 {
				t.Fatalf("解码阶段应拒绝，检索却被调用 %d 次", q.retrieveCalls)
			}
		})
	}
}
