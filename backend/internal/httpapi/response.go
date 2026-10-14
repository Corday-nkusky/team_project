// Package httpapi 组装路由、中间件与各接口处理器。
package httpapi

import (
	"encoding/json"
	"net/http"
)

// writeJSON 输出 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr 输出统一形态的错误体：{"error":{"code":...,"message":...}}。
//
// 登录失败一律用同一 code / message，使「账号不存在」与「口令错误」不可区分（R1.2）。
func writeErr(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// decodeJSON 解析请求体，限制体积。
func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}
