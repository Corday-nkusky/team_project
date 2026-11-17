package store

// 本文件是「MCP 服务登记 / 调用记录」的全部持久化读写。隔离纪律：所有查询一律带
// class_id；auth_token 只在写入路径出现，任何读取查询都不 SELECT 它（spec MC2/MC4）。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// MCPServer 是宿主侧登记的一条工具服务（auth_token 仅写入时用，读出时为空）。
type MCPServer struct {
	ID              int64
	ClassID         int64
	OwnerID         int64
	Name            string
	BaseURL         string
	AuthToken       string
	TokenConfigured bool
	Connected       bool
	Tools           []MCPTool
	LastError       string
	CreatedAt       string
	UpdatedAt       string
}

// MCPTool 是工具发现清单里的一条。
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// MCPCall 是一条只增不改的调用记录。
type MCPCall struct {
	ID             int64
	ClassID        int64
	ActorID        int64
	ActorName      string
	ToolRef        string
	Result         string
	ConversationID string
	Round          int
	Detail         string
	CreatedAt      string
}

// UpsertMCPServer 新建或按 (class_id, name) 更新服务登记。
// token 为空表示不改动既有令牌（避免列表回显后误清空）。
func (s *Store) UpsertMCPServer(ctx context.Context, in MCPServer) (MCPServer, error) {
	configured := in.AuthToken != ""
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO mcp_servers (class_id,owner_id,name,base_url,auth_token,token_configured,connected,tools_json,last_error)
		VALUES (?,?,?,?,?,?,0,NULL,'')
		ON DUPLICATE KEY UPDATE
		  base_url=VALUES(base_url), owner_id=VALUES(owner_id),
		  auth_token=IF(VALUES(auth_token)='',auth_token,VALUES(auth_token)),
		  token_configured=IF(VALUES(auth_token)='',token_configured,VALUES(token_configured)),
		  connected=0, tools_json=NULL, last_error=''`,
		in.ClassID, in.OwnerID, in.Name, in.BaseURL, in.AuthToken, configured)
	if err != nil {
		return MCPServer{}, fmt.Errorf("写入 MCP 服务失败: %w", err)
	}
	return s.GetMCPServerByName(ctx, in.ClassID, in.Name)
}

// GetMCPServerByName 按班 + 名称读取（不含令牌明文）。
func (s *Store) GetMCPServerByName(ctx context.Context, classID int64, name string) (MCPServer, error) {
	return scanMCPServer(s.DB.QueryRowContext(ctx, `
		SELECT id,class_id,owner_id,name,base_url,token_configured,connected,tools_json,last_error,
		       DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%sZ'),DATE_FORMAT(updated_at,'%Y-%m-%dT%H:%i:%sZ')
		FROM mcp_servers WHERE class_id=? AND name=?`, classID, name))
}

// GetMCPServerByID 按 id + class_id 读取（不含令牌明文）；跨班返回 ErrNotFound。
func (s *Store) GetMCPServerByID(ctx context.Context, classID, id int64) (MCPServer, error) {
	return scanMCPServer(s.DB.QueryRowContext(ctx, `
		SELECT id,class_id,owner_id,name,base_url,token_configured,connected,tools_json,last_error,
		       DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%sZ'),DATE_FORMAT(updated_at,'%Y-%m-%dT%H:%i:%sZ')
		FROM mcp_servers WHERE class_id=? AND id=?`, classID, id))
}

// GetMCPAuthToken 仅宿主调用工具时读取令牌明文；不进入任何响应或日志。
func (s *Store) GetMCPAuthToken(ctx context.Context, classID, id int64) (string, error) {
	var token string
	err := s.DB.QueryRowContext(ctx,
		`SELECT auth_token FROM mcp_servers WHERE class_id=? AND id=?`, classID, id).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("读取令牌失败: %w", err)
	}
	return token, nil
}

// ListMCPServers 返回本班全部登记（不含令牌明文），id DESC。
func (s *Store) ListMCPServers(ctx context.Context, classID int64) ([]MCPServer, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id,class_id,owner_id,name,base_url,token_configured,connected,tools_json,last_error,
		       DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%sZ'),DATE_FORMAT(updated_at,'%Y-%m-%dT%H:%i:%sZ')
		FROM mcp_servers WHERE class_id=? ORDER BY id DESC`, classID)
	if err != nil {
		return nil, fmt.Errorf("列出 MCP 服务失败: %w", err)
	}
	defer rows.Close()
	out := []MCPServer{}
	for rows.Next() {
		sv, err := scanMCPServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sv)
	}
	return out, rows.Err()
}

// SetMCPConnected 连接成功后一次性写入工具清单并标记已连接（整体替换，不累加）。
func (s *Store) SetMCPConnected(ctx context.Context, classID, id int64, tools []MCPTool) error {
	b, err := json.Marshal(tools)
	if err != nil {
		return fmt.Errorf("序列化工具清单失败: %w", err)
	}
	_, err = s.DB.ExecContext(ctx, `
		UPDATE mcp_servers SET connected=1, tools_json=?, last_error='' WHERE class_id=? AND id=?`,
		string(b), classID, id)
	if err != nil {
		return fmt.Errorf("标记已连接失败: %w", err)
	}
	return nil
}

// SetMCPDisconnected 断开：清空工具清单且不可再调用。
func (s *Store) SetMCPDisconnected(ctx context.Context, classID, id int64) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE mcp_servers SET connected=0, tools_json=NULL WHERE class_id=? AND id=?`, classID, id)
	if err != nil {
		return fmt.Errorf("断开 MCP 服务失败: %w", err)
	}
	return nil
}

// SetMCPLastError 连接失败时记可读原因，停在未连接。
func (s *Store) SetMCPLastError(ctx context.Context, classID, id int64, msg string) error {
	if len([]rune(msg)) > 480 {
		msg = string([]rune(msg)[:480])
	}
	_, err := s.DB.ExecContext(ctx, `
		UPDATE mcp_servers SET connected=0, tools_json=NULL, last_error=? WHERE class_id=? AND id=?`,
		msg, classID, id)
	if err != nil {
		return fmt.Errorf("记录连接错误失败: %w", err)
	}
	return nil
}

// DeleteMCPServer 删除本班的一条登记。
func (s *Store) DeleteMCPServer(ctx context.Context, classID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM mcp_servers WHERE class_id=? AND id=?`, classID, id)
	if err != nil {
		return fmt.Errorf("删除 MCP 服务失败: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordMCPCall 追加一条调用记录（只增不改，无 UPDATE 路径）。
func (s *Store) RecordMCPCall(ctx context.Context, in MCPCall) error {
	if len(in.Detail) > 480 {
		in.Detail = in.Detail[:480]
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO mcp_tool_calls (class_id,actor_id,actor_name,tool_ref,result,conversation_id,round,detail)
		VALUES (?,?,?,?,?,?,?,?)`,
		in.ClassID, in.ActorID, in.ActorName, in.ToolRef, in.Result, in.ConversationID, in.Round, in.Detail)
	if err != nil {
		return fmt.Errorf("写入调用记录失败: %w", err)
	}
	return nil
}

// ListMCPCalls 返回本班调用记录，id DESC，limit 由调用方钳制 [1,50]。
func (s *Store) ListMCPCalls(ctx context.Context, classID int64, limit int) ([]MCPCall, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id,class_id,actor_id,actor_name,tool_ref,result,conversation_id,round,detail,
		       DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%s.%fZ')
		FROM mcp_tool_calls WHERE class_id=? ORDER BY id DESC LIMIT ?`, classID, limit)
	if err != nil {
		return nil, fmt.Errorf("列出调用记录失败: %w", err)
	}
	defer rows.Close()
	out := []MCPCall{}
	for rows.Next() {
		var c MCPCall
		if err := rows.Scan(&c.ID, &c.ClassID, &c.ActorID, &c.ActorName, &c.ToolRef, &c.Result,
			&c.ConversationID, &c.Round, &c.Detail, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("读取调用记录失败: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanMCPServer(row rowScanner) (MCPServer, error) {
	var sv MCPServer
	var tokenCfg, connected int
	var tools []byte
	err := row.Scan(&sv.ID, &sv.ClassID, &sv.OwnerID, &sv.Name, &sv.BaseURL, &tokenCfg, &connected,
		&tools, &sv.LastError, &sv.CreatedAt, &sv.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MCPServer{}, ErrNotFound
	}
	if err != nil {
		return MCPServer{}, fmt.Errorf("读取 MCP 服务失败: %w", err)
	}
	sv.TokenConfigured = tokenCfg != 0
	sv.Connected = connected != 0
	sv.Tools = []MCPTool{}
	if len(tools) > 0 {
		_ = json.Unmarshal(tools, &sv.Tools)
	}
	return sv, nil
}
