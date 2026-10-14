// Package db 负责连接 MySQL、建表（启动自动迁移）与幂等种子数据。
//
// 表结构对应 tasks 2.1（用户/班级/刷新令牌）与 3.1（材料/切片），另含审计与会话表。
// 所有建表语句均使用 IF NOT EXISTS，重复启动安全。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Open 建立连接池。注意 sql.Open 不建立实际连接——连通性由 deps.Checker 判定；
// 这样在「依赖暂不可用」的开发模式下仍可构造好 Store，等依赖恢复后自动可用。
func Open(dsn string) (*sql.DB, error) {
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 MySQL 连接池失败: %w", err)
	}
	conn.SetConnMaxLifetime(3 * time.Minute)
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(5)
	return conn, nil
}

// schema 是全部建表语句（幂等）。
var schema = []string{
	`CREATE TABLE IF NOT EXISTS classes (
		id         BIGINT       NOT NULL AUTO_INCREMENT,
		name       VARCHAR(64)  NOT NULL,
		created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		UNIQUE KEY uk_classes_name (name)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS users (
		id            BIGINT       NOT NULL AUTO_INCREMENT,
		username      VARCHAR(64)  NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		role          ENUM('teacher','student') NOT NULL,
		class_id      BIGINT       NOT NULL,
		display_name  VARCHAR(64)  NOT NULL,
		created_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		UNIQUE KEY uk_users_username (username),
		KEY idx_users_class (class_id),
		CONSTRAINT fk_users_class FOREIGN KEY (class_id) REFERENCES classes (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	// refresh token 只存 SHA-256 摘要，绝不留明文（design D8）。
	`CREATE TABLE IF NOT EXISTS refresh_tokens (
		id         BIGINT     NOT NULL AUTO_INCREMENT,
		user_id    BIGINT     NOT NULL,
		token_hash CHAR(64)   NOT NULL,
		expires_at DATETIME   NOT NULL,
		revoked    TINYINT(1) NOT NULL DEFAULT 0,
		created_at DATETIME   NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		UNIQUE KEY uk_rt_hash (token_hash),
		KEY idx_rt_user (user_id),
		CONSTRAINT fk_rt_user FOREIGN KEY (user_id) REFERENCES users (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS materials (
		id           BIGINT       NOT NULL AUTO_INCREMENT,
		class_id     BIGINT       NOT NULL,
		owner_id     BIGINT       NOT NULL,
		title        VARCHAR(255) NOT NULL,
		content      MEDIUMTEXT   NOT NULL,
		index_status ENUM('pending','ready','failed') NOT NULL DEFAULT 'pending',
		index_error  VARCHAR(500) NOT NULL DEFAULT '',
		created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_mat_class_status (class_id, index_status),
		CONSTRAINT fk_mat_class FOREIGN KEY (class_id) REFERENCES classes (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	// class_id 冗余在切片上，使检索的班级过滤只走一张表（R2.2 隔离）。
	`CREATE TABLE IF NOT EXISTS chunks (
		id          BIGINT   NOT NULL AUTO_INCREMENT,
		material_id BIGINT   NOT NULL,
		class_id    BIGINT   NOT NULL,
		seq         INT      NOT NULL,
		text        TEXT     NOT NULL,
		created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_chunk_class (class_id),
		KEY idx_chunk_material (material_id),
		CONSTRAINT fk_chunk_material FOREIGN KEY (material_id) REFERENCES materials (id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	`CREATE TABLE IF NOT EXISTS audit_logs (
		id         BIGINT       NOT NULL AUTO_INCREMENT,
		actor_id   BIGINT       NULL,
		actor_name VARCHAR(64)  NOT NULL DEFAULT '',
		class_id   BIGINT       NULL,
		action     VARCHAR(32)  NOT NULL,
		result     VARCHAR(16)  NOT NULL,
		detail     VARCHAR(500) NOT NULL DEFAULT '',
		created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_audit_created (created_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	// 会话记录仅在回答完整生成后落库；客户端中断则不留记录（R6.3）。
	`CREATE TABLE IF NOT EXISTS conversations (
		id         BIGINT     NOT NULL AUTO_INCREMENT,
		user_id    BIGINT     NOT NULL,
		class_id   BIGINT     NOT NULL,
		question   TEXT       NOT NULL,
		answer     MEDIUMTEXT NOT NULL,
		citations  JSON       NULL,
		created_at DATETIME   NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_conv_user (user_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
}

// Migrate 执行全部建表语句。
func Migrate(ctx context.Context, conn *sql.DB) error {
	for i, ddl := range schema {
		if _, err := conn.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("建表失败(第 %d 条): %w", i+1, err)
		}
	}
	return nil
}
