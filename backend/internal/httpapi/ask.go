package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
	"github.com/Corday-nkusky/team_project/backend/internal/qa"
)

type askReq struct {
	Question string `json:"question"`
}

// handleAsk 处理 POST /api/ask，以 SSE 增量返回回答（R5 / R6 / R7，design D3）。
//
// 事件序列：meta → (degraded) → delta* → done；失败时以 error 结束且不发 done。
// 只有完整回答才落库，中断与空回答都不留半条记录（R6.4 / R6.5）。
func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	ctx := r.Context()

	var req askReq
	if err := decodeJSON(r, &req); err != nil {
		writeDecodeErr(w, err, "请求体格式不正确")
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
		logServerError(r, "retrieve", err)
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

	if len(cites) == 0 {
		// 未命中：固定文案 + no_evidence，不生成编造内容（R5.2 / R7.3）。
		sw.event("meta", map[string]any{"citations": []qa.Citation{}, "no_evidence": true})
		sw.event("delta", map[string]any{"text": qa.NoEvidenceText})
		sw.event("done", map[string]any{"conversation_id": nil})
		s.auditAsk(ctx, c, "fail", "no_evidence")
		return
	}

	sw.event("meta", map[string]any{"citations": cites, "no_evidence": false})

	filt := &citationFilter{max: len(cites)}
	// 越界引用被丢弃时只记数量，不记录回答内容（R7.4）。
	defer func() {
		if filt.Dropped() > 0 {
			slog.WarnContext(ctx, "citation_marker_dropped",
				"request_id", requestIDFrom(ctx),
				"count", filt.Dropped(),
				"max", len(cites),
			)
		}
	}()

	var answer strings.Builder
	if !s.qa.LLMEnabled() {
		text, aborted := sw.streamText(filt, qa.FallbackAnswer(req.Question, cites), 20*time.Millisecond)
		answer.WriteString(text)
		if aborted {
			s.auditAbort(c)
			return
		}
		s.finishWithFlush(r, sw, filt, c, req.Question, &answer, cites)
		return
	}

	sawDelta := false
	sawDone := false
	interrupted := false
	degraded := false

	ch, err := s.qa.ChatStream(ctx, qa.SystemPrompt(), qa.UserPrompt(req.Question, cites))
	if err != nil {
		degraded = true
	} else {
		for ev := range ch {
			if ev.Err != nil {
				// 首字前失败可安全降级；已有增量则视为中断，保住「半截不当完整」。
				if sawDelta {
					interrupted = true
				} else {
					degraded = true
				}
				break
			}
			if ev.Done {
				sawDone = true
				break
			}
			if ev.Delta == "" {
				continue
			}
			sawDelta = true
			safe := filt.Write(ev.Delta)
			answer.WriteString(safe)
			if safe != "" && !sw.event("delta", map[string]any{"text": safe}) {
				s.auditAbort(c)
				return
			}
		}
	}

	if ctx.Err() != nil {
		// 客户端断开：不发事件、不落库（R6.3）。
		s.auditAbort(c)
		return
	}
	if interrupted || (sawDelta && !sawDone && !degraded) {
		// 上游在已有增量后中断：明确告知，不发送 done（R6.4）。
		sw.event("error", map[string]any{
			"code":    "stream_incomplete",
			"message": "生成中断，请重试",
		})
		s.auditAsk(ctx, c, "fail", "生成中断")
		return
	}

	if degraded {
		// 模型首字前不可用：告知已降级，再以本班材料片段作答（R6.5）。
		sw.event("degraded", map[string]any{"reason": "upstream_error"})
		text, aborted := sw.streamText(filt, qa.FallbackAnswer(req.Question, cites), 0)
		answer.WriteString(text)
		if aborted {
			s.auditAbort(c)
			return
		}
	}

	s.finishWithFlush(r, sw, filt, c, req.Question, &answer, cites)
}

// streamText 把固定文本按小片经引用过滤器增量写出，返回过滤后的完整文本。
// aborted 为 true 表示客户端连接已断开。
func (sw *sseWriter) streamText(filt *citationFilter, text string, pace time.Duration) (string, bool) {
	var out strings.Builder
	for _, piece := range chunkRunes(text, 24) {
		if safe := filt.Write(piece); safe != "" {
			out.WriteString(safe)
			if !sw.event("delta", map[string]any{"text": safe}) {
				return out.String(), true
			}
		}
		if pace > 0 {
			time.Sleep(pace) // 让增量可见，兼作演示节奏
		}
	}
	return out.String(), false
}

// finishWithFlush 放出过滤器尾部缓冲；空回答按失败处理，否则落库并结束（R6.1 / R6.6）。
func (s *Server) finishWithFlush(r *http.Request, sw *sseWriter, filt *citationFilter,
	c auth.Claims, question string, answer *strings.Builder, cites []qa.Citation) {

	if tail := filt.Flush(); tail != "" {
		answer.WriteString(tail)
		if !sw.event("delta", map[string]any{"text": tail}) {
			s.auditAbort(c)
			return
		}
	}
	if strings.TrimSpace(answer.String()) == "" {
		sw.event("error", map[string]any{
			"code":    "empty_answer",
			"message": "模型未返回内容，请重试",
		})
		s.auditAsk(r.Context(), c, "fail", "空回答")
		return
	}
	s.finishAsk(r, sw, c, question, answer.String(), cites)
}

// finishAsk 落库会话、留痕并发送结束事件（R6.2）。
func (s *Server) finishAsk(r *http.Request, sw *sseWriter, c auth.Claims,
	question, answer string, cites []qa.Citation) {

	ctx := r.Context()
	payload, err := json.Marshal(cites)
	if err != nil {
		logServerError(r, "marshal_citations", err)
		payload = []byte("[]")
	}
	if err := s.rec.SaveConversation(ctx, c.UserID, c.ClassID, question, answer, payload); err != nil {
		logServerError(r, "save_conversation", err)
	}
	if err := s.rec.Audit(ctx, &c.UserID, c.Name, &c.ClassID, "ask", "ok",
		fmt.Sprintf("引用%d条", len(cites))); err != nil {
		logServerError(r, "audit_ask", err)
	}
	sw.event("done", map[string]any{"conversation_id": nil})
}

// auditAsk 写一条问答审计；审计失败不回滚已完成的回答，但必须留下 ERROR 日志。
func (s *Server) auditAsk(ctx context.Context, c auth.Claims, result, detail string) {
	if err := s.rec.Audit(ctx, &c.UserID, c.Name, &c.ClassID, "ask", result, detail); err != nil {
		slog.ErrorContext(ctx, "audit_write_failed",
			"request_id", requestIDFrom(ctx),
			"operation", "audit_ask",
			"error", err.Error(),
		)
	}
}

// auditAbort 记录客户端中断；此时请求 ctx 已取消，改用 Background 保证审计能写入。
func (s *Server) auditAbort(c auth.Claims) {
	_ = s.rec.Audit(context.Background(), &c.UserID, c.Name, &c.ClassID, "ask", "fail", "客户端中断")
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
