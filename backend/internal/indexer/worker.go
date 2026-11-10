// Package indexer 是独立于 Web 的索引任务消费者（第 8 课「消费者进程」）。
//
// 队列持久化在 MySQL 的 index_tasks 表里：FOR UPDATE SKIP LOCKED 负责竞争消费，
// lease_until 负责超时回收，两者合起来提供「至少一次投递」。重复执行由
// chunks 唯一约束与确定性 Qdrant 点 ID 吸收，不是靠「先查一遍」。

package indexer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/config"
	"github.com/Corday-nkusky/team_project/backend/internal/llm"
	"github.com/Corday-nkusky/team_project/backend/internal/qa"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

// IndexService 是消费者需要的全部检索侧能力。
type IndexService interface {
	Index(ctx context.Context, materialID, classID int64, content string) error
}

// TaskStore 是消费者需要的任务表与审计读写；*store.Store 满足该接口。
type TaskStore interface {
	RecoverStaleTasks(ctx context.Context) (int64, error)
	ClaimTasks(ctx context.Context, limit int, lease time.Duration) ([]store.IndexTask, error)
	GetMaterial(ctx context.Context, id, classID int64) (store.Material, error)
	CompleteTask(ctx context.Context, taskID string) error
	FailTask(ctx context.Context, taskID, errMsg string, retry bool, backoff time.Duration, terminal string) error
	Audit(ctx context.Context, actorID *int64, actorName string, classID *int64, action, result, detail string) error
}

// Options 是 worker 的调优参数，全部由 config 外置注入。
type Options struct {
	Poll          time.Duration
	Lease         time.Duration
	TaskTimeout   time.Duration
	MaxAttempts   int
	Backoff       time.Duration
	BackoffMax    time.Duration
	Batch         int
	StatusTimeout time.Duration
}

// FromConfig 把配置转成 worker 参数；租约必须长于单任务超时，
// 否则任务还在执行就被别的消费者当成崩溃回收。
func FromConfig(cfg config.Config) Options {
	lease := cfg.WorkerLease
	if minLease := cfg.WorkerTaskTimeout + 30*time.Second; lease < minLease {
		lease = minLease
	}
	return Options{
		Poll: cfg.WorkerPoll, Lease: lease, TaskTimeout: cfg.WorkerTaskTimeout,
		MaxAttempts: cfg.WorkerMaxAttempts, Backoff: cfg.WorkerBackoff,
		BackoffMax: cfg.WorkerBackoffMax, Batch: cfg.WorkerBatch,
		StatusTimeout: 15 * time.Second,
	}
}

// failureClass 是失败分类的结果，判据是「同样输入重试是否可能得到不同结果」。
type failureClass int

const (
	classTransient failureClass = iota
	classPermanent
	classRateLimited
)

func classify(err error) failureClass {
	switch {
	case errors.Is(err, qa.ErrPermanent), errors.Is(err, llm.ErrAuth),
		errors.Is(err, llm.ErrAuthConfig), errors.Is(err, store.ErrNotFound):
		return classPermanent
	case errors.Is(err, llm.ErrRateLimited):
		return classRateLimited
	default:
		return classTransient
	}
}

// Worker 串行消费任务；单实例起步，多实例时 SKIP LOCKED 保证一条任务只被一个消费者领取。
type Worker struct {
	st       TaskStore
	svc      IndexService
	opt      Options
	jitterFn func() float64
}

// New 构造 Worker。jitter 默认取 math/rand，单测可注入固定值。
func New(st TaskStore, svc IndexService, opt Options) *Worker {
	return &Worker{st: st, svc: svc, opt: opt, jitterFn: rand.Float64}
}

// Run 阻塞运行，直到 ctx 被取消（SIGINT/SIGTERM 优雅停止）。
// 启动时先回收上次进程遗留的 running 任务，保证判据二与判据三可复现。
func (w *Worker) Run(ctx context.Context) error {
	if n, err := w.st.RecoverStaleTasks(ctx); err != nil {
		slog.Error("worker_recover_failed", "error", err.Error())
	} else if n > 0 {
		slog.Info("worker_recovered", "tasks", n)
	}
	slog.Info("worker_started", "poll_ms", w.opt.Poll.Milliseconds(),
		"lease_s", int64(w.opt.Lease.Seconds()), "batch", w.opt.Batch)

	ticker := time.NewTicker(w.opt.Poll)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			slog.Info("worker_stopped")
			return nil
		}
		tasks, err := w.st.ClaimTasks(ctx, w.opt.Batch, w.opt.Lease)
		if err != nil {
			if ctx.Err() != nil {
				slog.Info("worker_stopped")
				return nil
			}
			slog.Error("worker_claim_failed", "error", err.Error())
		} else {
			for _, t := range tasks {
				if ctx.Err() != nil {
					slog.Info("worker_stopped")
					return nil
				}
				w.handle(ctx, t)
			}
		}
		select {
		case <-ctx.Done():
			slog.Info("worker_stopped")
			return nil
		case <-ticker.C:
		}
	}
}

// handle 执行一条任务。关键在于任务执行使用独立 context（不是 HTTP 请求 ctx），
// 且状态回写使用另一个不受任务超时影响的 context。
func (w *Worker) handle(ctx context.Context, t store.IndexTask) {
	start := time.Now()
	taskCtx, cancel := context.WithTimeout(ctx, w.opt.TaskTimeout)
	m, err := w.st.GetMaterial(taskCtx, t.MaterialID, t.ClassID)
	if err == nil {
		err = w.svc.Index(taskCtx, t.MaterialID, t.ClassID, m.Content)
	}
	cancel()

	writeCtx, writeCancel := context.WithTimeout(context.Background(), w.opt.StatusTimeout)
	defer writeCancel()

	if err == nil {
		if e := w.st.CompleteTask(writeCtx, t.TaskID); e != nil {
			slog.Error("worker_complete_failed", "task_id", t.TaskID, "error", e.Error())
			return
		}
		w.audit(writeCtx, t, "ok", "")
		slog.Info("task_succeeded", "task_id", t.TaskID, "material_id", t.MaterialID,
			"class_id", t.ClassID, "attempts", t.Attempts, "duration_ms", time.Since(start).Milliseconds())
		return
	}

	class := classify(err)
	retry := false
	terminal := store.TaskFailed
	backoff := time.Duration(0)
	switch class {
	case classPermanent:
		terminal = store.TaskFailed
	case classRateLimited:
		// 课程网关按天限额，短退避重试只会继续烧额度，直接转死信等人介入。
		terminal = store.TaskDead
	default:
		if t.Attempts >= w.opt.MaxAttempts {
			terminal = store.TaskDead
		} else {
			retry = true
			backoff = w.backoff(t.Attempts)
		}
	}
	if e := w.st.FailTask(writeCtx, t.TaskID, err.Error(), retry, backoff, terminal); e != nil {
		slog.Error("worker_fail_failed", "task_id", t.TaskID, "error", e.Error())
		return
	}
	status := "fail"
	if retry {
		status = "retry"
	}
	w.audit(writeCtx, t, status, err.Error())
	slog.Info("task_failed", "task_id", t.TaskID, "material_id", t.MaterialID,
		"class_id", t.ClassID, "attempts", t.Attempts, "class", int(class),
		"status", status, "error", clip(err.Error(), 200),
		"duration_ms", time.Since(start).Milliseconds())
}

// backoff 是 base * 2^(attempts-1) 叠加 ±20% 抖动，并受上限约束。
func (w *Worker) backoff(attempts int) time.Duration {
	d := w.opt.Backoff
	for i := 1; i < attempts && d < w.opt.BackoffMax; i++ {
		if d > w.opt.BackoffMax/2 {
			d = w.opt.BackoffMax
			break
		}
		d *= 2
	}
	if d > w.opt.BackoffMax {
		d = w.opt.BackoffMax
	}
	jitter := 0.8 + 0.4*w.jitterFn() // 0.8–1.2
	return time.Duration(float64(d) * jitter)
}

func (w *Worker) audit(ctx context.Context, t store.IndexTask, result, errMsg string) {
	detail := fmt.Sprintf("task:%s;material:%d;attempts:%d", t.TaskID, t.MaterialID, t.Attempts)
	if errMsg != "" {
		detail += ";" + clip(errMsg, 160)
	}
	classID := t.ClassID
	if err := w.st.Audit(ctx, nil, "worker", &classID, "index_task", result, detail); err != nil {
		slog.Error("worker_audit_failed", "task_id", t.TaskID, "error", err.Error())
	}
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
