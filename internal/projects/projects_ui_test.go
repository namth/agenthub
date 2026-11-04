package projects_test

// Test nghiệm thu TASK-028 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/multi-project.md §4 (SCR-09: số task đang mở, nút "Tạo lại webhook"), §6, BR-26.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestListProjectsOpenTasks(t *testing.T) {
	r := newRig(t)
	for _, q := range []string{
		`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t1','p1','a','b','u-owner','IN_PROGRESS')`,
		`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t2','p1','a','b','u-owner','PENDING_APPROVAL')`,
		`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t3','p1','a','b','u-owner','DONE')`,
		`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t4','p1','a','b','u-owner','REJECTED')`,
		`INSERT INTO tasks (id, project_id, parent_id, title, description, created_by, status) VALUES ('t5','p1','t1','a','b','u-owner','READY')`,
	} {
		if _, err := r.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	_, e := r.do("GET", "/api/projects", owner, nil)
	var ps []map[string]any
	json.Unmarshal(e.Data, &ps)
	if len(ps) != 1 {
		t.Fatalf("projects = %s", e.Data)
	}
	// Task gốc (parent_id NULL) chưa DONE/REJECTED.
	if ps[0]["open_tasks"] != float64(2) {
		t.Fatalf("open_tasks = %v, muốn 2", ps[0]["open_tasks"])
	}
}

func TestRecreateWebhook(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.gh.hookErr = errors.New("403 Resource not accessible")
	p, err := r.svc.AddProject(ctx, "namth/edusystem")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/admin/projects/" + p.ID + "/webhook"

	// Vẫn lỗi → 502 GITHUB_ERROR, webhook_id vẫn NULL.
	if code, e := r.do("POST", path, owner, nil); code != 502 || e.Error.Code != "GITHUB_ERROR" {
		t.Fatalf("webhook lỗi: %d %+v, muốn 502 GITHUB_ERROR", code, e.Error)
	}
	r.gh.hookErr = nil
	code, e := r.do("POST", path, owner, nil)
	if code != 200 || !e.Success {
		t.Fatalf("tạo lại = %d %+v", code, e.Error)
	}
	var out map[string]any
	json.Unmarshal(e.Data, &out)
	if out["webhook_ok"] != true || out["id"] != p.ID {
		t.Fatalf("data = %s", e.Data)
	}
	var wid int64
	r.db.QueryRow(`SELECT webhook_id FROM projects WHERE id=?`, p.ID).Scan(&wid)
	if wid <= 0 {
		t.Fatalf("webhook_id = %d", wid)
	}
	last := r.gh.hooks[len(r.gh.hooks)-1]
	if last != "namth/edusystem|https://agent.oa.io.vn/api/webhooks/github|whsec" {
		t.Fatalf("hook = %q", last)
	}
	if code, _ := r.do("POST", path, partner, nil); code != 403 {
		t.Fatalf("partner = %d, muốn 403", code)
	}
	if code, _ := r.do("POST", "/api/admin/projects/khong-co/webhook", owner, nil); code != 404 {
		t.Fatalf("không có = %d, muốn 404", code)
	}
}
