// Package llm 是 OpenAI 兼容模型网关的客户端：嵌入与流式对话（design D3）。
//
// 未配置密钥时 Enabled() 为 false，调用方走「无模型」降级路径，
// 使系统在零密钥环境下仍可完整跑通（见 README「本地运行」）。
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client 是模型网关客户端。
type Client struct {
	base       string
	key        string
	chatModel  string
	embedModel string
	http       *http.Client
}

// New 构造客户端。base 形如 https://api.deepseek.com/v1。
func New(base, key, chatModel, embedModel string) *Client {
	return &Client{
		base:       strings.TrimRight(base, "/"),
		key:        key,
		chatModel:  chatModel,
		embedModel: embedModel,
		http:       &http.Client{Timeout: 120 * time.Second},
	}
}

// Enabled 表示是否配置了密钥（无密钥则降级）。
func (c *Client) Enabled() bool { return strings.TrimSpace(c.key) != "" }

// ErrDisabled 表示未配置模型网关。
var ErrDisabled = errors.New("未配置模型网关（LLM_API_KEY 为空）")

// Embed 批量生成嵌入向量。
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	body := map[string]any{"model": c.embedModel, "input": texts}
	raw, err := c.post(ctx, "/embeddings", body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析嵌入响应失败: %w", err)
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("嵌入数量不符：期望 %d，得到 %d", len(texts), len(out.Data))
	}
	vecs := make([][]float32, len(out.Data))
	for i, d := range out.Data {
		vecs[i] = d.Embedding
	}
	return vecs, nil
}

// Event 是流式回答的一个增量事件。
type Event struct {
	Delta string
	Err   error
	Done  bool
}

// ChatStream 发起流式对话，返回增量事件通道。通道在所有增量送达后被关闭。
func (c *Client) ChatStream(ctx context.Context, system, user string) (<-chan Event, error) {
	if !c.Enabled() {
		return nil, ErrDisabled
	}
	payload := map[string]any{
		"model":  c.chatModel,
		"stream": true,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	}
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(payload); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/chat/completions", buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用模型失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b := new(bytes.Buffer)
		_, _ = b.ReadFrom(resp.Body)
		return nil, fmt.Errorf("模型返回 %d: %s", resp.StatusCode, truncate(b.String(), 200))
	}

	ch := make(chan Event, 8)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		sawDone := false
		emitErr := func(err error) {
			select {
			case ch <- Event{Err: err}:
			case <-ctx.Done():
			}
		}
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "[DONE]" {
				sawDone = true
				select {
				case ch <- Event{Done: true}:
				case <-ctx.Done():
				}
				return
			}
			var piece struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &piece); err != nil {
				continue // 忽略无法解析的心跳/注释帧
			}
			for _, ch2 := range piece.Choices {
				if ch2.Delta.Content != "" {
					select {
					case ch <- Event{Delta: ch2.Delta.Content}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
		if err := sc.Err(); err != nil {
			if ctx.Err() == nil {
				emitErr(err)
			}
			return
		}
		// 连接正常关闭但未收到 [DONE]：流被上游截断，不能当作完整回答（R6.4）。
		if !sawDone && ctx.Err() == nil {
			emitErr(io.ErrUnexpectedEOF)
		}
	}()
	return ch, nil
}

func (c *Client) post(ctx context.Context, path string, body any) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := json.NewEncoder(buf).Encode(body); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用模型失败: %w", err)
	}
	defer resp.Body.Close()
	out := new(bytes.Buffer)
	_, _ = out.ReadFrom(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("模型返回 %d: %s", resp.StatusCode, truncate(out.String(), 200))
	}
	return out.Bytes(), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
