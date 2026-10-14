package db

import (
	"context"
	"database/sql"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// seedUser 描述一个种子账号。
type seedUser struct {
	username    string
	password    string
	role        string
	className   string
	displayName string
}

// seedClasses / seedUsers 是本地开发与验收用的固定数据（可用环境变量关闭）。
var (
	seedClasses = []string{"软件工程1班", "软件工程2班"}
	seedUsers   = []seedUser{
		{"teacher1", "teacher123", "teacher", "软件工程1班", "王老师"},
		{"student1", "student123", "student", "软件工程1班", "小明"},
		{"teacher2", "teacher123", "teacher", "软件工程2班", "李老师"},
		{"student2", "student123", "student", "软件工程2班", "小红"},
	}
)

// Seed 写入幂等的演示数据：两个班级 + 每班一名教师与一名学生。
//
// 幂等性由唯一键保证（班级名、用户名），重复执行不会产生重复行，
// 也不会覆盖已改动的口令（只在账号不存在时创建）。
func Seed(ctx context.Context, conn *sql.DB) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开启种子事务失败: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	classIDs := map[string]int64{}
	for _, name := range seedClasses {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO classes (name) VALUES (?) ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`,
			name); err != nil {
			return fmt.Errorf("种子：写入班级 %q 失败: %w", name, err)
		}
		var id int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM classes WHERE name = ?`, name).Scan(&id); err != nil {
			return fmt.Errorf("种子：读取班级 %q 失败: %w", name, err)
		}
		classIDs[name] = id
	}

	for _, u := range seedUsers {
		var existing int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE username = ?`, u.username).Scan(&existing)
		switch {
		case err == sql.ErrNoRows:
			hash, herr := bcrypt.GenerateFromPassword([]byte(u.password), bcrypt.DefaultCost)
			if herr != nil {
				return fmt.Errorf("种子：口令哈希失败: %w", herr)
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO users (username, password_hash, role, class_id, display_name) VALUES (?,?,?,?,?)`,
				u.username, string(hash), u.role, classIDs[u.className], u.displayName); err != nil {
				return fmt.Errorf("种子：写入用户 %q 失败: %w", u.username, err)
			}
		case err != nil:
			return fmt.Errorf("种子：查询用户 %q 失败: %w", u.username, err)
		}
	}

	return tx.Commit()
}
