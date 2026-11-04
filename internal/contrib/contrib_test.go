package contrib_test

// Test nghiệm thu TASK-025 — do trưởng phòng viết, file bị khoá (tests.lock). DEC-21, BR-36, TC-56.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/namth/agenthub/internal/contrib"
	"github.com/namth/agenthub/internal/database"
)

func seed(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	q := []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-owner','1','namth','ADMIN'), ('u-a','2','doi-tac-a','PARTNER')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1','agenthub','namth/agenthub'), ('p2','edu','namth/edusystem')`,
		`INSERT INTO project_members (project_id, user_id, role) VALUES ('p1','u-a','PARTNER')`,
		`INSERT INTO workers (id, name, os, token_hash, owner_user_id) VALUES ('mac','mac','darwin','h','u-owner'), ('mb-a','mb-a','darwin','h','u-a'), ('cu','cu','linux','h',NULL)`,
		`INSERT INTO agents (id, worker_id, kind) VALUES ('mac/claude','mac','CLAUDE'), ('mac/antigravity','mac','ANTIGRAVITY'), ('mb-a/antigravity','mb-a','ANTIGRAVITY'), ('cu/antigravity','cu','ANTIGRAVITY')`,
		`INSERT INTO tasks (id, project_id, title, description, created_by, status, pr_url, updated_at) VALUES
			('t1','p1','Schema','x','u-owner','DONE','https://x/pull/1','2026-11-04 10:00:00'),
			('t2','p1','API','x','u-owner','WAITING_QA',NULL,'2026-11-04 10:00:00'),
			('t3','p2','Edu','x','u-owner','DONE','https://x/pull/3','2026-11-04 11:00:00'),
			('t9','p1','Cũ','x','u-owner','DONE','https://x/pull/9','2026-10-01 10:00:00')`,
		`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status) VALUES
			('j1','p1','t1','CODE','ANTIGRAVITY','DONE'), ('j2','p1','t2','CODE','ANTIGRAVITY','RUNNING'),
			('j3','p2','t3','CODE','ANTIGRAVITY','DONE'), ('j4','p1','t1','QA_REVIEW','CLAUDE','DONE'), ('j9','p1','t9','CODE','ANTIGRAVITY','DONE')`,
		// mb-a (partner a): j1 lần 1 hết limit (30 phút), j2 đang chạy; mac (owner): j1 lần 2 xong (1 giờ), j3 xong (2 giờ), j4 QA xong (30 phút)
		`INSERT INTO assignments (id, job_id, agent_id, attempt, status, started_at, ended_at) VALUES
			('a1','j1','mb-a/antigravity',1,'EXHAUSTED','2026-11-04 08:00:00','2026-11-04 08:30:00'),
			('a2','j1','mac/antigravity',2,'SUBMITTED','2026-11-04 08:40:00','2026-11-04 09:40:00'),
			('a3','j2','mb-a/antigravity',1,'ACTIVE','2026-11-04 09:50:00',NULL),
			('a4','j3','mac/antigravity',1,'SUBMITTED','2026-11-04 09:00:00','2026-11-04 11:00:00'),
			('a5','j4','mac/claude',1,'SUBMITTED','2026-11-04 09:45:00','2026-11-04 10:15:00'),
			('a6','j9','cu/antigravity',1,'SUBMITTED','2026-10-01 08:00:00','2026-10-01 09:00:00')`,
	}
	for _, s := range q {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	return db
}

func serve(t *testing.T, db *sql.DB) *httptest.Server {
	h := contrib.Handler(db, func(r *http.Request) (string, bool, bool) {
		u := r.Header.Get("X-Test-User")
		return u, u == "u-owner", u != ""
	})
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

type row struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Machines int    `json:"machines"`
	Jobs     struct {
		Done      int            `json:"done"`
		Failed    int            `json:"failed"`
		Exhausted int            `json:"exhausted"`
		ByKind    map[string]int `json:"by_kind"`
	} `json:"jobs"`
	TasksDone int     `json:"tasks_done"`
	PRsMerged int     `json:"prs_merged"`
	BusyHours float64 `json:"busy_hours"`
}

func get(t *testing.T, s *httptest.Server, user, path string, out any) int {
	t.Helper()
	req, _ := http.NewRequest("GET", s.URL+path, nil)
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var e struct {
		Data json.RawMessage `json:"data"`
	}
	json.NewDecoder(res.Body).Decode(&e)
	if out != nil {
		json.Unmarshal(e.Data, out)
	}
	return res.StatusCode
}

const day = "from=2026-11-04T00:00:00Z&to=2026-11-05T00:00:00Z"

func byKey(rows []row) map[string]row {
	m := map[string]row{}
	for _, r := range rows {
		m[r.Key] = r
	}
	return m
}

func TestByPartnerOwnerView(t *testing.T) {
	s := serve(t, seed(t))
	var rows []row
	if code := get(t, s, "u-owner", "/api/contributions?by=partner&"+day, &rows); code != 200 {
		t.Fatalf("status %d", code)
	}
	m := byKey(rows)
	o, a := m["u-owner"], m["u-a"]
	if o.Label != "namth" || o.Machines != 1 || o.Jobs.Done != 3 || o.Jobs.ByKind["CODE"] != 2 || o.Jobs.ByKind["QA_REVIEW"] != 1 {
		t.Fatalf("owner = %+v", o)
	}
	if o.BusyHours < 3.49 || o.BusyHours > 3.51 {
		t.Fatalf("giờ máy owner = %.2f, muốn 3.5 (1 + 2 + 0.5)", o.BusyHours)
	}
	if o.TasksDone != 2 || o.PRsMerged != 2 {
		t.Fatalf("owner task DONE / PR merge = %d / %d, muốn 2 / 2 (t1, t3)", o.TasksDone, o.PRsMerged)
	}
	if a.Label != "doi-tac-a" || a.Machines != 1 || a.Jobs.Done != 0 || a.Jobs.Exhausted != 1 || a.TasksDone != 0 {
		t.Fatalf("partner a = %+v", a)
	}
	if a.BusyHours < 0.49 || a.BusyHours > 0.51 {
		t.Fatalf("giờ máy partner = %.2f, muốn 0.5 (assignment đang chạy chưa tính)", a.BusyHours)
	}
	if _, ok := m["system"]; ok {
		t.Fatal("máy không chủ (cu) không có việc trong khoảng lọc thì không hiện")
	}
}

func TestByProjectAndRange(t *testing.T) {
	s := serve(t, seed(t))
	var rows []row
	get(t, s, "u-owner", "/api/contributions?by=project&"+day, &rows)
	m := byKey(rows)
	if m["p1"].Label != "namth/agenthub" || m["p1"].Jobs.Done != 2 || m["p1"].TasksDone != 1 || m["p2"].Jobs.Done != 1 {
		t.Fatalf("theo dự án = %+v", m)
	}
	get(t, s, "u-owner", "/api/contributions?by=partner&from=2026-10-01T00:00:00Z&to=2026-10-02T00:00:00Z", &rows)
	m = byKey(rows)
	if sys, ok := m["system"]; !ok || sys.Label != "Hệ thống" || sys.Jobs.Done != 1 {
		t.Fatalf("máy không chủ quy về 'Hệ thống': %+v", m)
	}
}

func TestPartnerScope(t *testing.T) {
	s := serve(t, seed(t))
	var rows []row
	get(t, s, "u-a", "/api/contributions?by=partner&"+day, &rows)
	if len(rows) != 1 || rows[0].Key != "u-a" {
		t.Fatalf("partner chỉ thấy phần mình: %+v", rows)
	}
	get(t, s, "u-a", "/api/contributions?by=project&"+day, &rows)
	if len(rows) != 1 || rows[0].Key != "p1" {
		t.Fatalf("partner chỉ thấy dự án mình là thành viên: %+v", rows)
	}
	if code := get(t, s, "", "/api/contributions?by=partner", nil); code != 401 {
		t.Fatalf("chưa đăng nhập: %d", code)
	}
	if code := get(t, s, "u-owner", "/api/contributions?by=gi-do", nil); code != 400 {
		t.Fatalf("by sai: %d", code)
	}
}

func TestTimeline(t *testing.T) {
	s := serve(t, seed(t))
	var items []map[string]any
	get(t, s, "u-owner", "/api/contributions/timeline?partner=u-a&"+day, &items)
	if len(items) != 2 {
		t.Fatalf("dòng thời gian partner a = %d mục, muốn 2: %+v", len(items), items)
	}
	if items[0]["at"] == nil || !strings.HasPrefix(items[0]["at"].(string), "2026-11-04T09:50") {
		t.Fatalf("mới nhất trước: %+v", items[0])
	}
	first := items[1]
	if first["worker"] != "mb-a" || first["agent"] != "mb-a/antigravity" || first["job_kind"] != "CODE" || first["task_title"] != "Schema" ||
		first["project"] != "namth/agenthub" || first["result"] != "EXHAUSTED" || first["attempt"] != float64(1) {
		t.Fatalf("mục = %+v", first)
	}
	if code := get(t, s, "u-a", "/api/contributions/timeline?partner=u-owner&"+day, &items); code != 403 {
		t.Fatalf("partner xem dòng thời gian người khác: %d", code)
	}
	get(t, s, "u-owner", "/api/contributions/timeline?project=p2&"+day, &items)
	if len(items) != 1 || items[0]["task_title"] != "Edu" {
		t.Fatalf("theo dự án p2: %+v", items)
	}
}
