package httpapi

import (
	"context"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/config"
	"github.com/Corday-nkusky/team_project/backend/internal/qa"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
	"github.com/Corday-nkusky/team_project/backend/internal/web"
)

// Checker 抽象依赖探活，便于用桩替换（spec R9 的单元测试）。
type Checker interface {
	Check(ctx context.Context) map[string]error
}

// qaService 是 httpapi 需要的问答能力（生产由 *qa.Service 实现）。
//
// 用窄接口而非具体类型，一是让问答流的失败分支可在无数据库的情况下单测，
// 二是防止 handler 直接触达检索/向量细节（分层规范）。
type qaService interface {
	Retrieve(ctx context.Context, classID int64, question string) ([]qa.Citation, error)
	LLMEnabled() bool
	ChatStream(ctx context.Context, system, user string) (<-chan qa.StreamEvent, error)
	Index(ctx context.Context, materialID, classID int64, content string) error
}

// askRecorder 是问答落库与审计所需的最小存储能力（spec R6.4 / R6.3）。
type askRecorder interface {
	Audit(ctx context.Context, actorID *int64, actorName string, classID *int64, action, result, detail string) error
	SaveConversation(ctx context.Context, userID, classID int64, question, answer string, citationsJSON []byte) error
}

// Server 汇总依赖并提供路由。
type Server struct {
	cfg     config.Config
	st      *store.Store
	rec     askRecorder
	qa      qaService
	checker Checker
	version string
}

// New 构造 Server。version 由构建期注入（`-X main.version=...`），空值回退为 dev。
func New(cfg config.Config, st *store.Store, svc *qa.Service, checker Checker, version string) *Server {
	if version == "" {
		version = "dev"
	}
	return &Server{cfg: cfg, st: st, rec: st, qa: svc, checker: checker, version: version}
}

// Handler 返回装配完成的 http.Handler。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 探活（tasks 1.4）。
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("/health", methodNotAllowed(http.MethodGet))

	// 认证（公开）。
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("/api/login", methodNotAllowed(http.MethodPost))
	mux.HandleFunc("POST /api/refresh", s.handleRefresh)
	mux.HandleFunc("/api/refresh", methodNotAllowed(http.MethodPost))
	mux.HandleFunc("POST /api/logout", s.handleLogout)
	mux.HandleFunc("/api/logout", methodNotAllowed(http.MethodPost))

	// 需要登录的接口。
	mux.Handle("GET /api/me", s.requireAuth(http.HandlerFunc(s.handleMe)))
	mux.HandleFunc("/api/me", methodNotAllowed(http.MethodGet))
	mux.Handle("GET /api/materials", s.requireAuth(http.HandlerFunc(s.handleListMaterials)))
	mux.Handle("POST /api/ask", s.requireAuth(http.HandlerFunc(s.handleAsk)))
	mux.HandleFunc("/api/ask", methodNotAllowed(http.MethodPost))

	// 仅教师：上传与重索引（R2.3）。
	mux.Handle("POST /api/materials",
		s.requireAuth(requireRole("teacher", http.HandlerFunc(s.handleCreateMaterial))))
	mux.HandleFunc("/api/materials", methodNotAllowed(http.MethodGet, http.MethodPost))
	mux.Handle("POST /api/materials/{id}/reindex",
		s.requireAuth(requireRole("teacher", http.HandlerFunc(s.handleReindex))))
	mux.HandleFunc("/api/materials/{id}/reindex", methodNotAllowed(http.MethodPost))

	// 未注册的 API 路径统一返回 JSON 404，绝不回落到 SPA（spec R11.2）。
	mux.HandleFunc("/api/", notFoundAPI)
	mux.HandleFunc("/api", notFoundAPI)

	// 前端静态资源（同源，免跨域）。用无方法模式注册，避免与更具体的 /api/ 兜底冲突；
	// 非 GET/HEAD 的页面请求返回 JSON 405，而不是把 index.html 当响应。
	static := s.staticHandler()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "该接口不支持此请求方法")
			return
		}
		static.ServeHTTP(w, r)
	})

	return withLogging(mux)
}

// methodNotAllowed 为已注册路径兜底未支持的方法，返回 JSON 405 并写明 Allow（spec R11.1）。
func methodNotAllowed(allow ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", strings.Join(allow, ", "))
		writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", "该接口不支持此请求方法")
	}
}

// notFoundAPI 让未知的 /api 路径返回统一 JSON 错误体（spec R11.2）。
func notFoundAPI(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusNotFound, "not_found", "接口不存在")
}

// handleHealth 依赖全好 → 200；任一不可用 → 503（如实报告）。
// 响应含 status / version / time / deps / llm，且不含任何密钥（spec R9）。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	status := http.StatusOK
	body := map[string]any{
		"status":  "ok",
		"version": s.version,
		"time":    time.Now().UTC().Format(time.RFC3339),
	}
	detail := map[string]string{}
	for name, err := range s.checker.Check(ctx) {
		if err != nil {
			detail[name] = err.Error()
			status = http.StatusServiceUnavailable
		} else {
			detail[name] = "ok"
		}
	}
	if status != http.StatusOK {
		body["status"] = "degraded"
	}
	body["deps"] = detail
	body["llm"] = map[string]any{"enabled": s.cfg.LLMEnabled()}
	writeJSON(w, status, body)
}

// staticHandler 提供嵌入的前端资源；未知路径回落到 index.html（SPA 路由）。
func (s *Server) staticHandler() http.Handler {
	sub, err := web.FS()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeErr(w, http.StatusInternalServerError, "internal", "静态资源不可用")
		})
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "not_found", "接口不存在")
			return
		}
		clean := strings.TrimPrefix(r.URL.Path, "/")
		if clean == "" {
			clean = "index.html"
		}
		if _, err := fs.Stat(sub, clean); err != nil {
			// 非静态文件 → SPA 回落。
			serveIndex(w, sub)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, sub fs.FS) {
	b, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><title>CampusClaw</title></head>
<body style="font-family:system-ui;padding:2rem;line-height:1.7">
<h2>前端尚未构建</h2>
<p>请在 <code>frontend/</code> 目录执行 <code>npm install &amp;&amp; npm run build</code>，
或使用 <code>docker compose up</code>（会自动构建前端）。</p>
<p>接口本身已可用：<code>GET /health</code>。</p>
</body></html>`))
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}
