// CampusClaw 后端入口：装配依赖、建表种子、启动 HTTP 服务。
//
// 对应 tasks：
//   - 1.3 启动即确认 MySQL 与 Qdrant 就绪，失败则打日志并以非零码退出；
//   - 1.4 GET /health：依赖全好返回 200，任一不可用返回 503；
//   - 2.x 登录与登录态；3.x 材料上传；4.x 检索问答（均在 internal/ 内实现）。
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
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

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[campusclaw] ")

	cfg := config.Load()

	checker, err := deps.New(cfg.DSN(), cfg.QdrantURL)
	if err != nil {
		log.Fatalf("依赖检查器初始化失败: %v", err)
	}
	defer checker.Close()

	if cfg.RequireDeps {
		if err := waitForDeps(checker, cfg.DepWait); err != nil {
			log.Fatalf("依赖未就绪，进程退出: %v", err)
		}
		log.Print("依赖就绪：mysql / qdrant")
	} else {
		log.Print("REQUIRE_DEPS=false，已跳过启动依赖预检（仅限本地开发）")
	}

	conn, err := db.Open(cfg.DSN())
	if err != nil {
		log.Fatalf("打开数据库失败: %v", err)
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
		log.Printf("模型网关已启用：%s（chat=%s）", cfg.APIBase, cfg.ChatModel)
	} else {
		log.Print("未配置 LLM_API_KEY：问答走「抽取式」降级（回答直接取材于本班材料）")
	}

	handler := httpapi.New(cfg, st, svc, checker).Handler()
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("HTTP 服务监听 %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Print("收到退出信号，正在关闭…")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Print("已退出")
}

// migrateAndSeed 建表并写入幂等种子数据；失败只告警，不阻断（便于无库环境下调试）。
func migrateAndSeed(conn *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		log.Printf("警告：MySQL 暂不可用（%v），跳过建表与种子", err)
		return
	}
	if err := db.Migrate(ctx, conn); err != nil {
		log.Fatalf("建表失败: %v", err)
	}
	if err := db.Seed(ctx, conn); err != nil {
		log.Fatalf("写入种子数据失败: %v", err)
	}
	log.Print("数据表与种子数据已就绪")
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
		log.Printf("等待依赖就绪… %s", strings.Join(failed, "；"))
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
