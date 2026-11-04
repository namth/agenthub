package database_test

// Test nghiệm thu TASK-030 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-auth.md §5 (migration 004), TC-66.

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/namth/agenthub/internal/database"
)

func TestMigration004(t *testing.T) { // TC-66
	path := filepath.Join(t.TempDir(), "v3.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Dựng DB đang ở phiên bản 003 (chạy 001–003 thật từ thư mục migrations).
	if _, err := raw.Exec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"001_init.sql", "002_multi_project.sql", "003_join_machine.sql"} {
		body, err := os.ReadFile(filepath.Join("migrations", f))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := raw.Exec(string(body)); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO schema_migrations (version) VALUES (1), (2), (3)`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', '1', 'namth', 'ADMIN')`,
		`INSERT INTO workers (id, name, os, token_hash) VALUES ('mac', 'mac', 'darwin', 'h')`,
		`INSERT INTO agents (id, worker_id, kind, status, lead_eligible) VALUES ('mac/claude', 'mac', 'CLAUDE', 'IDLE', 1)`,
		`INSERT INTO agents (id, worker_id, kind, status, lead_eligible) VALUES ('mac/antigravity', 'mac', 'ANTIGRAVITY', 'IDLE', 1)`,
	} {
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	raw.Close()

	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("Open (chạy 004): %v", err)
	}
	defer db.Close()
	var done int
	db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 4`).Scan(&done)
	if done != 1 {
		t.Fatal("chưa ghi schema_migrations phiên bản 4")
	}
	check := func(q string, want any) {
		t.Helper()
		var got any
		if err := db.QueryRow(q).Scan(&got); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		if b, ok := got.([]byte); ok {
			got = string(b)
		}
		if got != want {
			t.Fatalf("%s = %v, muốn %v", q, got, want)
		}
	}
	check(`SELECT lead_eligible FROM agents WHERE id='mac/antigravity'`, int64(0))
	check(`SELECT lead_eligible FROM agents WHERE id='mac/claude'`, int64(1))
	check(`SELECT auth_status FROM agents WHERE id='mac/claude'`, "UNKNOWN")
	check(`SELECT gh_auth FROM workers WHERE id='mac'`, "UNKNOWN")
	check(`SELECT COUNT(*) FROM pragma_table_info('agents') WHERE name IN ('auth_status','auth_checked_at','auth_detail')`, int64(3))
	check(`SELECT COUNT(*) FROM pragma_table_info('users') WHERE name='last_seen_at'`, int64(1))
	if _, err := db.Exec(`UPDATE agents SET auth_status='LOGGED' WHERE id='mac/claude'`); err == nil {
		t.Fatal("CHECK phải chặn auth_status ngoài OK/NEEDS_AUTH/UNKNOWN")
	}
	if _, err := db.Exec(`UPDATE workers SET gh_auth='YES' WHERE id='mac'`); err == nil {
		t.Fatal("CHECK phải chặn gh_auth sai giá trị")
	}
}
