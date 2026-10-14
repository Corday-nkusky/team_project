// Package qdrant 是 Qdrant 向量库的最小 HTTP 客户端（仅覆盖本项目所需操作）。
//
// 隔离纪律：向量检索一律带 class_id 过滤条件（spec R2.2 / R7.2），
// 班级条件由服务端从会话推导，不接受客户端传入。
package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client 是 Qdrant 客户端。
type Client struct {
	base   string
	http   *http.Client
	apiKey string
}

// New 构造客户端。base 形如 http://127.0.0.1:6333。
func New(base, apiKey string) *Client {
	return &Client{
		base:   base,
		apiKey: apiKey,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Point 是一个向量点。
//
// ID 必须是「无符号整数或 UUID 字符串」——Qdrant 不接受任意字符串，
// 因此这里用切片主键（整数）作为点 ID。
type Point struct {
	ID      any            `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

// ScoredPoint 是检索命中的向量点。
//
// ID 回传可能是数字或字符串（取决于写入时的类型），故用 any 承接。
type ScoredPoint struct {
	ID      any            `json:"id"`
	Score   float64        `json:"score"`
	Payload map[string]any `json:"payload"`
}

// EnsureCollection 确保集合存在（不存在则创建，余弦距离）。
func (c *Client) EnsureCollection(ctx context.Context, name string, dim int) error {
	// 已存在？
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/collections/"+name, nil)
	c.auth(req)
	resp, err := c.http.Do(req)
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
	}
	body := map[string]any{
		"vectors": map[string]any{"size": dim, "distance": "Cosine"},
	}
	_, err = c.do(ctx, http.MethodPut, "/collections/"+name, body)
	if err != nil {
		return fmt.Errorf("创建集合 %s 失败: %w", name, err)
	}
	return nil
}

// Upsert 写入/覆盖向量点。
func (c *Client) Upsert(ctx context.Context, collection string, points []Point) error {
	_, err := c.do(ctx, http.MethodPut, "/collections/"+collection+"/points?wait=true",
		map[string]any{"points": points})
	return err
}

// DeleteByMaterial 删除某材料的全部向量（重索引前清理）。
func (c *Client) DeleteByMaterial(ctx context.Context, collection string, materialID int64) error {
	body := map[string]any{
		"filter": map[string]any{
			"must": []any{matchKey("material_id", materialID)},
		},
	}
	_, err := c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/delete?wait=true", body)
	return err
}

// Search 在指定班级范围内做向量检索。
func (c *Client) Search(ctx context.Context, collection string, vector []float32, classID int64, limit int) ([]ScoredPoint, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
		"filter": map[string]any{
			"must": []any{matchKey("class_id", classID)},
		},
	}
	raw, err := c.do(ctx, http.MethodPost, "/collections/"+collection+"/points/search", body)
	if err != nil {
		return nil, err
	}
	var out struct {
		Result []ScoredPoint `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析检索结果失败: %w", err)
	}
	return out.Result, nil
}

// Healthy 探测向量库可用性。
func (c *Client) Healthy(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/healthz", nil)
	if err != nil {
		return err
	}
	c.auth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("/healthz 返回 %d", resp.StatusCode)
	}
	return nil
}

func matchKey(key string, val any) map[string]any {
	return map[string]any{"key": key, "match": map[string]any{"value": val}}
}

func (c *Client) auth(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}
}

func (c *Client) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.auth(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data := new(bytes.Buffer)
	_, _ = data.ReadFrom(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s 返回 %d: %s", method, path, resp.StatusCode, truncate(data.String(), 200))
	}
	return data.Bytes(), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
