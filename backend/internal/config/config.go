// Package config 从环境变量读取运行配置。
//
// 纪律（spec R8 / design D1）：
//   - 部署关键项（监听地址、MySQL、Qdrant、JWT 密钥）必须由环境变量提供，
//     代码内不保留可用默认值；缺失、为空或格式非法时返回错误，由调用方在监听端口前退出。
//   - 调优项（TTL、切片、阈值）保留显式默认值，但一旦被设置就必须能解析，解析失败同样报错。
//   - 错误信息只出现键名与原因，绝不回显键值，避免密钥进入日志。
package config

import (
	"fmt"
	"net/url"
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
	// JWTSecret 用于签发 HS256 access token；必须外置（spec R8.2、R8.4）。
	JWTSecret string
	// AccessTTL access token 有效期（默认 15 分钟）。
	AccessTTL time.Duration
	// RefreshTTL refresh token 有效期（默认 30 天）。
	RefreshTTL time.Duration

	// --- 模型网关（design D3）---
	// APIBase 为 OpenAI 兼容网关根地址；配置了 APIKey 时必填。
	// APIKey 为空时进入「无模型」降级模式：上传仍可索引，问答改用基于检索片段的
	// 抽取式回答，保证系统零密钥可跑通（spec R8.6）。
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
	//
	// 覆盖率默认 0.30 而非 0.25：长查询里两三个偶然命中的子串（如「诺贝尔」含「贝尔」、
	// 「I-量子光合作用」含「量子」）能把 3/11 的覆盖率推过 0.25，跨领域问题仍被误判
	// 为有依据（独立走查 N01 的贝尔不等式用例）。0.30 与正常问句仍有明显间距
	// （实测「教师应该在哪里上传讲义」为 4/7≈0.57）。
	MinKeywordCoverage float64
	MinVectorScore     float64
}

// ValidationError 汇总全部配置问题；Problems 只含键名与原因。
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return "配置校验失败: " + strings.Join(e.Problems, "；")
}

// Load 读取并校验环境变量。返回错误时保证未做任何副作用，由调用方退出。
func Load() (Config, error) {
	var problems []string

	httpAddr := required("HTTP_ADDR", &problems)

	host := required("MYSQL_HOST", &problems)
	port := requiredPort("MYSQL_PORT", &problems)
	user := required("MYSQL_USER", &problems)
	pass := required("MYSQL_PASSWORD", &problems)
	database := required("MYSQL_DATABASE", &problems)

	qdrantURL := required("QDRANT_URL", &problems)
	if qdrantURL != "" && !validHTTPURL(qdrantURL) {
		problems = append(problems, "QDRANT_URL 必须是 http(s) 绝对地址")
	}

	secret := required("JWT_SECRET", &problems)
	if secret != "" {
		if isExampleSecret(secret) {
			problems = append(problems, "JWT_SECRET 仍是示例值，不能用于运行")
		} else if len(secret) < 32 {
			problems = append(problems, "JWT_SECRET 长度不足 32 字符")
		}
	}

	apiKey := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	apiBase := strings.TrimSpace(os.Getenv("LLM_API_BASE"))
	if apiKey != "" {
		if apiBase == "" {
			problems = append(problems, "LLM_API_KEY 已设置时必须同时提供 LLM_API_BASE")
		} else if !validHTTPURL(apiBase) {
			problems = append(problems, "LLM_API_BASE 必须是 http(s) 绝对地址")
		}
	}

	cfg := Config{
		HTTPAddr:  httpAddr,
		MySQLHost: host,
		MySQLPort: port,
		MySQLUser: user,
		MySQLPass: pass,
		MySQLDB:   database,
		QdrantURL: strings.TrimRight(qdrantURL, "/"),

		RequireDeps: optBool("REQUIRE_DEPS", true, &problems),
		DepWait:     time.Duration(optInt("DEP_WAIT_SECONDS", 60, 0, 3600, &problems)) * time.Second,

		JWTSecret:  secret,
		AccessTTL:  time.Duration(optInt("ACCESS_TTL_SECONDS", 15*60, 60, 7*24*3600, &problems)) * time.Second,
		RefreshTTL: time.Duration(optInt("REFRESH_TTL_SECONDS", 30*24*3600, 300, 365*24*3600, &problems)) * time.Second,

		APIBase:    strings.TrimRight(apiBase, "/"),
		APIKey:     apiKey,
		ChatModel:  optString("LLM_CHAT_MODEL", "deepseek-chat"),
		EmbedModel: optString("LLM_EMBED_MODEL", "text-embedding-3-small"),

		ChunkSize:    optInt("CHUNK_SIZE", 500, 50, 4000, &problems),
		ChunkOverlap: optInt("CHUNK_OVERLAP", 80, 0, 2000, &problems),
		TopK:         optInt("RETRIEVAL_TOPK", 4, 1, 20, &problems),

		MinKeywordCoverage: optFloat("MIN_KEYWORD_COVERAGE", 0.30, 0, 1, &problems),
		MinVectorScore:     optFloat("MIN_VECTOR_SCORE", 0.55, 0, 1, &problems),
	}
	if cfg.ChunkOverlap >= cfg.ChunkSize {
		problems = append(problems, "CHUNK_OVERLAP 必须小于 CHUNK_SIZE")
	}

	if len(problems) > 0 {
		return Config{}, &ValidationError{Problems: problems}
	}
	return cfg, nil
}

// DSNFromEnv 只解析数据库连接所需的变量，供 e2e 等本地工具使用。
// 与 Load 不同，它不要求 JWT_SECRET / QDRANT_URL 等运行期配置。
func DSNFromEnv() (string, error) {
	var problems []string
	host := required("MYSQL_HOST", &problems)
	port := requiredPort("MYSQL_PORT", &problems)
	user := required("MYSQL_USER", &problems)
	pass := required("MYSQL_PASSWORD", &problems)
	database := required("MYSQL_DATABASE", &problems)
	if len(problems) > 0 {
		return "", &ValidationError{Problems: problems}
	}
	return dsn(user, pass, host, port, database), nil
}

// DSN 返回 go-sql-driver/mysql 的连接串。
func (c Config) DSN() string {
	return dsn(c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDB)
}

func dsn(user, pass, host string, port int, database string) string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&loc=Local",
		user, pass, host, port, database,
	)
}

// LLMEnabled 表示是否配置了模型网关（无密钥则启用降级模式）。
func (c Config) LLMEnabled() bool { return strings.TrimSpace(c.APIKey) != "" }

// isExampleSecret 拒绝仓库内出现过的示例密钥：它们公开可查，等同于空密钥。
func isExampleSecret(s string) bool {
	lower := strings.ToLower(s)
	for _, marker := range []string{"change-me", "changeme", "dev-insecure", "replace-with", "example", "your-secret"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func validHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// required 读取必填项；缺失或空白都记为问题。绝不回显值。
func required(key string, problems *[]string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		*problems = append(*problems, key+" 未设置或为空")
	}
	return v
}

// requiredPort 读取必填端口；非整数或越界都记为问题。
func requiredPort(key string, problems *[]string) int {
	raw := required(key, problems)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 65535 {
		*problems = append(*problems, key+" 必须是 1–65535 的整数")
		return 0
	}
	return n
}

func optString(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func optInt(key string, def, min, max int, problems *[]string) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		*problems = append(*problems, fmt.Sprintf("%s 必须是 %d–%d 的整数", key, min, max))
		return def
	}
	return n
}

func optBool(key string, def bool, problems *[]string) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		*problems = append(*problems, key+" 必须是 true/false")
		return def
	}
	return b
}

func optFloat(key string, def, min, max float64, problems *[]string) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < min || f > max {
		*problems = append(*problems, fmt.Sprintf("%s 必须是 %g–%g 的数值", key, min, max))
		return def
	}
	return f
}
