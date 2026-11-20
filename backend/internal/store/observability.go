package store

import (
	"context"
	"fmt"
)

// AuditEvent 是只增不改的操作留痕。技能、用量、耗时与对象字段为后续 additive migration；
// 旧动作通过 Audit 写入时仍保留兼容默认值。
type AuditEvent struct {
	ID         int64  `json:"id"`
	ActorID    *int64 `json:"actor_id"`
	ActorName  string `json:"actor_name"`
	ClassID    *int64 `json:"class_id"`
	Action     string `json:"action"`
	Result     string `json:"result"`
	Detail     string `json:"detail"`
	SkillName  string `json:"skill_name"`
	UsageCount int64  `json:"usage_count"`
	DurationMS int64  `json:"duration_ms"`
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	CreatedAt  string `json:"created_at"`
}

// AuditEvent 追加一条完整留痕；任何字段都不含口令、令牌或问题正文。
func (s *Store) WriteAuditEvent(ctx context.Context, in AuditEvent) error {
	if in.UsageCount < 0 {
		in.UsageCount = 0
	}
	in.Action = clipRunes(in.Action, 32)
	in.Result = clipRunes(in.Result, 16)
	in.Detail = clipRunes(in.Detail, 480)
	in.SkillName = clipRunes(in.SkillName, 64)
	in.ObjectType = clipRunes(in.ObjectType, 32)
	in.ObjectID = clipRunes(in.ObjectID, 128)
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO audit_logs
		  (actor_id,actor_name,class_id,action,result,detail,skill_name,usage_count,duration_ms,object_type,object_id)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		in.ActorID, in.ActorName, in.ClassID, in.Action, in.Result, in.Detail,
		in.SkillName, in.UsageCount, in.DurationMS, in.ObjectType, in.ObjectID)
	if err != nil {
		return fmt.Errorf("写入操作留痕失败: %w", err)
	}
	return nil
}

// ListAuditEvents 只查询本班记录，id 是唯一 tiebreaker。
func (s *Store) ListAuditEvents(ctx context.Context, classID int64, limit int) ([]AuditEvent, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id,actor_id,actor_name,class_id,action,result,detail,skill_name,usage_count,
		       duration_ms,object_type,object_id,DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%sZ')
		  FROM audit_logs WHERE class_id=? ORDER BY id DESC LIMIT ?`, classID, limit)
	if err != nil {
		return nil, fmt.Errorf("查询本班操作留痕失败: %w", err)
	}
	defer rows.Close()
	out := []AuditEvent{}
	for rows.Next() {
		var event AuditEvent
		if err := rows.Scan(&event.ID, &event.ActorID, &event.ActorName, &event.ClassID,
			&event.Action, &event.Result, &event.Detail, &event.SkillName, &event.UsageCount,
			&event.DurationMS, &event.ObjectType, &event.ObjectID, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("读取本班操作留痕失败: %w", err)
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

// SkillObservation 是技能调用量与错误率的真实聚合，不根据当前配置反推历史。
type SkillObservation struct {
	SkillName  string  `json:"skill_name"`
	Calls      int64   `json:"calls"`
	Errors     int64   `json:"errors"`
	UsageCount int64   `json:"usage_count"`
	ErrorRate  float64 `json:"error_rate"`
}

// ObserveSkills 聚合本班问答留痕；未启用技能的请求归入“基础问答”。
// 资料未命中是正常产品结果（detail=no_evidence），不计入技能错误率。
func (s *Store) ObserveSkills(ctx context.Context, classID int64) ([]SkillObservation, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT CASE WHEN skill_name='' THEN '基础问答' ELSE skill_name END AS skill,
		       COUNT(*) AS calls,
		       SUM(CASE WHEN result<>'ok' AND detail<>'no_evidence' THEN 1 ELSE 0 END) AS errors,
		       COALESCE(SUM(usage_count),0) AS usage_count
		  FROM audit_logs WHERE class_id=? AND action='ask'
		 GROUP BY CASE WHEN skill_name='' THEN '基础问答' ELSE skill_name END
		 ORDER BY skill`, classID)
	if err != nil {
		return nil, fmt.Errorf("聚合技能观测失败: %w", err)
	}
	defer rows.Close()
	out := []SkillObservation{}
	for rows.Next() {
		var item SkillObservation
		if err := rows.Scan(&item.SkillName, &item.Calls, &item.Errors, &item.UsageCount); err != nil {
			return nil, fmt.Errorf("读取技能观测失败: %w", err)
		}
		if item.Calls > 0 {
			item.ErrorRate = float64(item.Errors) / float64(item.Calls)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// QueueDepth 返回本班待执行与执行中的索引任务数。
func (s *Store) QueueDepth(ctx context.Context, classID int64) (int64, error) {
	var depth int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM index_tasks WHERE class_id=? AND status IN ('pending','running')`, classID).Scan(&depth)
	if err != nil {
		return 0, fmt.Errorf("查询本班队列深度失败: %w", err)
	}
	return depth, nil
}

func clipRunes(value string, limit int) string {
	r := []rune(value)
	if len(r) <= limit {
		return value
	}
	return string(r[:limit])
}
