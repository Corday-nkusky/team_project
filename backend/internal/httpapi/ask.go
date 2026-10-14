package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
	"github.com/Corday-nkusky/team_project/backend/internal/qa"
)

type askReq struct {
	Question string `json:"question"`
}

// handleAsk 处理 POST /api/ask，以 SSE 增量返回回答（R5 / R6 / R7）。
func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	ctx := r.Context()

	var req askReq
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "请求体格式不正确")
		return
	}
	if strings.TrimSpace(req.Question) == "" {
		// 空问题直接拒绝，绝不调用模型（R5.3）。
		writeErr(w, http.StatusBadRequest, "empty_question", "问题不能为空")
		return
	}

	// 检索只在本班 ready 材料内进行（R2.2 / R7.2）。
	cites, err := s.qa.Retrieve(ctx, c.ClassID, req.Question)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", "检索失败")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "internal", "当前服务器不支持流式响应")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sw := &sseWriter{w: w, f: flusher}
	aborted := false

	if len(cites) == 0 {
		// 未命中：固定文案 + no_evidence，不生成编造内容（R5.2 / R7.3）。
		sw.event("meta", map[string]any{"citations": []qa.Citation{}, "no_evidence": true})
		sw.event("delta", map[string]any{"text": qa.NoEvidenceText})
		sw.event("done", map[string]any{"conversation_id": nil})
		_ = s.st.Audit(ctx, &c.UserID, c.Name, &c.ClassID, "ask", "fail", "no_evidence")
		return
	}

	sw.event("meta", map[string]any{"citations": cites, "no_evidence": false})

	var answer strings.Builder
	if s.qa.LLMEnabled() {
		ch, err := s.qa.ChatStream(ctx, qa.SystemPrompt(), qa.UserPrompt(req.Question, cites))
		if err != nil {
			// 模型不可用：退回抽取式回答，仍保证有据可依（链路不中断）。
			aborted = false
			answer.Reset()
			for _, piece := range chunkRunes(qa.FallbackAnswer(req.Question, cites), 24) {
				if !sw.event("delta", map[string]any{"text": piece}) {
					aborted = true
					break
				}
			}
			if aborted {
				_ = s.st.Audit(ctx, &c.UserID, c.Name, &c.ClassID, "ask", "fail", "客户端中断")
				return
			}
			answer.WriteString(qa.FallbackAnswer(req.Question, cites))
			s.finishAsk(ctx, sw, c, req.Question, answer.String(), cites)
			return
		}
		for ev := range ch {
			if ev.Err != nil {
				break
			}
			if ev.Done {
				break
			}
			if ev.Delta == "" {
				continue
			}
			answer.WriteString(ev.Delta)
			if !sw.event("delta", map[string]any{"text": ev.Delta}) {
				aborted = true
				break
			}
		}
	} else {
		// 零密钥降级：直接以本班材料片段作答（R7.1）。
		for _, piece := range chunkRunes(qa.FallbackAnswer(req.Question, cites), 24) {
			answer.WriteString(piece)
			if !sw.event("delta", map[string]any{"text": piece}) {
				aborted = true
				break
			}
			time.Sleep(20 * time.Millisecond) // 让增量可见，兼作演示节奏
		}
	}

	if aborted || ctx.Err() != nil {
		// 客户端断开：不落库，不留半条记录（R6.3）。
		_ = s.st.Audit(context.Background(), &c.UserID, c.Name, &c.ClassID, "ask", "fail", "客户端中断")
		return
	}
	s.finishAsk(ctx, sw, c, req.Question, answer.String(), cites)
}

// finishAsk 落库会话、留痕并发送结束事件（R6.2）。
func (s *Server) finishAsk(ctx context.Context, sw *sseWriter, c auth.Claims, question, answer string, cites []qa.Citation) {
	payload, _ := json.Marshal(cites)
	_ = s.st.SaveConversation(ctx, c.UserID, c.ClassID, question, answer, payload)
	_ = s.st.Audit(ctx, &c.UserID, c.Name, &c.ClassID, "ask", "ok", fmt.Sprintf("引用%d条", len(cites)))
	sw.event("done", map[string]any{"conversation_id": nil})
}

// sseWriter 负责按 SSE 协议分帧写出并立即 Flush（design D6）。
type sseWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

// event 写一个事件；返回 false 表示底层连接已断（调用方应停止生成）。
func (s *sseWriter) event(name string, data any) bool {
	b, err := json.Marshal(data)
	if err != nil {
		return true
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", name, b); err != nil {
		return false
	}
	s.f.Flush()
	return true
}

// chunkRunes 把字符串按 rune 切成固定大小片段，便于制造可见的增量。
func chunkRunes(s string, size int) []string {
	r := []rune(s)
	var out []string
	for i := 0; i < len(r); i += size {
		end := i + size
		if end > len(r) {
			end = len(r)
		}
		out = append(out, string(r[i:end]))
	}
	return out
}
