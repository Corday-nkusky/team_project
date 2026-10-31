//go:build llm_integration

package llm_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/llm"
)

// TestConfiguredGateway 直连真实网关，验证嵌入与流式对话两条链路。
//
// 只在显式启用 build tag 且提供真实配置时运行：
//
//	LLM_API_KEY=... LLM_API_BASE=... LLM_CHAT_MODEL=... LLM_EMBED_MODEL=... \
//	  go test -tags=llm_integration ./internal/llm
func TestConfiguredGateway(t *testing.T) {
	key := strings.TrimSpace(os.Getenv("LLM_API_KEY"))
	if key == "" {
		t.Skip("LLM_API_KEY 为空，跳过真实网关集成测试")
	}
	base := strings.TrimSpace(os.Getenv("LLM_API_BASE"))
	chatModel := strings.TrimSpace(os.Getenv("LLM_CHAT_MODEL"))
	embedModel := strings.TrimSpace(os.Getenv("LLM_EMBED_MODEL"))
	if base == "" || chatModel == "" || embedModel == "" {
		t.Fatal("启用真实网关测试时必须提供 LLM_API_BASE、LLM_CHAT_MODEL 与 LLM_EMBED_MODEL")
	}

	client := llm.New(base, key, chatModel, embedModel)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	vecs, err := client.Embed(ctx, []string{"campusclaw 集成测试嵌入探针"})
	if err != nil {
		t.Fatalf("嵌入调用失败: %v", err)
	}
	if len(vecs) != 1 || len(vecs[0]) == 0 {
		t.Fatalf("嵌入结果不符合预期: %d 条向量", len(vecs))
	}
	t.Logf("嵌入维度=%d", len(vecs[0]))

	ch, err := client.ChatStream(ctx, "只回复两个字：收到", "请按系统提示回答")
	if err != nil {
		t.Fatalf("对话调用失败: %v", err)
	}
	var answer strings.Builder
	done := false
	for ev := range ch {
		if ev.Err != nil {
			t.Fatalf("流式对话中断: %v", ev.Err)
		}
		if ev.Done {
			done = true
		}
		answer.WriteString(ev.Delta)
	}
	if !done {
		t.Fatal("流式对话未收到结束事件")
	}
	if strings.TrimSpace(answer.String()) == "" {
		t.Fatal("流式对话没有返回任何内容")
	}
	t.Logf("流式回答长度=%d", len([]rune(answer.String())))
}
