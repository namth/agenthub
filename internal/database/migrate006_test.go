package database_test

// Test nghiệm thu TASK-049 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-accounts.md §5; TC-97. Dựng lại bảng agents không mất dữ liệu, bỏ UNIQUE(worker_id, kind).

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/namth/agenthub/internal/database"
)

func dbAtV5(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v5.db")
	raw, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	mustExec := func(q string) {
		t.Helper()
		if _, err := raw.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`)
	for _, f := range []string{"001_init.sql", "002_multi_project.sql", "003_join_machine.sql", "004_agent_auth_office.sql"} {
		body, err := os.ReadFile(filepath.Join("migrations", f))
		if err != nil {
			t.Fatal(err)
		}
		mustExec(string(body))
	}
	// 005 có marker foreign_keys=off: chạy như Migrate làm (tắt FK quanh nó).
	body, err := os.ReadFile(filepath.Join("migrations", "005_ops_remote.sql"))
	if err != nil {
		t.Fatal(err)
	}
	mustExec(`PRAGMA foreign_keys=OFF`)
	mustExec(string(body))
	mustExec(`PRAGMA foreign_keys=ON`)
	for _, q := range []string{
		`INSERT INTO schema_migrations (version) VALUES (1), (2), (3), (4), (5)`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', '1', 'namth', 'ADMIN')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'agenthub', 'namth/agenthub')`,
		`INSERT INTO workers (id, name, os, token_hash) VALUES ('m', 'm', 'darwin', 'h')`,
		`INSERT INTO workers (id, name, os, token_hash) VALUES ('x', 'x', 'darwin', 'h2')`,
		`INSERT INTO agents (id, worker_id, kind, auth_status, lead_priority) VALUES ('m/claude', 'm', 'CLAUDE', 'OK', 7)`,
		`INSERT INTO agents (id, worker_id, kind, auth_status, status, cooldown_until, cooldown_reason) VALUES ('m/antigravity', 'm', 'ANTIGRAVITY', 'UNKNOWN', 'COOLDOWN', '2026-11-08 00:00:00', 'EXHAUSTED')`,
		`INSERT INTO agents (id, worker_id, kind, auth_status, auth_detail) VALUES ('x/claude', 'x', 'CLAUDE', 'NEEDS_AUTH', 'Not logged in')`,
		`INSERT INTO tasks (id, project_id, title, description, status, created_by) VALUES ('t1', 'p1', 'A', 'B', 'IN_PROGRESS', 'u1')`,
		`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status) VALUES ('j1', 'p1', 't1', 'CODE', 'ANTIGRAVITY', 'DONE')`,
		`INSERT INTO assignments (id, job_id, agent_id, attempt, status) VALUES ('a1', 'j1', 'm/antigravity', 1, 'SUBMITTED')`,
		`INSERT INTO agent_projects (agent_id, project_id) VALUES ('m/claude', 'p1')`,
		`UPDATE lead_lease SET state='HELD', agent_id='m/claude', epoch=4`,
	} {
		mustExec(q)
	}
	return path
}

func TestMigration006AgentAccounts(t *testing.T) { // TC-97
	path := dbAtV5(t)
	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("Open (chạy 006): %v", err)
	}
	defer db.Close()

	if n := count(t, db, `SELECT COUNT(*) FROM schema_migrations WHERE version = 6`); n != 1 {
		t.Fatal("chưa ghi schema_migrations 6")
	}
	if n := count(t, db, `SELECT COUNT(*) FROM agents`); n != 3 {
		t.Fatalf("mất agent: còn %d/3", n)
	}
	type row struct {
		name   string
		everOK int
		prio   sql.NullInt64
		status string
	}
	get := func(id string) row {
		var r row
		if err := db.QueryRow(`SELECT name, ever_ok, lead_priority, status FROM agents WHERE id = ?`, id).Scan(&r.name, &r.everOK, &r.prio, &r.status); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		return r
	}
	if r := get("m/claude"); r.name != "claude" || r.everOK != 1 || r.prio.Int64 != 7 {
		t.Fatalf("m/claude = %+v (auth OK → ever_ok 1, giữ lead_priority)", r)
	}
	if r := get("m/antigravity"); r.name != "antigravity" || r.everOK != 1 || r.status != "COOLDOWN" {
		t.Fatalf("m/antigravity = %+v (có assignment → ever_ok 1, giữ COOLDOWN)", r)
	}
	if r := get("x/claude"); r.name != "claude" || r.everOK != 0 {
		t.Fatalf("x/claude = %+v (chưa từng OK → ever_ok 0)", r)
	}
	var removed sql.NullString
	db.QueryRow(`SELECT removed_at FROM agents WHERE id='m/claude'`).Scan(&removed)
	if removed.Valid {
		t.Fatal("removed_at phải NULL sau migration")
	}

	// Bỏ UNIQUE(worker_id, kind): thêm được tài khoản CLAUDE thứ hai trên cùng máy.
	if _, err := db.Exec(`INSERT INTO agents (id, worker_id, kind, name) VALUES ('m/claude-2', 'm', 'CLAUDE', 'claude-2')`); err != nil {
		t.Fatalf("không thêm được tài khoản thứ 2 cùng kind: %v", err)
	}
	// Chèn kiểu cũ (không name) vẫn chạy: trigger điền name từ id, ever_ok mặc định 1.
	if _, err := db.Exec(`INSERT INTO agents (id, worker_id, kind) VALUES ('x/antigravity', 'x', 'ANTIGRAVITY')`); err != nil {
		t.Fatalf("chèn agent kiểu cũ: %v", err)
	}
	var nm string
	var ev int
	db.QueryRow(`SELECT name, ever_ok FROM agents WHERE id='x/antigravity'`).Scan(&nm, &ev)
	if nm != "antigravity" || ev != 1 {
		t.Fatalf("agent kiểu cũ: name=%q ever_ok=%d, muốn antigravity/1", nm, ev)
	}
	// UNIQUE(worker_id, name) vẫn giữ.
	if _, err := db.Exec(`INSERT INTO agents (id, worker_id, kind, name) VALUES ('m/khac', 'm', 'CLAUDE', 'claude-2')`); err == nil {
		t.Fatal("trùng name trong cùng máy phải bị chặn")
	}

	// Bảng con tham chiếu agents không mất, FK sạch.
	if n := count(t, db, `SELECT COUNT(*) FROM assignments WHERE agent_id='m/antigravity'`); n != 1 {
		t.Fatal("mất assignment")
	}
	if n := count(t, db, `SELECT COUNT(*) FROM agent_projects WHERE agent_id='m/claude'`); n != 1 {
		t.Fatal("mất agent_projects")
	}
	if n := count(t, db, `SELECT COUNT(*) FROM lead_lease WHERE agent_id='m/claude' AND epoch=4`); n != 1 {
		t.Fatal("mất lead_lease")
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check còn lỗi sau 006")
	}
	var fk int
	db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if fk != 1 {
		t.Fatal("foreign_keys phải bật lại sau migration")
	}
}
