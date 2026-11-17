// Command mcp-server 是CampusClaw 自建的 MCP 工具服务（spec MC1）。
//
// 单入口 POST /mcp 接受 JSON-RPC 2.0：initialize / tools/list / tools/call；
// GET /health 免认证。令牌从 MCP_AUTH_TOKEN 读取，不设默认值、不进代码/日志。
// 只依赖标准库，避免引入新的依赖审计面。
package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var version = "dev"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// tools 是工具清单。说明写清适用条件与边界，输入结构决定参数能否被校验。
var tools = []tool{
	{
		Name:        "formula_hint",
		Description: "当学生就某道题卡住、需要思路但不应直接给答案时使用；只返回分步解题线索，不含最终答案。不适用于要求核对具体材料事实的提问。",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"topic": map[string]any{"type": "string", "description": "题目所属主题"},
			},
			"required": []any{"topic"},
		},
	},
	{
		Name:        "unit_convert",
		Description: "当问题只涉及标准单位换算（长度/质量/时间）时使用；返回换算结果与依据。不适用于非数值或需要材料事实的问题。",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"value": map[string]any{"type": "number", "description": "待换算的数值"},
				"from":  map[string]any{"type": "string", "description": "原单位"},
				"to":    map[string]any{"type": "string", "description": "目标单位"},
			},
			"required": []any{"value", "from", "to"},
		},
	},
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	token := os.Getenv("MCP_AUTH_TOKEN")
	if token == "" {
		slog.Error("启动中止：MCP_AUTH_TOKEN 未设置")
		os.Exit(1)
	}
	addr := os.Getenv("MCP_ADDR")
	if addr == "" {
		addr = ":8090"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"status":"ok","service":"mcp-server","version":"` + version + `"}`))
	})
	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) { handleMCP(w, r, token) })
	mux.HandleFunc("/mcp", methodNotAllowed(http.MethodPost))

	srv := &http.Server{
		Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("mcp-server_starting", "addr", addr, "version", version, "tools", len(tools))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("mcp-server 退出", "error", err.Error())
		os.Exit(1)
	}
}

func methodNotAllowed(method string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", method)
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writeRPC(w http.ResponseWriter, id json.RawMessage, result any, rerr *rpcError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if len(id) == 0 {
		id = json.RawMessage(`null`)
	}
	resp := rpcResponse{JSONRPC: "2.0", ID: id, Result: result, Error: rerr}
	_ = json.NewEncoder(w).Encode(resp)
}

func handleMCP(w http.ResponseWriter, r *http.Request, token string) {
	raw := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) ||
		subtle.ConstantTimeCompare([]byte(strings.TrimSpace(strings.TrimPrefix(raw, prefix))), []byte(token)) != 1 {
		writeRPC(w, json.RawMessage(`null`), nil, &rpcError{Code: -32001, Message: "invalid_token"})
		return
	}
	var req rpcRequest
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := dec.Decode(&req); err != nil || req.JSONRPC != "2.0" {
		writeRPC(w, json.RawMessage(`null`), nil, &rpcError{Code: -32600, Message: "invalid_request"})
		return
	}
	switch req.Method {
	case "initialize":
		writeRPC(w, req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "campus-tools", "version": version},
		}, nil)
	case "tools/list":
		writeRPC(w, req.ID, map[string]any{"tools": tools}, nil)
	case "tools/call":
		handleCall(w, r.Context(), req)
	default:
		writeRPC(w, req.ID, nil, &rpcError{Code: -32601, Message: "method_not_found"})
	}
}

func handleCall(w http.ResponseWriter, ctx context.Context, req rpcRequest) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil || params.Name == "" {
		writeRPC(w, req.ID, nil, &rpcError{Code: -32602, Message: "invalid_params"})
		return
	}
	var text string
	switch params.Name {
	case "formula_hint":
		topic, _ := params.Arguments["topic"].(string)
		if strings.TrimSpace(topic) == "" {
			writeRPC(w, req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": "请说明题目主题，我再给分步线索。"}},
			}, nil)
			return
		}
		text = "关于「" + topic + "」：先写下已知条件与目标，再回想相关定义，最后把定义套到已知条件上走一步；卡住时回到材料里找对应句子。"
	case "unit_convert":
		value, _ := params.Arguments["value"].(float64)
		from, _ := params.Arguments["from"].(string)
		to, _ := params.Arguments["to"].(string)
		factor, ok := convertFactor(from, to)
		if !ok {
			writeRPC(w, req.ID, map[string]any{
				"content": []map[string]any{{"type": "text", "text": "暂不支持该单位组合，请只使用长度/质量/时间的标准单位。"}},
			}, nil)
			return
		}
		text = strconv.FormatFloat(value*factor, 'f', -1, 64) + " " + to + "（按 1 " + from + " = " +
			strconv.FormatFloat(factor, 'f', -1, 64) + " " + to + " 换算）"
	default:
		writeRPC(w, req.ID, nil, &rpcError{Code: -32602, Message: "unknown_tool"})
		return
	}
	writeRPC(w, req.ID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": false,
	}, nil)
}

// convertFactor 只处理同量纲的标准单位；返回 (倍率, 是否支持)。
func convertFactor(from, to string) (float64, bool) {
	groups := [][]string{
		{"mm", "cm", "m", "km"},
		{"g", "kg", "t"},
		{"s", "min", "h"},
	}
	base := map[string]float64{
		"mm": 0.001, "cm": 0.01, "m": 1, "km": 1000,
		"g": 0.001, "kg": 1, "t": 1000,
		"s": 1, "min": 60, "h": 3600,
	}
	for _, g := range groups {
		in, out := false, false
		for _, u := range g {
			if u == from {
				in = true
			}
			if u == to {
				out = true
			}
		}
		if in && out {
			return base[from] / base[to], true
		}
	}
	return 0, false
}
