package httpapi

// 索引任务的提交契约与只读状态接口（spec async-indexing A2）。
//
// 提交响应只是「号码牌」：状态码 202，只含 task_id / material_id / status / status_url，
// 不含切片数、不含 ready、不含任何索引结果。状态接口只读，绝不触发执行。

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/store"
)

// taskAcceptedView 是 202 的响应体，也是课件要求的「任务标识 + 状态地址」。
type taskAcceptedView struct {
	TaskID     string `json:"task_id"`
	MaterialID int64  `json:"material_id"`
	Status     string `json:"status"`
	StatusURL  string `json:"status_url"`
}

// acceptedView 用任务当前状态构造 202 响应；刚入队时状态为 pending，
// 幂等命中已有任务时如实返回 succeeded / failed 等当前状态。
func (s *Server) acceptedView(r *http.Request, classID int64, taskID string, materialID int64) taskAcceptedView {
	status := store.TaskPending
	if t, err := s.st.GetTask(r.Context(), taskID, classID); err == nil {
		status = t.Status
	}
	return taskAcceptedView{
		TaskID: taskID, MaterialID: materialID, Status: status,
		StatusURL: "/api/tasks/" + taskID,
	}
}

// taskView 是状态接口的响应体；不含索引结果，只读可反复调用。
type taskView struct {
	TaskID     string     `json:"task_id"`
	MaterialID int64      `json:"material_id"`
	Kind       string     `json:"kind"`
	Status     string     `json:"status"`
	Attempts   int        `json:"attempts"`
	Error      string     `json:"error,omitempty"`
	RequestID  string     `json:"request_id,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

func timePtr(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	v := nt.Time
	return &v
}

// handleGetTask 处理 GET /api/tasks/{task_id}：本班只读查询，跨班统一 404。
func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	taskID := r.PathValue("task_id")
	if !validTaskID(taskID) {
		writeErr(w, http.StatusBadRequest, "bad_request", "任务标识不合法")
		return
	}
	t, err := s.st.GetTask(r.Context(), taskID, c.ClassID)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "任务不存在")
		return
	}
	if err != nil {
		logServerError(r, "get_task", err)
		writeErr(w, http.StatusInternalServerError, "internal", "查询失败")
		return
	}
	view := taskView{
		TaskID: t.TaskID, MaterialID: t.MaterialID, Kind: t.Kind, Status: t.Status,
		Attempts: t.Attempts, Error: t.Error, RequestID: t.RequestID, CreatedAt: t.CreatedAt,
		StartedAt:  timePtr(t.StartedAt),
		FinishedAt: timePtr(t.FinishedAt),
	}
	writeJSON(w, http.StatusOK, view)
}

// validTaskID 只接受 32 位十六进制，避免任意字符串进入查询。
func validTaskID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
