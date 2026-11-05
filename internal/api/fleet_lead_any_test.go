package api_test

// Test nghiệm thu TASK-032 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: DEC-29 — lead mọi loại agent qua API Fleet.

import "testing"

func TestAdminAssignAntigravityWhenEligible(t *testing.T) {
	r := newRig(t)
	r.machine("a", "CLAUDE", "ANTIGRAVITY")
	if code, e := r.do("POST", "/api/admin/lead/assign", "ADMIN", map[string]any{"agent_id": "a/antigravity"}); code != 409 || e.Error.Code != "AGENT_NOT_ELIGIBLE" {
		t.Fatalf("agy lead_eligible=0: %d %+v, muốn 409", code, e.Error)
	}
	if code, e := r.do("PATCH", "/api/admin/agents/a%2Fantigravity", "ADMIN", map[string]any{"lead_eligible": true, "lead_priority": 2}); code != 200 {
		t.Fatalf("bật lead cho agy: %d %+v", code, e.Error)
	}
	var lp int
	r.db.QueryRow(`SELECT lead_priority FROM agents WHERE id='a/antigravity'`).Scan(&lp)
	if lp != 2 {
		t.Fatalf("lead_priority = %d, muốn 2", lp)
	}
	code, e := r.do("POST", "/api/admin/lead/assign", "ADMIN", map[string]any{"agent_id": "a/antigravity"})
	if code != 200 {
		t.Fatalf("trao vai cho agy đã bật: %d %+v", code, e.Error)
	}
	if l, _ := r.disp.Lead(); l.AgentID != "a/antigravity" {
		t.Fatalf("lead = %+v", l)
	}
}
