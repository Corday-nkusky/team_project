// Package deps 负责外部依赖（MySQL / Qdrant）的连通性检查。
package deps

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Checker 持有依赖连接，供启动预检与 /health 共用。
type Checker struct {
	db     *sql.DB
	qdrant string
	client *http.Client
}

// New 建立 Checker。注意 sql.Open 本身不建立连接，真正的连通性由 Check 判定。
func New(dsn, qdrantURL string) (*Checker, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL 连接池失败: %w", err)
	}
	db.SetConnMaxLifetime(3 * time.Minute)
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)

	return &Checker{
		db:     db,
		qdrant: strings.TrimRight(qdrantURL, "/"),
		client: &http.Client{Timeout: 3 * time.Second},
	}, nil
}

// Close 释放连接池。
func (c *Checker) Close() error { return c.db.Close() }

// Check 返回逐项检查结果；值为 nil 表示该项就绪。永不 panic。
func (c *Checker) Check(ctx context.Context) map[string]error {
	return map[string]error{
		"mysql":  c.checkMySQL(ctx),
		"qdrant": c.checkQdrant(ctx),
	}
}

func (c *Checker) checkMySQL(ctx context.Context) error {
	if err := c.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping 失败: %w", err)
	}
	return nil
}

func (c *Checker) checkQdrant(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.qdrant+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("/healthz 返回 %d", resp.StatusCode)
	}
	return nil
}
