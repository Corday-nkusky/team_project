package httpapi

// 历史问答只读接口（spec ask-history H1）。
//
// 数据本来就在 conversations 表里（完整回答才落库，见 R6.3），这里只补「读出来」：
// 不新建表、不重新检索、不改写回答。隔离条件由 store 的 SQL 保证。

import (
	"encoding/json"
	"net/http"
	"strconv"
)

const (
	defaultConversationLimit = 20
	maxConversationLimit     = 50
)

// handleListConversations 处理 GET /api/conversations：本人 + 本班，倒序，最多 50 条。
func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())

	limit := defaultConversationLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, "bad_request", "limit 必须是正整数")
			return
		}
		if n > maxConversationLimit {
			n = maxConversationLimit
		}
		limit = n
	}

	items, err := s.rec.ListConversationsJSON(r.Context(), c.UserID, c.ClassID, limit)
	if err != nil {
		logServerError(r, "list_conversations", err)
		writeErr(w, http.StatusInternalServerError, "internal", "查询失败")
		return
	}
	if len(items) == 0 {
		items = json.RawMessage("[]")
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
