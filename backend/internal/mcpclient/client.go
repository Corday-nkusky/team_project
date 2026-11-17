// Package mcpclient 实现宿主侧的 JSON-RPC 2.0 客户端与工具调用编排（spec MC1/MC2/MC3）。
//
// 分层：本包属层 0，只依赖标准库与 store(0)。不依赖 qa/llm，模型决策结果由上层传入。
// 令牌只走请求头，不进响应、不进日志、不进错误文案。
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTimeout = 8 * time.Second

// rpcRequest / rpcResponse 是 JSON-RPC 2.0 的最小报文。
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// Tool 与 store.MCPTool 字段对齐（避免本包依赖 store 的结构体，测试更独立）。
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Client 以 Base URL 连到一台 MCP 服务。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: defaultTimeout},
	}
}

// call 发一条 JSON-RPC 请求并返回 result 原文。
func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return nil, fmt.Errorf("编码请求失败: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/mcp", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接工具服务失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("工具服务返回 HTTP %d", resp.StatusCode)
	}
	var out rpcResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析 JSON-RPC 响应失败: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("工具服务错误(%d): %s", out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}

// Initialize 完成能力协商。
func (c *Client) Initialize(ctx context.Context) (map[string]any, error) {
	raw, err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]any{"name": "campusclaw-host", "version": "1.0"},
		"capabilities":    map[string]any{},
	})
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析 initialize 结果失败: %w", err)
	}
	return out, nil
}

// ListTools 返回工具发现清单。
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	raw, err := c.call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析 tools/list 结果失败: %w", err)
	}
	return out.Tools, nil
}

// CallTool 执行一个工具，返回文本内容。
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	raw, err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args})
	if err != nil {
		return "", err
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("解析 tools/call 结果失败: %w", err)
	}
	var b strings.Builder
	for _, item := range out.Content {
		if item.Type == "text" {
			b.WriteString(item.Text)
		}
	}
	if out.IsError {
		return "", fmt.Errorf("工具执行失败: %s", strings.TrimSpace(b.String()))
	}
	return b.String(), nil
}
