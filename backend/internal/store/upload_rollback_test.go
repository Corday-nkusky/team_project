//go:build mysqlintegration

package store_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Corday-nkusky/team_project/backend/internal/db"
	"github.com/Corday-nkusky/team_project/backend/internal/store"
	"github.com/go-sql-driver/mysql"
)

func TestUploadRollbackAndRetry(t *testing.T) {
	addr := os.Getenv("MYSQL_TEST_ADDR")
	if addr == "" {
		t.Fatal("set MYSQL_TEST_ADDR to a disposable MySQL 8.4 instance")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Net, cfg.Addr = "root", "tcp", addr
	cfg.Passwd = os.Getenv("MYSQL_TEST_PASSWORD")
	admin, err := db.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()

	schema := "upload_rollback_" + strings.ToLower(rand.Text())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE "+schema); err != nil {
			t.Errorf("clean up %s: %v", schema, err)
		}
	}()

	cfg.DBName = schema
	conn, err := db.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.Migrate(ctx, conn); err != nil {
		t.Fatal(err)
	}
	observer, err := conn.Conn(ctx) // Pin a separate session across both upload attempts.
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	exec := func(query string, args ...any) sql.Result {
		t.Helper()
		result, err := observer.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	classID, err := exec("INSERT INTO classes (name) VALUES (?)", schema).LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	ownerID, err := exec("INSERT INTO users (username,password_hash,role,class_id,display_name) VALUES (?, '', 'teacher', ?, 'Teacher')", schema, classID).LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	title, content := schema, "test material"
	upload := func() (int64, string, error) {
		return store.New(conn).CreateMaterialWithTask(ctx, classID, ownerID, title, content, schema)
	}
	assertCounts := func(want int) {
		t.Helper()
		var materials, tasks int
		err := observer.QueryRowContext(ctx, `SELECT
			(SELECT COUNT(*) FROM materials WHERE class_id=? AND title=?),
			(SELECT COUNT(*) FROM index_tasks WHERE class_id=?)`, classID, title, classID).Scan(&materials, &tasks)
		if err != nil {
			t.Fatal(err)
		}
		if materials != want || tasks != want {
			t.Fatalf("materials=%d tasks=%d, want %d each", materials, tasks, want)
		}
	}

	exec("ALTER TABLE index_tasks ADD CONSTRAINT reject_test_upload CHECK (kind <> 'upload')")
	_, _, err = upload()
	var violation *mysql.MySQLError
	if !errors.As(err, &violation) || violation.Number != 3819 || !strings.Contains(violation.Message, "reject_test_upload") {
		t.Fatalf("expected reject_test_upload check violation, got %v", err)
	}
	assertCounts(0)

	exec("ALTER TABLE index_tasks DROP CHECK reject_test_upload")
	materialID, taskID, err := upload()
	if err != nil {
		t.Fatal(err)
	}
	assertCounts(1)
	var status string
	err = observer.QueryRowContext(ctx, `SELECT t.status FROM index_tasks t
		JOIN materials m ON m.id=t.material_id AND m.class_id=t.class_id
		WHERE t.class_id=? AND t.task_id=? AND m.id=? AND m.title=? AND t.kind='upload'`,
		classID, taskID, materialID, title).Scan(&status)
	if err != nil || status != store.TaskPending {
		t.Fatalf("retry material=%d task=%s: status=%q err=%v, want pending", materialID, taskID, status, err)
	}
}
