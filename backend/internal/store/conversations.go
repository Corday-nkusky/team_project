package store

import (
	"context"
	"encoding/json"
	"time"
)

// conversationRow 是历史问答列表的一行。citations 原样透传（H1.1），
// 不重新检索、不拼接回答，也不把 user_id / class_id 暴露给客户端。
type conversationRow struct {
	ID        int64           `json:"id"`
	Question  string          `json:"question"`
	Answer    string          `json:"answer"`
	Citations json.RawMessage `json:"citations"`
	CreatedAt time.Time       `json:"created_at"`
}

// ListConversationsJSON 按「本人 + 本班」倒序取最近的完整问答，返回 JSON 数组。
//
// 隔离纪律：user_id 与 class_id 都进 WHERE，排序 id DESC 自带唯一 tiebreaker。
// limit 由 handler 校验并截断，这里只负责执行。citations 为 NULL 或非法 JSON
// （早期数据）时归一为 []，保证客户端拿到的永远是数组。
func (s *Store) ListConversationsJSON(ctx context.Context, userID, classID int64, limit int) (json.RawMessage, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, question, answer, citations, created_at
		 FROM conversations WHERE user_id = ? AND class_id = ?
		 ORDER BY id DESC LIMIT ?`, userID, classID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]conversationRow, 0)
	for rows.Next() {
		var c conversationRow
		var cites []byte
		if err := rows.Scan(&c.ID, &c.Question, &c.Answer, &cites, &c.CreatedAt); err != nil {
			return nil, err
		}
		if len(cites) == 0 || string(cites) == "null" || !json.Valid(cites) {
			cites = []byte("[]")
		}
		c.Citations = json.RawMessage(cites)
		items = append(items, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return json.Marshal(items)
}
