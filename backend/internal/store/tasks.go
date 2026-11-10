package store

// 本文件是「索引任务队列」的全部持久化读写：
//   - 事务性发件箱（材料与任务同事务）；
//   - 由业务含义派生的幂等键 + 数据库唯一约束；
//   - 只读状态查询、SKIP LOCKED 抢占、租约回收、状态机与终态落库。
//
// 队列语义是「至少一次投递」：领取即 attempts+1，租约过期回收为 pending，
// 重复执行由 chunks 唯一约束与确定性向量 ID 吸收（见 qa/point_id.go）。

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// 任务状态与类型。failed 是不可重试终态，dead 是重试耗尽的死信。
const (
	TaskPending   = "pending"
	TaskRunning   = "running"
	TaskSucceeded = "succeeded"
	TaskFailed    = "failed"
	TaskDead      = "dead"

	TaskKindUpload   = "upload"
	TaskKindReindex  = "reindex"
	taskIDBytes      = 16
	maxTaskErrorRune = 480
)

// IndexTask 是一条持久化的索引工作记录。
type IndexTask struct {
	ID             int64
	TaskID         string
	ClassID        int64
	MaterialID     int64
	Kind           string
	Status         string
	Attempts       int
	IdempotencyKey string
	Error          string
	RequestID      string
	RunAt          time.Time
	LeaseUntil     sql.NullTime
	StartedAt      sql.NullTime
	FinishedAt     sql.NullTime
	CreatedAt      time.Time
}

// ContentHash 由标题与正文派生（不含随机量），用于入口幂等。
func ContentHash(title, content string) string {
	sum := sha256.Sum256([]byte(title + "\x00" + content))
	return hex.EncodeToString(sum[:])
}

// ContentDigest 只由正文派生，供重索引键区分内容版本。
func ContentDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// UploadIdempotencyKey 由班级与内容派生：同一班同一份提交永远得到同一个键。
func UploadIdempotencyKey(classID int64, contentHash string) string {
	return "upload:" + strconv.FormatInt(classID, 10) + ":" + contentHash
}

// ReindexIdempotencyKey 由材料、内容与版本派生：内容不变时重复点击同键，
// 索引成功后 revision+1，允许对同一内容再次重建。
func ReindexIdempotencyKey(materialID int64, digest string, revision int) string {
	return "reindex:" + strconv.FormatInt(materialID, 10) + ":" + digest + ":r" + strconv.Itoa(revision)
}

func newTaskID() string {
	var b [taskIDBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

func clipError(s string) string {
	r := []rune(s)
	if len(r) > maxTaskErrorRune {
		return string(r[:maxTaskErrorRune])
	}
	return s
}

// CreateMaterialWithTask 在一个事务里同时写出材料与 pending 任务。
// 同一内容重复提交时由唯一约束复用已有材料与任务，不产生第二份结果。
func (s *Store) CreateMaterialWithTask(ctx context.Context, classID, ownerID int64,
	title, content, requestID string) (materialID int64, taskID string, err error) {
	hash := ContentHash(title, content)
	idem := UploadIdempotencyKey(classID, hash)

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO materials (class_id, owner_id, title, content, content_hash, index_status, index_error)
		VALUES (?,?,?,?,?,'pending','')
		ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`, classID, ownerID, title, content, hash)
	if err != nil {
		return 0, "", fmt.Errorf("落库材料失败: %w", err)
	}
	materialID, err = res.LastInsertId()
	if err != nil {
		return 0, "", fmt.Errorf("读取材料 id 失败: %w", err)
	}

	generated := newTaskID()
	res, err = tx.ExecContext(ctx, `
		INSERT INTO index_tasks (task_id, class_id, material_id, kind, status, idempotency_key, request_id)
		VALUES (?,?,?,'upload','pending',?,?)
		ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`,
		generated, classID, materialID, idem, requestID)
	if err != nil {
		return 0, "", fmt.Errorf("写入索引任务失败: %w", err)
	}
	taskRowID, err := res.LastInsertId()
	if err != nil {
		return 0, "", fmt.Errorf("读取任务 id 失败: %w", err)
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT task_id FROM index_tasks WHERE id=?`, taskRowID).Scan(&taskID); err != nil {
		return 0, "", fmt.Errorf("读取任务标识失败: %w", err)
	}
	return materialID, taskID, tx.Commit()
}

// EnqueueReindex 为本班材料入队重索引任务。
// 命中已有任务时：进行中或已成功则原样返回（去重）；终态失败则重放同一行，
// 而不是新建任务——重试入口同样只出一份结果。
func (s *Store) EnqueueReindex(ctx context.Context, materialID, classID int64, requestID string) (string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	var content string
	var revision int
	err = tx.QueryRowContext(ctx,
		`SELECT content, revision FROM materials WHERE id=? AND class_id=? FOR UPDATE`,
		materialID, classID).Scan(&content, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("读取待重索引材料失败: %w", err)
	}
	idem := ReindexIdempotencyKey(materialID, ContentDigest(content), revision)

	var existingID int64
	var existingTaskID, status string
	err = tx.QueryRowContext(ctx,
		`SELECT id, task_id, status FROM index_tasks WHERE idempotency_key=? FOR UPDATE`, idem).
		Scan(&existingID, &existingTaskID, &status)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("查询重索引任务失败: %w", err)
	}
	if err == nil {
		if status == TaskFailed || status == TaskDead {
			if _, err := tx.ExecContext(ctx, `
				UPDATE index_tasks SET status='pending', attempts=0, error='', request_id=?,
				  run_at=NOW(3), lease_until=NULL, started_at=NULL, finished_at=NULL
				WHERE id=?`, requestID, existingID); err != nil {
				return "", fmt.Errorf("重放任务失败: %w", err)
			}
			if _, err := tx.ExecContext(ctx,
				`UPDATE materials SET index_status='pending', index_error='' WHERE id=?`, materialID); err != nil {
				return "", fmt.Errorf("重置材料状态失败: %w", err)
			}
		}
		return existingTaskID, tx.Commit()
	}

	taskID := newTaskID()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO index_tasks (task_id, class_id, material_id, kind, status, idempotency_key, request_id)
		VALUES (?,?,?,'reindex','pending',?,?)`, taskID, classID, materialID, idem, requestID); err != nil {
		return "", fmt.Errorf("写入重索引任务失败: %w", err)
	}
	return taskID, tx.Commit()
}

// GetTask 只读查询任务；跨班与不存在都返回 ErrNotFound。
func (s *Store) GetTask(ctx context.Context, taskID string, classID int64) (IndexTask, error) {
	var t IndexTask
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, task_id, class_id, material_id, kind, status, attempts, idempotency_key,
		       error, request_id, run_at, lease_until, started_at, finished_at, created_at
		  FROM index_tasks WHERE task_id=? AND class_id=?`, taskID, classID).Scan(
		&t.ID, &t.TaskID, &t.ClassID, &t.MaterialID, &t.Kind, &t.Status, &t.Attempts,
		&t.IdempotencyKey, &t.Error, &t.RequestID, &t.RunAt, &t.LeaseUntil,
		&t.StartedAt, &t.FinishedAt, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, fmt.Errorf("查询任务失败: %w", err)
	}
	return t, nil
}

// ChunkVersion 返回本班切片的最大自增 id。Web 进程据此发现 worker 在另一个
// 进程里写入的新切片，使按班缓存的词面索引跨进程失效。
func (s *Store) ChunkVersion(ctx context.Context, classID int64) (int64, error) {
	var v int64
	err := s.DB.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(id),0) FROM chunks WHERE class_id=?`, classID).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("读取切片版本失败: %w", err)
	}
	return v, nil
}

// RecoverStaleTasks 把租约过期的 running 任务退回 pending，进程崩溃后任务不丢。
func (s *Store) RecoverStaleTasks(ctx context.Context) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE index_tasks SET status='pending', lease_until=NULL,
		       error=CONCAT('租约到期重投: ', LEFT(error, 400))
		 WHERE status='running' AND lease_until IS NOT NULL AND lease_until < NOW(3)`)
	if err != nil {
		return 0, fmt.Errorf("回收过期任务失败: %w", err)
	}
	return res.RowsAffected()
}

// ClaimTasks 用 FOR UPDATE SKIP LOCKED 原子领取一批到期任务，attempts+1 并写入租约。
// MySQL 8.4 的 SKIP LOCKED 让多个 worker 竞争同一队列时不需要额外加锁。
func (s *Store) ClaimTasks(ctx context.Context, limit int, lease time.Duration) ([]IndexTask, error) {
	if limit < 1 {
		limit = 1
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, task_id, class_id, material_id, kind, attempts
		  FROM index_tasks
		 WHERE status='pending' AND run_at <= NOW(3)
		 ORDER BY run_at, id LIMIT ?
		 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, fmt.Errorf("扫描待办任务失败: %w", err)
	}
	type claimed struct {
		id       int64
		task     IndexTask
		attempts int
	}
	var picked []claimed
	for rows.Next() {
		var c claimed
		if err := rows.Scan(&c.id, &c.task.TaskID, &c.task.ClassID, &c.task.MaterialID,
			&c.task.Kind, &c.attempts); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("读取待办任务失败: %w", err)
		}
		picked = append(picked, c)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	_ = rows.Close()

	out := make([]IndexTask, 0, len(picked))
	for _, c := range picked {
		if _, err := tx.ExecContext(ctx, `
			UPDATE index_tasks SET status='running', attempts=attempts+1, started_at=NOW(3),
			       lease_until=DATE_ADD(NOW(3), INTERVAL ? MICROSECOND), error=''
			 WHERE id=?`, lease.Microseconds(), c.id); err != nil {
			return nil, fmt.Errorf("领取任务 %s 失败: %w", c.task.TaskID, err)
		}
		t := c.task
		t.ID, t.Status, t.Attempts = c.id, TaskRunning, c.attempts+1
		out = append(out, t)
	}
	return out, tx.Commit()
}

// CompleteTask 把任务与材料状态在同一个事务里推到成功终态。
func (s *Store) CompleteTask(ctx context.Context, taskID string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var materialID, classID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT material_id, class_id FROM index_tasks WHERE task_id=? FOR UPDATE`, taskID).
		Scan(&materialID, &classID); err != nil {
		return fmt.Errorf("读取任务关联失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE materials SET index_status='ready', index_error='', revision=revision+1
		 WHERE id=? AND class_id=?`, materialID, classID); err != nil {
		return fmt.Errorf("更新材料状态失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE index_tasks SET status='succeeded', finished_at=NOW(3), lease_until=NULL, error=''
		 WHERE task_id=?`, taskID); err != nil {
		return fmt.Errorf("更新任务状态失败: %w", err)
	}
	return tx.Commit()
}

// FailTask 处理一次执行失败：retry 为真时按退避时间退回 pending，否则写终态。
// terminal 取值只允许 failed / dead；材料同步置 failed 并保留原因。
func (s *Store) FailTask(ctx context.Context, taskID, errMsg string, retry bool,
	backoff time.Duration, terminal string) error {
	if terminal != TaskFailed && terminal != TaskDead {
		terminal = TaskDead
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var materialID, classID int64
	if err := tx.QueryRowContext(ctx,
		`SELECT material_id, class_id FROM index_tasks WHERE task_id=? FOR UPDATE`, taskID).
		Scan(&materialID, &classID); err != nil {
		return fmt.Errorf("读取任务关联失败: %w", err)
	}
	msg := clipError(errMsg)
	if retry {
		if _, err := tx.ExecContext(ctx, `
			UPDATE index_tasks SET status='pending', error=?, lease_until=NULL, finished_at=NULL,
			       run_at=DATE_ADD(NOW(3), INTERVAL ? MICROSECOND)
			 WHERE task_id=?`, msg, backoff.Microseconds(), taskID); err != nil {
			return fmt.Errorf("退回任务失败: %w", err)
		}
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE index_tasks SET status=?, finished_at=NOW(3), lease_until=NULL, error=?
		 WHERE task_id=?`, terminal, msg, taskID); err != nil {
		return fmt.Errorf("写入任务终态失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE materials SET index_status='failed', index_error=? WHERE id=? AND class_id=?`,
		msg, materialID, classID); err != nil {
		return fmt.Errorf("写入材料失败状态失败: %w", err)
	}
	return tx.Commit()
}
