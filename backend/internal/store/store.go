// Package store 封装全部数据库读写。
//
// 隔离纪律：凡按班级可见的数据，查询一律带 class_id 条件（spec R2），
// 不做「先查后判」——避免越权数据在内存里出现。
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound 表示目标不存在（或对当前班级不可见）。
var ErrNotFound = errors.New("未找到")

// Store 持有连接池。
type Store struct{ DB *sql.DB }

// New 构造 Store。
func New(db *sql.DB) *Store { return &Store{DB: db} }

// User 是账号及其归属。
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	ClassID      int64
	ClassName    string
	DisplayName  string
}

// GetUserByUsername 按用户名取账号。
func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	return s.scanUser(ctx, `SELECT u.id,u.username,u.password_hash,u.role,u.class_id,c.name,u.display_name
		FROM users u JOIN classes c ON c.id=u.class_id WHERE u.username=?`, username)
}

// GetUserByID 按 id 取账号。
func (s *Store) GetUserByID(ctx context.Context, id int64) (User, error) {
	return s.scanUser(ctx, `SELECT u.id,u.username,u.password_hash,u.role,u.class_id,c.name,u.display_name
		FROM users u JOIN classes c ON c.id=u.class_id WHERE u.id=?`, id)
}

func (s *Store) scanUser(ctx context.Context, q string, arg any) (User, error) {
	var u User
	err := s.DB.QueryRowContext(ctx, q, arg).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.ClassID, &u.ClassName, &u.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	if err != nil {
		return u, fmt.Errorf("查询用户失败: %w", err)
	}
	return u, nil
}

// --- refresh token ---

// SaveRefreshToken 保存 refresh token 摘要。
func (s *Store) SaveRefreshToken(ctx context.Context, userID int64, hash string, expiresAt time.Time) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES (?,?,?)`,
		userID, hash, expiresAt)
	if err != nil {
		return fmt.Errorf("保存 refresh token 失败: %w", err)
	}
	return nil
}

// ConsumeRefreshToken 校验 refresh token：有效则标记为已用并返回其 user_id。
//
// 「标记已用」即轮换：旧令牌随即失效（R1.6）。返回 ErrNotFound 表示
// 令牌不存在 / 已过期 / 已吊销 / 已被轮换（R1.7），调用方一律回 401。
func (s *Store) ConsumeRefreshToken(ctx context.Context, hash string) (int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var (
		id     int64
		userID int64
	)
	err = tx.QueryRowContext(ctx,
		`SELECT id, user_id FROM refresh_tokens
		 WHERE token_hash=? AND revoked=0 AND expires_at > NOW() FOR UPDATE`, hash).Scan(&id, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("查询 refresh token 失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE refresh_tokens SET revoked=1 WHERE id=?`, id); err != nil {
		return 0, fmt.Errorf("吊销 refresh token 失败: %w", err)
	}
	return userID, tx.Commit()
}

// RevokeRefreshToken 吊销单个 refresh token（登出，R1.5）。
func (s *Store) RevokeRefreshToken(ctx context.Context, hash string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE refresh_tokens SET revoked=1 WHERE token_hash=?`, hash)
	if err != nil {
		return fmt.Errorf("吊销 refresh token 失败: %w", err)
	}
	return nil
}

// RevokeAllRefreshTokens 吊销某账号的全部 refresh token。
func (s *Store) RevokeAllRefreshTokens(ctx context.Context, userID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE refresh_tokens SET revoked=1 WHERE user_id=?`, userID)
	return err
}

// --- 审计（R4）---

// Audit 写一条操作留痕。detail 不得包含明文口令等敏感信息（R4.2）。
func (s *Store) Audit(ctx context.Context, actorID *int64, actorName string, classID *int64, action, result, detail string) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO audit_logs (actor_id, actor_name, class_id, action, result, detail) VALUES (?,?,?,?,?,?)`,
		actorID, actorName, classID, action, result, detail)
	return err
}

// CountAudit 统计某动作的留痕条数（供验收核对）。
func (s *Store) CountAudit(ctx context.Context, action string) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs WHERE action=?`, action).Scan(&n)
	return n, err
}

// --- 材料与切片（R3）---

// Material 是一条教学材料。
type Material struct {
	ID          int64
	ClassID     int64
	OwnerID     int64
	Title       string
	Content     string
	IndexStatus string
	IndexError  string
	CreatedAt   time.Time
}

// Chunk 是一段可检索切片。
type Chunk struct {
	ID         int64
	MaterialID int64
	ClassID    int64
	Seq        int
	Text       string
	// MaterialTitle 由联表带出，便于直接构造引用条目。
	MaterialTitle string
}

// CreateMaterial 落库材料原文，初始状态 pending。
func (s *Store) CreateMaterial(ctx context.Context, classID, ownerID int64, title, content string) (int64, error) {
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO materials (class_id, owner_id, title, content) VALUES (?,?,?,?)`,
		classID, ownerID, title, content)
	if err != nil {
		return 0, fmt.Errorf("落库材料失败: %w", err)
	}
	return res.LastInsertId()
}

// SetMaterialStatus 更新索引状态（ready / failed）。
func (s *Store) SetMaterialStatus(ctx context.Context, id int64, status, indexErr string) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE materials SET index_status=?, index_error=? WHERE id=?`, status, indexErr, id)
	return err
}

// GetMaterial 按 id 取材料，并强制 class_id 匹配（跨班即视为不存在，R2.1）。
func (s *Store) GetMaterial(ctx context.Context, id, classID int64) (Material, error) {
	var m Material
	err := s.DB.QueryRowContext(ctx,
		`SELECT id,class_id,owner_id,title,content,index_status,index_error,created_at
		 FROM materials WHERE id=? AND class_id=?`, id, classID).Scan(
		&m.ID, &m.ClassID, &m.OwnerID, &m.Title, &m.Content, &m.IndexStatus, &m.IndexError, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	if err != nil {
		return m, fmt.Errorf("查询材料失败: %w", err)
	}
	return m, nil
}

// GetMaterialForReindex 按 id 取材料（不分班，供重索引内部使用，调用方须先校验归属）。
func (s *Store) GetMaterialForReindex(ctx context.Context, id, classID int64) (Material, error) {
	return s.GetMaterial(ctx, id, classID)
}

// ListMaterials 列出本班材料（按创建时间倒序）。
func (s *Store) ListMaterials(ctx context.Context, classID int64) ([]Material, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id,class_id,owner_id,title,index_status,index_error,created_at
		 FROM materials WHERE class_id=? ORDER BY id DESC`, classID)
	if err != nil {
		return nil, fmt.Errorf("列出材料失败: %w", err)
	}
	defer rows.Close()

	var out []Material
	for rows.Next() {
		var m Material
		if err := rows.Scan(&m.ID, &m.ClassID, &m.OwnerID, &m.Title, &m.IndexStatus, &m.IndexError, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ReplaceChunks 用新切片替换某材料的全部旧切片（重索引时调用）。
func (s *Store) ReplaceChunks(ctx context.Context, materialID, classID int64, texts []string) ([]Chunk, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE material_id=?`, materialID); err != nil {
		return nil, fmt.Errorf("清理旧切片失败: %w", err)
	}

	out := make([]Chunk, 0, len(texts))
	for i, t := range texts {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO chunks (material_id, class_id, seq, text) VALUES (?,?,?,?)`,
			materialID, classID, i, t)
		if err != nil {
			return nil, fmt.Errorf("写入切片失败: %w", err)
		}
		id, _ := res.LastInsertId()
		out = append(out, Chunk{ID: id, MaterialID: materialID, ClassID: classID, Seq: i, Text: t})
	}
	return out, tx.Commit()
}

// ListChunksByClass 取本班全部切片（关键词检索在内存打分，见 retrieval 包）。
func (s *Store) ListChunksByClass(ctx context.Context, classID int64) ([]Chunk, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT c.id,c.material_id,c.class_id,c.seq,c.text,m.title
		 FROM chunks c JOIN materials m ON m.id=c.material_id
		 WHERE c.class_id=? AND m.index_status='ready'`, classID)
	if err != nil {
		return nil, fmt.Errorf("列出切片失败: %w", err)
	}
	defer rows.Close()

	var out []Chunk
	for rows.Next() {
		var c Chunk
		if err := rows.Scan(&c.ID, &c.MaterialID, &c.ClassID, &c.Seq, &c.Text, &c.MaterialTitle); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- 会话（R6.3：仅在完整生成后落库）---

// SaveConversation 保存一次完整问答。
func (s *Store) SaveConversation(ctx context.Context, userID, classID int64, question, answer string, citationsJSON []byte) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO conversations (user_id, class_id, question, answer, citations) VALUES (?,?,?,?,?)`,
		userID, classID, question, answer, citationsJSON)
	return err
}
