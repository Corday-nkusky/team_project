// Command worker 是独立于 Web 服务的索引任务消费者入口。
//
// 与 cmd/server 共用同一份配置、同一套 QA 链路，但不监听端口、不建表、不写种子：
// 建表由 Web 服务启动时完成，Compose 用 depends_on: backend(service_healthy) 保证顺序。
// 这样 docker compose stop worker / start worker 就能单独停启消费者（判据二）。

package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/config"
	"github.com/Corday-nkusky/team_project/backend/internal/db"
	"github.com/Corday-nkusky/team_project/backend/internal/indexer"
	"github.com/Corday-nkusky/team_project/backend/internal/llm"
	"github.com/Corday-nkusky/team_project/backend/internal/qa"
	"github.com/Corday-nkusky/team_project/backend/internal/qdrant"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

// version 由构建期注入，与 server 使用同一编号。
var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("启动中止：配置无效", "reason", err.Error())
		os.Exit(1)
	}

	conn, err := db.Open(cfg.DSN())
	if err != nil {
		slog.Error("启动中止：打开数据库失败", "error", err.Error())
		os.Exit(1)
	}
	defer conn.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := waitForTaskTable(ctx, conn); err != nil {
		slog.Error("启动中止：任务表未就绪", "error", err.Error(), "version", version)
		os.Exit(1)
	}

	st := store.New(conn)
	vec := qdrant.New(cfg.QdrantURL, "")
	model := llm.New(cfg.APIBase, cfg.APIKey, cfg.ChatModel, cfg.EmbedModel)
	svc := qa.New(st, vec, model, cfg.TopK, cfg.ChunkSize, cfg.ChunkOverlap,
		cfg.MinKeywordCoverage, cfg.MinVectorScore)

	w := indexer.New(st, svc, indexer.FromConfig(cfg))
	slog.Info("worker_ready", "version", version, "llm_enabled", model.Enabled())
	if err := w.Run(ctx); err != nil {
		slog.Error("worker_abnormal_exit", "error", err.Error())
		os.Exit(1)
	}
}

// waitForTaskTable 等待 Web 服务完成迁移。worker 不并发执行 DDL，
// 避免两个进程同时做 ALTER TABLE；超时后如实退出，不假装就绪。
func waitForTaskTable(ctx context.Context, conn *sql.DB) error {
	deadline := time.Now().Add(90 * time.Second)
	for {
		probe, cancel := context.WithTimeout(ctx, 3*time.Second)
		var n int
		err := conn.QueryRowContext(probe, `SELECT COUNT(*) FROM index_tasks`).Scan(&n)
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !time.Now().Before(deadline) {
			return err
		}
		slog.Info("等待任务表就绪", "error", err.Error())
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
