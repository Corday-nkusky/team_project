package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

type stubObservationStore struct {
	events     []store.AuditEvent
	skills     []store.SkillObservation
	depth      int64
	err        error
	classCalls []int64
}

func (s *stubObservationStore) ListAuditEvents(_ context.Context, classID int64, _ int) ([]store.AuditEvent, error) {
	s.classCalls = append(s.classCalls, classID)
	return s.events, s.err
}

func (s *stubObservationStore) ObserveSkills(_ context.Context, classID int64) ([]store.SkillObservation, error) {
	s.classCalls = append(s.classCalls, classID)
	return s.skills, s.err
}

func (s *stubObservationStore) QueueDepth(_ context.Context, classID int64) (int64, error) {
	s.classCalls = append(s.classCalls, classID)
	return s.depth, s.err
}

func TestObservabilityTeacherGetsRealData(t *testing.T) {
	obs := &stubObservationStore{
		skills: []store.SkillObservation{{SkillName: "解题引导", Calls: 4, Errors: 1, ErrorRate: 0.25}},
		depth:  3,
	}
	s := skillTestServer()
	s.obs = obs
	s.metrics.observe(http.MethodPost, "/api/ask", http.StatusOK, 120*time.Millisecond)
	req := withBearer(t, s, httptest.NewRequest(http.MethodGet, "/api/observability", nil),
		auth.Claims{UserID: 1, ClassID: 7, Role: "teacher"})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("教师观测应 200，得到 %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Skills     []store.SkillObservation `json:"skills"`
		Requests   []RequestObservation     `json:"requests"`
		QueueDepth int64                    `json:"queue_depth"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.QueueDepth != 3 || len(body.Skills) != 1 || body.Skills[0].ErrorRate != 0.25 ||
		len(body.Requests) != 1 || body.Requests[0].P95MS != 120 {
		t.Fatalf("观测数据不完整：%+v", body)
	}
	for _, classID := range obs.classCalls {
		if classID != 7 {
			t.Fatalf("观测查询越班：%d", classID)
		}
	}
}

func TestObservabilityStudentForbiddenAudited(t *testing.T) {
	recorder := &stubRec{}
	s := skillTestServer()
	s.rec = recorder
	req := withBearer(t, s, httptest.NewRequest(http.MethodGet, "/api/observability", nil),
		auth.Claims{UserID: 2, ClassID: 7, Role: "student"})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("学生读取观测应 403，得到 %d", rec.Code)
	}
	if len(recorder.events) != 1 || recorder.events[0].Action != "forbidden" ||
		recorder.events[0].ClassID == nil || *recorder.events[0].ClassID != 7 {
		t.Fatalf("拒绝路径未正确留痕：%+v", recorder.events)
	}
}

func TestListAuditFailureAndClassIsolation(t *testing.T) {
	obs := &stubObservationStore{err: errors.New("database unavailable")}
	s := skillTestServer()
	s.obs = obs
	req := withBearer(t, s, httptest.NewRequest(http.MethodGet, "/api/audit", nil),
		auth.Claims{UserID: 1, ClassID: 9, Role: "teacher"})
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("查询失败不得伪装为空，期望 500，得到 %d", rec.Code)
	}
	if len(obs.classCalls) != 1 || obs.classCalls[0] != 9 {
		t.Fatalf("审计查询必须带本班 class_id：%v", obs.classCalls)
	}
}

func TestRequestMetricsP95(t *testing.T) {
	metrics := newRequestMetrics()
	for i := 1; i <= 100; i++ {
		status := http.StatusOK
		if i == 100 {
			status = http.StatusInternalServerError
		}
		metrics.observe(http.MethodGet, "/api/materials", status, time.Duration(i)*time.Millisecond)
	}
	items := metrics.snapshot()
	if len(items) != 1 || items[0].P95MS != 95 || items[0].Errors != 1 || items[0].Calls != 100 {
		t.Fatalf("P95/错误数/样本数不符：%+v", items)
	}
}
