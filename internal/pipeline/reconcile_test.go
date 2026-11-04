package pipeline_test

// Test do trưởng phòng viết: tự vá job còn thiếu (task kẹt PLANNING / sub-task READY không có job).

import (
	"context"
	"testing"
)

func TestReconcileRequeuesMissingJobs(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	n, err := r.p.Reconcile(ctx)
	if err != nil || n != 1 {
		t.Fatalf("t1 PLANNING không có SPEC → tạo 1 job, nhận %d %v", n, err)
	}
	if c := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id='t1' AND kind='SPEC' AND status='QUEUED'`); c != 1 {
		t.Fatalf("SPEC = %d", c)
	}
	if n, _ := r.p.Reconcile(ctx); n != 0 {
		t.Fatalf("chạy lại không được tạo trùng (%d)", n)
	}

	sub := r.toCode()
	r.exec(`UPDATE jobs SET status='FAILED' WHERE task_id=? AND kind='CODE'`, sub)
	if n, _ := r.p.Reconcile(ctx); n != 1 {
		t.Fatalf("sub-task READY mất job CODE → tạo lại 1, nhận %d", n)
	}
	var branch string
	r.db.QueryRow(`SELECT branch_name FROM jobs WHERE task_id=? AND kind='CODE' AND status='QUEUED'`, sub).Scan(&branch)
	if branch == "" {
		t.Fatal("job CODE bù phải dùng lại nhánh cũ")
	}
}
