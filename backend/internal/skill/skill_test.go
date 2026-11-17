package skill

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Corday-nkusky/team_project/backend/internal/qa"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

type stubReader struct {
	skills []store.Skill
	err    error
}

func (r stubReader) ListEnabledSkills(context.Context, int64) ([]store.Skill, error) {
	return r.skills, r.err
}

func TestNoSkillReturnsBasePromptVerbatim(t *testing.T) {
	l := New(stubReader{skills: nil})
	active, prompt, err := l.Load(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if active.Enabled {
		t.Fatal("无技能时 active.Enabled 应为 false")
	}
	if prompt != qa.SystemPrompt() {
		t.Fatal("无技能时提示词必须逐字等于 qa.SystemPrompt()")
	}
}

func TestEnabledSkillComposesPrompt(t *testing.T) {
	l := New(stubReader{skills: []store.Skill{{
		Name: "解题引导", Kind: "solve_hint", Enabled: true,
		Instruction: "先问学生卡在哪一步", ToolRefs: []string{"formula_hint@campus-tools"},
	}}})
	active, prompt, err := l.Load(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !active.Enabled || active.Name != "解题引导" {
		t.Fatalf("active=%+v", active)
	}
	if len(active.ToolRefs) != 1 {
		t.Fatalf("tool_refs=%v", active.ToolRefs)
	}
	for _, want := range []string{"解题引导", "先问学生卡在哪一步", "不直接给出最终答案", "[编号]"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("提示词缺少 %q", want)
		}
	}
	if !strings.HasPrefix(prompt, qa.SystemPrompt()) {
		t.Fatal("叠加必须以基础提示词为前缀")
	}
}

func TestReaderFailureDegradesToBase(t *testing.T) {
	l := New(stubReader{err: errors.New("db down")})
	active, prompt, err := l.Load(context.Background(), 1)
	if err == nil {
		t.Fatal("技能查询失败应返回 error 供上层记日志")
	}
	if active.Enabled || prompt != qa.SystemPrompt() {
		t.Fatal("技能查询失败时必须按无技能处理")
	}
}

func TestDecorateFallbackDiffersBySwitch(t *testing.T) {
	base := "（以下回答直接摘自本班材料）"
	if got := DecorateFallback(base, Active{Enabled: false}); got != base {
		t.Fatal("关闭技能时降级文本应逐字不变")
	}
	got := DecorateFallback(base, Active{Enabled: true, Name: "解题引导"})
	if !strings.Contains(got, "解题引导") || got == base {
		t.Fatal("启用技能时降级文本应有引导差异")
	}
}
