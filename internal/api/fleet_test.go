package api_test

// Test nghiệm thu TASK-006 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/lead-rotation.md §2, §4, §6.3; TC-23, TC-24.

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/api"
	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
)

type clock struct{ t time.Time }

func (c *clock) Now() time.Time { return c.t }

type rig struct {
	t    *testing.T
	db   *sql.DB
	disp *dispatcher.Service
	clk  *clock
	h    http.Handler
}

// authz giả: header X-Test-Role thay cho phiên đăng nhập.
func authz(r *http.Request) (string, bool) {
	role := r.Header.Get("X-Test-Role")
	return role, role != ""
}

func newRig(t *testing.T) *rig {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	clk := &clock{t: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)}
	disp := dispatcher.New(db, clk, dispatcher.DefaultConfig(), nil)
	r := &rig{t: t, db: db, disp: disp, clk: clk, h: api.FleetHandler(db, disp, authz)}
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', 'gh1', 'namth', 'ADMIN')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'AgentHub', 'namth/agenthub')`,
		`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t1', 'p1', 'A', 'B', 'u1', 'READY')`,
	} {
		r.exec(q)
	}
	return r
}

func (r *rig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.db.Exec(q, args...); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
}

func (r *rig) machine(id string, kinds ...string) {
	r.t.Helper()
	r.exec(`INSERT INTO workers (id, name, os, token_hash) VALUES (?, ?, 'darwin', 'h')`, id, id)
	if _, err := r.disp.Connect(id, kinds); err != nil {
		r.t.Fatal(err)
	}
}

type env struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func (r *rig) do(method, path, role string, body any) (int, env) {
	r.t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		req.Header.Set("X-Test-Role", role)
	}
	rr := httptest.NewRecorder()
	r.h.ServeHTTP(rr, req)
	var e env
	if err := json.Unmarshal(rr.Body.Bytes(), &e); err != nil {
		r.t.Fatalf("%s %s: body không phải JSON docs/07: %q", method, path, rr.Body.String())
	}
	return rr.Code, e
}

type fleet struct {
	Lead struct {
		State       string  `json:"state"`
		AgentID     *string `json:"agent_id"`
		Epoch       int64   `json:"epoch"`
		ExpiresAt   *string `json:"expires_at"`
		VacantSince *string `json:"vacant_since"`
		ETA         *string `json:"eta"`
		WaitingJobs int     `json:"waiting_jobs"`
	} `json:"lead"`
	Workers []struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		OS           string  `json:"os"`
		ConnStatus   string  `json:"conn_status"`
		OfflineSince *string `json:"offline_since"`
		Agents       []struct {
			ID              string  `json:"id"`
			Kind            string  `json:"kind"`
			Status          string  `json:"status"`
			CooldownUntil   *string `json:"cooldown_until"`
			LeadPriority    *int    `json:"lead_priority"`
			SoftMaxJobs     *int    `json:"soft_max_jobs"`
			SoftWindowHours int     `json:"soft_window_hours"`
			JobsInWindow    int     `json:"jobs_in_window"`
			CurrentJob      *struct {
				JobID   string `json:"job_id"`
				TaskID  string `json:"task_id"`
				Kind    string `json:"kind"`
				Attempt int    `json:"attempt"`
			} `json:"current_job"`
		} `json:"agents"`
	} `json:"workers"`
}

func (r *rig) fleet() fleet {
	r.t.Helper()
	code, e := r.do("GET", "/api/fleet", "ADMIN", nil)
	if code != 200 || !e.Success {
		r.t.Fatalf("GET /api/fleet = %d %+v", code, e)
	}
	var f fleet
	if err := json.Unmarshal(e.Data, &f); err != nil {
		r.t.Fatal(err)
	}
	return f
}

// ---------- phân quyền ----------

func TestRBAC(t *testing.T) { // TC-23
	r := newRig(t)
	r.machine("mac", "CLAUDE")
	reqs := []struct{ method, path string }{
		{"GET", "/api/fleet"},
		{"POST", "/api/admin/lead/assign"},
		{"POST", "/api/admin/lead/revoke"},
		{"PATCH", "/api/admin/agents/mac%2Fclaude"},
	}
	for _, q := range reqs {
		if code, e := r.do(q.method, q.path, "", map[string]any{}); code != 401 || e.Error == nil || e.Error.Code != "UNAUTHORIZED" {
			t.Errorf("%s %s chưa đăng nhập: %d %+v, muốn 401 UNAUTHORIZED", q.method, q.path, code, e.Error)
		}
		if code, e := r.do(q.method, q.path, "PARTNER", map[string]any{}); code != 403 || e.Error == nil || e.Error.Code != "FORBIDDEN" {
			t.Errorf("%s %s bởi PARTNER: %d %+v, muốn 403 FORBIDDEN", q.method, q.path, code, e.Error)
		}
	}
	if l := r.lead(); l.Epoch != 1 {
		t.Fatal("request bị chặn không được làm đổi lease")
	}
}

func (r *rig) lead() dispatcher.Lease {
	l, err := r.disp.Lead()
	if err != nil {
		r.t.Fatal(err)
	}
	return l
}

// ---------- GET /api/fleet ----------

func TestFleetEmpty(t *testing.T) {
	r := newRig(t)
	f := r.fleet()
	if f.Lead.State != "VACANT" || f.Lead.AgentID != nil || len(f.Workers) != 0 {
		t.Fatalf("fleet rỗng = %+v", f)
	}
	code, e := r.do("GET", "/api/fleet", "ADMIN", nil)
	if code != 200 || string(e.Data) == "null" || !bytes.Contains(e.Data, []byte(`"workers":[]`)) {
		t.Fatalf("workers rỗng phải là mảng [] chứ không phải null: %s", e.Data)
	}
}

func TestFleetShape(t *testing.T) {
	r := newRig(t)
	r.machine("mac-mini", "CLAUDE", "ANTIGRAVITY")
	r.machine("laptop", "CLAUDE")
	r.exec(`UPDATE agents SET lead_priority = 2, soft_max_jobs = 20, soft_window_hours = 5 WHERE id='laptop/claude'`)
	if _, err := r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "CODE", RequiredAgent: "ANTIGRAVITY", BranchName: "feat/task-t1"}); err != nil {
		t.Fatal(err)
	}
	a, err := r.disp.RequestJob("mac-mini", "mac-mini/antigravity")
	if err != nil || a == nil {
		t.Fatal(err)
	}
	reset := r.clk.t.Add(40 * time.Minute)
	if err := r.disp.Exhausted("laptop", "laptop/claude", "", &reset, "limit", "", ""); err != nil {
		t.Fatal(err)
	}

	f := r.fleet()
	if f.Lead.State != "HELD" || f.Lead.AgentID == nil || *f.Lead.AgentID != "mac-mini/claude" || f.Lead.Epoch != 1 || f.Lead.ExpiresAt == nil {
		t.Fatalf("lead = %+v", f.Lead)
	}
	if _, err := time.Parse(time.RFC3339, *f.Lead.ExpiresAt); err != nil {
		t.Fatalf("expires_at phải RFC3339: %q", *f.Lead.ExpiresAt)
	}
	if len(f.Workers) != 2 || f.Workers[0].ID != "laptop" || f.Workers[1].ID != "mac-mini" {
		t.Fatalf("workers phải sắp theo tên: %+v", f.Workers)
	}
	mm := f.Workers[1]
	if mm.ConnStatus != "ONLINE" || mm.OS != "darwin" || len(mm.Agents) != 2 {
		t.Fatalf("mac-mini = %+v", mm)
	}
	var agy, lap = mm.Agents[0], f.Workers[0].Agents[0]
	if agy.Kind != "ANTIGRAVITY" {
		agy = mm.Agents[1]
	}
	if agy.Status != "BUSY" || agy.CurrentJob == nil || agy.CurrentJob.TaskID != "t1" || agy.CurrentJob.Kind != "CODE" || agy.CurrentJob.Attempt != 1 || agy.JobsInWindow != 1 {
		t.Fatalf("agent bận = %+v / %+v", agy, agy.CurrentJob)
	}
	if lap.Status != "COOLDOWN" || lap.CooldownUntil == nil || lap.LeadPriority == nil || *lap.LeadPriority != 2 ||
		lap.SoftMaxJobs == nil || *lap.SoftMaxJobs != 20 || lap.SoftWindowHours != 5 || lap.CurrentJob != nil {
		t.Fatalf("agent nghỉ = %+v", lap)
	}
	if cu, err := time.Parse(time.RFC3339, *lap.CooldownUntil); err != nil || !cu.Equal(reset) {
		t.Fatalf("cooldown_until = %q, muốn %s", *lap.CooldownUntil, reset.Format(time.RFC3339))
	}
}

func TestFleetVacantShowsETAAndWaiting(t *testing.T) { // TC-27 phần API
	r := newRig(t)
	r.machine("c", "CLAUDE")
	reset := r.clk.t.Add(90 * time.Minute)
	if err := r.disp.Exhausted("c", "c/claude", "", &reset, "limit", "", ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "SPEC", RequiredAgent: "CLAUDE"})
	}
	r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "CODE", RequiredAgent: "ANTIGRAVITY"})
	f := r.fleet()
	if f.Lead.State != "VACANT" || f.Lead.ETA == nil || f.Lead.VacantSince == nil {
		t.Fatalf("lead VACANT phải có eta và vacant_since: %+v", f.Lead)
	}
	if eta, _ := time.Parse(time.RFC3339, *f.Lead.ETA); !eta.Equal(reset) {
		t.Fatalf("eta = %s, muốn MIN(cooldown_until) Claude = %s", *f.Lead.ETA, reset.Format(time.RFC3339))
	}
	if f.Lead.WaitingJobs != 2 {
		t.Fatalf("waiting_jobs = %d, muốn 2 (chỉ job CLAUDE đang QUEUED)", f.Lead.WaitingJobs)
	}
}

// ---------- trao / thu hồi ----------

func TestAssignAndRevoke(t *testing.T) {
	r := newRig(t)
	r.machine("a", "CLAUDE", "ANTIGRAVITY")
	r.machine("b", "CLAUDE")

	code, e := r.do("POST", "/api/admin/lead/assign", "ADMIN", map[string]any{"agent_id": "b/claude"})
	if code != 200 || !e.Success || !bytes.Contains(e.Data, []byte(`"b/claude"`)) || !bytes.Contains(e.Data, []byte(`"epoch":2`)) {
		t.Fatalf("assign = %d %s", code, e.Data)
	}
	if code, e := r.do("POST", "/api/admin/lead/assign", "ADMIN", map[string]any{"agent_id": "a/antigravity"}); code != 409 || e.Error.Code != "AGENT_NOT_ELIGIBLE" {
		t.Fatalf("trao cho Antigravity: %d %+v, muốn 409 AGENT_NOT_ELIGIBLE", code, e.Error)
	}
	if code, e := r.do("POST", "/api/admin/lead/assign", "ADMIN", map[string]any{"agent_id": "khong/co"}); code != 404 || e.Error.Code != "NOT_FOUND" {
		t.Fatalf("agent không tồn tại: %d %+v", code, e.Error)
	}
	if code, e := r.do("POST", "/api/admin/lead/assign", "ADMIN", map[string]any{}); code != 400 || e.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("thiếu agent_id: %d %+v", code, e.Error)
	}

	code, e = r.do("POST", "/api/admin/lead/revoke", "ADMIN", map[string]any{"reason": "thử"})
	if code != 200 || !bytes.Contains(e.Data, []byte(`"a/claude"`)) {
		t.Fatalf("revoke = %d %s, muốn chuyển sang a/claude", code, e.Data)
	}
	r.exec(`UPDATE agents SET status='COOLDOWN', cooldown_until='2026-11-04 12:00:00' WHERE id='b/claude'`)
	r.do("POST", "/api/admin/lead/revoke", "ADMIN", nil) // a bị loại 10 phút, b đang nghỉ → VACANT
	if code, e := r.do("POST", "/api/admin/lead/revoke", "ADMIN", nil); code != 409 || e.Error.Code != "NO_LEAD" {
		t.Fatalf("thu hồi khi VACANT: %d %+v, muốn 409 NO_LEAD", code, e.Error)
	}
}

func TestAssignToCooldownAgent(t *testing.T) { // TC-24
	r := newRig(t)
	r.machine("a", "CLAUDE")
	r.machine("b", "CLAUDE")
	reset := r.clk.t.Add(time.Hour)
	r.disp.Exhausted("b", "b/claude", "", &reset, "limit", "", "")
	if code, e := r.do("POST", "/api/admin/lead/assign", "ADMIN", map[string]any{"agent_id": "b/claude"}); code != 409 || e.Error.Code != "AGENT_NOT_ELIGIBLE" {
		t.Fatalf("%d %+v, muốn 409 AGENT_NOT_ELIGIBLE", code, e.Error)
	}
}

// ---------- PATCH agent ----------

func TestPatchAgent(t *testing.T) {
	r := newRig(t)
	r.machine("mac", "CLAUDE", "ANTIGRAVITY")

	code, e := r.do("PATCH", "/api/admin/agents/mac%2Fclaude", "ADMIN", map[string]any{"lead_priority": 3, "soft_max_jobs": 50, "soft_window_hours": 6})
	if code != 200 || !e.Success {
		t.Fatalf("PATCH = %d %+v", code, e.Error)
	}
	var lp, sm, sw sql.NullInt64
	r.db.QueryRow(`SELECT lead_priority, soft_max_jobs, soft_window_hours FROM agents WHERE id='mac/claude'`).Scan(&lp, &sm, &sw)
	if lp.Int64 != 3 || sm.Int64 != 50 || sw.Int64 != 6 {
		t.Fatalf("DB = %v %v %v", lp, sm, sw)
	}
	var out struct {
		ID           string `json:"id"`
		LeadPriority *int   `json:"lead_priority"`
	}
	json.Unmarshal(e.Data, &out)
	if out.ID != "mac/claude" || out.LeadPriority == nil || *out.LeadPriority != 3 {
		t.Fatalf("data = %s", e.Data)
	}

	// null xoá giá trị; trường vắng mặt giữ nguyên.
	if code, _ := r.do("PATCH", "/api/admin/agents/mac%2Fclaude", "ADMIN", map[string]any{"lead_priority": nil}); code != 200 {
		t.Fatalf("PATCH null: %d", code)
	}
	r.db.QueryRow(`SELECT lead_priority, soft_max_jobs FROM agents WHERE id='mac/claude'`).Scan(&lp, &sm)
	if lp.Valid || sm.Int64 != 50 {
		t.Fatalf("null phải xoá lead_priority, giữ soft_max_jobs: %v %v", lp, sm)
	}

	bad := []map[string]any{
		{"lead_priority": 0}, {"lead_priority": 100}, {"lead_priority": "1"},
		{"soft_max_jobs": 0}, {"soft_max_jobs": 501},
		{"soft_window_hours": 0}, {"soft_window_hours": 25}, {"soft_window_hours": nil},
		{"status": "IDLE"},
	}
	for _, b := range bad {
		if code, e := r.do("PATCH", "/api/admin/agents/mac%2Fclaude", "ADMIN", b); code != 400 || e.Error.Code != "VALIDATION_ERROR" {
			t.Errorf("body %v: %d %+v, muốn 400 VALIDATION_ERROR", b, code, e.Error)
		}
	}
	// DEC-29 (TASK-032): lead_priority áp dụng cho mọi loại agent.
	if code, e := r.do("PATCH", "/api/admin/agents/mac%2Fantigravity", "ADMIN", map[string]any{"lead_priority": 1}); code != 200 {
		t.Fatalf("lead_priority cho Antigravity: %d %+v, muốn 200 (DEC-29)", code, e.Error)
	}
	if code, e := r.do("PATCH", "/api/admin/agents/khong%2Fco", "ADMIN", map[string]any{"soft_window_hours": 2}); code != 404 || e.Error.Code != "NOT_FOUND" {
		t.Fatalf("agent không tồn tại: %d %+v", code, e.Error)
	}
}
