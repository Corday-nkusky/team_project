// Package config 从环境变量读取运行配置。
//
// 纪律：连接地址与密钥一律经环境变量注入，不写进仓库（样例见 deploy/.env.example）。
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 是全部运行期配置。
type Config struct {
	HTTPAddr string

	MySQLHost string
	MySQLPort int
	MySQLUser string
	MySQLPass string
	MySQLDB   string

	QdrantURL string

	// RequireDeps 为 true 时，启动阶段必须确认依赖就绪，否则进程退出（tasks 1.3）。
	// 本地开发可设 REQUIRE_DEPS=false：服务照常启动，/health 如实报告降级。
	RequireDeps bool

	// DepWait 是启动阶段等待依赖就绪的最长时间。
	DepWait time.Duration

	// --- 认证（spec R1 / design D5、D8）---
	// JWTSecret 用于签发 HS256 access token；必须外置，不设默认值即可跑通本地开发（仅限开发）。
	JWTSecret string
	// AccessTTL access token 有效期（默认 15 分钟）。
	AccessTTL time.Duration
	// RefreshTTL refresh token 有效期（默认 30 天）。
	RefreshTTL time.Duration

	// --- 模型网关（design D3）---
	// APIBase 为 OpenAI 兼容网关根地址；APIKey 为空时进入「无模型」降级模式
	// （上传仍可索引，问答改用基于检索片段的抽取式回答，保证系统零密钥可跑通）。
	APIBase string
	APIKey  string
	// ChatModel 生成回答的模型名；EmbedModel 生成向量用的嵌入模型名。
	ChatModel  string
	EmbedModel string

	// --- 检索（design D4）---
	// ChunkSize / ChunkOverlap 为切片参数（按字符计，适配中文）。
	ChunkSize    int
	ChunkOverlap int
	// TopK 是检索返回的片段数上限。
	TopK int
	// MinKeywordCoverage / MinVectorScore 是「有无依据」的判定门槛（spec R5.2）：
	// 命中信息量充足的查询词达到该比例，或向量相似度达到该分值，才认为材料中确有相关内容。
	MinKeywordCoverage float64
	MinVectorScore     float64
}

// Load 读取环境变量并套用默认值。
func Load() Config {
	return Config{
		HTTPAddr:    env("HTTP_ADDR", ":8080"),
		MySQLHost:   env("MYSQL_HOST", "127.0.0.1"),
		MySQLPort:   envInt("MYSQL_PORT", 3306),
		MySQLUser:   env("MYSQL_USER", "campusclaw"),
		MySQLPass:   env("MYSQL_PASSWORD", ""),
		MySQLDB:     env("MYSQL_DATABASE", "campusclaw"),
		QdrantURL:   strings.TrimRight(env("QDRANT_URL", "http://127.0.0.1:6333"), "/"),
		RequireDeps: envBool("REQUIRE_DEPS", true),
		DepWait:     time.Duration(envInt("DEP_WAIT_SECONDS", 60)) * time.Second,

		JWTSecret:  env("JWT_SECRET", "dev-insecure-secret-change-me"),
		AccessTTL:  time.Duration(envInt("ACCESS_TTL_SECONDS", 15*60)) * time.Second,
		RefreshTTL: time.Duration(envInt("REFRESH_TTL_SECONDS", 30*24*3600)) * time.Second,

		APIBase:    strings.TrimRight(env("LLM_API_BASE", "https://api.deepseek.com/v1"), "/"),
		APIKey:     env("LLM_API_KEY", ""),
		ChatModel:  env("LLM_CHAT_MODEL", "deepseek-chat"),
		EmbedModel: env("LLM_EMBED_MODEL", "text-embedding-3-small"),

		ChunkSize:    envInt("CHUNK_SIZE", 500),
		ChunkOverlap: envInt("CHUNK_OVERLAP", 80),
		TopK:         envInt("RETRIEVAL_TOPK", 4),

		MinKeywordCoverage: envFloat("MIN_KEYWORD_COVERAGE", 0.25),
		MinVectorScore:     envFloat("MIN_VECTOR_SCORE", 0.55),
	}
}

// DSN 返回 go-sql-driver/mysql 的连接串。
func (c Config) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&loc=Local",
		c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDB,
	)
}

// LLMEnabled 表示是否配置了模型网关（无密钥则启用降级模式）。
func (c Config) LLMEnabled() bool { return strings.TrimSpace(c.APIKey) != "" }

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}
