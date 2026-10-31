package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func newFakeGateway(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := New(srv.URL, "test-key", "chat-model", "embed-model")
	return c, srv.Close
}

func collect(t *testing.T, ch <-chan Event) (text string, done bool, err error) {
	t.Helper()
	var b strings.Builder
	for ev := range ch {
		if ev.Delta != "" {
			b.WriteString(ev.Delta)
		}
		if ev.Done {
			done = true
		}
		if ev.Err != nil {
			err = ev.Err
		}
	}
	return b.String(), done, err
}

func TestChatStreamCompletesOnDone(t *testing.T) {
	c, closeFn := newFakeGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})
	defer closeFn()

	ch, err := c.ChatStream(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("ChatStream 返回错误: %v", err)
	}
	text, done, streamErr := collect(t, ch)
	if text != "你好" || !done || streamErr != nil {
		t.Fatalf("正常流结果不符：text=%q done=%v err=%v", text, done, streamErr)
	}
}

func TestChatStreamTreatsEOFWithoutDoneAsError(t *testing.T) {
	c, closeFn := newFakeGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"半截回答\"}}]}\n\n"))
		// 故意不发送 [DONE]，连接关闭。
	})
	defer closeFn()

	ch, err := c.ChatStream(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("ChatStream 返回错误: %v", err)
	}
	text, done, streamErr := collect(t, ch)
	if text != "半截回答" {
		t.Fatalf("应保留已收到的增量，得到 %q", text)
	}
	if done {
		t.Fatal("未收到 [DONE] 不应标记完成")
	}
	if streamErr == nil {
		t.Fatal("未收到 [DONE] 应返回错误，避免把半截当完整")
	}
}

func TestChatStreamTreatsEmptyBodyAsError(t *testing.T) {
	c, closeFn := newFakeGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// 空流，直接关闭。
	})
	defer closeFn()

	ch, err := c.ChatStream(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("ChatStream 返回错误: %v", err)
	}
	text, done, streamErr := collect(t, ch)
	if text != "" || done || streamErr == nil {
		t.Fatalf("空流应得到错误且不完成：text=%q done=%v err=%v", text, done, streamErr)
	}
}

func TestChatStreamReportsUpstreamStatus(t *testing.T) {
	c, closeFn := newFakeGateway(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	})
	defer closeFn()

	if _, err := c.ChatStream(context.Background(), "sys", "user"); err == nil {
		t.Fatal("上游 500 应返回错误，由调用方决定降级")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

// TestChatStreamReportsScanError 对应 R6.4：读取流时发生 IO 错误也要上报。
func TestChatStreamReportsScanError(t *testing.T) {
	c := New("http://gateway.invalid", "test-key", "chat-model", "embed-model")
	c.http = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := io.MultiReader(
			strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"半截\"}}]}\n\n"),
			errReader{},
		)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body), Header: make(http.Header)}, nil
	})}

	ch, err := c.ChatStream(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("ChatStream 返回错误: %v", err)
	}
	text, done, streamErr := collect(t, ch)
	if text != "半截" {
		t.Fatalf("应保留已收到的增量，得到 %q", text)
	}
	if done {
		t.Fatal("扫描错误不应标记完成")
	}
	if streamErr == nil {
		t.Fatal("扫描错误应上报")
	}
}

// fakeEmbedGateway 按输入文本生成可识别的向量，并记录每批的输入。
type fakeEmbedGateway struct {
	mu       sync.Mutex
	batches  [][]string
	failAt   int // 第几次请求返回 500（从 1 开始；0 表示不失败）
	dimShift bool
}

func (f *fakeEmbedGateway) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("意外的路径: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("解析嵌入请求失败: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Model != "embed-model" {
			t.Errorf("模型名不符: %q", req.Model)
		}
		f.mu.Lock()
		f.batches = append(f.batches, append([]string(nil), req.Input...))
		index := len(f.batches)
		f.mu.Unlock()
		if f.failAt == index {
			http.Error(w, `{"error":{"message":"boom"}}`, http.StatusInternalServerError)
			return
		}

		type item struct {
			Embedding []float32 `json:"embedding"`
		}
		resp := struct {
			Data []item `json:"data"`
		}{}
		dim := 2
		if f.dimShift && index > 1 {
			dim = 3
		}
		for _, text := range req.Input {
			n, err := strconv.Atoi(text)
			if err != nil {
				t.Errorf("输入文本应为数字: %q", text)
				n = -1
			}
			vec := make([]float32, dim)
			vec[0] = float32(n)
			vec[1] = float32(index)
			resp.Data = append(resp.Data, item{Embedding: vec})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func (f *fakeEmbedGateway) snapshot() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.batches))
	for i, b := range f.batches {
		out[i] = append([]string(nil), b...)
	}
	return out
}

// 长材料必须分批，且相同切片只嵌入一次、按原位置展开。
func TestEmbedBatchesAndDedupes(t *testing.T) {
	fake := &fakeEmbedGateway{}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	c := New(srv.URL, "test-key", "chat-model", "embed-model")

	texts := make([]string, 70)
	for i := range texts {
		texts[i] = fmt.Sprint(i)
	}
	texts[10], texts[40], texts[41] = texts[9], texts[39], texts[39]

	vecs, err := c.Embed(context.Background(), texts)
	if err != nil {
		t.Fatalf("Embed 返回错误: %v", err)
	}
	if len(vecs) != len(texts) {
		t.Fatalf("向量数应与输入一致：期望 %d，得到 %d", len(texts), len(vecs))
	}
	for i, v := range vecs {
		want, _ := strconv.Atoi(texts[i])
		if len(v) == 0 || v[0] != float32(want) {
			t.Fatalf("第 %d 条向量未按原顺序映射: %v", i, v)
		}
	}

	batches := fake.snapshot()
	if len(batches) != 3 {
		t.Fatalf("67 条唯一文本应按 %d 条分批成 3 批，得到 %d 批", embedBatchSize, len(batches))
	}
	total := 0
	for i, batch := range batches {
		if len(batch) > embedBatchSize {
			t.Fatalf("第 %d 批超过上限: %d", i+1, len(batch))
		}
		total += len(batch)
	}
	if total != 67 {
		t.Fatalf("去重后应只提交 67 条，实际提交 %d 条", total)
	}
}

// 任一批失败即整体失败，后续批次不再提交。
func TestEmbedFailsOnAnyBatch(t *testing.T) {
	fake := &fakeEmbedGateway{failAt: 2}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	c := New(srv.URL, "test-key", "chat-model", "embed-model")

	texts := make([]string, embedBatchSize*2)
	for i := range texts {
		texts[i] = fmt.Sprint(i)
	}
	if _, err := c.Embed(context.Background(), texts); err == nil {
		t.Fatal("第二批失败应返回错误")
	}
	if got := len(fake.snapshot()); got != 2 {
		t.Fatalf("失败后不应继续提交后续批次，实际请求 %d 次", got)
	}
}

// 不同批次维度漂移必须报错，避免写入维度不一致的向量（LG2.3）。
func TestEmbedRejectsDimensionDrift(t *testing.T) {
	fake := &fakeEmbedGateway{dimShift: true}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	c := New(srv.URL, "test-key", "chat-model", "embed-model")

	texts := make([]string, embedBatchSize+5)
	for i := range texts {
		texts[i] = fmt.Sprint(i)
	}
	_, err := c.Embed(context.Background(), texts)
	if err == nil || !strings.Contains(err.Error(), "维度不一致") {
		t.Fatalf("维度漂移应报错，得到 %v", err)
	}
}

func TestEmbedEmptyInputSkipsGateway(t *testing.T) {
	fake := &fakeEmbedGateway{}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	c := New(srv.URL, "test-key", "chat-model", "embed-model")

	vecs, err := c.Embed(context.Background(), nil)
	if err != nil || len(vecs) != 0 {
		t.Fatalf("空输入应返回空结果，得到 vecs=%v err=%v", vecs, err)
	}
	if got := len(fake.snapshot()); got != 0 {
		t.Fatalf("空输入不应请求网关，实际 %d 次", got)
	}
}
