package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
)

type ctxKey string

const (
	claimsKey    ctxKey = "claims"
	requestIDKey ctxKey = "request_id"
)

// claimsFrom 从请求上下文取出已认证的用户身份。
func claimsFrom(ctx context.Context) (auth.Claims, bool) {
	c, ok := ctx.Value(claimsKey).(auth.Claims)
	return c, ok
}

// requestIDFrom 从请求上下文取出本次请求的标识（spec R10）。
func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
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

// withLogging 为每条请求写一条单行 JSON 访问日志（spec R10 / design D2）。
//
// 只记录方法、路径、状态、耗时与请求标识；不记录请求头、请求体与查询串，
// 以免口令或令牌进入日志（R10.3）。
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := acceptableRequestID(r.Header.Get("X-Request-ID"))
		if rid == "" {
			rid = newRequestID()
		}
		w.Header().Set("X-Request-ID", rid)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey, rid))

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		slog.Info("http_request",
			"request_id", rid,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// logServerError 只在确实返回 500 的分支调用（design D6）。
// 字段固定为 request_id / operation / error；不记录请求体、令牌或 SQL 原文。
func logServerError(r *http.Request, operation string, err error) {
	slog.ErrorContext(r.Context(), "http_server_error",
		"request_id", requestIDFrom(r.Context()),
		"operation", operation,
		"error", err.Error(),
	)
}

// acceptableRequestID 透传客户端 ID 的条件：长度 1–128 且全部为可打印 ASCII（R10.5）。
// 不满足时返回空串，由调用方重新生成。
func acceptableRequestID(raw string) string {
	if len(raw) < 1 || len(raw) > 128 {
		return ""
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] > 0x7e {
			return ""
		}
	}
	return raw
}

// newRequestID 生成 16 字节随机数的十六进制形式。
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand 失败极罕见；退化为时间戳，仍保证非空且单行。
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}
