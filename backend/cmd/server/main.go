// CampusClaw 后端入口：最小 HTTP 服务 + 依赖探活。
//
// 本文件对应 tasks 1.3 / 1.4：
//   - 1.3 启动即确认 MySQL 与 Qdrant 就绪，失败则打日志并以非零码退出；
//   - 1.4 GET /health：依赖全好返回 200，任一不可用返回 503。
package main

import (
	"context"
	"encoding/json"
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
	"github.com/Corday-nkusky/team_project/backend/internal/deps"
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

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(checker))

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           mux,
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

// healthHandler 依赖全好 → 200；任一不可用 → 503（如实报告，不假装健康）。
func healthHandler(c *deps.Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		status := http.StatusOK
		body := map[string]any{"status": "ok"}
		detail := map[string]string{}

		for name, err := range c.Check(ctx) {
			if err != nil {
				detail[name] = err.Error()
				status = http.StatusServiceUnavailable
			} else {
				detail[name] = "ok"
			}
		}
		if status != http.StatusOK {
			body["status"] = "degraded"
		}
		body["deps"] = detail

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}
}
