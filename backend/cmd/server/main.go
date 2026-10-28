// CampusClaw 后端入口：装配依赖、建表种子、启动 HTTP 服务。
//
// 对应 spec：
//   - R8 配置外置：缺项/非法项在监听前以非零码退出，日志只报键名；
//   - R9 GET /health：免认证，含 version/time/deps，依赖不可用时 503；
//   - R10 结构化日志：单行 JSON，每条请求一个 request_id。
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/config"
	"github.com/Corday-nkusky/team_project/backend/internal/db"
	"github.com/Corday-nkusky/team_project/backend/internal/deps"
	"github.com/Corday-nkusky/team_project/backend/internal/httpapi"
	"github.com/Corday-nkusky/team_project/backend/internal/llm"
	"github.com/Corday-nkusky/team_project/backend/internal/qa"
	"github.com/Corday-nkusky/team_project/backend/internal/qdrant"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

// version 由构建期注入：go build -ldflags "-X main.version=v0.0.1-skeleton"。
var version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		// 只报键名与原因，绝不回显键值（spec R8.2 / R8.4）。
		slog.Error("启动中止：配置无效", "reason", err.Error())
		os.Exit(1)
	}

	checker, err := deps.New(cfg.DSN(), cfg.QdrantURL)
	if err != nil {
		slog.Error("启动中止：依赖检查器初始化失败", "error", err.Error())
		os.Exit(1)
	}
	defer checker.Close()

	if cfg.RequireDeps {
		if err := waitForDeps(checker, cfg.DepWait); err != nil {
			slog.Error("启动中止：依赖未就绪", "error", err.Error())
			os.Exit(1)
		}
		slog.Info("依赖就绪", "mysql", "ok", "qdrant", "ok")
	} else {
		slog.Warn("已跳过启动依赖预检（仅限本地开发）", "require_deps", false)
	}

	conn, err := db.Open(cfg.DSN())
	if err != nil {
		slog.Error("启动中止：打开数据库失败", "error", err.Error())
		os.Exit(1)
	}
	defer conn.Close()

	// 建表与种子：仅在 MySQL 可用时执行；不可用时开发模式仍可启动（接口将报错）。
	migrateAndSeed(conn)

	st := store.New(conn)
	vec := qdrant.New(cfg.QdrantURL, "")
	model := llm.New(cfg.APIBase, cfg.APIKey, cfg.ChatModel, cfg.EmbedModel)
	svc := qa.New(st, vec, model, cfg.TopK, cfg.ChunkSize, cfg.ChunkOverlap,
		cfg.MinKeywordCoverage, cfg.MinVectorScore)
	if model.Enabled() {
		slog.Info("模型网关已启用", "api_base", cfg.APIBase, "chat_model", cfg.ChatModel)
	} else {
		slog.Info("未配置 LLM_API_KEY，问答走抽取式降级")
	}

	handler := httpapi.New(cfg, st, svc, checker, version).Handler()
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("server_starting", "http_addr", cfg.HTTPAddr, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("启动中止：HTTP 服务异常退出", "error", err.Error())
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("收到退出信号，正在关闭")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	slog.Info("已退出")
}

// migrateAndSeed 建表并写入幂等种子数据；失败只告警，不阻断（便于无库环境下调试）。
func migrateAndSeed(conn *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		slog.Warn("MySQL 暂不可用，跳过建表与种子", "error", err.Error())
		return
	}
	if err := db.Migrate(ctx, conn); err != nil {
		slog.Error("启动中止：建表失败", "error", err.Error())
		os.Exit(1)
	}
	if err := db.Seed(ctx, conn); err != nil {
		slog.Error("启动中止：写入种子数据失败", "error", err.Error())
		os.Exit(1)
	}
	slog.Info("数据表与种子数据已就绪")
}

// waitForDeps 轮询依赖直至全部就绪或超过 wait 时限。
func waitForDeps(c *deps.Checker, wait time.Duration) error {
	deadline := time.Now().Add(wait)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		failed := failures(c.Check(ctx))
		cancel()

		if len(failed) == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return errors.New(strings.Join(failed, "；"))
		}
		slog.Info("等待依赖就绪", "failed", strings.Join(failed, "；"))
		time.Sleep(2 * time.Second)
	}
}

// failures 把检查结果压成可读的失败列表。
func failures(results map[string]error) []string {
	var out []string
	for name, err := range results {
		if err != nil {
			out = append(out, fmt.Sprintf("%s: %v", name, err))
		}
	}
	return out
}
