package pipeline_test

// Test nghiệm thu TASK-018 — do trưởng phòng viết, file bị khoá (tests.lock).
// Chốt chặn 1 (CI) trước Chốt chặn 2 (Claude review) — docs/13 §2; DONE khi merge — docs/12.

import (
	"context"
	"testing"

	"github.com/namth/agenthub/internal/pipeline"
)

func newGateRig(t *testing.T) (*rig, string, string) {
	t.Helper()
	r := newRig(t)
	r.p = pipeline.New(r.db, r.disp, r.gh, "p1", pipeline.WithCIGate(true))
	sub := r.toCode()
	a := r.run("m/antigravity", "https://github.com/namth/agenthub/pull/15", nil)
	return r, sub, a.BranchName
}

func TestCIGateHoldsReviewUntilSuccess(t *testing.T) {
	r, sub, branch := newGateRig(t)
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, sub); s != "WAITING_QA" {
		t.Fatalf("task = %s", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='QA_REVIEW'`, sub); n != 0 {
		t.Fatalf("bật CI gate: chưa có CI thì chưa tạo QA_REVIEW (%d)", n)
	}
	if err := r.p.OnCI(context.Background(), branch, "success", "https://ci/run/1"); err != nil {
		t.Fatal(err)
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='QA_REVIEW' AND status='QUEUED'`, sub); n != 1 {
		t.Fatalf("CI đạt → 1 QA_REVIEW, có %d", n)
	}
	r.p.OnCI(context.Background(), branch, "success", "https://ci/run/1") // GitHub gửi lại
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='QA_REVIEW'`, sub); n != 1 {
		t.Fatalf("CI đạt lặp lại không được tạo thêm QA_REVIEW (%d)", n)
	}
}

func TestCIFailureSendsBackToCode(t *testing.T) {
	r, sub, branch := newGateRig(t)
	if err := r.p.OnCI(context.Background(), branch, "failure", "https://ci/run/2"); err != nil {
		t.Fatal(err)
	}
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, sub); s != "READY" {
		t.Fatalf("CI lỗi: task = %s, muốn READY (giao sửa)", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='CODE' AND status='QUEUED' AND branch_name=?`, sub, branch); n != 1 {
		t.Fatalf("CI lỗi phải tạo job CODE cùng nhánh: %d", n)
	}
	if e := r.str(`SELECT entry FROM lead_journal WHERE task_id=? ORDER BY id DESC LIMIT 1`, sub); e == "" || !contains(e, "https://ci/run/2") {
		t.Fatalf("lead_journal phải ghi link CI lỗi: %q", e)
	}
}

func TestCIUnknownBranchAndOtherConclusionsIgnored(t *testing.T) {
	r, sub, branch := newGateRig(t)
	if err := r.p.OnCI(context.Background(), "feat/khong-co", "success", "x"); err != nil {
		t.Fatalf("nhánh lạ phải bỏ qua, không lỗi: %v", err)
	}
	r.p.OnCI(context.Background(), branch, "cancelled", "x")
	if s := r.str(`SELECT status FROM tasks WHERE id=?`, sub); s != "WAITING_QA" {
		t.Fatalf("cancelled không đổi trạng thái: %s", s)
	}
}

func TestWithoutGateReviewImmediately(t *testing.T) {
	r := newRig(t)
	sub := r.toCode()
	r.run("m/antigravity", "https://x/pull/1", nil)
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='QA_REVIEW'`, sub); n != 1 {
		t.Fatalf("tắt CI gate (mặc định) giữ hành vi cũ: %d", n)
	}
}

func TestMergedMovesDoneAndRollsUpParent(t *testing.T) {
	r := newRig(t)
	r.exec(`UPDATE tasks SET status='READY' WHERE id='t1'`)
	r.exec(`INSERT INTO tasks (id, project_id, parent_id, title, description, created_by, status, pr_url) VALUES ('s1', 'p1', 't1', 'A', 'B', 'u1', 'QA_PASSED', 'https://x/pull/1')`)
	r.exec(`INSERT INTO tasks (id, project_id, parent_id, title, description, created_by, status, pr_url) VALUES ('s2', 'p1', 't1', 'C', 'D', 'u1', 'QA_PASSED', 'https://x/pull/2')`)
	ctx := context.Background()
	if err := r.p.OnMerged(ctx, "https://x/pull/1"); err != nil {
		t.Fatal(err)
	}
	if s := r.str(`SELECT status FROM tasks WHERE id='s1'`); s != "DONE" {
		t.Fatalf("s1 = %s, muốn DONE", s)
	}
	if s := r.str(`SELECT status FROM tasks WHERE id='t1'`); s == "DONE" {
		t.Fatal("cha chưa DONE khi còn sub-task chưa merge")
	}
	r.p.OnMerged(ctx, "https://x/pull/2")
	if s := r.str(`SELECT status FROM tasks WHERE id='t1'`); s != "DONE" {
		t.Fatalf("mọi sub-task DONE → cha DONE, nhận %s", s)
	}
	if err := r.p.OnMerged(ctx, "https://x/pull/999"); err != nil {
		t.Fatalf("PR lạ phải bỏ qua: %v", err)
	}
	r.exec(`INSERT INTO tasks (id, project_id, title, description, created_by, status, pr_url) VALUES ('s3', 'p1', 'E', 'F', 'u1', 'IN_PROGRESS', 'https://x/pull/3')`)
	r.p.OnMerged(ctx, "https://x/pull/3")
	if s := r.str(`SELECT status FROM tasks WHERE id='s3'`); s != "IN_PROGRESS" {
		t.Fatalf("chỉ QA_PASSED mới sang DONE (docs/12 không nhảy cóc): %s", s)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
