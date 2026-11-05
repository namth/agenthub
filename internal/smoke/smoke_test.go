package smoke_test

// Test nghiệm thu TASK-044 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/ops-readiness.md §3–§7 (BR-60, DEC-36); TC-89.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/smoke"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

// fakeOps giả lập tasks.Service: tạo task, duyệt = tạo job SPEC QUEUED.
type fakeOps struct {
	db      *sql.DB
	mu      sync.Mutex
	n       int
	cleaned []string
}

func (f *fakeOps) CreateIn(_ context.Context, userID, projectID, title, desc string) (string, error) {
	f.mu.Lock()
	f.n++
	id := "smk" + string(rune('0'+f.n))
	f.mu.Unlock()
	_, err := f.db.Exec(`INSERT INTO tasks (id, project_id, title, description, created_by, github_issue_id) VALUES (?, ?, ?, ?, ?, 900)`, id, projectID, title, desc, userID)
	return id, err
}
func (f *fakeOps) Approve(_ context.Context, taskID string) error {
	if _, err := f.db.Exec(`UPDATE tasks SET status='PLANNING' WHERE id=?`, taskID); err != nil {
		return err
	}
	_, err := f.db.Exec(`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status, priority) SELECT 'job-spec-'||id, project_id, id, 'SPEC', 'CLAUDE', 'QUEUED', 100 FROM tasks WHERE id=?`, taskID)
	return err
}
func (f *fakeOps) Cleanup(_ context.Context, taskID string) error {
	f.mu.Lock()
	f.cleaned = append(f.cleaned, taskID)
	f.mu.Unlock()
	return nil
}

type rig struct {
	t   *testing.T
	db  *sql.DB
	clk *clock
	ops *fakeOps
	s   *smoke.Smoke
}

func newRig(t *testing.T) *rig {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-owner', '1', 'namth', 'ADMIN')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-p', '2', 'doi-tac', 'PARTNER')`,
		`INSERT INTO projects (id, name, github_repo, is_sandbox) VALUES ('sb', 'sandbox', 'namth/agenthub-sandbox', 1)`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('real', 'agenthub', 'namth/agenthub')`,
		`INSERT INTO workers (id, name, os, token_hash) VALUES ('m', 'm', 'darwin', 'h')`,
		`INSERT INTO agents (id, worker_id, kind) VALUES ('m/claude', 'm', 'CLAUDE')`,
		`INSERT INTO agents (id, worker_id, kind) VALUES ('m/antigravity', 'm', 'ANTIGRAVITY')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	c := &clock{t: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)}
	ops := &fakeOps{db: db}
	return &rig{t: t, db: db, clk: c, ops: ops, s: smoke.New(db, ops, c.now)}
}

func (r *rig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.db.Exec(q, args...); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
}

func (r *rig) track() smoke.Run {
	r.t.Helper()
	if err := r.s.Track(context.Background()); err != nil {
		r.t.Fatal(err)
	}
	runs, err := r.s.List("sb", 5)
	if err != nil || len(runs) == 0 {
		r.t.Fatalf("List: %v %v", runs, err)
	}
	return runs[0]
}

func stepOK(run smoke.Run, name string) bool {
	for _, s := range run.Steps {
		if s.Step == name {
			return s.OK != nil && *s.OK
		}
	}
	return false
}

func TestStartRules(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.s.Start(ctx, "real", "u-owner"); !errors.Is(err, smoke.ErrNotSandbox) {
		t.Fatalf("dự án thường: %v, muốn ErrNotSandbox", err)
	}
	id, err := r.s.Start(ctx, "sb", "u-owner")
	if err != nil || id == "" {
		t.Fatalf("Start = %q %v", id, err)
	}
	var st, task string
	r.db.QueryRow(`SELECT status, task_id FROM smoke_runs WHERE id=?`, id).Scan(&st, &task)
	var tstatus, creator string
	r.db.QueryRow(`SELECT status, created_by FROM tasks WHERE id=?`, task).Scan(&tstatus, &creator)
	if st != "RUNNING" || tstatus != "PLANNING" || creator != "u-owner" {
		t.Fatalf("run=%s task=%s creator=%s", st, tstatus, creator)
	}
	if _, err := r.s.Start(ctx, "sb", "u-owner"); !errors.Is(err, smoke.ErrConflict) {
		t.Fatalf("đang chạy mà Start lại: %v, muốn ErrConflict", err)
	}
	run := r.track()
	var prio int
	r.db.QueryRow(`SELECT priority FROM jobs WHERE task_id=? AND kind='SPEC'`, task).Scan(&prio)
	if prio != 50 || len(run.Steps) != 3 || run.Steps[0].Step != "SPEC" || run.Steps[1].Step != "CODE" || run.Steps[2].Step != "QA" {
		t.Fatalf("priority=%d steps=%+v", prio, run.Steps)
	}
}

func TestTrackToPassed(t *testing.T) { // TC-89
	r := newRig(t)
	id, _ := r.s.Start(context.Background(), "sb", "u-owner")
	var task string
	r.db.QueryRow(`SELECT task_id FROM smoke_runs WHERE id=?`, id).Scan(&task)
	r.clk.add(time.Minute)
	r.exec(`UPDATE jobs SET status='DONE' WHERE task_id=? AND kind='SPEC'`, task)
	r.exec(`INSERT INTO assignments (id, job_id, agent_id, attempt, status, started_at, ended_at) VALUES ('a1', 'job-spec-'||?, 'm/claude', 1, 'SUBMITTED', ?, ?)`, task, r.clk.now().Add(-50*time.Second).Format("2006-01-02 15:04:05"), r.clk.now().Format("2006-01-02 15:04:05"))
	r.exec(`INSERT INTO tasks (id, project_id, parent_id, title, description, created_by, status, pr_url) VALUES ('sub1', 'sb', ?, 'Add', 'x', 'u-owner', 'WAITING_QA', 'https://github.com/namth/agenthub-sandbox/pull/3')`, task)
	r.exec(`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status, priority) VALUES ('jc', 'sb', 'sub1', 'CODE', 'ANTIGRAVITY', 'QUEUED', 100)`)
	run := r.track()
	if !stepOK(run, "SPEC") || stepOK(run, "CODE") || run.Status != "RUNNING" {
		t.Fatalf("sau SPEC: %+v", run)
	}
	if run.Steps[0].AgentID != "m/claude" {
		t.Fatalf("bước SPEC phải ghi agent: %+v", run.Steps[0])
	}
	var p int
	r.db.QueryRow(`SELECT priority FROM jobs WHERE id='jc'`).Scan(&p)
	if p != 50 {
		t.Fatal("job CODE của sub-task cũng phải được nâng priority 50")
	}
	r.exec(`UPDATE jobs SET status='DONE' WHERE id='jc'`)
	r.track()
	r.exec(`UPDATE tasks SET status='QA_PASSED' WHERE id=?`, task)
	run = r.track()
	if run.Status != "PASSED" || !stepOK(run, "CODE") || !stepOK(run, "QA") || run.EndedAt == nil {
		t.Fatalf("xong: %+v", run)
	}
	r.track()
	r.ops.mu.Lock()
	defer r.ops.mu.Unlock()
	if len(r.ops.cleaned) != 1 || r.ops.cleaned[0] != task {
		t.Fatalf("Cleanup phải gọi đúng 1 lần: %v", r.ops.cleaned)
	}
}

func TestTrackFailedAndTimeout(t *testing.T) {
	r := newRig(t)
	id, _ := r.s.Start(context.Background(), "sb", "u-owner")
	var task string
	r.db.QueryRow(`SELECT task_id FROM smoke_runs WHERE id=?`, id).Scan(&task)
	r.exec(`UPDATE jobs SET status='FAILED' WHERE task_id=?`, task)
	r.exec(`INSERT INTO task_logs (task_id, worker_id, log_chunk) VALUES (?, 'm', 'lỗi: lệnh bị chặn go test')`, task)
	run := r.track()
	if run.Status != "FAILED" {
		t.Fatalf("job FAILED → run FAILED: %+v", run)
	}
	r2 := newRig(t)
	r2.s.Start(context.Background(), "sb", "u-owner")
	r2.clk.add(31 * time.Minute)
	if run := r2.track(); run.Status != "TIMEOUT" || run.EndedAt == nil {
		t.Fatalf("quá 30 phút → TIMEOUT: %+v", run)
	}
	if len(r2.ops.cleaned) != 1 {
		t.Fatal("TIMEOUT cũng phải Cleanup")
	}
}

func TestHTTP(t *testing.T) {
	r := newRig(t)
	h := smoke.Handler(r.s, func(req *http.Request) (string, string, bool) {
		u := req.Header.Get("X-Test-User")
		return u, map[string]string{"u-owner": "ADMIN", "u-p": "PARTNER"}[u], u != ""
	})
	do := func(method, path, user string) (int, map[string]any) {
		req := httptest.NewRequest(method, path, bytes.NewReader(nil))
		req.Header.Set("X-Test-User", user)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var m map[string]any
		json.Unmarshal(rr.Body.Bytes(), &m)
		return rr.Code, m
	}
	if code, _ := do("POST", "/api/admin/projects/sb/smoke-run", "u-p"); code != 403 {
		t.Fatalf("partner: %d", code)
	}
	if code, _ := do("POST", "/api/admin/projects/real/smoke-run", "u-owner"); code != 400 {
		t.Fatalf("dự án thường: %d, muốn 400", code)
	}
	code, m := do("POST", "/api/admin/projects/sb/smoke-run", "u-owner")
	if code != 202 {
		t.Fatalf("start: %d %v", code, m)
	}
	runID := m["data"].(map[string]any)["run_id"].(string)
	if code, _ := do("POST", "/api/admin/projects/sb/smoke-run", "u-owner"); code != 409 {
		t.Fatalf("lần 2 đang chạy: %d, muốn 409", code)
	}
	code, m = do("GET", "/api/admin/projects/sb/smoke-runs", "u-owner")
	if list, _ := m["data"].([]any); code != 200 || len(list) != 1 {
		t.Fatalf("list: %d %v", code, m)
	}
	code, m = do("GET", "/api/admin/projects/sb/smoke-runs/"+runID, "u-owner")
	if code != 200 || m["data"].(map[string]any)["status"] != "RUNNING" {
		t.Fatalf("get: %d %v", code, m)
	}
	if code, _ := do("GET", "/api/admin/projects/sb/smoke-runs/khong-co", "u-owner"); code != 404 {
		t.Fatalf("không có: %d", code)
	}
}
