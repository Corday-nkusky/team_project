package httpapi

// 问答链路的技能与 MCP 工具接入（spec SK2 / MC3）。
// 纪律：技能/工具加载失败都不得中断问答主路径，只记 ERROR 日志并按「无技能/无工具」继续。

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
	"github.com/Corday-nkusky/team_project/backend/internal/mcpclient"
	"github.com/Corday-nkusky/team_project/backend/internal/skill"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

// askContext 是本次问答加载到的技能与工具结果，供 meta 与生成链路共用。
type askContext struct {
	Active    skill.Active
	System    string
	ToolCalls []mcpclient.ToolEvent
	// ToolNotes 回传给模型的工具数据（作为用户消息一侧的数据，不是指令）。
	ToolNotes []string
}

// prepareAsk 加载技能、叠加提示词并执行显式绑定的工具。
//
// conversationID 在首次问答时为空；调用记录按「谁 + 哪一轮」记，会话号后续由 done 回执。
func (s *Server) prepareAsk(ctx context.Context, c auth.Claims, question string) askContext {
	result := askContext{Active: skill.Active{Enabled: false}}
	active, system, err := s.skills.Load(ctx, c.ClassID)
	if err != nil {
		// 技能查询失败：按无技能继续（SK2.3 / R5.11）。
		slog.ErrorContext(ctx, "skill_load_failed",
			"request_id", requestIDFrom(ctx), "error", err.Error())
	}
	result.Active = active
	result.System = system
	result.ToolCalls = []mcpclient.ToolEvent{}

	if !active.Enabled || len(active.ToolRefs) == 0 {
		return result
	}

	// 按班加载已连接服务器的工具清单，供引用解析与参数校验。
	servers, err := s.mcp.ListMCPServers(ctx, c.ClassID)
	if err != nil {
		slog.ErrorContext(ctx, "mcp_servers_load_failed",
			"request_id", requestIDFrom(ctx), "error", err.Error())
		return result
	}
	index := make(map[string]store.MCPServer)
	for _, sv := range servers {
		index[sv.Name] = sv
	}

	calls := 0
	for round, ref := range active.ToolRefs {
		if !mcpclient.WithinLimits(round+1, calls) {
			// 超限：优雅降级，停止调用，用已有信息作答（MC3.3）。
			result.ToolCalls = append(result.ToolCalls, mcpclient.ToolEvent{
				ToolRef: ref, Result: mcpclient.ResultDenied, Reason: "调用次数超过上限，已改用已有信息作答",
			})
			break
		}
		parts := strings.SplitN(ref, "@", 2)
		if len(parts) != 2 {
			result.ToolCalls = append(result.ToolCalls, mcpclient.ToolEvent{
				ToolRef: ref, Result: mcpclient.ResultDenied, Reason: "工具引用格式错误",
			})
			s.recordToolCall(ctx, c, ref, mcpclient.ResultDenied, "格式错误", round+1)
			continue
		}
		sv, ok := index[parts[1]]
		if !ok || !sv.Connected {
			result.ToolCalls = append(result.ToolCalls, mcpclient.ToolEvent{
				ToolRef: ref, Result: mcpclient.ResultDenied, Reason: "工具服务不存在或未连接",
			})
			s.recordToolCall(ctx, c, ref, mcpclient.ResultDenied, "未连接", round+1)
			continue
		}
		client, err := s.mcpClientFor(ctx, sv)
		if err != nil {
			slog.ErrorContext(ctx, "mcp_token_failed", "request_id", requestIDFrom(ctx), "error", err.Error())
			result.ToolCalls = append(result.ToolCalls, mcpclient.ToolEvent{
				ToolRef: ref, Result: mcpclient.ResultDenied, Reason: "读取令牌失败",
			})
			continue
		}
		tools := make([]mcpclient.Tool, 0, len(sv.Tools))
		for _, t := range sv.Tools {
			tools = append(tools, mcpclient.Tool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
		}
		out := mcpclient.Execute(ctx, mcpclient.CallRequest{
			ToolRef: ref,
			// 无网关时按显式绑定调用；参数取问题的主题词（formula_hint 需要 topic）。
			Arguments: map[string]any{"topic": question},
			ActorID:   c.UserID,
			ActorName: c.Name,
			Tools:     tools,
			Client:    client,
			Round:     round + 1,
		})
		result.ToolCalls = append(result.ToolCalls, out.Event)
		if out.Text != "" {
			result.ToolNotes = append(result.ToolNotes, fmt.Sprintf("[工具 %s 返回的数据] %s", ref, out.Text))
		}
		s.recordToolCall(ctx, c, ref, out.Event.Result, out.Event.Reason, round+1)
		calls++
	}
	return result
}

func (s *Server) recordToolCall(ctx context.Context, c auth.Claims, ref, result, detail string, round int) {
	err := s.mcp.RecordMCPCall(ctx, store.MCPCall{
		ClassID: c.ClassID, ActorID: c.UserID, ActorName: c.Name,
		ToolRef: ref, Result: result, Round: round, Detail: detail,
	})
	if err != nil {
		slog.ErrorContext(ctx, "mcp_call_record_failed",
			"request_id", requestIDFrom(ctx), "error", err.Error())
	}
}
