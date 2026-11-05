package pipeline_test

// Test nghiệm thu TASK-014 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: DEC-12, docs/12 §1 (máy trạng thái task), docs/04 §2–4.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/pipeline"
)

type fakeGH struct {
	mu     sync.Mutex
	next   int
	issues []string
}

func (g *fakeGH) CreateIssue(_ context.Context, title, body string, labels []string) (int, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	g.issues = append(g.issues, title+"|"+strings.Join(labels, ","))
	return 200 + g.next, fmt.Sprintf("https://github.com/namth/agenthub/issues/%d", 200+g.next), nil
}
func (g *fakeGH) SetLabels(context.Context, int, []string) error { return nil }
func (g *fakeGH) CloseIssue(context.Context, int, string) error  { return nil }

type clk struct{ t time.Time }

func (c *clk) Now() time.Time { return c.t }

type rig struct {
	t    *testing.T
	db   *sql.DB
	disp *dispatcher.Service
	gh   *fakeGH
	p    *pipeline.Pipeline
}

func newRig(t *testing.T) *rig {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &rig{t: t, db: db, gh: &fakeGH{}}
	var p *pipeline.Pipeline
	r.disp = dispatcher.New(db, &clk{t: time.Date(2026, 11, 4, 13, 0, 0, 0, time.UTC)}, dispatcher.DefaultConfig(),
		notifier(func(e dispatcher.Event) {
			if p != nil {
				p.Notify(e)
			}
		}))
	p = pipeline.New(db, r.disp, r.gh, "p1")
	r.p = p
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', 'g1', 'namth', 'ADMIN')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'AgentHub', 'namth/agenthub')`,
		`INSERT INTO tasks (id, project_id, github_issue_id, title, description, created_by, status) VALUES ('t1', 'p1', 101, 'Xuất báo cáo PDF', 'Khách cần xuất bảng tiến độ thành PDF', 'u1', 'PLANNING')`,
		`INSERT INTO workers (id, name, os, token_hash) VALUES ('m', 'm', 'darwin', 'h')`,
	} {
		r.exec(q)
	}
	if _, err := r.disp.Connect("m", []string{"CLAUDE", "ANTIGRAVITY"}); err != nil {
		t.Fatal(err)
	}
	return r
}

type notifier func(dispatcher.Event)

func (n notifier) Notify(e dispatcher.Event) { n(e) }

func (r *rig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.db.Exec(q, args...); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
}
func (r *rig) str(q string, args ...any) string {
	var s sql.NullString
	r.db.QueryRow(q, args...).Scan(&s)
	return s.String
}
func (r *rig) num(q string, args ...any) int {
	var n int
	r.db.QueryRow(q, args...).Scan(&n)
	return n
}

// run giao job kế tiếp cho agent rồi nộp kèm kết quả qua pipeline.
func (r *rig) run(agent, prURL string, result any) *dispatcher.Assignment {
	r.t.Helper()
	a, err := r.disp.RequestJob("m", agent)
	if err != nil || a == nil {
		r.t.Fatalf("RequestJob(%s) = %v, %v", agent, a, err)
	}
	if err := r.disp.Submit("m", a.ID); err != nil {
		r.t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if result == nil {
		raw = nil
	}
	if err := r.p.OnSubmitted(context.Background(), "m", a.ID, prURL, raw); err != nil {
		r.t.Fatalf("OnSubmitted: %v", err)
	}
	return a
}

func spec3() map[string]any {
	var subs []map[string]any
	for i := 1; i <= 3; i++ {
		subs = append(subs, map[string]any{"title": fmt.Sprintf("Sub %d", i), "description": fmt.Sprintf("Làm phần %d", i), "dod": "go test xanh"})
	}
	return map[string]any{"subtasks": subs}
}

// ---------- prompt ----------

func TestPromptSpec(t *testing.T) {
	r := newRig(t)
	r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "SPEC", RequiredAgent: "CLAUDE"})
	a, _ := r.disp.RequestJob("m", "m/claude")
	p := r.p.PromptFor(*a)
	for _, want := range []string{"Xuất báo cáo PDF", "Khách cần xuất bảng tiến độ thành PDF", "subtasks", "dod", "#101"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt SPEC thiếu %q:\n%s", want, p)
		}
	}
}

// ---------- SPEC → sub-task ----------

func TestSpecCreatesSubtasksAndCodeJobs(t *testing.T) {
	r := newRig(t)
	r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "SPEC", RequiredAgent: "CLAUDE"})
	r.run("m/claude", "", spec3())

	if s := r.str(`SELECT status FROM tasks WHERE id='t1'`); s != "READY" {
		t.Fatalf("task cha = %s, muốn READY (docs/12: PLANNING → READY)", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM tasks WHERE parent_id='t1' AND status='READY' AND github_issue_id IS NOT NULL`); n != 3 {
		t.Fatalf("sub-task = %d, muốn 3 READY có issue", n)
	}
	if len(r.gh.issues) != 3 || !strings.Contains(r.gh.issues[0], "parent:#101") {
		t.Fatalf("issue con = %v, phải gắn nhãn parent:#101", r.gh.issues)
	}
	rows, _ := r.db.Query(`SELECT j.branch_name, t.github_issue_id FROM jobs j JOIN tasks t ON t.id = j.task_id WHERE j.kind='CODE' AND j.status='QUEUED' AND t.parent_id='t1'`)
	defer rows.Close()
	n := 0
	for rows.Next() {
		var br string
		var issue int
		rows.Scan(&br, &issue)
		if br != fmt.Sprintf("feat/task-%d", issue) {
			t.Fatalf("nhánh = %s, muốn feat/task-%d (docs/09)", br, issue)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("job CODE = %d, muốn 3", n)
	}
	// Prompt CODE của sub-task có mô tả và DoD.
	a, _ := r.disp.RequestJob("m", "m/antigravity")
	p := r.p.PromptFor(*a)
	if !strings.Contains(p, "Làm phần") || !strings.Contains(p, "go test xanh") || !strings.Contains(p, a.BranchName) {
		t.Fatalf("prompt CODE thiếu mô tả/DoD/nhánh:\n%s", p)
	}
}

func TestSpecInvalidTriggersReplan(t *testing.T) {
	for name, res := range map[string]any{
		"thiếu":       nil,
		"rỗng":        map[string]any{"subtasks": []any{}},
		"thiếu title": map[string]any{"subtasks": []any{map[string]any{"description": "x", "dod": "y"}}},
		"quá 20":      map[string]any{"subtasks": make([]map[string]any, 21)},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t)
			r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "SPEC", RequiredAgent: "CLAUDE"})
			r.run("m/claude", "", res)
			if n := r.num(`SELECT COUNT(*) FROM tasks WHERE parent_id='t1'`); n != 0 {
				t.Fatalf("kết quả sai không được tạo sub-task (%d)", n)
			}
			if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id='t1' AND kind='REPLAN' AND lead_only=1 AND status='QUEUED'`); n != 1 {
				t.Fatalf("REPLAN = %d, muốn 1", n)
			}
			if s := r.str(`SELECT status FROM tasks WHERE id='t1'`); s != "PLANNING" {
				t.Fatalf("task = %s, giữ PLANNING", s)
			}
		})
	}
}

// ---------- CODE → QA ----------

func (r *rig) toCode() string {
	r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "SPEC", RequiredAgent: "CLAUDE"})
	r.run("m/claude", "", map[string]any{"subtasks": []any{map[string]any{"title": "Sub 1", "description": "Làm phần 1", "dod": "go test xanh"}}})
	return r.str(`SELECT id FROM tasks WHERE parent_id='t1'`)
}

func TestCodeAssignedMovesInProgress(t *testing.T) {
	r := newRig(t)
	sub := r.toCode()
	if _, err := r.disp.RequestJob("m", "m/antigravity"); err != nil {
		t.Fatal(err)
	}
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, sub); s != "IN_PROGRESS" {
		t.Fatalf("nhận job CODE: task = %s, muốn IN_PROGRESS", s)
	}
}

func TestCodeSubmittedQueuesReview(t *testing.T) {
	r := newRig(t)
	sub := r.toCode()
	a := r.run("m/antigravity", "https://github.com/namth/agenthub/pull/15", map[string]any{"summary": "Xong"})
	if s := r.str(`SELECT status || '|' || pr_url FROM tasks WHERE id=?`, sub); s != "WAITING_QA|https://github.com/namth/agenthub/pull/15" {
		t.Fatalf("task = %s", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='QA_REVIEW' AND required_agent='CLAUDE' AND status='QUEUED' AND branch_name=?`, sub, a.BranchName); n != 1 {
		t.Fatalf("job QA_REVIEW = %d, muốn 1 cùng nhánh", n)
	}
	q, _ := r.disp.RequestJob("m", "m/claude")
	if p := r.p.PromptFor(*q); !strings.Contains(p, "pull/15") || !strings.Contains(p, "verdict") || !strings.Contains(p, "go test xanh") {
		t.Fatalf("prompt QA thiếu PR/verdict/DoD:\n%s", p)
	}
}

func TestQAPassAndChanges(t *testing.T) {
	r := newRig(t)
	sub := r.toCode()
	r.run("m/antigravity", "https://x/pull/1", nil)
	r.run("m/claude", "", map[string]any{"verdict": "CHANGES", "comments": "Thiếu test cho trường hợp rỗng"})
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, sub); s != "READY" {
		t.Fatalf("CHANGES: task = %s, muốn READY (giao sửa lại)", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='CODE' AND status='QUEUED'`, sub); n != 1 {
		t.Fatalf("CHANGES phải tạo job CODE mới: %d", n)
	}
	if !strings.Contains(r.str(`SELECT entry FROM lead_journal WHERE task_id=? ORDER BY id DESC LIMIT 1`, sub), "Thiếu test") {
		t.Fatal("nhận xét QA phải ghi vào lead_journal")
	}
	a, _ := r.disp.RequestJob("m", "m/antigravity")
	if p := r.p.PromptFor(*a); !strings.Contains(p, "Thiếu test cho trường hợp rỗng") || !strings.Contains(p, "DỮ LIỆU THAM KHẢO") {
		t.Fatalf("prompt sửa lần sau phải kèm nhận xét trong khối dữ liệu:\n%s", p)
	}
	r.disp.Submit("m", a.ID)
	r.p.OnSubmitted(context.Background(), "m", a.ID, "https://x/pull/1", nil)
	r.run("m/claude", "", map[string]any{"verdict": "PASS", "comments": "Ổn"})
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, sub); s != "QA_PASSED" {
		t.Fatalf("PASS: task = %s, muốn QA_PASSED", s)
	}
}

func TestQAInvalidVerdictReplans(t *testing.T) {
	r := newRig(t)
	sub := r.toCode()
	r.run("m/antigravity", "https://x/pull/1", nil)
	r.run("m/claude", "", map[string]any{"verdict": "MAYBE"})
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, sub); s != "WAITING_QA" {
		t.Fatalf("verdict sai: task = %s, giữ WAITING_QA", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='REPLAN'`, sub); n != 1 {
		t.Fatalf("verdict sai phải tạo REPLAN cho lead: %d", n)
	}
}

func TestStaleOrUnknownAssignmentIgnored(t *testing.T) {
	r := newRig(t)
	if err := r.p.OnSubmitted(context.Background(), "m", "khong-co", "", nil); err == nil {
		t.Fatal("assignment không tồn tại phải trả lỗi")
	}
}
