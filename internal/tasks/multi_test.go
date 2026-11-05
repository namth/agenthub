package tasks_test

// Test nghiệm thu TASK-026 — do trưởng phòng viết, file bị khoá (tests.lock).
// FEAT-04: BR-24, TC-38, TC-40, TC-45; DEC-17 (vai theo dự án).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/namth/agenthub/internal/tasks"
)

type multiRig struct {
	*rig
	ghs  map[string]*fakeGH // project id → GitHub giả
	svc  *tasks.Service
	h    http.Handler
	wh   http.Handler
	mrgd map[string][]int
}

// Vai giả: owner (admin1) ADMIN mọi dự án; partner1 PARTNER ở p1, không thuộc p2; partner2 ADMIN ở p2.
func roleIn(userID, projectID string) (string, bool) {
	switch {
	case userID == "admin1":
		return "ADMIN", true
	case userID == "partner1" && projectID == "p1":
		return "PARTNER", true
	case userID == "partner2" && projectID == "p2":
		return "ADMIN", true
	}
	return "", false
}

type mm struct {
	id   string
	list map[string][]int
}

func (m mm) MergePR(_ context.Context, n int) error {
	m.list[m.id] = append(m.list[m.id], n)
	return nil
}

func newMultiRig(t *testing.T) *multiRig {
	r := newRig(t)
	r.db.Exec(`INSERT INTO projects (id, name, github_repo) VALUES ('p2', 'Edu', 'namth/edusystem')`)
	r.db.Exec(`INSERT INTO users (id, github_id, username, role) VALUES ('partner2', 'g3', 'doi-tac-2', 'PARTNER')`)
	m := &multiRig{rig: r, ghs: map[string]*fakeGH{
		"p1": {labels: map[int][]string{}, closed: map[int]string{}},
		"p2": {next: 50, labels: map[int][]string{}, closed: map[int]string{}},
	}, mrgd: map[string][]int{}}
	m.svc = tasks.NewMultiService(r.db, func(projectID string) (tasks.GitHub, error) { return m.ghs[projectID], nil }, r.disp,
		tasks.WithRoleIn(roleIn), tasks.WithOwner(func(userID string) bool { return userID == "admin1" }),
		tasks.WithMergerFor(func(projectID string) tasks.Merger { return mm{projectID, m.mrgd} }))
	m.h = tasks.HandlerWithService(m.svc, authz)
	m.wh = tasks.WebhookHandlerMulti(r.db, secret, func(fullName string) (string, bool) {
		switch fullName {
		case "namth/agenthub":
			return "p1", true
		case "namth/edusystem":
			return "p2", true
		}
		return "", false
	})
	return m
}

func (m *multiRig) req(method, path, user string, body any) (int, env) {
	m.t.Helper()
	m.rig.h = m.h
	return m.rig.do(method, path, user, body)
}

func TestCreateInProjectUsesThatRepo(t *testing.T) { // TC-45
	m := newMultiRig(t)
	code, e := m.req("POST", "/api/tasks", "partner2:PARTNER", map[string]any{"project_id": "p2", "title": "Xuất điểm học sinh", "description": desc})
	if code != 201 {
		t.Fatalf("tạo task p2 = %d %+v", code, e.Error)
	}
	var d struct {
		ID            string `json:"id"`
		ProjectID     string `json:"project_id"`
		GitHubIssueID int    `json:"github_issue_id"`
	}
	json.Unmarshal(e.Data, &d)
	if d.ProjectID != "p2" || d.GitHubIssueID != 151 || len(m.ghs["p2"].created) != 1 || len(m.ghs["p1"].created) != 0 {
		t.Fatalf("issue phải tạo trên repo của dự án p2: %+v, p1=%v p2=%v", d, m.ghs["p1"].created, m.ghs["p2"].created)
	}
	if code, e := m.req("POST", "/api/tasks", "partner1:PARTNER", map[string]any{"project_id": "p2", "title": "Không được phép", "description": desc}); code != 403 || e.Error.Code != "FORBIDDEN" {
		t.Fatalf("không là thành viên p2: %d", code)
	}
	if code, e := m.req("POST", "/api/tasks", "partner1:PARTNER", map[string]any{"title": "Thiếu dự án", "description": desc}); code != 400 || e.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("thiếu project_id: %d", code)
	}
}

func TestKanbanScopedByProject(t *testing.T) { // TC-38
	m := newMultiRig(t)
	m.req("POST", "/api/tasks", "admin1:ADMIN", map[string]any{"project_id": "p1", "title": "Task của p1", "description": desc})
	m.req("POST", "/api/tasks", "admin1:ADMIN", map[string]any{"project_id": "p2", "title": "Task của p2", "description": desc})
	count := func(user, q string) (int, int) {
		code, e := m.req("GET", "/api/tasks"+q, user, nil)
		var b board
		json.Unmarshal(e.Data, &b)
		n := 0
		for _, c := range b.Columns {
			n += len(c.Tasks)
		}
		return code, n
	}
	if code, n := count("partner1:PARTNER", "?project_id=p1"); code != 200 || n != 1 {
		t.Fatalf("partner1 xem p1: %d %d", code, n)
	}
	if code, _ := count("partner1:PARTNER", "?project_id=p2"); code != 403 {
		t.Fatalf("partner1 xem p2: %d, muốn 403", code)
	}
	if code, _ := count("partner1:PARTNER", ""); code != 400 {
		t.Fatalf("không phải owner mà thiếu project_id: %d, muốn 400", code)
	}
	if code, n := count("admin1:ADMIN", ""); code != 200 || n != 2 {
		t.Fatalf("owner không truyền project_id = mọi dự án: %d %d", code, n)
	}
}

func TestApproveMergeRoleByProject(t *testing.T) {
	m := newMultiRig(t)
	_, e := m.req("POST", "/api/tasks", "partner2:PARTNER", map[string]any{"project_id": "p2", "title": "Task của p2", "description": desc})
	var d struct{ ID string }
	json.Unmarshal(e.Data, &d)
	if code, _ := m.req("POST", "/api/tasks/"+d.ID+"/approve", "partner1:PARTNER", nil); code != 403 {
		t.Fatalf("partner1 duyệt task p2: %d", code)
	}
	if code, e := m.req("POST", "/api/tasks/"+d.ID+"/approve", "partner2:PARTNER", nil); code != 200 {
		t.Fatalf("ADMIN của p2 (không phải owner) duyệt được task p2: %d %+v", code, e.Error)
	}
	if got := m.ghs["p2"].labels[151]; len(got) == 0 {
		t.Fatal("nhãn cập nhật trên repo p2")
	}
	m.db.Exec(`UPDATE tasks SET status='QA_PASSED', pr_url='https://github.com/namth/edusystem/pull/7' WHERE id=?`, d.ID)
	if code, _ := m.req("POST", "/api/tasks/"+d.ID+"/merge", "partner2:PARTNER", nil); code != 200 {
		t.Fatalf("merge: %d", code)
	}
	if len(m.mrgd["p2"]) != 1 || m.mrgd["p2"][0] != 7 || len(m.mrgd["p1"]) != 0 {
		t.Fatalf("merge phải dùng merger của dự án p2: %v", m.mrgd)
	}
	var jp string
	m.db.QueryRow(`SELECT project_id FROM jobs WHERE task_id=? AND kind='SPEC'`, d.ID).Scan(&jp)
	if jp != "p2" {
		t.Fatalf("job SPEC phải thuộc dự án của task: %q", jp)
	}
}

func TestWebhookRoutesByRepository(t *testing.T) { // TC-40
	m := newMultiRig(t)
	post := func(payload map[string]any) int {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/webhooks/github", bytes.NewReader(body))
		req.Header.Set("X-GitHub-Event", "issues")
		req.Header.Set("X-Hub-Signature-256", sign(body))
		rr := httptest.NewRecorder()
		m.wh.ServeHTTP(rr, req)
		return rr.Code
	}
	ev := issueEvent("opened", 900, "Từ repo edu")
	ev["repository"] = map[string]any{"full_name": "namth/edusystem"}
	post(ev)
	var pid string
	m.db.QueryRow(`SELECT project_id FROM tasks WHERE github_issue_id=900`).Scan(&pid)
	if pid != "p2" {
		t.Fatalf("issue repo edusystem phải vào dự án p2, nhận %q", pid)
	}
	lạ := issueEvent("opened", 901, "Repo lạ")
	lạ["repository"] = map[string]any{"full_name": "nguoi-khac/repo"}
	if code := post(lạ); code != 202 && code != 200 && code != 204 {
		t.Fatalf("repo không quản lý: %d", code)
	}
	var n int
	m.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE github_issue_id=901`).Scan(&n)
	if n != 0 {
		t.Fatal("repo không quản lý không được tạo task")
	}
}
