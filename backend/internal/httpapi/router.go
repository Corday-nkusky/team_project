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

// Server 汇总依赖并提供路由。
type Server struct {
	cfg     config.Config
	st      *store.Store
	qa      *qa.Service
	checker Checker
	version string
}

// New 构造 Server。version 由构建期注入（`-X main.version=...`），空值回退为 dev。
func New(cfg config.Config, st *store.Store, svc *qa.Service, checker Checker, version string) *Server {
	if version == "" {
		version = "dev"
	}
	return &Server{cfg: cfg, st: st, qa: svc, checker: checker, version: version}
}

// Handler 返回装配完成的 http.Handler。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 探活（tasks 1.4）。
	mux.HandleFunc("GET /health", s.handleHealth)

	// 认证（公开）。
	mux.HandleFunc("POST /api/login", s.handleLogin)
	mux.HandleFunc("POST /api/refresh", s.handleRefresh)
	mux.HandleFunc("POST /api/logout", s.handleLogout)

	// 需要登录的接口。
	mux.Handle("GET /api/me", s.requireAuth(http.HandlerFunc(s.handleMe)))
	mux.Handle("GET /api/materials", s.requireAuth(http.HandlerFunc(s.handleListMaterials)))
	mux.Handle("POST /api/ask", s.requireAuth(http.HandlerFunc(s.handleAsk)))

	// 仅教师：上传与重索引（R2.3）。
	mux.Handle("POST /api/materials",
		s.requireAuth(requireRole("teacher", http.HandlerFunc(s.handleCreateMaterial))))
	mux.Handle("POST /api/materials/{id}/reindex",
		s.requireAuth(requireRole("teacher", http.HandlerFunc(s.handleReindex))))

	// 前端静态资源（同源，免跨域）。
	mux.Handle("GET /", s.staticHandler())

	return withLogging(mux)
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
