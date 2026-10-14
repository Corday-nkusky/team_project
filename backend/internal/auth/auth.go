// Package auth 提供口令哈希、access token（HS256 JWT）与 refresh token 的工具。
//
// access token 为无状态 JWT（design D8）；refresh token 为随机串，仅存摘要入库。
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Claims 是 access token 的载荷。
type Claims struct {
	UserID  int64  `json:"uid"`
	Role    string `json:"role"`
	ClassID int64  `json:"cid"`
	Name    string `json:"name"`
	Exp     int64  `json:"exp"`
	Iat     int64  `json:"iat"`
}

// ErrInvalidToken 表示 token 缺失、格式错误、签名不符或已过期。
var ErrInvalidToken = errors.New("token 无效")

// --- 口令 ---

// HashPassword 用 bcrypt 生成口令哈希（不存明文）。
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword 校验口令；错误口令与不存在的账号都返回 false，不区分原因（R1.2）。
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// --- access token（HS256 JWT，标准库实现，零第三方依赖）---

// SignAccess 签发 access token。
func SignAccess(secret string, c Claims, ttl time.Duration) (string, error) {
	now := time.Now()
	c.Iat = now.Unix()
	c.Exp = now.Add(ttl).Unix()

	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	pb, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	signing := b64(hb) + "." + b64(pb)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	return signing + "." + b64(mac.Sum(nil)), nil
}

// VerifyAccess 校验并解析 access token。
func VerifyAccess(secret, token string) (Claims, error) {
	var c Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return c, ErrInvalidToken
	}
	signing := parts[0] + "." + parts[1]

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signing))
	want := b64(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return c, ErrInvalidToken
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return c, ErrInvalidToken
	}
	if err := json.Unmarshal(pb, &c); err != nil {
		return c, ErrInvalidToken
	}
	if time.Now().Unix() >= c.Exp {
		return c, ErrInvalidToken
	}
	return c, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// --- refresh token ---

// NewRefreshToken 生成随机 refresh token 及其入库用摘要（SHA-256 hex）。
// 明文只返回给客户端一次，服务端只保存摘要（design D8）。
func NewRefreshToken() (plain, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("生成随机令牌失败: %w", err)
	}
	plain = base64.RawURLEncoding.EncodeToString(buf)
	return plain, HashToken(plain), nil
}

// HashToken 返回 token 的 SHA-256 十六进制摘要（用于入库比对）。
func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
