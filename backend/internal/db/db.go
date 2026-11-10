// Package db 负责连接 MySQL、建表（启动自动迁移）与幂等种子数据。
//
// 表结构对应 tasks 2.1（用户/班级/刷新令牌）与 3.1（材料/切片），另含审计与会话表。
// 所有建表语句均使用 IF NOT EXISTS，重复启动安全。
package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
		content      LONGTEXT     NOT NULL,
		content_hash CHAR(64)     NULL DEFAULT NULL,
		revision     INT          NOT NULL DEFAULT 0,
		index_status ENUM('pending','ready','failed') NOT NULL DEFAULT 'pending',
		index_error  VARCHAR(500) NOT NULL DEFAULT '',
		created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		KEY idx_mat_class_status (class_id, index_status),
		UNIQUE KEY uk_material_hash (class_id, content_hash),
		CONSTRAINT fk_mat_class FOREIGN KEY (class_id) REFERENCES classes (id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	// class_id 冗余在切片上，使检索的班级过滤只走一张表（R2.2 隔离）。
	// uk_chunk_seq 是重复投递的兜底：同一材料的同一序号只能有一行切片。
	`CREATE TABLE IF NOT EXISTS chunks (
		id           BIGINT      NOT NULL AUTO_INCREMENT,
		material_id  BIGINT      NOT NULL,
		class_id     BIGINT      NOT NULL,
		seq          INT         NOT NULL,
		text         TEXT        NOT NULL,
		heading_path VARCHAR(500) NOT NULL DEFAULT '',
		created_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (id),
		UNIQUE KEY uk_chunk_seq (material_id, seq),
		KEY idx_chunk_class (class_id),
		KEY idx_chunk_material (material_id),
		CONSTRAINT fk_chunk_material FOREIGN KEY (material_id) REFERENCES materials (id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

	// index_tasks 既是任务记录也是持久化队列：业务写入与任务记录同事务，
	// worker 用 FOR UPDATE SKIP LOCKED 抢占，lease_until 负责超时回收。
	`CREATE TABLE IF NOT EXISTS index_tasks (
		id               BIGINT       NOT NULL AUTO_INCREMENT,
		task_id          CHAR(32)     NOT NULL,
		class_id         BIGINT       NOT NULL,
		material_id      BIGINT       NOT NULL,
		kind             ENUM('upload','reindex') NOT NULL,
		status           ENUM('pending','running','succeeded','failed','dead') NOT NULL DEFAULT 'pending',
		attempts         INT          NOT NULL DEFAULT 0,
		idempotency_key  VARCHAR(191) NOT NULL,
		error            VARCHAR(500) NOT NULL DEFAULT '',
		request_id       VARCHAR(128) NOT NULL DEFAULT '',
		run_at           DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
		lease_until      DATETIME(3)  NULL,
		started_at       DATETIME(3)  NULL,
		finished_at      DATETIME(3)  NULL,
		created_at       DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
		PRIMARY KEY (id),
		UNIQUE KEY uk_task_id (task_id),
		UNIQUE KEY uk_idem (idempotency_key),
		KEY idx_task_claim (status, run_at),
		KEY idx_task_class (class_id, task_id),
		CONSTRAINT fk_task_material FOREIGN KEY (material_id) REFERENCES materials (id) ON DELETE CASCADE
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

// Migrate 执行全部建表语句，并做一次幂等的列扩容。
func Migrate(ctx context.Context, conn *sql.DB) error {
	for i, ddl := range schema {
		if _, err := conn.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("建表失败(第 %d 条): %w", i+1, err)
		}
	}
	if err := migrateMaterialContent(ctx, conn); err != nil {
		return err
	}
	if err := migrateChunkHeadingPath(ctx, conn); err != nil {
		return err
	}
	if err := migrateMaterialAsyncColumns(ctx, conn); err != nil {
		return err
	}
	return migrateChunkSeqUnique(ctx, conn)
}

// columnExists / indexExists 供幂等迁移使用：information_schema 判定，重复执行安全。
func columnExists(ctx context.Context, conn *sql.DB, table, column string) (bool, error) {
	var n int
	err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, column).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("检查 %s.%s 列失败: %w", table, column, err)
	}
	return n > 0, nil
}

func indexExists(ctx context.Context, conn *sql.DB, table, index string) (bool, error) {
	var n int
	err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?`, table, index).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("检查 %s.%s 索引失败: %w", table, index, err)
	}
	return n > 0, nil
}

// migrateMaterialAsyncColumns 为旧库补上入口幂等与版本列（先加列，再加唯一约束）。
// content_hash 允许 NULL：MySQL 唯一索引允许多个 NULL，旧材料因此不受影响。
func migrateMaterialAsyncColumns(ctx context.Context, conn *sql.DB) error {
	hasHash, err := columnExists(ctx, conn, "materials", "content_hash")
	if err != nil {
		return err
	}
	if !hasHash {
		if _, err := conn.ExecContext(ctx,
			`ALTER TABLE materials ADD COLUMN content_hash CHAR(64) NULL DEFAULT NULL`); err != nil {
			return fmt.Errorf("新增 materials.content_hash 列失败: %w", err)
		}
	}
	hasRevision, err := columnExists(ctx, conn, "materials", "revision")
	if err != nil {
		return err
	}
	if !hasRevision {
		if _, err := conn.ExecContext(ctx,
			`ALTER TABLE materials ADD COLUMN revision INT NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("新增 materials.revision 列失败: %w", err)
		}
	}
	hasIndex, err := indexExists(ctx, conn, "materials", "uk_material_hash")
	if err != nil {
		return err
	}
	if !hasIndex {
		if _, err := conn.ExecContext(ctx,
			`ALTER TABLE materials ADD UNIQUE KEY uk_material_hash (class_id, content_hash)`); err != nil {
			return fmt.Errorf("新增 materials 幂等唯一约束失败: %w", err)
		}
	}
	return nil
}

// migrateChunkSeqUnique 在加唯一约束前清掉历史重复切片（保留最小 id）。
// 这是「先加约束再切读写」的向前兼容步骤：旧行不丢内容，只丢重复行。
func migrateChunkSeqUnique(ctx context.Context, conn *sql.DB) error {
	has, err := indexExists(ctx, conn, "chunks", "uk_chunk_seq")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := conn.ExecContext(ctx, `
		DELETE c FROM chunks c
		JOIN (SELECT material_id, seq, MIN(id) AS keep_id
		        FROM chunks GROUP BY material_id, seq HAVING COUNT(*) > 1) d
		  ON c.material_id = d.material_id AND c.seq = d.seq
		WHERE c.id > d.keep_id`); err != nil {
		return fmt.Errorf("清理重复切片失败: %w", err)
	}
	if _, err := conn.ExecContext(ctx,
		`ALTER TABLE chunks ADD UNIQUE KEY uk_chunk_seq (material_id, seq)`); err != nil {
		return fmt.Errorf("新增 chunks 唯一约束失败: %w", err)
	}
	return nil
}

// migrateChunkHeadingPath 为旧库的 chunks 表补上结构路径列。
// 与 migrateMaterialContent 同样采用 information_schema 查列的幂等模式（向前兼容，先加列再切读写）。
func migrateChunkHeadingPath(ctx context.Context, conn *sql.DB) error {
	var n int
	err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'chunks' AND COLUMN_NAME = 'heading_path'`).Scan(&n)
	if err != nil {
		return fmt.Errorf("检查 chunks.heading_path 列失败: %w", err)
	}
	if n > 0 {
		return nil
	}
	if _, err := conn.ExecContext(ctx,
		`ALTER TABLE chunks ADD COLUMN heading_path VARCHAR(500) NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("新增 chunks.heading_path 列失败: %w", err)
	}
	return nil
}

// migrateMaterialContent 把早期 MEDIUMTEXT（16 MiB）的原文列扩为 LONGTEXT，
// 与 32 MiB 上传上限对齐（spec R3.5）。只增容量、不丢数据，重复执行安全。
func migrateMaterialContent(ctx context.Context, conn *sql.DB) error {
	var dataType string
	err := conn.QueryRowContext(ctx, `
		SELECT DATA_TYPE FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'materials' AND COLUMN_NAME = 'content'`).
		Scan(&dataType)
	if err != nil {
		return fmt.Errorf("检查 materials.content 列类型失败: %w", err)
	}
	if strings.EqualFold(dataType, "longtext") {
		return nil
	}
	if _, err := conn.ExecContext(ctx, `ALTER TABLE materials MODIFY COLUMN content LONGTEXT NOT NULL`); err != nil {
		return fmt.Errorf("扩容 materials.content 失败: %w", err)
	}
	return nil
}
