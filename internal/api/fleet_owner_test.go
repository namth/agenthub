package api_test

// Test nghiệm thu TASK-028 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/join-machine.md §4 (SCR-05 nhóm theo chủ, công tắc trưởng phòng, cột phiên bản),
// docs/features/multi-project.md §4 (agent chọn dự án), §6 (PATCH /api/admin/agents/{id}).

import (
	"encoding/json"
	"sort"
	"testing"
)

type fleetX struct {
	Workers []struct {
		ID      string  `json:"id"`
		Version *string `json:"version"`
		Paused  bool    `json:"paused"`
		Owner   *struct {
			ID    string `json:"id"`
			Login string `json:"login"`
		} `json:"owner"`
		Agents []struct {
			ID           string    `json:"id"`
			LeadEligible bool      `json:"lead_eligible"`
			Projects     *[]string `json:"projects"`
			CurrentJob   *struct {
				TaskID    string `json:"task_id"`
				ProjectID string `json:"project_id"`
				Project   string `json:"project"`
				TaskTitle string `json:"task_title"`
			} `json:"current_job"`
		} `json:"agents"`
	} `json:"workers"`
}

func (r *rig) fleetX() fleetX {
	r.t.Helper()
	code, e := r.do("GET", "/api/fleet", "ADMIN", nil)
	if code != 200 {
		r.t.Fatalf("GET /api/fleet = %d", code)
	}
	var f fleetX
	if err := json.Unmarshal(e.Data, &f); err != nil {
		r.t.Fatal(err)
	}
	return f
}

func TestFleetOwnerVersionPaused(t *testing.T) {
	r := newRig(t)
	r.exec(`INSERT INTO users (id, github_id, username, role) VALUES ('u2', 'gh2', 'doi-tac', 'PARTNER')`)
	r.machine("he-thong", "CLAUDE")
	r.machine("may-doi-tac", "ANTIGRAVITY")
	r.exec(`UPDATE workers SET owner_user_id='u2', version='v1.2.3', paused=1 WHERE id='may-doi-tac'`)

	f := r.fleetX()
	if len(f.Workers) != 2 {
		t.Fatalf("workers = %d", len(f.Workers))
	}
	byID := map[string]int{}
	for i, w := range f.Workers {
		byID[w.ID] = i
	}
	sys := f.Workers[byID["he-thong"]]
	if sys.Owner != nil || sys.Version != nil || sys.Paused {
		t.Fatalf("máy hệ thống: owner=null, version=null, paused=false; có %+v", sys)
	}
	p := f.Workers[byID["may-doi-tac"]]
	if p.Owner == nil || p.Owner.ID != "u2" || p.Owner.Login != "doi-tac" {
		t.Fatalf("owner = %+v, muốn {u2 doi-tac}", p.Owner)
	}
	if p.Version == nil || *p.Version != "v1.2.3" || !p.Paused {
		t.Fatalf("version/paused = %v %v", p.Version, p.Paused)
	}
}

func TestFleetAgentLeadEligibleAndProjects(t *testing.T) {
	r := newRig(t)
	r.exec(`INSERT INTO projects (id, name, github_repo) VALUES ('p2', 'Edu', 'namth/edusystem')`)
	r.machine("mac", "CLAUDE", "ANTIGRAVITY")
	r.exec(`UPDATE agents SET lead_eligible=0 WHERE id='mac/antigravity'`)
	r.exec(`INSERT INTO agent_projects (agent_id, project_id) VALUES ('mac/antigravity','p2'), ('mac/antigravity','p1')`)

	f := r.fleetX()
	ags := f.Workers[0].Agents
	if len(ags) != 2 {
		t.Fatalf("agents = %d", len(ags))
	}
	for _, a := range ags {
		switch a.ID {
		case "mac/claude":
			if !a.LeadEligible {
				t.Errorf("mac/claude lead_eligible mặc định true")
			}
			if a.Projects != nil {
				t.Errorf("không có dòng agent_projects → projects = null (mọi dự án), có %v", *a.Projects)
			}
		case "mac/antigravity":
			if a.LeadEligible {
				t.Errorf("mac/antigravity lead_eligible = false")
			}
			if a.Projects == nil {
				t.Fatalf("projects phải là mảng")
			}
			got := append([]string{}, *a.Projects...)
			sort.Strings(got)
			if len(got) != 2 || got[0] != "p1" || got[1] != "p2" {
				t.Errorf("projects = %v, muốn [p1 p2]", got)
			}
		}
	}
}

func TestFleetCurrentJobProject(t *testing.T) {
	r := newRig(t)
	r.machine("mac", "CLAUDE")
	r.exec(`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status) VALUES ('j1', 'p1', 't1', 'CODE', 'CLAUDE', 'RUNNING')`)
	r.exec(`INSERT INTO assignments (id, job_id, agent_id, attempt, status) VALUES ('a1', 'j1', 'mac/claude', 1, 'ACTIVE')`)

	f := r.fleetX()
	cj := f.Workers[0].Agents[0].CurrentJob
	if cj == nil {
		t.Fatal("current_job = nil")
	}
	if cj.TaskID != "t1" || cj.ProjectID != "p1" || cj.Project != "namth/agenthub" || cj.TaskTitle != "A" {
		t.Fatalf("current_job = %+v, muốn task t1 'A' dự án p1 namth/agenthub", cj)
	}
}

func TestPatchAgentLeadEligible(t *testing.T) {
	r := newRig(t)
	r.machine("mac", "CLAUDE")
	if code, e := r.do("PATCH", "/api/admin/agents/mac%2Fclaude", "ADMIN", map[string]any{"lead_eligible": false}); code != 200 || !e.Success {
		t.Fatalf("PATCH lead_eligible = %d %+v", code, e.Error)
	}
	var le int
	r.db.QueryRow(`SELECT lead_eligible FROM agents WHERE id='mac/claude'`).Scan(&le)
	if le != 0 {
		t.Fatalf("lead_eligible DB = %d", le)
	}
	r.do("PATCH", "/api/admin/agents/mac%2Fclaude", "ADMIN", map[string]any{"lead_eligible": true})
	r.db.QueryRow(`SELECT lead_eligible FROM agents WHERE id='mac/claude'`).Scan(&le)
	if le != 1 {
		t.Fatalf("lead_eligible DB = %d sau khi bật", le)
	}
	for _, bad := range []map[string]any{{"lead_eligible": "yes"}, {"lead_eligible": nil}, {"lead_eligible": 1}} {
		if code, e := r.do("PATCH", "/api/admin/agents/mac%2Fclaude", "ADMIN", bad); code != 400 || e.Error.Code != "VALIDATION_ERROR" {
			t.Errorf("%v: %d, muốn 400 VALIDATION_ERROR", bad, code)
		}
	}
	if code, _ := r.do("PATCH", "/api/admin/agents/mac%2Fclaude", "PARTNER", map[string]any{"lead_eligible": true}); code != 403 {
		t.Fatalf("partner PATCH = %d, muốn 403", code)
	}
}

func TestPatchAgentProjects(t *testing.T) {
	r := newRig(t)
	r.exec(`INSERT INTO projects (id, name, github_repo) VALUES ('p2', 'Edu', 'namth/edusystem')`)
	r.machine("mac", "ANTIGRAVITY")
	count := func() int {
		var n int
		r.db.QueryRow(`SELECT COUNT(*) FROM agent_projects WHERE agent_id='mac/antigravity'`).Scan(&n)
		return n
	}
	if code, e := r.do("PATCH", "/api/admin/agents/mac%2Fantigravity", "ADMIN", map[string]any{"projects": []string{"p1", "p2"}}); code != 200 || !e.Success {
		t.Fatalf("PATCH projects = %d %+v", code, e.Error)
	}
	if count() != 2 {
		t.Fatalf("agent_projects = %d, muốn 2", count())
	}
	// Thay thế toàn bộ, không cộng dồn.
	r.do("PATCH", "/api/admin/agents/mac%2Fantigravity", "ADMIN", map[string]any{"projects": []string{"p2"}})
	var only string
	r.db.QueryRow(`SELECT project_id FROM agent_projects WHERE agent_id='mac/antigravity'`).Scan(&only)
	if count() != 1 || only != "p2" {
		t.Fatalf("thay thế: %d %s", count(), only)
	}
	// null hoặc [] → mọi dự án (xoá hết dòng).
	r.do("PATCH", "/api/admin/agents/mac%2Fantigravity", "ADMIN", map[string]any{"projects": nil})
	if count() != 0 {
		t.Fatalf("null phải xoá hết: %d", count())
	}
	r.do("PATCH", "/api/admin/agents/mac%2Fantigravity", "ADMIN", map[string]any{"projects": []string{"p1"}})
	r.do("PATCH", "/api/admin/agents/mac%2Fantigravity", "ADMIN", map[string]any{"projects": []string{}})
	if count() != 0 {
		t.Fatalf("[] phải xoá hết: %d", count())
	}
	// Dự án không tồn tại hoặc sai kiểu → 400, không đổi gì.
	r.do("PATCH", "/api/admin/agents/mac%2Fantigravity", "ADMIN", map[string]any{"projects": []string{"p1"}})
	for _, bad := range []map[string]any{{"projects": []string{"p1", "khong-co"}}, {"projects": "p1"}, {"projects": []int{1}}} {
		if code, e := r.do("PATCH", "/api/admin/agents/mac%2Fantigravity", "ADMIN", bad); code != 400 || e.Error.Code != "VALIDATION_ERROR" {
			t.Errorf("%v: %d, muốn 400 VALIDATION_ERROR", bad, code)
		}
	}
	if count() != 1 {
		t.Fatalf("body lỗi không được đổi dữ liệu: %d", count())
	}
}
