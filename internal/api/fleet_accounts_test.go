package api_test

// Test nghiệm thu TASK-049 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-accounts.md §6 (DELETE /api/admin/agents/{id}, fleet chỉ trả agent ever_ok và chưa xoá); TC-100.

import (
	"encoding/json"
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
)

func TestFleetHidesNeverOKAndRemoved(t *testing.T) {
	r := newRig(t)
	r.machine("m", dispatcher.KindClaude)
	r.exec(`INSERT INTO agents (id, worker_id, kind, name, ever_ok) VALUES ('m/claude-2', 'm', 'CLAUDE', 'claude-2', 1)`)
	r.exec(`INSERT INTO agents (id, worker_id, kind, name, ever_ok, auth_status) VALUES ('m/claude-3', 'm', 'CLAUDE', 'claude-3', 0, 'NEEDS_AUTH')`)
	r.exec(`INSERT INTO agents (id, worker_id, kind, name, ever_ok, removed_at) VALUES ('m/claude-4', 'm', 'CLAUDE', 'claude-4', 1, CURRENT_TIMESTAMP)`)

	code, e := r.do("GET", "/api/fleet", "ADMIN", nil)
	if code != 200 {
		t.Fatalf("GET /api/fleet = %d", code)
	}
	var f struct {
		Workers []struct {
			ID     string `json:"id"`
			Agents []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"agents"`
		} `json:"workers"`
	}
	if err := json.Unmarshal(e.Data, &f); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, w := range f.Workers {
		for _, a := range w.Agents {
			got[a.ID] = a.Name
		}
	}
	if len(got) != 2 || got["m/claude"] != "claude" || got["m/claude-2"] != "claude-2" {
		t.Fatalf("fleet phải chỉ có m/claude, m/claude-2 (kèm name): %v", got)
	}
}

func TestDeleteAgent(t *testing.T) { // TC-100
	r := newRig(t)
	r.machine("m", dispatcher.KindClaude)
	r.exec(`INSERT INTO agents (id, worker_id, kind, name, ever_ok) VALUES ('m/claude-2', 'm', 'CLAUDE', 'claude-2', 1)`)

	if code, _ := r.do("DELETE", "/api/admin/agents/m%2Fclaude-2", "PARTNER", nil); code != 403 {
		t.Fatalf("PARTNER xoá ở fleet = %d, muốn 403", code)
	}
	if code, e := r.do("DELETE", "/api/admin/agents/m%2Fkhong-co", "ADMIN", nil); code != 404 || e.Error == nil || e.Error.Code != "NOT_FOUND" {
		t.Fatalf("agent không có = %d %+v, muốn 404 NOT_FOUND", code, e)
	}

	// Đang chạy việc → 409.
	if _, err := r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: dispatcher.JobCode, RequiredAgent: dispatcher.KindClaude, BranchName: "feat/t1"}); err != nil {
		t.Fatal(err)
	}
	a, err := r.disp.RequestJob("m", "m/claude-2")
	if err != nil || a == nil {
		t.Fatalf("RequestJob: %v %v", a, err)
	}
	if code, e := r.do("DELETE", "/api/admin/agents/m%2Fclaude-2", "ADMIN", nil); code != 409 || e.Error == nil || e.Error.Code != "CONFLICT" {
		t.Fatalf("agent đang chạy việc = %d %+v, muốn 409 CONFLICT", code, e)
	}
	if err := r.disp.Submit("m", a.ID); err != nil {
		t.Fatal(err)
	}

	code, e := r.do("DELETE", "/api/admin/agents/m%2Fclaude-2", "ADMIN", nil)
	if code != 200 || !e.Success {
		t.Fatalf("ADMIN xoá = %d %+v", code, e)
	}
	var body struct {
		Removed string `json:"removed"`
	}
	json.Unmarshal(e.Data, &body)
	if body.Removed != "m/claude-2" {
		t.Fatalf("data.removed = %q", body.Removed)
	}
	for _, w := range r.fleet().Workers {
		for _, ag := range w.Agents {
			if ag.ID == "m/claude-2" {
				t.Fatal("agent đã xoá vẫn hiện trong fleet")
			}
		}
	}
	if code, _ := r.do("DELETE", "/api/admin/agents/m%2Fclaude-2", "ADMIN", nil); code != 404 {
		t.Fatalf("xoá lần 2 = %d, muốn 404", code)
	}
}
