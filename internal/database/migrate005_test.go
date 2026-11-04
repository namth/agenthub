package database_test

// Test nghiệm thu TASK-036 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: FEAT-07 §5, FEAT-09 §5, FEAT-10 §5; TC-74. Trọng tâm: dựng lại bảng tasks không mất dữ liệu.

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/namth/agenthub/internal/database"
)

// dbAtV4 dựng DB đang ở phiên bản 004 (chạy 001–004 thật) với dữ liệu phủ mọi bảng con của tasks.
func dbAtV4(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v4.db")
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
	for _, q := range []string{
		`INSERT INTO schema_migrations (version) VALUES (1), (2), (3), (4)`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', '1', 'namth', 'ADMIN')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'agenthub', 'namth/agenthub')`,
		`INSERT INTO workers (id, name, os, token_hash) VALUES ('w1', 'w1', 'darwin', 'h')`,
		`INSERT INTO agents (id, worker_id, kind) VALUES ('w1/claude', 'w1', 'CLAUDE')`,
		`INSERT INTO tasks (id, project_id, title, description, status, created_by, branch_name, pr_url, github_issue_id, assigned_worker_id)
		   VALUES ('t1', 'p1', 'Cha', 'Mô tả cha', 'READY', 'u1', 'feat/t1', 'https://x/pr/1', 101, 'w1')`,
		`INSERT INTO tasks (id, project_id, parent_id, title, description, status, created_by) VALUES ('t2', 'p1', 't1', 'Con', 'Mô tả con', 'IN_PROGRESS', 'u1')`,
		`INSERT INTO tasks (id, project_id, title, description, status, created_by) VALUES ('t3', 'p1', 'Xong', 'Mô tả', 'DONE', 'u1')`,
		`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status) VALUES ('j1', 'p1', 't2', 'CODE', 'ANTIGRAVITY', 'QUEUED')`,
		`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status) VALUES ('j2', 'p1', 't1', 'SPEC', 'CLAUDE', 'DONE')`,
		`INSERT INTO assignments (id, job_id, agent_id, attempt, status) VALUES ('a1', 'j2', 'w1/claude', 1, 'SUBMITTED')`,
		`INSERT INTO task_logs (task_id, worker_id, assignment_id, log_chunk) VALUES ('t1', 'w1', 'a1', 'dòng log')`,
		`INSERT INTO lead_journal (epoch, agent_id, task_id, kind, entry) VALUES (1, 'w1/claude', 't1', 'NOTE', 'ghi chú')`,
	} {
		mustExec(q)
	}
	return path
}

func count(t *testing.T, db *sql.DB, q string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}

func TestMigration005KeepsData(t *testing.T) { // TC-74
	path := dbAtV4(t)
	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("Open (chạy 005): %v", err)
	}
	defer db.Close()

	if n := count(t, db, `SELECT COUNT(*) FROM schema_migrations WHERE version = 5`); n != 1 {
		t.Fatal("chưa ghi schema_migrations phiên bản 5")
	}
	for q, want := range map[string]int{
		`SELECT COUNT(*) FROM tasks`:                                  3,
		`SELECT COUNT(*) FROM jobs`:                                   2,
		`SELECT COUNT(*) FROM assignments`:                            1,
		`SELECT COUNT(*) FROM task_logs`:                              1,
		`SELECT COUNT(*) FROM lead_journal`:                           1,
		`SELECT COUNT(*) FROM tasks WHERE id='t2' AND parent_id='t1'`: 1,
		`SELECT COUNT(*) FROM tasks WHERE id='t1' AND status='READY' AND branch_name='feat/t1' AND github_issue_id=101 AND assigned_worker_id='w1'`: 1,
	} {
		if n := count(t, db, q); n != want {
			t.Fatalf("%s = %d, muốn %d (mất dữ liệu khi dựng lại tasks)", q, n, want)
		}
	}
	// CHECK mới nhận WAITING_USER, vẫn chặn giá trị lạ.
	if _, err := db.Exec(`UPDATE tasks SET status='WAITING_USER' WHERE id='t1'`); err != nil {
		t.Fatalf("WAITING_USER phải hợp lệ: %v", err)
	}
	if _, err := db.Exec(`UPDATE tasks SET status='LA_GI' WHERE id='t1'`); err == nil {
		t.Fatal("CHECK phải chặn trạng thái lạ")
	}
	// Khoá ngoại vẫn hoạt động sau khi dựng lại.
	if _, err := db.Exec(`INSERT INTO tasks (id, project_id, title, description, created_by) VALUES ('tx', 'khong-co', 'x', 'y', 'u1')`); err == nil {
		t.Fatal("khoá ngoại project_id phải còn hiệu lực")
	}
	if _, err := db.Exec(`INSERT INTO jobs (id, project_id, task_id, kind, required_agent) VALUES ('jx', 'p1', 'khong-co', 'CODE', 'ANTIGRAVITY')`); err == nil {
		t.Fatal("jobs.task_id → tasks phải còn hiệu lực")
	}
	// ON DELETE CASCADE vẫn đúng: xoá cha → xoá con + job + log của cha.
	if _, err := db.Exec(`DELETE FROM tasks WHERE id='t1'`); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM tasks WHERE id IN ('t1','t2')`); n != 0 {
		t.Fatalf("xoá cha phải xoá con (cascade), còn %d", n)
	}
	if n := count(t, db, `SELECT COUNT(*) FROM task_logs`); n != 0 {
		t.Fatal("task_logs của task bị xoá phải bị xoá theo")
	}
	for _, idx := range []string{"idx_tasks_status", "idx_tasks_parent", "idx_tasks_assigned_worker"} {
		if n := count(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='`+idx+`'`); n != 1 {
			t.Fatalf("thiếu index %s sau khi dựng lại tasks", idx)
		}
	}
	if n := count(t, db, `SELECT COUNT(*) FROM sqlite_master WHERE name='tasks_new'`); n != 0 {
		t.Fatal("còn sót bảng tasks_new")
	}
	var fk int
	db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if fk != 1 {
		t.Fatal("sau migration, kết nối của DB phải bật lại foreign_keys")
	}
}

func TestMigration005NewSchema(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cols := func(table string) map[string]bool {
		t.Helper()
		rows, err := db.Query(`SELECT name FROM pragma_table_info('` + table + `')`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		m := map[string]bool{}
		for rows.Next() {
			var n string
			rows.Scan(&n)
			m[n] = true
		}
		return m
	}
	want := map[string][]string{
		"task_clarifications": {"id", "task_id", "round", "questions", "answers", "status", "answered_by", "answered_via", "asked_by_agent", "reminded_at", "answered_at", "created_at"},
		"projects":            {"allowed_commands", "needs_ci_for_integration", "is_sandbox"},
		"smoke_runs":          {"id", "project_id", "task_id", "status", "steps", "started_by", "started_at", "ended_at"},
		"personal_tokens":     {"id", "user_id", "name", "token_hash", "last4", "expires_at", "last_used_at", "revoked_at", "created_at"},
		"lead_proposals":      {"id", "task_id", "kind", "reason", "proposed_by", "epoch", "status", "decided_by", "created_at", "decided_at"},
	}
	for table, cs := range want {
		got := cols(table)
		for _, c := range cs {
			if !got[c] {
				t.Errorf("%s thiếu cột %s", table, c)
			}
		}
	}
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', '1', 'namth', 'ADMIN')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'a', 'namth/a')`,
		`INSERT INTO tasks (id, project_id, title, description, created_by) VALUES ('t1', 'p1', 'x', 'y', 'u1')`,
		`INSERT INTO task_clarifications (id, task_id, round, questions) VALUES ('c1', 't1', 1, '[]')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	bad := []string{
		`INSERT INTO task_clarifications (id, task_id, round, questions) VALUES ('c2', 't1', 1, '[]')`,                      // trùng (task, round)
		`INSERT INTO task_clarifications (id, task_id, round, questions, status) VALUES ('c3', 't1', 2, '[]', 'LA')`,        // status lạ
		`INSERT INTO task_clarifications (id, task_id, round, questions, answered_via) VALUES ('c4', 't1', 3, '[]', 'FAX')`, // kênh lạ
		`INSERT INTO smoke_runs (id, project_id, status) VALUES ('s1', 'p1', 'LA')`,
		`INSERT INTO lead_proposals (id, task_id, kind, reason, proposed_by, epoch) VALUES ('l1', 't1', 'DELETE', 'x', 'a', 1)`,
		`INSERT INTO personal_tokens (id, user_id, name, token_hash, last4) VALUES ('k1', 'u1', 'n', 'h', '1234')`, // thiếu expires_at
	}
	for _, q := range bad {
		if _, err := db.Exec(q); err == nil {
			t.Errorf("phải bị chặn: %s", q)
		}
	}
	if n := count(t, db, `SELECT is_sandbox + needs_ci_for_integration FROM projects WHERE id='p1'`); n != 0 {
		t.Fatal("is_sandbox / needs_ci_for_integration mặc định 0")
	}
	// Chạy lại Migrate an toàn.
	if err := database.Migrate(db); err != nil {
		t.Fatalf("Migrate lần 2: %v", err)
	}
}
