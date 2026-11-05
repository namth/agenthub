package tasks_test

// Test nghiệm thu TASK-048 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/remote-command.md §6 (proposals accept/reject), BR-64; TC-95.

import (
	"encoding/json"
	"testing"
)

func seedProposal(r *rig) {
	r.t.Helper()
	for _, q := range []string{
		`INSERT INTO tasks (id, project_id, github_issue_id, title, description, status, created_by) VALUES ('t-p', 'p1', 120, 'Báo cáo trùng', 'mô tả đủ dài để hợp lệ', 'PLANNING', 'partner1')`,
		`INSERT INTO tasks (id, project_id, parent_id, github_issue_id, title, description, status, created_by) VALUES ('t-p1', 'p1', 't-p', 121, 'Con', 'mô tả', 'READY', 'partner1')`,
		`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status) VALUES ('jq', 'p1', 't-p1', 'CODE', 'ANTIGRAVITY', 'QUEUED')`,
		`INSERT INTO lead_proposals (id, task_id, kind, reason, proposed_by, epoch) VALUES ('lp1', 't-p', 'CANCEL', 'Trùng #98', 'm/claude', 3)`,
		`INSERT INTO lead_proposals (id, task_id, kind, reason, proposed_by, epoch) VALUES ('lp2', 't-p', 'CANCEL', 'Lý do khác', 'm/claude', 3)`,
	} {
		if _, err := r.db.Exec(q); err != nil {
			r.t.Fatalf("%s: %v", q, err)
		}
	}
}

func TestProposalDecisions(t *testing.T) {
	r := newRig(t)
	seedProposal(r)
	_, e := r.do("GET", "/api/tasks/t-p", "admin1:ADMIN", nil)
	var d struct {
		Proposals []map[string]any `json:"proposals"`
	}
	json.Unmarshal(e.Data, &d)
	if len(d.Proposals) != 2 || d.Proposals[0]["reason"] == nil {
		t.Fatalf("GET task phải có proposals PENDING: %s", e.Data)
	}
	if code, e := r.do("POST", "/api/tasks/t-p/proposals/lp1/accept", "partner1:PARTNER", nil); code != 403 || e.Error.Code != "FORBIDDEN" {
		t.Fatalf("partner xác nhận: %d", code)
	}
	if code, e := r.do("POST", "/api/tasks/t-p/proposals/lp2/reject", "admin1:ADMIN", nil); code != 200 {
		t.Fatalf("reject: %d %+v", code, e.Error)
	}
	if r.status("t-p") != "PLANNING" {
		t.Fatal("từ chối đề xuất không đổi task")
	}
	if code, e := r.do("POST", "/api/tasks/t-p/proposals/lp1/accept", "admin1:ADMIN", nil); code != 200 {
		t.Fatalf("accept: %d %+v", code, e.Error)
	}
	if r.status("t-p") != "REJECTED" || r.status("t-p1") != "REJECTED" {
		t.Fatalf("accept → task và sub-task REJECTED: %s %s", r.status("t-p"), r.status("t-p1"))
	}
	var js, ps string
	r.db.QueryRow(`SELECT status FROM jobs WHERE id='jq'`).Scan(&js)
	r.db.QueryRow(`SELECT status FROM lead_proposals WHERE id='lp1'`).Scan(&ps)
	if js != "CANCELLED" || ps != "ACCEPTED" {
		t.Fatalf("job=%s proposal=%s", js, ps)
	}
	r.gh.mu.Lock()
	_, closed := r.gh.closed[120]
	r.gh.mu.Unlock()
	if !closed {
		t.Fatal("accept phải đóng issue GitHub của task")
	}
	if code, e := r.do("POST", "/api/tasks/t-p/proposals/lp1/accept", "admin1:ADMIN", nil); code != 409 || e.Error.Code != "INVALID_TRANSITION" {
		t.Fatalf("accept lần 2: %d", code)
	}
}
