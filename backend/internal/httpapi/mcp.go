package httpapi

// MCP 宿主侧接口（spec MC2/MC4）：登记、列表、连接、断开、工具清单、调用记录。
// 令牌只写不回；所有查询带 class_id；跨班 404，学生管理操作 403（由路由 requireRole 保证）。

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Corday-nkusky/team_project/backend/internal/mcpclient"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

type mcpServerView struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	BaseURL         string `json:"base_url"`
	TokenConfigured bool   `json:"token_configured"`
	Connected       bool   `json:"connected"`
	LastError       string `json:"last_error,omitempty"`
}

func toMCPServerView(sv store.MCPServer) mcpServerView {
	return mcpServerView{
		ID: sv.ID, Name: sv.Name, BaseURL: sv.BaseURL,
		TokenConfigured: sv.TokenConfigured, Connected: sv.Connected, LastError: sv.LastError,
	}
}

type mcpServerReq struct {
	Name      string `json:"name"`
	BaseURL   string `json:"base_url"`
	AuthToken string `json:"auth_token"`
}

func (s *Server) handleUpsertMCPServer(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	var req mcpServerReq
	if err := decodeJSON(r, &req); err != nil {
		writeDecodeErr(w, err, "请求体格式不正确")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.BaseURL = strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
	req.AuthToken = strings.TrimSpace(req.AuthToken)
	if req.Name == "" || len([]rune(req.Name)) > 64 {
		writeErr(w, http.StatusBadRequest, "bad_request", "服务名称必须为 1–64 字符")
		return
	}
	if !validHTTPURL(req.BaseURL) {
		writeErr(w, http.StatusBadRequest, "bad_request", "Base URL 必须是 http(s) 绝对地址")
		return
	}
	saved, err := s.st.UpsertMCPServer(r.Context(), store.MCPServer{
		ClassID: c.ClassID, OwnerID: c.UserID, Name: req.Name,
		BaseURL: req.BaseURL, AuthToken: req.AuthToken,
	})
	if err != nil {
		logServerError(r, "upsert_mcp_server", err)
		writeErr(w, http.StatusInternalServerError, "internal", "登记服务失败")
		return
	}
	// 响应只回布尔标记，绝不回令牌。
	writeJSON(w, http.StatusOK, toMCPServerView(saved))
}

func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	list, err := s.mcp.ListMCPServers(r.Context(), c.ClassID)
	if err != nil {
		logServerError(r, "list_mcp_servers", err)
		writeErr(w, http.StatusInternalServerError, "internal", "读取服务列表失败")
		return
	}
	out := make([]mcpServerView, 0, len(list))
	for _, sv := range list {
		out = append(out, toMCPServerView(sv))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) serverFromPath(w http.ResponseWriter, r *http.Request) (store.MCPServer, bool) {
	c, _ := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "服务 id 不合法")
		return store.MCPServer{}, false
	}
	sv, err := s.mcp.GetMCPServerByID(r.Context(), c.ClassID, id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "服务不存在")
		return store.MCPServer{}, false
	}
	if err != nil {
		logServerError(r, "get_mcp_server", err)
		writeErr(w, http.StatusInternalServerError, "internal", "读取服务失败")
		return store.MCPServer{}, false
	}
	return sv, true
}

// handleMCPConnect = 握手 + 工具发现两步；任一步失败停在未连接并记可读原因。
func (s *Server) handleMCPConnect(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.serverFromPath(w, r)
	if !ok {
		return
	}
	client, err := s.mcpClientFor(r.Context(), sv)
	if err != nil {
		logServerError(r, "mcp_token", err)
		writeErr(w, http.StatusInternalServerError, "internal", "读取令牌失败")
		return
	}
	if _, err := client.Initialize(r.Context()); err != nil {
		s.markConnectError(r, sv, err)
		writeErr(w, http.StatusBadGateway, "mcp_connect_failed", "握手失败："+err.Error())
		return
	}
	tools, err := client.ListTools(r.Context())
	if err != nil {
		s.markConnectError(r, sv, err)
		writeErr(w, http.StatusBadGateway, "mcp_connect_failed", "工具发现失败："+err.Error())
		return
	}
	// 三条工具字段落库：名称/说明/输入结构缺一不可。
	stTools := make([]store.MCPTool, 0, len(tools))
	for _, t := range tools {
		if strings.TrimSpace(t.Name) == "" || strings.TrimSpace(t.Description) == "" || t.InputSchema == nil {
			s.markConnectError(r, sv, errors.New("工具声明缺少名称/说明/输入结构"))
			writeErr(w, http.StatusBadGateway, "mcp_connect_failed", "工具声明不完整")
			return
		}
		stTools = append(stTools, store.MCPTool{
			Name: t.Name, Description: t.Description, InputSchema: t.InputSchema,
		})
	}
	if err := s.mcp.SetMCPConnected(r.Context(), sv.ClassID, sv.ID, stTools); err != nil {
		logServerError(r, "mcp_connected", err)
		writeErr(w, http.StatusInternalServerError, "internal", "保存连接状态失败")
		return
	}
	fresh, _ := s.mcp.GetMCPServerByID(r.Context(), sv.ClassID, sv.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": sv.ID, "connected": true, "tools": fresh.Tools,
	})
}

func (s *Server) markConnectError(r *http.Request, sv store.MCPServer, err error) {
	if e := s.mcp.SetMCPLastError(r.Context(), sv.ClassID, sv.ID, err.Error()); e != nil {
		logServerError(r, "mcp_last_error", e)
	}
}

func (s *Server) handleMCPDisconnect(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.serverFromPath(w, r)
	if !ok {
		return
	}
	if err := s.mcp.SetMCPDisconnected(r.Context(), sv.ClassID, sv.ID); err != nil {
		logServerError(r, "mcp_disconnect", err)
		writeErr(w, http.StatusInternalServerError, "internal", "断开失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": sv.ID, "connected": false})
}

func (s *Server) handleMCPTools(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.serverFromPath(w, r)
	if !ok {
		return
	}
	if !sv.Connected {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	out := make([]mcpclient.Tool, 0, len(sv.Tools))
	for _, t := range sv.Tools {
		out = append(out, mcpclient.Tool{
			Name: t.Name, Description: t.Description, InputSchema: t.InputSchema,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s *Server) handleDeleteMCPServer(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.serverFromPath(w, r)
	if !ok {
		return
	}
	// 服务器被删除后，引用它的技能无处归属：按课程预期，本次调用不写记录。
	if err := s.mcp.DeleteMCPServer(r.Context(), sv.ClassID, sv.ID); err != nil {
		logServerError(r, "delete_mcp_server", err)
		writeErr(w, http.StatusInternalServerError, "internal", "删除失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type mcpCallView struct {
	ID             int64  `json:"id"`
	ToolRef        string `json:"tool_ref"`
	Result         string `json:"result"`
	ActorName      string `json:"actor_name"`
	ConversationID string `json:"conversation_id,omitempty"`
	Round          int    `json:"round"`
	Detail         string `json:"detail,omitempty"`
	CreatedAt      string `json:"created_at"`
}

func (s *Server) handleListMCPCalls(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, "bad_request", "limit 必须是正整数")
			return
		}
		limit = n
	}
	if limit > 50 {
		limit = 50
	}
	list, err := s.mcp.ListMCPCalls(r.Context(), c.ClassID, limit)
	if err != nil {
		logServerError(r, "list_mcp_calls", err)
		writeErr(w, http.StatusInternalServerError, "internal", "读取调用记录失败")
		return
	}
	out := make([]mcpCallView, 0, len(list))
	for _, call := range list {
		out = append(out, mcpCallView{
			ID: call.ID, ToolRef: call.ToolRef, Result: call.Result,
			ActorName: call.ActorName, ConversationID: call.ConversationID,
			Round: call.Round, Detail: call.Detail, CreatedAt: call.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// validHTTPURL 只接受 http(s) 绝对地址（避免 Base URL 指向内网文件协议等）。
func validHTTPURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}
