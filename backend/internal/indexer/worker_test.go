package indexer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/llm"
	"github.com/Corday-nkusky/team_project/backend/internal/qa"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

type failCall struct {
	taskID   string
	errMsg   string
	retry    bool
	backoff  time.Duration
	terminal string
}

type fakeTaskStore struct {
	material store.Material
	getErr   error
	complete int
	fails    []failCall
	audits   []string
	onClaim  func()
}

func (f *fakeTaskStore) RecoverStaleTasks(context.Context) (int64, error) { return 0, nil }
func (f *fakeTaskStore) ClaimTasks(context.Context, int, time.Duration) ([]store.IndexTask, error) {
	if f.onClaim != nil {
		f.onClaim()
	}
	return nil, nil
}
func (f *fakeTaskStore) GetMaterial(context.Context, int64, int64) (store.Material, error) {
	return f.material, f.getErr
}
func (f *fakeTaskStore) CompleteTask(context.Context, string) error {
	f.complete++
	return nil
}
func (f *fakeTaskStore) FailTask(_ context.Context, taskID, errMsg string, retry bool,
	backoff time.Duration, terminal string) error {
	f.fails = append(f.fails, failCall{taskID, errMsg, retry, backoff, terminal})
	return nil
}
func (f *fakeTaskStore) Audit(_ context.Context, _ *int64, _ string, _ *int64,
	_, result, _ string) error {
	f.audits = append(f.audits, result)
	return nil
}

type fakeIndex struct {
	err   error
	calls int
}

func (f *fakeIndex) Index(context.Context, int64, int64, string) error {
	f.calls++
	return f.err
}

func testOptions() Options {
	return Options{
		Poll: 5 * time.Millisecond, Lease: time.Minute, TaskTimeout: time.Minute,
		MaxAttempts: 3, Backoff: time.Second, BackoffMax: 10 * time.Second,
		Batch: 1, StatusTimeout: 5 * time.Second,
	}
}

func testTask(attempts int) store.IndexTask {
	return store.IndexTask{
		ID: 1, TaskID: "task-test", ClassID: 2, MaterialID: 3,
		Kind: store.TaskKindUpload, Status: store.TaskRunning, Attempts: attempts,
	}
}

func TestHandleSuccess(t *testing.T) {
	st := &fakeTaskStore{material: store.Material{ID: 3, ClassID: 2, Content: "正文"}}
	svc := &fakeIndex{}
	w := New(st, svc, testOptions())
	w.jitterFn = func() float64 { return 0.5 }
	w.handle(context.Background(), testTask(1))

	if svc.calls != 1 || st.complete != 1 || len(st.fails) != 0 {
		t.Fatalf("成功路径应执行一次索引并写 succeeded: calls=%d complete=%d fails=%v",
			svc.calls, st.complete, st.fails)
	}
	if len(st.audits) != 1 || st.audits[0] != "ok" {
		t.Fatalf("成功应写一条 ok 审计: %v", st.audits)
	}
}

func TestHandleFailureClasses(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		attempts  int
		wantRetry bool
		wantTerm  string
	}{
		{"permanent 直接失败", fmt.Errorf("包装: %w", qa.ErrPermanent), 1, false, store.TaskFailed},
		{"认证配置错误直接失败", fmt.Errorf("包装: %w", llm.ErrAuthConfig), 1, false, store.TaskFailed},
		{"网关额度耗尽转死信", fmt.Errorf("包装: %w", llm.ErrRateLimited), 1, false, store.TaskDead},
		{"瞬时失败退避重试", errors.New("Qdrant 暂时不可达"), 1, true, ""},
		{"重试达上限转死信", errors.New("Qdrant 暂时不可达"), 3, false, store.TaskDead},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &fakeTaskStore{material: store.Material{ID: 3, ClassID: 2, Content: "正文"}}
			svc := &fakeIndex{err: tc.err}
			w := New(st, svc, testOptions())
			w.jitterFn = func() float64 { return 0.5 }
			w.handle(context.Background(), testTask(tc.attempts))

			if len(st.fails) != 1 {
				t.Fatalf("应写一次失败状态，得到 %d 次", len(st.fails))
			}
			got := st.fails[0]
			if got.retry != tc.wantRetry {
				t.Fatalf("retry 期望 %v，得到 %v", tc.wantRetry, got.retry)
			}
			if !tc.wantRetry && got.terminal != tc.wantTerm {
				t.Fatalf("终态期望 %q，得到 %q", tc.wantTerm, got.terminal)
			}
			if tc.wantRetry && got.backoff <= 0 {
				t.Fatal("重试应有正退避时间")
			}
			if !tc.wantRetry && st.audits[len(st.audits)-1] != "fail" {
				t.Fatalf("终态应写 fail 审计: %v", st.audits)
			}
		})
	}
}

func TestBackoffIsExponentialAndCapped(t *testing.T) {
	w := New(&fakeTaskStore{}, &fakeIndex{}, testOptions())
	w.jitterFn = func() float64 { return 0.5 } // jitter = 1.0，结果可直接比较
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second}
	for i, exp := range want {
		if got := w.backoff(i + 1); got != exp {
			t.Fatalf("attempts=%d 退避期望 %v，得到 %v", i+1, exp, got)
		}
	}
}

func TestClassify(t *testing.T) {
	if classify(errors.New("网络抖动")) != classTransient {
		t.Fatal("普通错误应可重试")
	}
	if classify(fmt.Errorf("%w", llm.ErrAuth)) != classPermanent {
		t.Fatal("401/403 应判为不可重试")
	}
	if classify(fmt.Errorf("%w", store.ErrNotFound)) != classPermanent {
		t.Fatal("材料已删除应判为不可重试")
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	st := &fakeTaskStore{onClaim: cancel}
	w := New(st, &fakeIndex{}, testOptions())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("优雅停止不应返回错误: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ctx 取消后 worker 未退出")
	}
}
