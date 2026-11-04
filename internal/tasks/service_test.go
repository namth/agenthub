package tasks_test

// Test nghiệm thu TASK-011 — do trưởng phòng viết, file bị khoá (tests.lock).

import (
	"context"
	"errors"
	"testing"

	"github.com/namth/agenthub/internal/tasks"
)

func TestServiceApproveRejectShareRules(t *testing.T) {
	r := newRig(t)
	svc := tasks.NewService(r.db, r.gh, r.disp, "p1")
	id := r.create("partner1:PARTNER", "Xuất báo cáo PDF", desc)
	ctx := context.Background()
	if err := svc.Approve(ctx, id); err != nil {
		t.Fatal(err)
	}
	if r.status(id) != "PLANNING" {
		t.Fatalf("status = %s", r.status(id))
	}
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='SPEC'`, id).Scan(&n)
	if n != 1 {
		t.Fatalf("job SPEC = %d", n)
	}
	if err := svc.Approve(ctx, id); !errors.Is(err, tasks.ErrInvalidTransition) {
		t.Fatalf("duyệt lần hai: %v, muốn ErrInvalidTransition", err)
	}
	if err := svc.Approve(ctx, "khong-co"); !errors.Is(err, tasks.ErrNotFound) {
		t.Fatalf("không có task: %v, muốn ErrNotFound", err)
	}
	id2 := r.create("partner1:PARTNER", "Task thứ hai", desc)
	if err := svc.Reject(ctx, id2, " "); !errors.Is(err, tasks.ErrValidation) {
		t.Fatalf("lý do rỗng: %v, muốn ErrValidation", err)
	}
	if err := svc.Reject(ctx, id2, "Từ chối qua Telegram"); err != nil {
		t.Fatal(err)
	}
	if r.status(id2) != "REJECTED" || r.gh.closed[102] == "" {
		t.Fatalf("reject: %s, closed %q", r.status(id2), r.gh.closed[102])
	}
}

func TestServiceCreateHookAndErrors(t *testing.T) {
	r := newRig(t)
	var got []tasks.Created
	svc := tasks.NewService(r.db, r.gh, r.disp, "p1", tasks.WithOnCreated(func(c tasks.Created) { got = append(got, c) }))
	ctx := context.Background()
	task, err := svc.Create(ctx, "partner1", "  Xuất báo cáo PDF ", desc)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == "" || task.Status != "PENDING_APPROVAL" || task.GitHubIssueID != 101 || task.Title != "Xuất báo cáo PDF" {
		t.Fatalf("task = %+v", task)
	}
	if len(got) != 1 || got[0].TaskID != task.ID || got[0].Issue != 101 || got[0].Title != "Xuất báo cáo PDF" ||
		got[0].Requester != "doi-tac" || got[0].Description != desc {
		t.Fatalf("OnCreated = %+v (requester là username của người tạo)", got)
	}
	if _, err := svc.Create(ctx, "partner1", "ngắn", desc); !errors.Is(err, tasks.ErrValidation) {
		t.Fatalf("tiêu đề ngắn: %v", err)
	}
	r.gh.fail = true
	if _, err := svc.Create(ctx, "partner1", "Tiêu đề hợp lệ", desc); !errors.Is(err, tasks.ErrGitHub) {
		t.Fatalf("GitHub lỗi: %v, muốn ErrGitHub", err)
	}
	if len(got) != 1 {
		t.Fatal("tạo thất bại không được gọi OnCreated")
	}
}

func TestHandlerUsesServiceHook(t *testing.T) {
	r := newRig(t)
	var got []tasks.Created
	svc := tasks.NewService(r.db, r.gh, r.disp, "p1", tasks.WithOnCreated(func(c tasks.Created) { got = append(got, c) }))
	r.h = tasks.HandlerWithService(svc, authz)
	r.create("admin1:ADMIN", "Tạo qua HTTP", desc)
	if len(got) != 1 || got[0].Requester != "namth" {
		t.Fatalf("HTTP tạo task cũng phải gọi OnCreated: %+v", got)
	}
}
