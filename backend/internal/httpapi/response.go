// Package httpapi 组装路由、中间件与各接口处理器。
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
)

// maxJSONBody 是普通 JSON 接口的请求体上限（上传接口使用单独上限）。
const maxJSONBody = 1 << 20

// statusError 是带 HTTP 状态码与错误码的客户端错误，供 handler 统一映射。
type statusError struct {
	status  int
	code    string
	message string
}

func (e *statusError) Error() string { return e.message }

func badRequest(code, message string) error {
	return &statusError{status: http.StatusBadRequest, code: code, message: message}
}

func payloadTooLarge(message string) error {
	return &statusError{status: http.StatusRequestEntityTooLarge, code: "payload_too_large", message: message}
}

func unsupportedMedia(message string) error {
	return &statusError{status: http.StatusUnsupportedMediaType, code: "unsupported_media_type", message: message}
}

var errTrailingJSON = &statusError{
	status: http.StatusBadRequest, code: "bad_request",
	message: "请求体只能包含一个 JSON 对象",
}

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

// writeDecodeErr 把 JSON 解码错误映射为 413 / 415 / 400（spec R11）。
func writeDecodeErr(w http.ResponseWriter, err error, fallbackMessage string) {
	var se *statusError
	if errors.As(err, &se) {
		writeErr(w, se.status, se.code, se.message)
		return
	}
	writeErr(w, http.StatusBadRequest, "bad_request", fallbackMessage)
}

// writeDecodeErrAs 在保留 413 / 415 的前提下，把其余解码错误映射为指定状态
// （例如 refresh 接口把格式错误统一视为凭证无效，避免泄漏细节）。
func writeDecodeErrAs(w http.ResponseWriter, err error, status int, code, message string) {
	var se *statusError
	if errors.As(err, &se) && (se.status == http.StatusRequestEntityTooLarge ||
		se.status == http.StatusUnsupportedMediaType) {
		writeErr(w, se.status, se.code, se.message)
		return
	}
	writeErr(w, status, code, message)
}

// decodeJSON 解析请求体：要求 application/json、限制 1 MiB、拒绝尾随内容。
func decodeJSON(r *http.Request, v any) error {
	return decodeJSONLimit(r, v, maxJSONBody)
}

// decodeJSONLimit 与 decodeJSON 相同，但允许上传接口使用更大的上限。
func decodeJSONLimit(r *http.Request, v any, limit int64) error {
	if err := requireJSONContentType(r); err != nil {
		return err
	}
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, limit))
	if err := dec.Decode(v); err != nil {
		return normalizeDecodeError(err)
	}
	// 顶层值之后必须直接结束；有任何 token 或垃圾内容都视为请求体不合法（R11.5）。
	if _, err := dec.Token(); err == nil {
		return errTrailingJSON
	} else if !errors.Is(err, io.EOF) {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return payloadTooLarge("请求体超过大小上限")
		}
		return errTrailingJSON
	}
	return nil
}

func requireJSONContentType(r *http.Request) error {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return unsupportedMedia("请求必须使用 application/json")
	}
	mediaType, _, err := mime.ParseMediaType(ct)
	if err != nil || mediaType != "application/json" {
		return unsupportedMedia("请求必须使用 application/json")
	}
	return nil
}

func normalizeDecodeError(err error) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return payloadTooLarge("请求体超过大小上限")
	}
	return fmt.Errorf("解析 JSON 失败: %w", err)
}
