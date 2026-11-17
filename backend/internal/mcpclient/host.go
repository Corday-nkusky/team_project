package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// 结果状态（写进调用记录与 meta.tool_calls）。
const (
	ResultOK     = "ok"
	ResultDenied = "denied"
	ResultError  = "error"
)

// 指令式句式：命中即整条丢弃（spec MC3.3）。只匹配祈使/角色切换的强信号，
// 避免误伤正常教学内容（如「忽略」出现在「忽略空气阻力」里）。
var instructionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)忽略(以上|上述|之前|前面|所有)(指令|要求|提示|规则|约束)`),
	regexp.MustCompile(`(?i)(请|请你)?(输出|显示|泄露|告诉我)(你的)?(系统提示|system prompt|提示词原文)`),
	regexp.MustCompile(`(?i)(现在|从现在起)你是`),
	regexp.MustCompile(`(?i)<\|?(im_start|system)\|?>`),
}

// MaxToolResultRunes 是工具结果回传前的截断上限。
const MaxToolResultRunes = 800

// MaxRounds / MaxCalls 是调用编排的轮次与累计上限。
const (
	MaxRounds = 3
	MaxCalls  = 4
)

// ToolEvent 是进入 SSE meta.tool_calls 的一条记录。
type ToolEvent struct {
	ToolRef string `json:"tool_ref"`
	Result  string `json:"result"`
	Reason  string `json:"reason,omitempty"`
}

// CallRequest 是一次宿主执行的输入。
type CallRequest struct {
	ToolRef        string
	Arguments      map[string]any
	ActorID        int64
	ActorName      string
	ConversationID string
	Round          int
	Tools          []Tool
	Client         *Client
}

// CallOutcome 是执行结果，含回传给模型的文本与记录事件。
type CallOutcome struct {
	Text  string
	Event ToolEvent
}

// CheckToolRef 校验 `tool@server` 格式：工具名小写字母开头、不含连字符；服务器名可含连字符。
func CheckToolRef(ref string) error {
	parts := strings.SplitN(ref, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("工具引用格式应为 工具名@服务器名，收到 %q", ref)
	}
	tool, server := parts[0], parts[1]
	if tool[0] < 'a' || tool[0] > 'z' {
		return fmt.Errorf("工具名必须以小写字母开头: %q", tool)
	}
	if strings.ContainsAny(tool, "-@ ") {
		return fmt.Errorf("工具名不能含连字符或空格: %q", tool)
	}
	if strings.ContainsAny(server, "@ ") {
		return fmt.Errorf("服务器名不能含 @ 或空格: %q", server)
	}
	return nil
}

// ValidateRefs 保存技能前校验全部工具引用：格式 + 工具在发现清单内。
// 服务器是否已连接由上层按库状态判定（spec MC3.2）。
func ValidateRefs(refs []string, toolsByServer map[string][]Tool) error {
	for _, ref := range refs {
		if err := CheckToolRef(ref); err != nil {
			return err
		}
		parts := strings.SplitN(ref, "@", 2)
		found := false
		for _, t := range toolsByServer[parts[1]] {
			if t.Name == parts[0] {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("工具 %q 不在服务器 %q 的发现清单内", parts[0], parts[1])
		}
	}
	return nil
}

// ValidateArguments 按工具的 inputSchema 做最小校验：required 字段齐全、类型匹配。
func ValidateArguments(tool Tool, args map[string]any) error {
	root, _ := tool.InputSchema["type"].(string)
	if root != "" && root != "object" {
		return fmt.Errorf("工具 %q 的输入结构必须是 object", tool.Name)
	}
	required, _ := tool.InputSchema["required"].([]any)
	for _, r := range required {
		key, ok := r.(string)
		if !ok {
			continue
		}
		if _, exists := args[key]; !exists {
			return fmt.Errorf("工具 %q 缺少必填参数 %q", tool.Name, key)
		}
	}
	props, _ := tool.InputSchema["properties"].(map[string]any)
	for key, raw := range args {
		spec, ok := props[key].(map[string]any)
		if !ok {
			return fmt.Errorf("工具 %q 不接受参数 %q", tool.Name, key)
		}
		want, _ := spec["type"].(string)
		if !typeMatches(want, raw) {
			return fmt.Errorf("工具 %q 的参数 %q 类型应为 %s", tool.Name, key, want)
		}
	}
	return nil
}

func typeMatches(want string, v any) bool {
	switch want {
	case "", "string":
		_, ok := v.(string)
		return ok
	case "number", "integer":
		switch v.(type) {
		case float64, int, int64, json.Number:
			return true
		}
		return false
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	}
	return true
}

// Execute 执行一次工具调用：先校验参数，再执行，最后做结果检查。
// 参数不合法 → "被拒"；结果含指令句式 → 整条丢弃并记"被拒"；执行失败 → "出错"。
func Execute(ctx context.Context, req CallRequest) CallOutcome {
	ev := ToolEvent{ToolRef: req.ToolRef, Result: ResultError}
	name := strings.SplitN(req.ToolRef, "@", 2)[0]

	var tool *Tool
	for i := range req.Tools {
		if req.Tools[i].Name == name {
			tool = &req.Tools[i]
			break
		}
	}
	if tool == nil {
		ev.Result = ResultDenied
		ev.Reason = "工具不在发现清单内"
		return CallOutcome{Event: ev}
	}
	if err := ValidateArguments(*tool, req.Arguments); err != nil {
		ev.Result = ResultDenied
		ev.Reason = err.Error()
		return CallOutcome{Event: ev}
	}
	if req.Client == nil {
		ev.Result = ResultError
		ev.Reason = "工具服务未连接"
		return CallOutcome{Event: ev}
	}
	text, err := req.Client.CallTool(ctx, name, req.Arguments)
	if err != nil {
		ev.Result = ResultError
		ev.Reason = truncate(err.Error(), 200)
		return CallOutcome{Event: ev}
	}
	for _, p := range instructionPatterns {
		if p.MatchString(text) {
			ev.Result = ResultDenied
			ev.Reason = "工具结果含指令式句式，已丢弃"
			return CallOutcome{Event: ev}
		}
	}
	text = truncate(text, MaxToolResultRunes)
	if strings.TrimSpace(text) == "" {
		ev.Result = ResultError
		ev.Reason = "工具返回空内容"
		return CallOutcome{Event: ev}
	}
	ev.Result = ResultOK
	return CallOutcome{Text: text, Event: ev}
}

// WithinLimits 判断本次调用是否仍在轮次与累计上限内（超限由上层优雅降级）。
func WithinLimits(round, calls int) bool {
	return round >= 1 && round <= MaxRounds && calls < MaxCalls
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
