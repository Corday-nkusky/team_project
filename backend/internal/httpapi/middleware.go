package httpapi

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
)

type ctxKey string

const claimsKey ctxKey = "claims"

// claimsFrom 从请求上下文取出已认证的用户身份。
func claimsFrom(ctx context.Context) (auth.Claims, bool) {
	c, ok := ctx.Value(claimsKey).(auth.Claims)
	return c, ok
}

// requireAuth 校验 `Authorization: Bearer <access token>`。
//
// 缺失、格式错误、签名不符或过期一律 401（R1.3 / R1.4），不区分原因以免泄漏信息。
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(raw, prefix) {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "缺少或格式不正确的 Authorization 头")
			return
		}
		claims, err := auth.VerifyAccess(s.cfg.JWTSecret, strings.TrimSpace(strings.TrimPrefix(raw, prefix)))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "凭证无效或已过期")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), claimsKey, claims)))
	})
}

// requireRole 限制某接口的角色（如上传仅教师可用，R2.3）。
func requireRole(role string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := claimsFrom(r.Context())
		if !ok || c.Role != role {
			writeErr(w, http.StatusForbidden, "forbidden", "当前角色无权执行该操作")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// statusRecorder 记录响应状态码供访问日志使用。
//
// 关键：必须透传 Flush()（design D6 / tasks 4.5）——否则 SSE 会退化为「一次性返回」。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap 供 http.ResponseController 定位底层 writer。
func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// withLogging 打印访问日志（课纲要的「日志」证据）。
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}
