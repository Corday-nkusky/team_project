package llm

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAuthHeaderStaticBearerForOtherGateways(t *testing.T) {
	c := New("https://example.com/v1", "sk-plain", "chat", "embed")
	got, err := c.authHeader()
	if err != nil || got != "Bearer sk-plain" {
		t.Fatalf("非智谱网关应使用静态 Bearer: %q err=%v", got, err)
	}
}

func TestAuthHeaderRejectsIncompleteZhipuKey(t *testing.T) {
	// 故意用假 Key：只验证「没有点号」会被拒绝，不能用真实 Key 的 id 段落回仓库。
	c := New("https://open.bigmodel.cn/api/paas/v4", "test-id-without-secret", "glm", "embedding-3")
	_, err := c.authHeader()
	if !errors.Is(err, ErrAuthConfig) {
		t.Fatalf("缺少 secret 应返回 ErrAuthConfig，得到 %v", err)
	}
	if !strings.Contains(err.Error(), "id.secret") {
		t.Fatalf("错误信息应指明 Key 格式: %v", err)
	}
}

func TestZhipuJWTSignatureAndPayload(t *testing.T) {
	token, err := zhipuJWT("keyid", "secret", time.Hour)
	if err != nil {
		t.Fatalf("签发失败: %v", err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("JWT 应为三段，得到 %d 段", len(parts))
	}
	headerRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerRaw, &header); err != nil || header["alg"] != "HS256" || header["sign_type"] != "SIGN" {
		t.Fatalf("header 不符: %q err=%v", headerRaw, err)
	}
	payloadRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["api_key"] != "keyid" || payload["exp"].(float64) <= payload["timestamp"].(float64) {
		t.Fatalf("payload 不符: %s", payloadRaw)
	}
	mac := hmac.New(sha256.New, []byte("secret"))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if parts[2] != want {
		t.Fatal("签名不是用 secret 对 header.payload 做的 HMAC-SHA256")
	}
}
