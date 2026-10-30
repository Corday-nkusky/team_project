package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
