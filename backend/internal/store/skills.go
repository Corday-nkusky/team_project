package store

// 本文件是「解题引导技能」的全部持久化读写。隔离纪律：所有查询一律带 class_id，
// 不做「先查后判」，避免越权数据在内存里出现（spec SK1）。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Skill 是一条按班配置的技能。
type Skill struct {
	ID          int64
	ClassID     int64
	OwnerID     int64
	Name        string
	Kind        string
	Enabled     bool
	Instruction string
	ToolRefs    []string
	Version     int
	CreatedAt   string
	UpdatedAt   string
}

// UpsertSkill 新建或按 (class_id, name) 更新技能；更新时 version 递增。
// 返回写入后的完整记录（含 id 与 version），供接口回显。
func (s *Store) UpsertSkill(ctx context.Context, in Skill) (Skill, error) {
	var refs []byte
	var err error
	if refs, err = json.Marshal(in.ToolRefs); err != nil {
		return Skill{}, fmt.Errorf("序列化工具引用失败: %w", err)
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO skills (class_id, owner_id, name, kind, enabled, instruction, tool_refs, version)
		VALUES (?,?,?,?,?,?,?,1)
		ON DUPLICATE KEY UPDATE
		  kind=VALUES(kind), enabled=VALUES(enabled), instruction=VALUES(instruction),
		  tool_refs=VALUES(tool_refs), owner_id=VALUES(owner_id), version=version+1`,
		in.ClassID, in.OwnerID, in.Name, in.Kind, in.Enabled, in.Instruction, string(refs))
	if err != nil {
		return Skill{}, fmt.Errorf("写入技能失败: %w", err)
	}
	return s.GetSkillByName(ctx, in.ClassID, in.Name)
}

// GetSkillByName 按班 + 名称取一条技能。
func (s *Store) GetSkillByName(ctx context.Context, classID int64, name string) (Skill, error) {
	return scanSkill(s.DB.QueryRowContext(ctx, `
		SELECT id,class_id,owner_id,name,kind,enabled,instruction,tool_refs,version,
		       DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%sZ'),DATE_FORMAT(updated_at,'%Y-%m-%dT%H:%i:%sZ')
		FROM skills WHERE class_id=? AND name=?`, classID, name))
}

// GetSkillByID 按 id + class_id 取一条；跨班返回 ErrNotFound（与不存在同形）。
func (s *Store) GetSkillByID(ctx context.Context, classID, id int64) (Skill, error) {
	return scanSkill(s.DB.QueryRowContext(ctx, `
		SELECT id,class_id,owner_id,name,kind,enabled,instruction,tool_refs,version,
		       DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%sZ'),DATE_FORMAT(updated_at,'%Y-%m-%dT%H:%i:%sZ')
		FROM skills WHERE class_id=? AND id=?`, classID, id))
}

// ListSkills 返回本班全部技能，按 id DESC（唯一 tiebreaker）。
func (s *Store) ListSkills(ctx context.Context, classID int64) ([]Skill, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id,class_id,owner_id,name,kind,enabled,instruction,tool_refs,version,
		       DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%sZ'),DATE_FORMAT(updated_at,'%Y-%m-%dT%H:%i:%sZ')
		FROM skills WHERE class_id=? ORDER BY id DESC`, classID)
	if err != nil {
		return nil, fmt.Errorf("列出技能失败: %w", err)
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		sk, err := scanSkillRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

// ListEnabledSkills 返回本班启用的技能（问答链路按班加载）。
func (s *Store) ListEnabledSkills(ctx context.Context, classID int64) ([]Skill, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id,class_id,owner_id,name,kind,enabled,instruction,tool_refs,version,
		       DATE_FORMAT(created_at,'%Y-%m-%dT%H:%i:%sZ'),DATE_FORMAT(updated_at,'%Y-%m-%dT%H:%i:%sZ')
		FROM skills WHERE class_id=? AND enabled=1 ORDER BY id ASC`, classID)
	if err != nil {
		return nil, fmt.Errorf("加载启用技能失败: %w", err)
	}
	defer rows.Close()
	out := []Skill{}
	for rows.Next() {
		sk, err := scanSkillRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sk)
	}
	return out, rows.Err()
}

// DeleteSkill 删除本班的一条技能；跨班返回 ErrNotFound。
func (s *Store) DeleteSkill(ctx context.Context, classID, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM skills WHERE class_id=? AND id=?`, classID, id)
	if err != nil {
		return fmt.Errorf("删除技能失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSkill(row rowScanner) (Skill, error) {
	var sk Skill
	var enabled int
	var refs []byte
	err := row.Scan(&sk.ID, &sk.ClassID, &sk.OwnerID, &sk.Name, &sk.Kind, &enabled,
		&sk.Instruction, &refs, &sk.Version, &sk.CreatedAt, &sk.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Skill{}, ErrNotFound
	}
	if err != nil {
		return Skill{}, fmt.Errorf("读取技能失败: %w", err)
	}
	sk.Enabled = enabled != 0
	if len(refs) > 0 {
		_ = json.Unmarshal(refs, &sk.ToolRefs)
	}
	if sk.ToolRefs == nil {
		sk.ToolRefs = []string{}
	}
	return sk, nil
}

func scanSkillRows(rows *sql.Rows) (Skill, error) {
	return scanSkill(rows)
}
