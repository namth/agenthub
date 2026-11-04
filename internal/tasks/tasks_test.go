package tasks_test

// Test nghiệm thu TASK-007 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/04 §1, docs/07 §1.2, docs/08 §1.2, docs/12 §1, docs/03 §4, DEC-11.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/tasks"
)

// ---------- GitHub giả (interface) ----------

type fakeGH struct {
	mu      sync.Mutex
	next    int
	fail    bool
	created []string
	labels  map[int][]string
	closed  map[int]string
}

func (g *fakeGH) CreateIssue(_ context.Context, title, body string, labels []string) (int, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fail {
		return 0, "", errors.New("github 502")
	}
	g.next++
	g.created = append(g.created, title+"|"+strings.Join(labels, ","))
	g.labels[g.next] = labels
	return 100 + g.next, "https://github.com/namth/agenthub/issues/" + string(rune('0'+g.next)), nil
}
func (g *fakeGH) SetLabels(_ context.Context, n int, labels []string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.labels[n] = labels
	return nil
}
func (g *fakeGH) CloseIssue(_ context.Context, n int, comment string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed[n] = comment
	return nil
}

// ---------- dựng ----------

type clk struct{}

func (clk) Now() time.Time { return time.Date(2026, 11, 4, 11, 0, 0, 0, time.UTC) }

type rig struct {
	t    *testing.T
	db   *sql.DB
	gh   *fakeGH
	disp *dispatcher.Service
	h    http.Handler
	wh   http.Handler
}

// authz giả: header X-Test-User = "<user_id>:<ROLE>".
func authz(r *http.Request) (string, string, bool) {
	v := r.Header.Get("X-Test-User")
	i := strings.Index(v, ":")
	if i < 0 {
		return "", "", false
	}
	return v[:i], v[i+1:], true
}

const secret = "whsec"

func newRig(t *testing.T) *rig {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('admin1', 'g1', 'namth', 'ADMIN')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('partner1', 'g2', 'doi-tac', 'PARTNER')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'AgentHub', 'namth/agenthub')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	gh := &fakeGH{labels: map[int][]string{}, closed: map[int]string{}}
	disp := dispatcher.New(db, clk{}, dispatcher.DefaultConfig(), nil)
	return &rig{t: t, db: db, gh: gh, disp: disp,
		h:  tasks.Handler(db, gh, disp, "p1", authz),
		wh: tasks.WebhookHandler(db, secret, "p1")}
}

type env struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func (r *rig) do(method, path, user string, body any) (int, env) {
	r.t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rr := httptest.NewRecorder()
	r.h.ServeHTTP(rr, req)
	var e env
	if err := json.Unmarshal(rr.Body.Bytes(), &e); err != nil {
		r.t.Fatalf("%s %s: body không phải JSON: %q", method, path, rr.Body.String())
	}
	return rr.Code, e
}

func (r *rig) create(user, title, desc string) string {
	r.t.Helper()
	code, e := r.do("POST", "/api/tasks", user, map[string]any{"title": title, "description": desc})
	if code != 201 {
		r.t.Fatalf("POST /api/tasks = %d %+v", code, e.Error)
	}
	var d struct {
		ID string `json:"id"`
	}
	json.Unmarshal(e.Data, &d)
	return d.ID
}

func (r *rig) status(id string) string {
	var s string
	r.db.QueryRow(`SELECT status FROM tasks WHERE id=?`, id).Scan(&s)
	return s
}

const desc = "Khách hàng cần xuất bảng tiến độ thành file PDF để gửi ban giám đốc."

// ---------- tạo yêu cầu ----------

func TestCreateTaskCreatesIssue(t *testing.T) {
	r := newRig(t)
	id := r.create("partner1:PARTNER", "Xuất báo cáo PDF", desc)
	var gi sql.NullInt64
	var status, by, pid string
	r.db.QueryRow(`SELECT github_issue_id, status, created_by, project_id FROM tasks WHERE id=?`, id).Scan(&gi, &status, &by, &pid)
	if gi.Int64 != 101 || status != "PENDING_APPROVAL" || by != "partner1" || pid != "p1" {
		t.Fatalf("task = issue %v, %s, %s, %s", gi, status, by, pid)
	}
	if len(r.gh.created) != 1 || r.gh.created[0] != "Xuất báo cáo PDF|status:pending-approval,type:feature" {
		t.Fatalf("issue tạo = %v", r.gh.created)
	}
}

func TestCreateValidation(t *testing.T) { // docs/03 §4
	r := newRig(t)
	bad := []map[string]any{
		{"title": "ngắn", "description": desc},
		{"title": strings.Repeat("a", 121), "description": desc},
		{"title": "Tiêu đề hợp lệ", "description": "quá ngắn"},
		{"title": "      ", "description": desc},
		{"description": desc},
	}
	for _, b := range bad {
		if code, e := r.do("POST", "/api/tasks", "admin1:ADMIN", b); code != 400 || e.Error == nil || e.Error.Code != "VALIDATION_ERROR" {
			t.Errorf("%v: %d %+v, muốn 400 VALIDATION_ERROR", b, code, e.Error)
		}
	}
	if len(r.gh.created) != 0 {
		t.Fatal("dữ liệu sai không được tạo issue")
	}
	if code, e := r.do("POST", "/api/tasks", "", map[string]any{"title": "Tiêu đề hợp lệ", "description": desc}); code != 401 || e.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("chưa đăng nhập: %d", code)
	}
}

func TestCreateGitHubFailureNoRow(t *testing.T) {
	r := newRig(t)
	r.gh.fail = true
	code, e := r.do("POST", "/api/tasks", "admin1:ADMIN", map[string]any{"title": "Tiêu đề hợp lệ", "description": desc})
	if code != 502 || e.Error == nil || e.Error.Code != "GITHUB_ERROR" {
		t.Fatalf("GitHub lỗi: %d %+v, muốn 502 GITHUB_ERROR", code, e.Error)
	}
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&n)
	if n != 0 {
		t.Fatal("GitHub lỗi thì không được lưu task (GitHub là nguồn sự thật)")
	}
}

// ---------- duyệt / từ chối ----------

func TestApproveMovesToPlanningAndQueuesSpec(t *testing.T) {
	r := newRig(t)
	id := r.create("partner1:PARTNER", "Xuất báo cáo PDF", desc)
	if code, e := r.do("POST", "/api/tasks/"+id+"/approve", "partner1:PARTNER", nil); code != 403 || e.Error.Code != "FORBIDDEN" {
		t.Fatalf("partner duyệt: %d, muốn 403 (TC-09)", code)
	}
	code, e := r.do("POST", "/api/tasks/"+id+"/approve", "admin1:ADMIN", nil)
	if code != 200 || !e.Success {
		t.Fatalf("approve = %d %+v", code, e.Error)
	}
	if r.status(id) != "PLANNING" {
		t.Fatalf("status = %s, muốn PLANNING", r.status(id))
	}
	if got := strings.Join(r.gh.labels[101], ","); !strings.Contains(got, "status:planning") || strings.Contains(got, "status:pending-approval") {
		t.Fatalf("labels issue = %s", got)
	}
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE task_id=? AND kind='SPEC' AND required_agent='CLAUDE' AND status='QUEUED'`, id).Scan(&n)
	if n != 1 {
		t.Fatalf("job SPEC = %d, muốn 1 (Claude lập kế hoạch)", n)
	}
	if code, e := r.do("POST", "/api/tasks/"+id+"/approve", "admin1:ADMIN", nil); code != 409 || e.Error.Code != "INVALID_TRANSITION" {
		t.Fatalf("duyệt lần hai: %d %+v, muốn 409 INVALID_TRANSITION (không nhảy cóc)", code, e.Error)
	}
	if code, e := r.do("POST", "/api/tasks/khong-co/approve", "admin1:ADMIN", nil); code != 404 || e.Error.Code != "NOT_FOUND" {
		t.Fatalf("task không tồn tại: %d", code)
	}
}

func TestRejectRequiresReasonAndClosesIssue(t *testing.T) {
	r := newRig(t)
	id := r.create("partner1:PARTNER", "Xuất báo cáo PDF", desc)
	if code, e := r.do("POST", "/api/tasks/"+id+"/reject", "admin1:ADMIN", map[string]any{"reason": "  "}); code != 400 || e.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("thiếu lý do: %d, muốn 400", code)
	}
	if code, _ := r.do("POST", "/api/tasks/"+id+"/reject", "admin1:ADMIN", map[string]any{"reason": "Ngoài phạm vi hợp đồng"}); code != 200 {
		t.Fatalf("reject = %d", code)
	}
	if r.status(id) != "REJECTED" || !strings.Contains(r.gh.closed[101], "Ngoài phạm vi hợp đồng") {
		t.Fatalf("status %s, comment đóng issue %q", r.status(id), r.gh.closed[101])
	}
	if code, e := r.do("POST", "/api/tasks/"+id+"/approve", "admin1:ADMIN", nil); code != 409 || e.Error.Code != "INVALID_TRANSITION" {
		t.Fatalf("duyệt task đã từ chối: %d", code)
	}
}

// ---------- Kanban ----------

type board struct {
	Columns []struct {
		Key   string `json:"key"`
		Title string `json:"title"`
		Tasks []map[string]any
	} `json:"columns"`
}

func (r *rig) board(user string) board {
	r.t.Helper()
	code, e := r.do("GET", "/api/tasks", user, nil)
	if code != 200 {
		r.t.Fatalf("GET /api/tasks = %d %+v", code, e.Error)
	}
	var b board
	json.Unmarshal(e.Data, &b)
	return b
}

func TestKanbanColumnsDEC11(t *testing.T) {
	r := newRig(t)
	a := r.create("partner1:PARTNER", "Task chờ duyệt", desc)
	b := r.create("partner1:PARTNER", "Task lập kế hoạch", desc)
	c := r.create("partner1:PARTNER", "Task bị từ chối", desc)
	r.do("POST", "/api/tasks/"+b+"/approve", "admin1:ADMIN", nil)
	r.do("POST", "/api/tasks/"+c+"/reject", "admin1:ADMIN", map[string]any{"reason": "không làm"})
	for _, s := range []struct{ id, st string }{{"x1", "IN_PROGRESS"}, {"x2", "WAITING_QA"}, {"x3", "QA_PASSED"}, {"x4", "DONE"}, {"x5", "READY"}} {
		r.db.Exec(`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES (?, 'p1', ?, 'mô tả', 'admin1', ?)`, s.id, "T "+s.st, s.st)
	}
	bd := r.board("admin1:ADMIN")
	keys := []string{}
	cols := map[string][]string{}
	for _, col := range bd.Columns {
		keys = append(keys, col.Key)
		for _, tk := range col.Tasks {
			cols[col.Key] = append(cols[col.Key], tk["id"].(string))
		}
	}
	if strings.Join(keys, ",") != "backlog,planning,in_progress,review,done" {
		t.Fatalf("cột = %v, muốn đúng 5 cột theo DEC-11", keys)
	}
	want := map[string][]string{"backlog": {a}, "planning": {b, "x5"}, "in_progress": {"x1"}, "review": {"x2", "x3"}, "done": {"x4"}}
	for k, ids := range want {
		got := strings.Join(cols[k], ",")
		for _, id := range ids {
			if !strings.Contains(got, id) {
				t.Errorf("cột %s = %s, thiếu %s", k, got, id)
			}
		}
		if len(cols[k]) != len(ids) {
			t.Errorf("cột %s có %d thẻ, muốn %d", k, len(cols[k]), len(ids))
		}
	}
	for _, ids := range cols {
		for _, id := range ids {
			if id == c {
				t.Fatal("task REJECTED không hiện trên Kanban")
			}
		}
	}
}

func TestPartnerSeesOnlyRootTasksWithoutInternals(t *testing.T) {
	r := newRig(t)
	root := r.create("partner1:PARTNER", "Task tổng", desc)
	r.db.Exec(`INSERT INTO tasks (id, project_id, parent_id, title, description, created_by, status, assigned_worker_id, branch_name) VALUES ('sub1', 'p1', ?, 'Sub', 'mô tả', 'admin1', 'READY', NULL, 'feat/task-sub1')`, root)
	count := func(b board) (int, bool) {
		n, leaked := 0, false
		for _, col := range b.Columns {
			for _, tk := range col.Tasks {
				n++
				if _, ok := tk["branch_name"]; ok {
					leaked = true
				}
				if _, ok := tk["assigned_worker_id"]; ok {
					leaked = true
				}
			}
		}
		return n, leaked
	}
	if n, _ := count(r.board("admin1:ADMIN")); n != 2 {
		t.Fatalf("admin thấy %d thẻ, muốn 2 (gồm sub-task)", n)
	}
	n, leaked := count(r.board("partner1:PARTNER"))
	if n != 1 || leaked {
		t.Fatalf("partner thấy %d thẻ (lộ trường nội bộ: %v), muốn 1 thẻ tổng, không có branch_name/assigned_worker_id", n, leaked)
	}
	if code, e := r.do("GET", "/api/tasks", "", nil); code != 401 || e.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("chưa đăng nhập: %d", code)
	}
}

func TestTaskDetail(t *testing.T) {
	r := newRig(t)
	root := r.create("partner1:PARTNER", "Task tổng", desc)
	r.db.Exec(`INSERT INTO tasks (id, project_id, parent_id, title, description, created_by, status) VALUES ('sub1', 'p1', ?, 'Sub', 'mô tả', 'admin1', 'READY')`, root)
	code, e := r.do("GET", "/api/tasks/"+root, "admin1:ADMIN", nil)
	var d struct {
		ID            string           `json:"id"`
		Title         string           `json:"title"`
		Status        string           `json:"status"`
		GitHubIssueID int              `json:"github_issue_id"`
		Subtasks      []map[string]any `json:"subtasks"`
	}
	json.Unmarshal(e.Data, &d)
	if code != 200 || d.ID != root || d.GitHubIssueID != 101 || len(d.Subtasks) != 1 || d.Subtasks[0]["id"] != "sub1" {
		t.Fatalf("chi tiết = %d %s", code, e.Data)
	}
	if code, _ := r.do("GET", "/api/tasks/khong-co", "admin1:ADMIN", nil); code != 404 {
		t.Fatalf("không tồn tại: %d", code)
	}
}

// ---------- webhook ----------

func sign(body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func (r *rig) hook(event string, payload any, sig string) int {
	r.t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/webhooks/github", bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", event)
	if sig == "" {
		sig = sign(body)
	}
	req.Header.Set("X-Hub-Signature-256", sig)
	rr := httptest.NewRecorder()
	r.wh.ServeHTTP(rr, req)
	return rr.Code
}

func issueEvent(action string, number int, title string) map[string]any {
	return map[string]any{"action": action, "issue": map[string]any{"number": number, "title": title,
		"body": "Mô tả từ GitHub đủ dài để hợp lệ", "user": map[string]any{"login": "someone"}}}
}

func TestWebhookSignature(t *testing.T) { // TC-05
	r := newRig(t)
	if code := r.hook("issues", issueEvent("opened", 500, "Từ GitHub"), "sha256=deadbeef"); code != 401 {
		t.Fatalf("chữ ký sai: %d, muốn 401", code)
	}
	if code := r.hook("issues", issueEvent("opened", 500, "Từ GitHub"), "khong-co-tien-to"); code != 401 {
		t.Fatalf("chữ ký sai định dạng: %d, muốn 401", code)
	}
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&n)
	if n != 0 {
		t.Fatal("webhook sai chữ ký không được ghi gì")
	}
}

func TestWebhookIssueOpenedSyncsOnce(t *testing.T) {
	r := newRig(t)
	if code := r.hook("issues", issueEvent("opened", 500, "Từ GitHub"), ""); code != 200 && code != 202 {
		t.Fatalf("webhook hợp lệ: %d", code)
	}
	r.hook("issues", issueEvent("opened", 500, "Từ GitHub"), "") // GitHub gửi lại
	var n int
	var status, title, by string
	r.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE github_issue_id=500`).Scan(&n)
	r.db.QueryRow(`SELECT status, title, created_by FROM tasks WHERE github_issue_id=500`).Scan(&status, &title, &by)
	if n != 1 || status != "PENDING_APPROVAL" || title != "Từ GitHub" || by == "" {
		t.Fatalf("đồng bộ issue: %d dòng, %s, %s, created_by %q", n, status, title, by)
	}
	// Issue do chính AgentHub tạo → webhook không nhân đôi.
	r.create("admin1:ADMIN", "Tạo từ web", desc)
	r.hook("issues", issueEvent("opened", 101, "Tạo từ web"), "")
	r.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE github_issue_id=101`).Scan(&n)
	if n != 1 {
		t.Fatalf("issue 101 có %d task, muốn 1", n)
	}
	// Sự kiện khác: chấp nhận nhưng bỏ qua.
	if code := r.hook("ping", map[string]any{"zen": "x"}, ""); code != 200 && code != 202 && code != 204 {
		t.Fatalf("ping: %d", code)
	}
}
