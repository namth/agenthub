package tasks_test

// Test nghiệm thu TASK-033 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/virtual-office.md §3 (approve tại bàn người duyệt) — hook WithOnApproved.

import (
	"sync"
	"testing"

	"github.com/namth/agenthub/internal/tasks"
)

func TestOnApprovedHook(t *testing.T) {
	r := newRig(t)
	var mu sync.Mutex
	var got []string
	svc := tasks.NewService(r.db, r.gh, r.disp, "p1", tasks.WithOnApproved(func(taskID, userID string) {
		mu.Lock()
		got = append(got, taskID+"|"+userID)
		mu.Unlock()
	}))
	r.h = tasks.HandlerWithService(svc, authz)
	id := r.create("partner1:PARTNER", "Xuất báo cáo PDF", desc)
	if code, _ := r.do("POST", "/api/tasks/"+id+"/approve", "partner1:PARTNER", nil); code != 403 {
		t.Fatalf("partner duyệt = %d", code)
	}
	if code, e := r.do("POST", "/api/tasks/"+id+"/approve", "admin1:ADMIN", nil); code != 200 {
		t.Fatalf("approve = %d %+v", code, e.Error)
	}
	if code, _ := r.do("POST", "/api/tasks/"+id+"/approve", "admin1:ADMIN", nil); code != 409 {
		t.Fatalf("duyệt lại = %d, muốn 409", code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != id+"|admin1" {
		t.Fatalf("OnApproved = %v, muốn đúng 1 lần [%s|admin1] (không gọi khi bị từ chối/lỗi)", got, id)
	}
}
