package config

import (
	"strings"
	"testing"
)

// setBaseEnv 提供一套齐全且合法的必需项；各用例在此基础上增删。
func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("MYSQL_HOST", "127.0.0.1")
	t.Setenv("MYSQL_PORT", "3306")
	t.Setenv("MYSQL_USER", "campusclaw")
	t.Setenv("MYSQL_PASSWORD", "test-password")
	t.Setenv("MYSQL_DATABASE", "campusclaw")
	t.Setenv("QDRANT_URL", "http://127.0.0.1:6333")
	t.Setenv("JWT_SECRET", strings.Repeat("k", 40))
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_API_BASE", "")
}

func TestLoadValid(t *testing.T) {
	setBaseEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("期望成功，得到错误: %v", err)
	}
	if cfg.MySQLPort != 3306 || cfg.JWTSecret == "" {
		t.Fatalf("关键配置未解析: %+v", cfg)
	}
	if cfg.LLMEnabled() {
		t.Fatal("未设置 LLM_API_KEY 时应为降级模式（R8.6）")
	}
	if !strings.Contains(cfg.DSN(), "tcp(127.0.0.1:3306)/campusclaw") {
		t.Fatalf("DSN 不符合预期: %s", cfg.DSN())
	}
}

func TestLoadMissingJWTSecret(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("JWT_SECRET", "   ")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("缺少 JWT_SECRET 应报错并含键名，得到: %v", err)
	}
}

func TestLoadRejectsExampleSecret(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("JWT_SECRET", "dev-insecure-secret-change-me")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "示例值") {
		t.Fatalf("示例密钥应被拒绝，得到: %v", err)
	}
	if strings.Contains(err.Error(), "dev-insecure-secret-change-me") {
		t.Fatal("错误信息不得回显密钥值（R8.4）")
	}
}

func TestLoadRejectsShortSecret(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("JWT_SECRET", "short")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("过短密钥应被拒绝，得到: %v", err)
	}
}

func TestLoadMissingMySQLItems(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("MYSQL_HOST", "")
	t.Setenv("MYSQL_PORT", "")
	t.Setenv("MYSQL_USER", "")
	t.Setenv("MYSQL_PASSWORD", "")
	_, err := Load()
	if err == nil {
		t.Fatal("缺少 MySQL 连接项应报错（R8.3）")
	}
	for _, key := range []string{"MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD"} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("错误应逐项指明 %s，得到: %v", key, err)
		}
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	for _, raw := range []string{"abc", "0", "70000"} {
		t.Run(raw, func(t *testing.T) {
			setBaseEnv(t)
			t.Setenv("MYSQL_PORT", raw)
			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), "MYSQL_PORT") {
				t.Fatalf("非法端口 %q 应报错并指明 MYSQL_PORT，得到: %v", raw, err)
			}
		})
	}
}

func TestLoadLLMKeyRequiresBase(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("LLM_API_KEY", "sk-test")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "LLM_API_BASE") {
		t.Fatalf("配置密钥但缺少 API 地址应报错，得到: %v", err)
	}
}

func TestLoadRejectsBadChunkOverlap(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("CHUNK_SIZE", "100")
	t.Setenv("CHUNK_OVERLAP", "100")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "CHUNK_OVERLAP") {
		t.Fatalf("重叠不小于切片长度应报错，得到: %v", err)
	}
}

func TestDSNFromEnvDoesNotRequireJWT(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("JWT_SECRET", "")
	dsn, err := DSNFromEnv()
	if err != nil {
		t.Fatalf("DSNFromEnv 不应要求 JWT_SECRET: %v", err)
	}
	if !strings.Contains(dsn, "campusclaw") {
		t.Fatalf("DSN 异常: %s", dsn)
	}
}
