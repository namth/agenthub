package api_test

// Test nghiệm thu TASK-030 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-auth.md §6 (GET /api/fleet thêm gh_auth, auth_status, auth_detail, auth_checked_at).

import (
	"encoding/json"
	"testing"
)

func TestFleetAuthFields(t *testing.T) {
	r := newRig(t)
	r.machine("mac", "CLAUDE", "ANTIGRAVITY")
	r.exec(`UPDATE workers SET gh_auth='NEEDS_AUTH' WHERE id='mac'`)
	r.exec(`UPDATE agents SET auth_status='NEEDS_AUTH', auth_detail='claude: loggedIn=false', auth_checked_at='2026-11-04 09:00:00' WHERE id='mac/claude'`)
	code, e := r.do("GET", "/api/fleet", "ADMIN", nil)
	if code != 200 {
		t.Fatalf("GET /api/fleet = %d", code)
	}
	var f struct {
		Workers []struct {
			ID     string `json:"id"`
			GhAuth string `json:"gh_auth"`
			Agents []struct {
				ID            string  `json:"id"`
				AuthStatus    string  `json:"auth_status"`
				AuthDetail    *string `json:"auth_detail"`
				AuthCheckedAt *string `json:"auth_checked_at"`
			} `json:"agents"`
		} `json:"workers"`
	}
	if err := json.Unmarshal(e.Data, &f); err != nil || len(f.Workers) != 1 {
		t.Fatalf("data = %s (%v)", e.Data, err)
	}
	w := f.Workers[0]
	if w.GhAuth != "NEEDS_AUTH" {
		t.Fatalf("gh_auth = %q", w.GhAuth)
	}
	got := map[string]string{}
	for _, a := range w.Agents {
		got[a.ID] = a.AuthStatus
		if a.ID == "mac/claude" && (a.AuthDetail == nil || *a.AuthDetail != "claude: loggedIn=false" || a.AuthCheckedAt == nil) {
			t.Fatalf("agent claude = %+v", a)
		}
		if a.ID == "mac/antigravity" && a.AuthDetail != nil {
			t.Fatalf("agent agy auth_detail phải null: %+v", a)
		}
	}
	if got["mac/claude"] != "NEEDS_AUTH" || got["mac/antigravity"] != "UNKNOWN" {
		t.Fatalf("auth_status = %v", got)
	}
}
