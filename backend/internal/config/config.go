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
	}
}

// DSN 返回 go-sql-driver/mysql 的连接串。
func (c Config) DSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&loc=Local",
		c.MySQLUser, c.MySQLPass, c.MySQLHost, c.MySQLPort, c.MySQLDB,
	)
}

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
