package pipeline_test

// Test nghiệm thu TASK-048 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/remote-command.md §6 (result.json REPLAN/UNBLOCK), BR-64, DEC-40; TC-94.

import (
	"strings"
	"sync"
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/pipeline"
)

type replanHooks struct {
	mu       sync.Mutex
	proposal []string
	invalid  []string
	need     []string
}

func newReplanRig(t *testing.T) (*rig, *replanHooks) {
	t.Helper()
	r := newRig(t)
	h := &replanHooks{}
	r.p = pipeline.New(r.db, r.disp, r.gh, "p1",
		pipeline.WithOnProposal(func(taskID, pid, reason string) {
			h.mu.Lock()
			h.proposal = append(h.proposal, taskID+"|"+reason)
			h.mu.Unlock()
		}),
		pipeline.WithOnReplanInvalid(func(taskID string) { h.mu.Lock(); h.invalid = append(h.invalid, taskID); h.mu.Unlock() }),
		pipeline.WithOnNeedInfo(func(taskID string, round int, qs []pipeline.ClarifyQuestion) {
			h.mu.Lock()
			h.need = append(h.need, taskID)
			h.mu.Unlock()
		}))
	return r, h
}

// stuckSub: sub-task đã thất bại tối đa → PLANNING, job CODE FAILED, có job REPLAN lead_only chờ.
func stuckSub(r *rig) string {
	r.t.Helper()
	sub := r.toCode()
	r.exec(`UPDATE jobs SET status='FAILED' WHERE task_id=?`, sub)
	r.exec(`UPDATE tasks SET status='PLANNING', branch_name='feat/task-sub' WHERE id=?`, sub)
	if _, err := r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: sub, Kind: dispatcher.JobReplan, RequiredAgent: dispatcher.KindClaude, LeadOnly: true, Priority: 10}); err != nil {
		r.t.Fatal(err)
	}
	return sub
}

func TestReplanRetry(t *testing.T) {
	r, _ := newReplanRig(t)
	sub := stuckSub(r)
	r.run("m/claude", "", map[string]any{"action": "retry", "note": "Dùng testify, chạy go test ./internal/x"})
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, sub); s != "READY" {
		t.Fatalf("retry → READY, có %s", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='CODE' AND status='QUEUED' AND branch_name='feat/task-sub'`, sub); n != 1 {
		t.Fatalf("retry → 1 job CODE mới cùng nhánh, có %d", n)
	}
	if n := r.num(`SELECT COUNT(*) FROM lead_journal WHERE task_id=? AND kind='DECISION' AND entry LIKE '%testify%'`, sub); n != 1 {
		t.Fatal("retry phải ghi ghi chú vào lead_journal")
	}
}

func TestReplanSubtaskReplaced(t *testing.T) {
	r, _ := newReplanRig(t)
	sub := stuckSub(r)
	r.run("m/claude", "", map[string]any{"action": "replan", "note": "chia nhỏ", "subtasks": []any{
		map[string]any{"title": "Sub 1a", "description": "phần a", "dod": "go test xanh"},
		map[string]any{"title": "Sub 1b", "description": "phần b", "dod": "go test xanh"},
	}})
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, sub); s != "REJECTED" {
		t.Fatalf("sub-task bị thay → REJECTED, có %s", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM tasks WHERE parent_id='t1' AND status <> 'REJECTED'`); n != 2 {
		t.Fatalf("2 sub-task mới cùng cha, có %d", n)
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs j JOIN tasks t ON j.task_id=t.id WHERE t.parent_id='t1' AND t.status<>'REJECTED' AND j.kind='CODE' AND j.status='QUEUED'`); n != 2 {
		t.Fatalf("mỗi sub-task mới 1 job CODE, có %d", n)
	}
}

func TestReplanRootTask(t *testing.T) {
	r, _ := newReplanRig(t)
	old := r.toCode()
	r.exec(`UPDATE tasks SET status='PLANNING' WHERE id='t1'`)
	r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: dispatcher.JobReplan, RequiredAgent: dispatcher.KindClaude, LeadOnly: true, Priority: 10})
	r.run("m/claude", "", map[string]any{"action": "replan", "subtasks": []any{map[string]any{"title": "Mới", "description": "x", "dod": "go test"}}})
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, old); s != "REJECTED" {
		t.Fatalf("sub-task cũ chưa DONE → REJECTED, có %s", s)
	}
	if s := r.str(`SELECT status FROM tasks WHERE id='t1'`); s != "READY" {
		t.Fatalf("task gốc → READY, có %s", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM tasks WHERE parent_id='t1' AND title='Mới'`); n != 1 {
		t.Fatal("phải tạo sub-task mới")
	}
}

func TestReplanAskUser(t *testing.T) {
	r, h := newReplanRig(t)
	sub := stuckSub(r)
	r.run("m/claude", "", map[string]any{"action": "ask_user", "questions": []any{
		map[string]any{"id": "lib", "question": "Dùng thư viện nào?", "options": []string{"A", "B"}, "recommended": "A", "reason": ""},
	}})
	if s := r.str(`SELECT status FROM tasks WHERE id='t1'`); s != "WAITING_USER" {
		t.Fatalf("ask_user → task gốc WAITING_USER, có %s", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM task_clarifications WHERE task_id='t1' AND status='WAITING'`); n != 1 {
		t.Fatal("phải có vòng câu hỏi trên task gốc")
	}
	_ = sub
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.need) != 1 || h.need[0] != "t1" {
		t.Fatalf("OnNeedInfo = %v", h.need)
	}
}

func TestReplanCancelIsOnlyProposal(t *testing.T) {
	r, h := newReplanRig(t)
	sub := stuckSub(r)
	r.run("m/claude", "", map[string]any{"action": "cancel", "reason": "Trùng yêu cầu #98"})
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, sub); s != "PLANNING" {
		t.Fatalf("cancel không được tự huỷ task, có %s", s)
	}
	var st, by, reason string
	var epoch int
	r.db.QueryRow(`SELECT status, proposed_by, reason, epoch FROM lead_proposals WHERE task_id=?`, sub).Scan(&st, &by, &reason, &epoch)
	if st != "PENDING" || by != "m/claude" || reason != "Trùng yêu cầu #98" || epoch < 1 {
		t.Fatalf("proposal = %s %s %s %d", st, by, reason, epoch)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.proposal) != 1 || h.proposal[0] != sub+"|Trùng yêu cầu #98" {
		t.Fatalf("OnProposal = %v", h.proposal)
	}
}

func TestReplanInvalidNoLoop(t *testing.T) {
	r, h := newReplanRig(t)
	sub := stuckSub(r)
	r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: sub, Kind: dispatcher.JobReplan, RequiredAgent: dispatcher.KindClaude, LeadOnly: true, Priority: 10})
	before := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='REPLAN'`, sub)
	r.run("m/claude", "", map[string]any{"action": "lam-gi-do"})
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='REPLAN'`, sub); n != before {
		t.Fatalf("đã ≥ 2 REPLAN trong 24h → không sinh thêm, %d → %d", before, n)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.invalid) != 1 {
		t.Fatalf("OnReplanInvalid = %v", h.invalid)
	}
}

func TestReplanPromptSchema(t *testing.T) {
	r, _ := newReplanRig(t)
	sub := stuckSub(r)
	r.exec(`INSERT INTO lead_journal (epoch, agent_id, task_id, kind, entry) VALUES (1, 'm/claude', ?, 'NOTE', 'QA: thiếu test biên')`, sub)
	a, _ := r.disp.RequestJob("m", "m/claude")
	p := r.p.PromptFor(*a)
	for _, want := range []string{"retry", "replan", "ask_user", "cancel", "QA: thiếu test biên"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt REPLAN thiếu %q:\n%s", want, p)
		}
	}
}
