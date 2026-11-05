package database_test

// Test nghiệm thu TASK-001 — do trưởng phòng viết, file bị khoá (tests.lock).

import (
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/namth/agenthub/internal/database"
)

func openTemp(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "dir", "agenthub.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func names(t *testing.T, db *sql.DB, q string, args ...any) []string {
	t.Helper()
	rows, err := db.Query(q, args...)
	if err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func columns(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	cols := map[string]bool{}
	for _, c := range names(t, db, "SELECT name FROM pragma_table_info(?)", table) {
		cols[c] = true
	}
	if len(cols) == 0 {
		t.Fatalf("bảng %s không tồn tại", table)
	}
	return cols
}

// fixtures tạo một bộ dữ liệu hợp lệ tối thiểu để test ràng buộc.
func fixtures(t *testing.T, db *sql.DB) {
	t.Helper()
	mustExec(t, db, `INSERT INTO users (id, github_id, username, role) VALUES ('u1', 'gh1', 'namth', 'ADMIN')`)
	mustExec(t, db, `INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'AgentHub', 'namth/agenthub')`)
	mustExec(t, db, `INSERT INTO workers (id, name, os, token_hash) VALUES ('mac-mini', 'mac-mini', 'darwin', 'h')`)
	mustExec(t, db, `INSERT INTO agents (id, worker_id, kind) VALUES ('mac-mini/claude', 'mac-mini', 'CLAUDE')`)
	mustExec(t, db, `INSERT INTO tasks (id, project_id, title, description, created_by) VALUES ('t1', 'p1', 'Tiêu đề', 'Mô tả', 'u1')`)
	mustExec(t, db, `INSERT INTO jobs (id, project_id, task_id, kind, required_agent) VALUES ('j1', 'p1', 't1', 'CODE', 'ANTIGRAVITY')`)
	mustExec(t, db, `INSERT INTO assignments (id, job_id, agent_id, attempt) VALUES ('a1', 'j1', 'mac-mini/claude', 1)`)
}

func TestOpenCreatesFileAndAllTables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "agenthub.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("không tạo file DB ở %s: %v", path, err)
	}
	got := names(t, db, `SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`)
	// Danh sách cập nhật theo migration 005 (TASK-036): thêm lead_proposals, personal_tokens, smoke_runs, task_clarifications.
	want := []string{"agent_projects", "agents", "assignments", "jobs", "join_codes", "lead_journal", "lead_lease", "lead_proposals", "personal_tokens", "project_members", "projects", "schema_migrations", "settings", "smoke_runs", "task_clarifications", "task_logs", "tasks", "users", "workers"}
	if len(got) != len(want) {
		t.Fatalf("bảng = %v, muốn đúng %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("bảng = %v, muốn đúng %v", got, want)
		}
	}
}

func TestMigrateIsIdempotentAndSeeds(t *testing.T) {
	db := openTemp(t)
	for i := 0; i < 3; i++ {
		if err := database.Migrate(db); err != nil {
			t.Fatalf("Migrate lần %d: %v", i+2, err)
		}
	}
	var n int
	var state string
	var epoch int
	if err := db.QueryRow(`SELECT COUNT(*) FROM lead_lease`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("lead_lease phải đúng 1 dòng, có %d (err %v)", n, err)
	}
	if err := db.QueryRow(`SELECT state, epoch FROM lead_lease WHERE id = 1`).Scan(&state, &epoch); err != nil {
		t.Fatal(err)
	}
	if state != "VACANT" || epoch != 0 {
		t.Fatalf("lead_lease khởi tạo = (%s, %d), muốn (VACANT, 0)", state, epoch)
	}
	var v string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'office_enabled'`).Scan(&v); err != nil || v != "false" {
		t.Fatalf("settings.office_enabled = %q (err %v), muốn \"false\"", v, err)
	}
}

func TestSchemaColumns(t *testing.T) {
	db := openTemp(t)
	need := map[string][]string{
		"workers":      {"id", "name", "os", "conn_status", "offline_since", "token_hash", "last_heartbeat", "created_at"},
		"agents":       {"id", "worker_id", "kind", "status", "cooldown_until", "cooldown_reason", "lead_priority", "soft_max_jobs", "soft_window_hours", "idle_since", "created_at"},
		"jobs":         {"id", "project_id", "task_id", "kind", "required_agent", "lead_only", "status", "priority", "attempt", "branch_name", "current_assignment_id", "created_at", "updated_at"},
		"assignments":  {"id", "job_id", "agent_id", "attempt", "status", "handoff_note", "wip_commit", "last_log_at", "started_at", "ended_at"},
		"lead_lease":   {"id", "state", "agent_id", "epoch", "expires_at", "vacant_since"},
		"lead_journal": {"id", "epoch", "agent_id", "task_id", "kind", "entry", "created_at"},
		"task_logs":    {"id", "task_id", "worker_id", "assignment_id", "log_chunk", "timestamp"},
		"tasks":        {"id", "project_id", "parent_id", "github_issue_id", "title", "description", "status", "assigned_worker_id", "branch_name", "pr_url", "created_by", "created_at", "updated_at"},
		"settings":     {"key", "value", "updated_at"},
		"users":        {"id", "github_id", "username", "avatar_url", "role", "created_at"},
		"projects":     {"id", "name", "github_repo", "default_branch", "status", "created_at"},
	}
	for table, cols := range need {
		have := columns(t, db, table)
		for _, c := range cols {
			if !have[c] {
				t.Errorf("bảng %s thiếu cột %s", table, c)
			}
		}
	}
	// Cột cũ của workers phải bị bỏ (DEC-04).
	have := columns(t, db, "workers")
	for _, old := range []string{"agent_type", "status", "capabilities"} {
		if have[old] {
			t.Errorf("workers còn cột cũ %s (DEC-04 đã bỏ)", old)
		}
	}
}

func TestIndexesExist(t *testing.T) {
	db := openTemp(t)
	have := map[string]bool{}
	for _, n := range names(t, db, `SELECT name FROM sqlite_master WHERE type='index'`) {
		have[n] = true
	}
	for _, idx := range []string{"idx_tasks_status", "idx_tasks_parent", "idx_tasks_assigned_worker", "idx_task_logs_task",
		"idx_agents_status", "idx_jobs_queue", "idx_assignments_agent", "idx_lead_journal_task"} {
		if !have[idx] {
			t.Errorf("thiếu index %s", idx)
		}
	}
}

func TestForeignKeysOnEveryConnection(t *testing.T) {
	db := openTemp(t)
	db.SetMaxIdleConns(0) // ép pool mở kết nối mới cho mỗi truy vấn
	for i := 0; i < 5; i++ {
		var on int
		if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&on); err != nil {
			t.Fatal(err)
		}
		if on != 1 {
			t.Fatalf("lần %d: foreign_keys = %d, muốn 1", i, on)
		}
	}
	if _, err := db.Exec(`INSERT INTO agents (id, worker_id, kind) VALUES ('x/claude', 'khong-ton-tai', 'CLAUDE')`); err == nil {
		t.Fatal("chèn agent với worker không tồn tại phải lỗi khoá ngoại")
	}
}

func TestValidRowsAccepted(t *testing.T) {
	db := openTemp(t)
	fixtures(t, db)
	mustExec(t, db, `INSERT INTO agents (id, worker_id, kind, status, cooldown_reason, lead_priority, soft_max_jobs, soft_window_hours)
		VALUES ('mac-mini/antigravity', 'mac-mini', 'ANTIGRAVITY', 'COOLDOWN', 'SOFT_LIMIT', 99, 500, 24)`)
	mustExec(t, db, `INSERT INTO lead_journal (epoch, agent_id, task_id, kind, entry) VALUES (1, 'mac-mini/claude', 't1', 'HANDOVER', 'epoch 0 → 1')`)
	mustExec(t, db, `INSERT INTO task_logs (task_id, worker_id, assignment_id, log_chunk) VALUES ('t1', 'mac-mini', 'a1', 'log')`)
	mustExec(t, db, `UPDATE lead_lease SET state = 'HELD', agent_id = 'mac-mini/claude', epoch = 1 WHERE id = 1`)
	var s string
	if err := db.QueryRow(`SELECT status FROM jobs WHERE id = 'j1'`).Scan(&s); err != nil || s != "QUEUED" {
		t.Fatalf("jobs.status mặc định = %q, muốn QUEUED", s)
	}
	if err := db.QueryRow(`SELECT status FROM agents WHERE id = 'mac-mini/claude'`).Scan(&s); err != nil || s != "IDLE" {
		t.Fatalf("agents.status mặc định = %q, muốn IDLE", s)
	}
	if err := db.QueryRow(`SELECT status FROM assignments WHERE id = 'a1'`).Scan(&s); err != nil || s != "ACTIVE" {
		t.Fatalf("assignments.status mặc định = %q, muốn ACTIVE", s)
	}
	if err := db.QueryRow(`SELECT conn_status FROM workers WHERE id = 'mac-mini'`).Scan(&s); err != nil || s != "OFFLINE" {
		t.Fatalf("workers.conn_status mặc định = %q, muốn OFFLINE", s)
	}
}

func TestConstraintsRejectInvalid(t *testing.T) {
	cases := []struct{ name, q string }{
		{"users.role", `INSERT INTO users (id, github_id, username, role) VALUES ('u2', 'gh2', 'x', 'ROOT')`},
		{"workers.conn_status", `INSERT INTO workers (id, name, os, token_hash, conn_status) VALUES ('w2', 'w2', 'linux', 'h', 'BUSY')`},
		{"agents.kind", `INSERT INTO agents (id, worker_id, kind) VALUES ('mac-mini/gpt', 'mac-mini', 'GPT')`},
		{"agents.status", `INSERT INTO agents (id, worker_id, kind, status) VALUES ('mac-mini/antigravity', 'mac-mini', 'ANTIGRAVITY', 'OFFLINE')`},
		{"agents.cooldown_reason", `INSERT INTO agents (id, worker_id, kind, cooldown_reason) VALUES ('mac-mini/antigravity', 'mac-mini', 'ANTIGRAVITY', 'TIRED')`},
		{"agents.lead_priority<1", `INSERT INTO agents (id, worker_id, kind, lead_priority) VALUES ('mac-mini/antigravity', 'mac-mini', 'ANTIGRAVITY', 0)`},
		{"agents.lead_priority>99", `INSERT INTO agents (id, worker_id, kind, lead_priority) VALUES ('mac-mini/antigravity', 'mac-mini', 'ANTIGRAVITY', 100)`},
		{"agents.soft_max_jobs>500", `INSERT INTO agents (id, worker_id, kind, soft_max_jobs) VALUES ('mac-mini/antigravity', 'mac-mini', 'ANTIGRAVITY', 501)`},
		{"agents.soft_window_hours>24", `INSERT INTO agents (id, worker_id, kind, soft_window_hours) VALUES ('mac-mini/antigravity', 'mac-mini', 'ANTIGRAVITY', 25)`},
		{"agents UNIQUE(worker_id, name)", `INSERT INTO agents (id, worker_id, kind, name) VALUES ('mac-mini/khac', 'mac-mini', 'CLAUDE', 'claude')`}, // DEC-41: bỏ UNIQUE(worker_id, kind)
		{"tasks.status", `INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t2', 'p1', 'x', 'y', 'u1', 'FOO')`},
		{"jobs.kind", `INSERT INTO jobs (id, project_id, task_id, kind, required_agent) VALUES ('j2', 'p1', 't1', 'DEPLOY', 'CLAUDE')`},
		{"jobs.required_agent", `INSERT INTO jobs (id, project_id, task_id, kind, required_agent) VALUES ('j2', 'p1', 't1', 'CODE', 'GEMINI')`},
		{"jobs.status", `INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status) VALUES ('j2', 'p1', 't1', 'CODE', 'CLAUDE', 'PAUSED')`},
		{"assignments.status", `INSERT INTO assignments (id, job_id, agent_id, attempt, status) VALUES ('a2', 'j1', 'mac-mini/claude', 2, 'DONE')`},
		{"lead_lease chỉ 1 dòng", `INSERT INTO lead_lease (id, state, epoch) VALUES (2, 'VACANT', 0)`},
		{"lead_lease.state", `UPDATE lead_lease SET state = 'MAYBE' WHERE id = 1`},
		{"lead_journal.kind", `INSERT INTO lead_journal (epoch, kind, entry) VALUES (1, 'GOSSIP', 'x')`},
		{"jobs FK task", `INSERT INTO jobs (id, project_id, task_id, kind, required_agent) VALUES ('j3', 'p1', 'khong-co', 'CODE', 'CLAUDE')`},
	}
	if len(cases) < 19 {
		t.Fatal("không được bớt ca kiểm ràng buộc")
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := openTemp(t)
			fixtures(t, db)
			if _, err := db.Exec(c.q); err == nil {
				t.Fatalf("câu sau phải bị CHECK/UNIQUE/FK chặn nhưng lại chạy được: %s", c.q)
			}
		})
	}
}

func TestDeleteWorkerCascadesAgents(t *testing.T) {
	db := openTemp(t)
	mustExec(t, db, `INSERT INTO workers (id, name, os, token_hash) VALUES ('w9', 'w9', 'linux', 'h')`)
	mustExec(t, db, `INSERT INTO agents (id, worker_id, kind) VALUES ('w9/claude', 'w9', 'CLAUDE')`)
	mustExec(t, db, `DELETE FROM workers WHERE id = 'w9'`)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM agents WHERE worker_id = 'w9'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("xoá máy phải xoá agent theo (ON DELETE CASCADE), còn %d", n)
	}
}
