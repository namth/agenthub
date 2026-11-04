package database_test

// Test do trưởng phòng viết: cơ chế migration (TASK-019, TC-36, TC-37).

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/namth/agenthub/internal/database"
)

func TestMigrateLegacyDBKeepsData(t *testing.T) { // TC-36
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// DB "cũ": bảng tạo trước khi có schema_migrations, có sẵn dữ liệu.
	for _, q := range []string{
		`CREATE TABLE users (id TEXT PRIMARY KEY, github_id TEXT UNIQUE NOT NULL, username TEXT NOT NULL, avatar_url TEXT, role TEXT NOT NULL CHECK(role IN ('ADMIN','PARTNER')), created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE projects (id TEXT PRIMARY KEY, name TEXT NOT NULL, github_repo TEXT NOT NULL UNIQUE, default_branch TEXT NOT NULL DEFAULT 'main', status TEXT NOT NULL DEFAULT 'ACTIVE', created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', '1', 'namth', 'ADMIN')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'agenthub', 'namth/agenthub')`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()

	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("Open DB cũ: %v", err)
	}
	defer db.Close()
	var name string
	var weight int
	if err := db.QueryRow(`SELECT name, weight FROM projects WHERE id='p1'`).Scan(&name, &weight); err != nil || name != "agenthub" || weight != 1 {
		t.Fatalf("dữ liệu cũ + cột mới: %q %d %v", name, weight, err)
	}
	var versions int
	db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&versions)
	if versions < 3 {
		t.Fatalf("schema_migrations = %d, muốn ≥ 3", versions)
	}
}

func TestMigrateIdempotentVersions(t *testing.T) { // TC-37
	db, err := database.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for i := 0; i < 3; i++ {
		if err := database.Migrate(db); err != nil {
			t.Fatalf("lần %d: %v", i, err)
		}
	}
	var n, max int
	db.QueryRow(`SELECT COUNT(*), MAX(version) FROM schema_migrations`).Scan(&n, &max)
	if n != max || n < 3 {
		t.Fatalf("mỗi phiên bản đúng một dòng: count=%d max=%d", n, max)
	}
	for _, col := range []string{"owner_user_id", "version", "paused"} {
		var c int
		db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('workers') WHERE name=?`, col).Scan(&c)
		if c != 1 {
			t.Fatalf("workers thiếu cột %s", col)
		}
	}
}
