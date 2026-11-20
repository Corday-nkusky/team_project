package httpapi

import (
	"context"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/auth"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

const askAuditKey ctxKey = "ask_audit"

type askAuditState struct {
	started    time.Time
	skillName  string
	usageCount int64
	objectType string
	objectID   string
}

func withAskAudit(ctx context.Context) context.Context {
	return context.WithValue(ctx, askAuditKey, &askAuditState{
		started: time.Now(), usageCount: 1, objectType: "http_request", objectID: requestIDFrom(ctx),
	})
}

func noteAskContext(ctx context.Context, askCtx askContext) {
	state, _ := ctx.Value(askAuditKey).(*askAuditState)
	if state == nil {
		return
	}
	state.usageCount = 1 + int64(len(askCtx.ToolCalls))
	if askCtx.Active.Enabled {
		state.skillName = askCtx.Active.Name
	}
}

func noteAskObject(ctx context.Context, objectType, objectID string) {
	state, _ := ctx.Value(askAuditKey).(*askAuditState)
	if state != nil {
		state.objectType, state.objectID = objectType, objectID
	}
}

func (s *Server) newAskAuditEvent(ctx context.Context, c auth.Claims, result, detail string) store.AuditEvent {
	event := store.AuditEvent{
		ActorID: &c.UserID, ActorName: c.Name, ClassID: &c.ClassID,
		Action: "ask", Result: result, Detail: detail, UsageCount: 1,
		ObjectType: "http_request", ObjectID: requestIDFrom(ctx),
	}
	if state, _ := ctx.Value(askAuditKey).(*askAuditState); state != nil {
		event.SkillName = state.skillName
		event.UsageCount = state.usageCount
		event.DurationMS = time.Since(state.started).Milliseconds()
		event.ObjectType, event.ObjectID = state.objectType, state.objectID
	}
	return event
}

func (s *Server) writeAuditEvent(ctx context.Context, event store.AuditEvent) error {
	if s.rec == nil {
		return nil
	}
	if recorder, ok := s.rec.(interface {
		WriteAuditEvent(context.Context, store.AuditEvent) error
	}); ok {
		return recorder.WriteAuditEvent(ctx, event)
	}
	// 兼容旧测试桩与既有读取器；生产 Store 始终走完整七列。
	return s.rec.Audit(ctx, event.ActorID, event.ActorName, event.ClassID,
		event.Action, event.Result, event.Detail)
}
