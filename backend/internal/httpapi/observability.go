package httpapi

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

const maxRequestSamples = 256

type observationStore interface {
	ListAuditEvents(ctx context.Context, classID int64, limit int) ([]store.AuditEvent, error)
	ObserveSkills(ctx context.Context, classID int64) ([]store.SkillObservation, error)
	QueueDepth(ctx context.Context, classID int64) (int64, error)
}

type requestSample struct {
	durations []float64
	errors    int64
}

// requestMetrics 只保存进程内最近请求耗时，不保存正文、查询串或令牌。
type requestMetrics struct {
	mu      sync.RWMutex
	byRoute map[string]*requestSample
}

func newRequestMetrics() *requestMetrics {
	return &requestMetrics{byRoute: make(map[string]*requestSample)}
}

func (m *requestMetrics) observe(method, route string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	key := method + " " + route
	m.mu.Lock()
	defer m.mu.Unlock()
	sample := m.byRoute[key]
	if sample == nil {
		sample = &requestSample{}
		m.byRoute[key] = sample
	}
	if len(sample.durations) >= maxRequestSamples {
		copy(sample.durations, sample.durations[1:])
		sample.durations = sample.durations[:maxRequestSamples-1]
	}
	sample.durations = append(sample.durations, float64(duration.Microseconds())/1000)
	if status >= 500 {
		sample.errors++
	}
}

// RequestObservation 是按路由聚合的最近请求窗口；P95 使用最近 256 个样本。
type RequestObservation struct {
	Route  string  `json:"route"`
	Calls  int     `json:"calls"`
	Errors int64   `json:"errors"`
	P95MS  float64 `json:"p95_ms"`
}

func (m *requestMetrics) snapshot() []RequestObservation {
	if m == nil {
		return []RequestObservation{}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]RequestObservation, 0, len(m.byRoute))
	for route, sample := range m.byRoute {
		durations := append([]float64(nil), sample.durations...)
		sort.Float64s(durations)
		p95 := 0.0
		if len(durations) > 0 {
			index := int(math.Ceil(0.95*float64(len(durations)))) - 1
			p95 = durations[index]
		}
		out = append(out, RequestObservation{
			Route: route, Calls: len(durations), Errors: sample.errors, P95MS: math.Round(p95*100) / 100,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Route < out[j].Route })
	return out
}

func (s *Server) handleListAudit(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeErr(w, http.StatusBadRequest, "bad_request", "limit 必须为 1–100 的整数")
			return
		}
		limit = parsed
	}
	if s.obs == nil {
		writeErr(w, http.StatusInternalServerError, "internal", "观测存储未配置")
		return
	}
	items, err := s.obs.ListAuditEvents(r.Context(), c.ClassID, limit)
	if err != nil {
		logServerError(r, "list_audit", err)
		writeErr(w, http.StatusInternalServerError, "internal", "查询操作留痕失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleObservability(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	if s.obs == nil {
		writeErr(w, http.StatusInternalServerError, "internal", "观测存储未配置")
		return
	}
	skills, err := s.obs.ObserveSkills(r.Context(), c.ClassID)
	if err != nil {
		logServerError(r, "observe_skills", err)
		writeErr(w, http.StatusInternalServerError, "internal", "查询技能观测失败")
		return
	}
	depth, err := s.obs.QueueDepth(r.Context(), c.ClassID)
	if err != nil {
		logServerError(r, "observe_queue", err)
		writeErr(w, http.StatusInternalServerError, "internal", "查询队列深度失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"skills": skills, "requests": s.metrics.snapshot(), "queue_depth": depth,
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}
