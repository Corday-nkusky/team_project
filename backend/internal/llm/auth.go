package llm

// 智谱 v4 网关（open.bigmodel.cn / api.z.ai）不接受把 API Key 直接当 Bearer 令牌：
// 需要用 Key 的 id.secret 形式签发一枚短时效 JWT（HS256）再放进 Authorization。
// 其他 OpenAI 兼容网关仍使用静态 Bearer，行为不变。

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrAuthConfig 表示认证材料本身不成立（同样输入重试必然同结果），
// worker 会直接判为不可重试失败，不消耗重试额度。
var ErrAuthConfig = errors.New("模型网关认证配置无效")

func isZhipuBase(base string) bool {
	lower := strings.ToLower(base)
	return strings.Contains(lower, "bigmodel.cn") || strings.Contains(lower, "z.ai")
}

// authHeader 生成 Authorization 头。智谱网关要求 id.secret 形式的 Key；
// 缺 secret 时给出可诊断的错误，而不是让上游回一句含糊的 401。
func (c *Client) authHeader() (string, error) {
	if !isZhipuBase(c.base) {
		return "Bearer " + c.key, nil
	}
	id, secret, ok := strings.Cut(c.key, ".")
	if !ok || id == "" || secret == "" {
		return "", fmt.Errorf("%w: 智谱 v4 网关需要 id.secret 形式的完整 API Key", ErrAuthConfig)
	}
	token, err := zhipuJWT(id, secret, time.Hour)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrAuthConfig, err)
	}
	return "Bearer " + token, nil
}

// zhipuJWT 按智谱约定签发 HS256 JWT：header 带 sign_type，payload 用毫秒时间戳。
func zhipuJWT(id, secret string, ttl time.Duration) (string, error) {
	now := time.Now()
	header, err := jwtSegment(map[string]string{"alg": "HS256", "sign_type": "SIGN"})
	if err != nil {
		return "", err
	}
	payload, err := jwtSegment(map[string]any{
		"api_key":   id,
		"exp":       now.Add(ttl).UnixMilli(),
		"timestamp": now.UnixMilli(),
	})
	if err != nil {
		return "", err
	}
	unsigned := header + "." + payload
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + signature, nil
}

func jwtSegment(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
