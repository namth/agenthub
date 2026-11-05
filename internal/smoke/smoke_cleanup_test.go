package smoke_test

// Test do trưởng phòng viết (BR-60 bổ sung 2026-11-05): lần chạy thử kết thúc
// thì không để lại việc dở trong hàng đợi. Phát hiện khi chạy thật: TIMEOUT
// đóng issue nhưng job CODE vẫn QUEUED → máy thức dậy chạy tốn hạn mức.

import (
	"context"
	"testing"
	"time"
)

func TestFinishedRunLeavesNoQueuedWork(t *testing.T) {
	r := newRig(t)
	id, err := r.s.Start(context.Background(), "sb", "u-owner")
	if err != nil {
		t.Fatal(err)
	}
	var root string
	r.db.QueryRow(`SELECT task_id FROM smoke_runs WHERE id=?`, id).Scan(&root)
	r.exec(`UPDATE jobs SET status='DONE' WHERE task_id=?`, root)
	r.exec(`UPDATE tasks SET status='READY' WHERE id=?`, root)
	r.exec(`INSERT INTO tasks (id, project_id, parent_id, title, description, created_by, status) VALUES ('sub1', 'sb', ?, 'Add', 'x', 'u-owner', 'IN_PROGRESS')`, root)
	r.exec(`INSERT INTO tasks (id, project_id, parent_id, title, description, created_by, status) VALUES ('sub2', 'sb', ?, 'Xong', 'x', 'u-owner', 'DONE')`, root)
	r.exec(`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status, priority) VALUES ('job-code', 'sb', 'sub1', 'CODE', 'ANTIGRAVITY', 'QUEUED', 50)`)
	r.exec(`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('other', 'real', 'Việc thật', 'x', 'u-owner', 'IN_PROGRESS')`)
	r.exec(`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status, priority) VALUES ('job-other', 'real', 'other', 'CODE', 'ANTIGRAVITY', 'QUEUED', 10)`)

	r.clk.add(31 * time.Minute)
	if run := r.track(); run.Status != "TIMEOUT" {
		t.Fatalf("muốn TIMEOUT, có %s", run.Status)
	}

	var st string
	r.db.QueryRow(`SELECT status FROM jobs WHERE id='job-code'`).Scan(&st)
	if st != "CANCELLED" {
		t.Fatalf("job QUEUED của lần chạy phải CANCELLED, có %s", st)
	}
	r.db.QueryRow(`SELECT status FROM jobs WHERE id='job-other'`).Scan(&st)
	if st != "QUEUED" {
		t.Fatalf("job của dự án khác không được chạm, có %s", st)
	}
	for id, want := range map[string]string{root: "REJECTED", "sub1": "REJECTED", "sub2": "DONE"} {
		r.db.QueryRow(`SELECT status FROM tasks WHERE id=?`, id).Scan(&st)
		if st != want {
			t.Fatalf("task %s = %s, muốn %s", id, st, want)
		}
	}
	if len(r.ops.cleaned) != 1 {
		t.Fatalf("vẫn phải gọi Cleanup GitHub đúng 1 lần: %v", r.ops.cleaned)
	}
}
