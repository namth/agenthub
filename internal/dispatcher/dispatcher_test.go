package dispatcher_test

// Test nghiệm thu TASK-002 — do trưởng phòng viết, file bị khoá (tests.lock).
// Ánh xạ: TC-11 → TC-22 của docs/features/lead-rotation.md mục 8.

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
)

var _ dispatcher.API = (*dispatcher.Service)(nil)

// ---------- hạ tầng test ----------

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

type recorder struct {
	mu  sync.Mutex
	evs []dispatcher.Event
}

func (r *recorder) Notify(e dispatcher.Event) { r.mu.Lock(); r.evs = append(r.evs, e); r.mu.Unlock() }
func (r *recorder) count(typ string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.evs {
		if e.Type == typ {
			n++
		}
	}
	return n
}
func (r *recorder) last(typ string) (dispatcher.Event, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.evs) - 1; i >= 0; i-- {
		if r.evs[i].Type == typ {
			return r.evs[i], true
		}
	}
	return dispatcher.Event{}, false
}

type env struct {
	t   *testing.T
	db  *sql.DB
	clk *fakeClock
	rec *recorder
	svc *dispatcher.Service
	cfg dispatcher.Config
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	clk := &fakeClock{t: time.Date(2026, 11, 3, 20, 0, 0, 0, time.UTC)}
	rec := &recorder{}
	cfg := dispatcher.DefaultConfig()
	e := &env{t: t, db: db, clk: clk, rec: rec, cfg: cfg, svc: dispatcher.New(db, clk, cfg, rec)}
	e.exec(`INSERT INTO users (id, github_id, username, role) VALUES ('u1', 'gh1', 'namth', 'ADMIN')`)
	e.exec(`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'AgentHub', 'namth/agenthub')`)
	for i := 1; i <= 3; i++ {
		e.exec(fmt.Sprintf(`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t%d', 'p1', 'Task %d', 'Mô tả', 'u1', 'READY')`, i, i))
	}
	return e
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		e.t.Fatalf("exec %q: %v", q, err)
	}
}

func (e *env) str(q string, args ...any) string {
	e.t.Helper()
	var s sql.NullString
	if err := e.db.QueryRow(q, args...).Scan(&s); err != nil {
		e.t.Fatalf("query %q: %v", q, err)
	}
	return s.String
}

func (e *env) num(q string, args ...any) int64 {
	e.t.Helper()
	var n sql.NullInt64
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatalf("query %q: %v", q, err)
	}
	return n.Int64
}

// machine đăng ký máy (như POST /api/fleet/register) rồi kết nối với các agent.
func (e *env) machine(worker string, kinds ...string) {
	e.t.Helper()
	e.exec(`INSERT INTO workers (id, name, os, token_hash) VALUES (?, ?, 'darwin', 'h')`, worker, worker)
	if _, err := e.svc.Connect(worker, kinds); err != nil {
		e.t.Fatalf("Connect(%s): %v", worker, err)
	}
}

// keepAlive gửi heartbeat cho các máy để chúng không bị OFFLINE khi tua đồng hồ.
func (e *env) advance(d time.Duration, alive ...string) {
	e.t.Helper()
	step := 10 * time.Second
	for d > 0 {
		s := step
		if d < s {
			s = d
		}
		e.clk.Add(s)
		d -= s
		for _, w := range alive {
			if err := e.svc.Heartbeat(w); err != nil {
				e.t.Fatalf("Heartbeat(%s): %v", w, err)
			}
		}
		if err := e.svc.Tick(); err != nil {
			e.t.Fatalf("Tick: %v", err)
		}
	}
}

func (e *env) lead() dispatcher.Lease {
	e.t.Helper()
	l, err := e.svc.Lead()
	if err != nil {
		e.t.Fatal(err)
	}
	return l
}

func (e *env) job(task, kind, required string) string {
	e.t.Helper()
	id, err := e.svc.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: task, Kind: kind, RequiredAgent: required, BranchName: "feat/task-" + task})
	if err != nil {
		e.t.Fatalf("EnqueueJob: %v", err)
	}
	return id
}

func (e *env) request(worker, agent string) *dispatcher.Assignment {
	e.t.Helper()
	a, err := e.svc.RequestJob(worker, agent)
	if err != nil {
		e.t.Fatalf("RequestJob(%s): %v", agent, err)
	}
	return a
}

func ptr(t time.Time) *time.Time { return &t }

// ---------- Fleet ----------

func TestConnectRegistersAgentsAndOnline(t *testing.T) {
	e := newEnv(t)
	e.exec(`INSERT INTO workers (id, name, os, token_hash) VALUES ('mac-mini', 'mac-mini', 'darwin', 'h')`)
	agents, err := e.svc.Connect("mac-mini", []string{dispatcher.KindClaude, dispatcher.KindAntigravity})
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 2 {
		t.Fatalf("Connect trả %d agent, muốn 2", len(agents))
	}
	ids := map[string]bool{}
	for _, a := range agents {
		ids[a.ID] = true
		if a.Status != "IDLE" || a.WorkerID != "mac-mini" {
			t.Fatalf("agent %+v, muốn IDLE thuộc mac-mini", a)
		}
	}
	if !ids["mac-mini/claude"] || !ids["mac-mini/antigravity"] {
		t.Fatalf("agent id = %v, muốn mac-mini/claude và mac-mini/antigravity", ids)
	}
	if s := e.str(`SELECT conn_status FROM workers WHERE id='mac-mini'`); s != "ONLINE" {
		t.Fatalf("conn_status = %s, muốn ONLINE", s)
	}
	// Kết nối lại không nhân đôi agent.
	if _, err := e.svc.Connect("mac-mini", []string{dispatcher.KindClaude, dispatcher.KindAntigravity}); err != nil {
		t.Fatal(err)
	}
	if n := e.num(`SELECT COUNT(*) FROM agents WHERE worker_id='mac-mini'`); n != 2 {
		t.Fatalf("có %d agent sau khi kết nối lại, muốn 2", n)
	}
	if _, err := e.svc.Connect("khong-co", []string{dispatcher.KindClaude}); !errors.Is(err, dispatcher.ErrNotFound) {
		t.Fatalf("Connect máy chưa đăng ký: err = %v, muốn ErrNotFound", err)
	}
}

func TestFirstClaudeTakesLeaseAntigravityNever(t *testing.T) {
	e := newEnv(t)
	e.machine("server", dispatcher.KindAntigravity)
	if l := e.lead(); l.State != "VACANT" {
		t.Fatalf("chỉ có Antigravity: lease = %+v, muốn VACANT (BR-07)", l)
	}
	e.machine("mac-mini", dispatcher.KindClaude)
	l := e.lead()
	if l.State != "HELD" || l.AgentID != "mac-mini/claude" || l.Epoch != 1 {
		t.Fatalf("lease = %+v, muốn HELD bởi mac-mini/claude epoch 1", l)
	}
	if l.ExpiresAt == nil || !l.ExpiresAt.Equal(e.clk.Now().Add(e.cfg.LeaseTTL)) {
		t.Fatalf("expires_at = %v, muốn now + TTL", l.ExpiresAt)
	}
	if e.rec.count(dispatcher.EvLeaseGranted) != 1 {
		t.Fatal("phải phát đúng 1 LEASE_GRANTED")
	}
	if n := e.num(`SELECT COUNT(*) FROM lead_journal WHERE kind='HANDOVER' AND epoch=1`); n != 1 {
		t.Fatalf("lead_journal HANDOVER epoch 1 = %d, muốn 1 (BR-18)", n)
	}
}

func TestOfflineAfter45sNoReclaimBefore120s(t *testing.T) { // TC-18
	e := newEnv(t)
	e.machine("mac-mini", dispatcher.KindAntigravity)
	e.job("t1", dispatcher.JobCode, dispatcher.KindAntigravity)
	a := e.request("mac-mini", "mac-mini/antigravity")
	e.advance(40 * time.Second)
	if s := e.str(`SELECT conn_status FROM workers WHERE id='mac-mini'`); s != "ONLINE" {
		t.Fatalf("40s: conn_status = %s, muốn còn ONLINE", s)
	}
	e.advance(10 * time.Second)
	if s := e.str(`SELECT conn_status FROM workers WHERE id='mac-mini'`); s != "OFFLINE" {
		t.Fatalf("50s: conn_status = %s, muốn OFFLINE", s)
	}
	if e.rec.count(dispatcher.EvWorkerOffline) != 1 {
		t.Fatal("phải phát đúng 1 WORKER_OFFLINE")
	}
	e.advance(10 * time.Second)
	if err := e.svc.Heartbeat("mac-mini"); err != nil {
		t.Fatal(err)
	}
	if s := e.str(`SELECT conn_status FROM workers WHERE id='mac-mini'`); s != "ONLINE" {
		t.Fatalf("heartbeat lại: conn_status = %s, muốn ONLINE", s)
	}
	if s := e.str(`SELECT status FROM assignments WHERE id=?`, a.ID); s != "ACTIVE" {
		t.Fatalf("mất kết nối 60s: assignment = %s, muốn vẫn ACTIVE", s)
	}
}

func TestReclaimAfter120sOffline(t *testing.T) { // TC-19
	e := newEnv(t)
	e.machine("m1", dispatcher.KindAntigravity)
	e.machine("m2", dispatcher.KindAntigravity)
	job := e.job("t1", dispatcher.JobCode, dispatcher.KindAntigravity)
	a1 := e.request("m1", "m1/antigravity")
	e.advance(180*time.Second, "m2") // m1 im lặng: OFFLINE ở ~45–50s, thu hồi sau đó 120s
	if s := e.str(`SELECT status FROM assignments WHERE id=?`, a1.ID); s != "RECLAIMED" {
		t.Fatalf("assignment = %s, muốn RECLAIMED", s)
	}
	if s := e.str(`SELECT status FROM jobs WHERE id=?`, job); s != "QUEUED" {
		t.Fatalf("job = %s, muốn QUEUED", s)
	}
	if e.rec.count(dispatcher.EvJobReclaimed) != 1 {
		t.Fatal("phải phát đúng 1 JOB_RECLAIMED")
	}
	a2 := e.request("m2", "m2/antigravity")
	if a2 == nil || a2.JobID != job || a2.Attempt != 2 || a2.ID == a1.ID {
		t.Fatalf("giao lại = %+v, muốn cùng job, attempt 2, assignment_id mới", a2)
	}
}

func TestDisconnectMarksOfflineAndRevokesLead(t *testing.T) {
	e := newEnv(t)
	e.machine("a", dispatcher.KindClaude)
	e.machine("b", dispatcher.KindClaude)
	if err := e.svc.Disconnect("a"); err != nil {
		t.Fatal(err)
	}
	if s := e.str(`SELECT conn_status FROM workers WHERE id='a'`); s != "OFFLINE" {
		t.Fatalf("conn_status = %s, muốn OFFLINE ngay khi Disconnect", s)
	}
	l := e.lead()
	if l.AgentID != "b/claude" || l.Epoch != 2 {
		t.Fatalf("lease = %+v, muốn chuyển sang b/claude epoch 2", l)
	}
	ev, _ := e.rec.last(dispatcher.EvLeaseRevoked)
	if ev.Reason != dispatcher.RevokeOffline || ev.AgentID != "a/claude" || ev.Epoch != 1 {
		t.Fatalf("LEASE_REVOKED = %+v, muốn a/claude epoch 1 lý do OFFLINE", ev)
	}
}

// ---------- Lease ----------

func TestRenewExtendsAndStaleEpochRejected(t *testing.T) { // TC-12
	e := newEnv(t)
	e.machine("a", dispatcher.KindClaude)
	e.machine("b", dispatcher.KindClaude)
	e.advance(30*time.Second, "a", "b")
	l, err := e.svc.Renew("a", "a/claude", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !l.ExpiresAt.Equal(e.clk.Now().Add(e.cfg.LeaseTTL)) {
		t.Fatalf("Renew: expires_at = %v, muốn now + TTL", l.ExpiresAt)
	}
	before := e.str(`SELECT expires_at FROM lead_lease WHERE id=1`)
	if _, err := e.svc.Renew("a", "a/claude", 0); !errors.Is(err, dispatcher.ErrStaleEpoch) {
		t.Fatalf("Renew epoch cũ: err = %v, muốn ErrStaleEpoch", err)
	}
	if _, err := e.svc.Renew("b", "b/claude", 1); !errors.Is(err, dispatcher.ErrStaleEpoch) {
		t.Fatalf("Renew bởi agent không giữ lease: err = %v, muốn ErrStaleEpoch", err)
	}
	if _, err := e.svc.Renew("b", "a/claude", 1); !errors.Is(err, dispatcher.ErrAgentNotOwned) {
		t.Fatalf("Renew agent của máy khác: err = %v, muốn ErrAgentNotOwned", err)
	}
	if after := e.str(`SELECT expires_at FROM lead_lease WHERE id=1`); after != before {
		t.Fatal("Renew bị từ chối không được đổi expires_at")
	}
}

func TestLeaseExpiresAndGoesToBestCandidate(t *testing.T) { // TC-13
	e := newEnv(t)
	e.machine("a", dispatcher.KindClaude)
	e.machine("b", dispatcher.KindClaude)
	e.exec(`UPDATE agents SET lead_priority = 1 WHERE id='b/claude'`)
	e.exec(`UPDATE agents SET lead_priority = 2 WHERE id='a/claude'`)
	e.advance(60*time.Second, "a", "b")
	if l := e.lead(); l.AgentID != "a/claude" {
		t.Fatalf("không được giành lease khi chưa hết hạn: %+v", l)
	}
	e.advance(40*time.Second, "a", "b") // tổng 100s > 90s, không gia hạn
	l := e.lead()
	if l.AgentID != "b/claude" || l.Epoch != 2 {
		t.Fatalf("hết hạn: lease = %+v, muốn b/claude epoch 2", l)
	}
	ev, _ := e.rec.last(dispatcher.EvLeaseRevoked)
	if ev.Reason != dispatcher.RevokeExpired {
		t.Fatalf("lý do thu hồi = %s, muốn EXPIRED", ev.Reason)
	}
}

func TestCandidateOrdering(t *testing.T) { // TC-14, BR-06
	t.Run("lead_priority tăng dần, NULL cuối", func(t *testing.T) {
		e := newEnv(t)
		e.machine("w0", dispatcher.KindClaude)
		for _, w := range []string{"w1", "w2", "w3"} {
			e.machine(w, dispatcher.KindClaude)
		}
		e.exec(`UPDATE agents SET lead_priority = 2 WHERE id='w1/claude'`)
		e.exec(`UPDATE agents SET lead_priority = 1 WHERE id='w3/claude'`)
		if _, err := e.svc.AdminRevoke(); err != nil {
			t.Fatal(err)
		}
		if l := e.lead(); l.AgentID != "w3/claude" {
			t.Fatalf("lead = %s, muốn w3/claude (priority 1)", l.AgentID)
		}
	})
	t.Run("đồng hạng → còn nhiều hạn mềm hơn", func(t *testing.T) {
		e := newEnv(t)
		e.machine("w0", dispatcher.KindClaude)
		e.machine("w1", dispatcher.KindClaude)
		e.machine("w2", dispatcher.KindClaude)
		e.exec(`UPDATE agents SET lead_priority = 1 WHERE id IN ('w1/claude','w2/claude')`)
		e.exec(`UPDATE agents SET soft_max_jobs = 10 WHERE id='w1/claude'`)
		e.exec(`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, status) VALUES ('jx', 'p1', 't1', 'SPEC', 'CLAUDE', 'DONE')`)
		for i := 0; i < 8; i++ {
			e.exec(`INSERT INTO assignments (id, job_id, agent_id, attempt, status, started_at) VALUES (?, 'jx', 'w1/claude', 1, 'SUBMITTED', ?)`,
				fmt.Sprintf("old%d", i), e.clk.Now().Add(-time.Hour).UTC().Format("2006-01-02 15:04:05"))
		}
		if _, err := e.svc.AdminRevoke(); err != nil {
			t.Fatal(err)
		}
		if l := e.lead(); l.AgentID != "w2/claude" {
			t.Fatalf("lead = %s, muốn w2/claude (không giới hạn hạn mềm > còn 2)", l.AgentID)
		}
	})
	t.Run("đồng hạng → rảnh lâu nhất", func(t *testing.T) {
		e := newEnv(t)
		e.machine("w0", dispatcher.KindClaude)
		e.machine("w1", dispatcher.KindClaude)
		e.machine("w2", dispatcher.KindClaude)
		e.exec(`UPDATE agents SET idle_since = ? WHERE id='w1/claude'`, e.clk.Now().Add(-time.Minute).UTC().Format("2006-01-02 15:04:05"))
		e.exec(`UPDATE agents SET idle_since = ? WHERE id='w2/claude'`, e.clk.Now().Add(-time.Hour).UTC().Format("2006-01-02 15:04:05"))
		if _, err := e.svc.AdminRevoke(); err != nil {
			t.Fatal(err)
		}
		if l := e.lead(); l.AgentID != "w2/claude" {
			t.Fatalf("lead = %s, muốn w2/claude (rảnh lâu hơn)", l.AgentID)
		}
	})
}

func TestReleaseAndAdminActions(t *testing.T) {
	e := newEnv(t)
	e.machine("a", dispatcher.KindClaude, dispatcher.KindAntigravity)
	e.machine("b", dispatcher.KindClaude)
	if err := e.svc.Release("a", "a/claude", 7); !errors.Is(err, dispatcher.ErrStaleEpoch) {
		t.Fatalf("Release epoch sai: err = %v", err)
	}
	if err := e.svc.Release("a", "a/claude", 1); err != nil {
		t.Fatal(err)
	}
	if l := e.lead(); l.AgentID != "b/claude" || l.Epoch != 2 {
		t.Fatalf("sau Release: %+v, muốn b/claude epoch 2", l)
	}
	if ev, _ := e.rec.last(dispatcher.EvLeaseRevoked); ev.Reason != dispatcher.RevokeReleased {
		t.Fatalf("lý do = %s, muốn RELEASED", ev.Reason)
	}
	if _, err := e.svc.AdminAssign("a/antigravity"); !errors.Is(err, dispatcher.ErrAgentNotEligible) {
		t.Fatalf("trao vai cho Antigravity: err = %v, muốn ErrAgentNotEligible", err)
	}
	if _, err := e.svc.AdminAssign("khong/co"); !errors.Is(err, dispatcher.ErrNotFound) {
		t.Fatalf("trao vai agent không tồn tại: err = %v, muốn ErrNotFound", err)
	}
	l, err := e.svc.AdminAssign("a/claude")
	if err != nil || l.AgentID != "a/claude" || l.Epoch != 3 {
		t.Fatalf("AdminAssign: %+v %v, muốn a/claude epoch 3", l, err)
	}
	e.exec(`UPDATE agents SET status='COOLDOWN', cooldown_until=? WHERE id='b/claude'`, e.clk.Now().Add(time.Hour).UTC().Format("2006-01-02 15:04:05"))
	if _, err := e.svc.AdminAssign("b/claude"); !errors.Is(err, dispatcher.ErrAgentNotEligible) {
		t.Fatalf("trao vai cho agent COOLDOWN: err = %v, muốn ErrAgentNotEligible (TC-24)", err)
	}
}

func TestAdminRevokeBansAgentTenMinutes(t *testing.T) {
	e := newEnv(t)
	e.machine("a", dispatcher.KindClaude)
	l, err := e.svc.AdminRevoke()
	if err != nil {
		t.Fatal(err)
	}
	if l.State != "VACANT" {
		t.Fatalf("ứng viên duy nhất vừa bị thu hồi: lease = %+v, muốn VACANT", l)
	}
	if _, err := e.svc.AdminRevoke(); !errors.Is(err, dispatcher.ErrNoLead) {
		t.Fatalf("thu hồi khi VACANT: err = %v, muốn ErrNoLead", err)
	}
	e.advance(5*time.Minute, "a")
	if e.lead().State != "VACANT" {
		t.Fatal("trong 10 phút loại trừ không được trao lại")
	}
	e.advance(6*time.Minute, "a")
	if l := e.lead(); l.AgentID != "a/claude" {
		t.Fatalf("hết 10 phút: lease = %+v, muốn trao lại a/claude", l)
	}
}

func TestConcurrentLeaseChangesKeepEpochConsistent(t *testing.T) { // TC-11
	e := newEnv(t)
	e.machine("a", dispatcher.KindClaude)
	e.machine("b", dispatcher.KindClaude)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := "a/claude"
			if i%2 == 0 {
				target = "b/claude"
			}
			_, _ = e.svc.AdminAssign(target)
		}(i)
	}
	wg.Wait()
	epoch := e.num(`SELECT epoch FROM lead_lease WHERE id=1`)
	handovers := e.num(`SELECT COUNT(*) FROM lead_journal WHERE kind='HANDOVER'`)
	distinct := e.num(`SELECT COUNT(DISTINCT epoch) FROM lead_journal WHERE kind='HANDOVER'`)
	if epoch < 2 || handovers != epoch || distinct != epoch {
		t.Fatalf("epoch = %d, HANDOVER = %d (khác nhau %d): mỗi lần đổi lead phải tăng epoch đúng 1 và ghi đúng 1 HANDOVER", epoch, handovers, distinct)
	}
}

func TestLeadNote(t *testing.T) {
	e := newEnv(t)
	e.machine("a", dispatcher.KindClaude)
	if err := e.svc.LeadNote("a", "a/claude", 0, "t1", "cũ"); !errors.Is(err, dispatcher.ErrStaleEpoch) {
		t.Fatalf("LeadNote epoch cũ: err = %v", err)
	}
	if err := e.svc.LeadNote("a", "a/claude", 1, "t1", "Ưu tiên task 1"); err != nil {
		t.Fatal(err)
	}
	if s := e.str(`SELECT entry FROM lead_journal WHERE kind='NOTE' AND task_id='t1'`); s != "Ưu tiên task 1" {
		t.Fatalf("lead_journal NOTE = %q", s)
	}
}

// ---------- Jobs, hết limit, fencing ----------

func TestJobRoutingByRequiredAgent(t *testing.T) { // TC-04
	e := newEnv(t)
	e.machine("m", dispatcher.KindClaude, dispatcher.KindAntigravity)
	spec := e.job("t1", dispatcher.JobSpec, dispatcher.KindClaude)
	code := e.job("t2", dispatcher.JobCode, dispatcher.KindAntigravity)
	if a := e.request("m", "m/antigravity"); a == nil || a.JobID != code || a.Attempt != 1 || a.JobKind != "CODE" {
		t.Fatalf("Antigravity nhận %+v, muốn job CODE attempt 1", a)
	}
	if a := e.request("m", "m/claude"); a == nil || a.JobID != spec {
		t.Fatalf("Claude nhận %+v, muốn job SPEC", a)
	}
	if a := e.request("m", "m/antigravity"); a != nil {
		t.Fatalf("agent đang BUSY không được nhận thêm job (BR-17), nhận %+v", a)
	}
	if s := e.str(`SELECT status FROM agents WHERE id='m/antigravity'`); s != "BUSY" {
		t.Fatalf("agent sau khi nhận job = %s, muốn BUSY", s)
	}
	if _, err := e.svc.RequestJob("khac", "m/claude"); !errors.Is(err, dispatcher.ErrAgentNotOwned) {
		t.Fatalf("máy khác xin job hộ: err = %v, muốn ErrAgentNotOwned", err)
	}
}

func TestPriorityThenFIFO(t *testing.T) {
	e := newEnv(t)
	e.machine("m", dispatcher.KindAntigravity)
	first := e.job("t1", dispatcher.JobCode, dispatcher.KindAntigravity)
	e.clk.Add(time.Second)
	e.job("t2", dispatcher.JobCode, dispatcher.KindAntigravity)
	urgent, err := e.svc.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t3", Kind: "CODE", RequiredAgent: "ANTIGRAVITY", Priority: 5})
	if err != nil {
		t.Fatal(err)
	}
	if a := e.request("m", "m/antigravity"); a.JobID != urgent {
		t.Fatal("job priority nhỏ hơn phải được giao trước")
	}
	if err := e.svc.Submit("m", e.str(`SELECT id FROM assignments WHERE job_id=?`, urgent)); err != nil {
		t.Fatal(err)
	}
	if a := e.request("m", "m/antigravity"); a.JobID != first {
		t.Fatal("cùng priority: job tạo trước phải được giao trước")
	}
}

func TestExhaustedMidJobHandsOffToAnotherAgent(t *testing.T) { // TC-15
	e := newEnv(t)
	e.machine("A", dispatcher.KindAntigravity)
	e.machine("B", dispatcher.KindAntigravity)
	job := e.job("t1", dispatcher.JobCode, dispatcher.KindAntigravity)
	a1 := e.request("A", "A/antigravity")
	reset := e.clk.Now().Add(2 * time.Hour)
	if err := e.svc.Exhausted("A", "A/antigravity", a1.ID, ptr(reset), "usage limit", "3 files changed", "abc123"); err != nil {
		t.Fatal(err)
	}
	if s := e.str(`SELECT status FROM agents WHERE id='A/antigravity'`); s != "COOLDOWN" {
		t.Fatalf("agent = %s, muốn COOLDOWN", s)
	}
	if s := e.str(`SELECT cooldown_reason FROM agents WHERE id='A/antigravity'`); s != "EXHAUSTED" {
		t.Fatalf("cooldown_reason = %s, muốn EXHAUSTED", s)
	}
	if s := e.str(`SELECT status FROM assignments WHERE id=?`, a1.ID); s != "EXHAUSTED" {
		t.Fatalf("assignment = %s, muốn EXHAUSTED", s)
	}
	if s := e.str(`SELECT wip_commit FROM assignments WHERE id=?`, a1.ID); s != "abc123" {
		t.Fatalf("wip_commit = %q", s)
	}
	ev, ok := e.rec.last(dispatcher.EvAgentCooldown)
	if !ok || ev.Until == nil || !ev.Until.Equal(reset) {
		t.Fatalf("AGENT_COOLDOWN = %+v, muốn Until = resets_at", ev)
	}
	if a := e.request("A", "A/antigravity"); a != nil {
		t.Fatal("agent COOLDOWN không được nhận job")
	}
	a2 := e.request("B", "B/antigravity")
	if a2 == nil || a2.JobID != job || a2.Attempt != 2 || a2.ID == a1.ID || a2.BranchName != "feat/task-t1" || a2.HandoffNote != "3 files changed" {
		t.Fatalf("giao lại = %+v, muốn cùng job, attempt 2, id mới, cùng nhánh, kèm handoff_note", a2)
	}
}

func TestExhaustedWithoutResetUsesDefaultCooldown(t *testing.T) {
	e := newEnv(t)
	e.machine("A", dispatcher.KindAntigravity)
	if err := e.svc.Exhausted("A", "A/antigravity", "", nil, "rate limit", "", ""); err != nil {
		t.Fatal(err)
	}
	ev, _ := e.rec.last(dispatcher.EvAgentCooldown)
	if ev.Until == nil || !ev.Until.Equal(e.clk.Now().Add(e.cfg.DefaultCooldown)) {
		t.Fatalf("Until = %v, muốn now + DefaultCooldown", ev.Until)
	}
	e.advance(61*time.Minute, "A")
	if s := e.str(`SELECT status FROM agents WHERE id='A/antigravity'`); s != "IDLE" {
		t.Fatalf("hết cooldown: agent = %s, muốn IDLE (BR-10)", s)
	}
	if e.rec.count(dispatcher.EvAgentIdle) != 1 {
		t.Fatal("phải phát đúng 1 AGENT_IDLE")
	}
}

func TestStaleAssignmentRejected(t *testing.T) { // TC-16
	e := newEnv(t)
	e.machine("A", dispatcher.KindAntigravity)
	e.machine("B", dispatcher.KindAntigravity)
	job := e.job("t1", dispatcher.JobCode, dispatcher.KindAntigravity)
	a1 := e.request("A", "A/antigravity")
	if err := e.svc.Exhausted("A", "A/antigravity", a1.ID, nil, "limit", "", ""); err != nil {
		t.Fatal(err)
	}
	a2 := e.request("B", "B/antigravity")
	if err := e.svc.Submit("A", a1.ID); !errors.Is(err, dispatcher.ErrStaleAssignment) {
		t.Fatalf("nộp bằng assignment cũ: err = %v, muốn ErrStaleAssignment", err)
	}
	if err := e.svc.Log("A", a1.ID); !errors.Is(err, dispatcher.ErrStaleAssignment) {
		t.Fatalf("log bằng assignment cũ: err = %v, muốn ErrStaleAssignment", err)
	}
	if err := e.svc.Submit("A", a2.ID); !errors.Is(err, dispatcher.ErrAgentNotOwned) {
		t.Fatalf("máy A nộp hộ assignment của B: err = %v, muốn ErrAgentNotOwned", err)
	}
	if s := e.str(`SELECT status FROM jobs WHERE id=?`, job); s != "RUNNING" {
		t.Fatalf("job = %s, muốn vẫn RUNNING", s)
	}
	if err := e.svc.Submit("B", a2.ID); err != nil {
		t.Fatal(err)
	}
	if s := e.str(`SELECT status FROM jobs WHERE id=?`, job); s != "DONE" {
		t.Fatalf("job = %s, muốn DONE", s)
	}
	if s := e.str(`SELECT status FROM agents WHERE id='B/antigravity'`); s != "IDLE" {
		t.Fatalf("agent sau khi nộp = %s, muốn IDLE", s)
	}
	if err := e.svc.Submit("B", a2.ID); !errors.Is(err, dispatcher.ErrStaleAssignment) {
		t.Fatalf("nộp lần hai: err = %v, muốn ErrStaleAssignment", err)
	}
}

func TestExhaustedForeignAgentRejected(t *testing.T) { // TC-22
	e := newEnv(t)
	e.machine("A", dispatcher.KindAntigravity)
	e.machine("B", dispatcher.KindAntigravity)
	if err := e.svc.Exhausted("A", "B/antigravity", "", nil, "limit", "", ""); !errors.Is(err, dispatcher.ErrAgentNotOwned) {
		t.Fatalf("err = %v, muốn ErrAgentNotOwned", err)
	}
	if s := e.str(`SELECT status FROM agents WHERE id='B/antigravity'`); s != "IDLE" {
		t.Fatalf("agent máy B = %s, phải không đổi", s)
	}
}

func TestMaxAttemptsFailsAndReplans(t *testing.T) { // TC-20, BR-15
	e := newEnv(t)
	e.machine("lead", dispatcher.KindClaude)
	for _, w := range []string{"A", "B", "C"} {
		e.machine(w, dispatcher.KindAntigravity)
	}
	job := e.job("t1", dispatcher.JobCode, dispatcher.KindAntigravity)
	for i, w := range []string{"A", "B", "C"} {
		a := e.request(w, w+"/antigravity")
		if a == nil || a.Attempt != i+1 {
			t.Fatalf("lần %d: %+v", i+1, a)
		}
		if err := e.svc.Exhausted(w, w+"/antigravity", a.ID, nil, "limit", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if s := e.str(`SELECT status FROM jobs WHERE id=?`, job); s != "FAILED" {
		t.Fatalf("sau 3 lần: job = %s, muốn FAILED", s)
	}
	if s := e.str(`SELECT status FROM tasks WHERE id='t1'`); s != "PLANNING" {
		t.Fatalf("task = %s, muốn PLANNING", s)
	}
	if e.rec.count(dispatcher.EvJobFailed) != 1 {
		t.Fatal("phải phát đúng 1 JOB_FAILED")
	}
	if n := e.num(`SELECT COUNT(*) FROM jobs WHERE task_id='t1' AND kind='REPLAN' AND lead_only=1 AND priority=10 AND status='QUEUED'`); n != 1 {
		t.Fatalf("có %d job REPLAN lead_only priority 10, muốn 1", n)
	}
}

func TestLeadOnlyJobsGoOnlyToLeadAndFirst(t *testing.T) { // BR-17
	e := newEnv(t)
	e.machine("L", dispatcher.KindClaude)
	e.machine("X", dispatcher.KindClaude)
	normal := e.job("t2", dispatcher.JobSpec, dispatcher.KindClaude)
	e.clk.Add(time.Second)
	replan, err := e.svc.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "REPLAN", RequiredAgent: "CLAUDE", LeadOnly: true, Priority: 200})
	if err != nil {
		t.Fatal(err)
	}
	if a := e.request("L", "L/claude"); a == nil || a.JobID != replan {
		t.Fatalf("lead nhận %+v, muốn job lead_only trước dù job thường cũ hơn và priority nhỏ hơn", a)
	}
	if a := e.request("X", "X/claude"); a == nil || a.JobID != normal {
		t.Fatalf("agent không phải lead nhận %+v, muốn job thường", a)
	}
}

func TestVacancyKeepsCodeFlowingAndRestores(t *testing.T) { // TC-17, BR-07
	e := newEnv(t)
	e.machine("C", dispatcher.KindClaude)
	e.machine("G", dispatcher.KindAntigravity)
	reset := e.clk.Now().Add(30 * time.Minute)
	if err := e.svc.Exhausted("C", "C/claude", "", ptr(reset), "usage limit", "", ""); err != nil {
		t.Fatal(err)
	}
	if l := e.lead(); l.State != "VACANT" {
		t.Fatalf("lease = %+v, muốn VACANT", l)
	}
	if ev, _ := e.rec.last(dispatcher.EvLeaseRevoked); ev.Reason != dispatcher.RevokeExhausted {
		t.Fatalf("lý do = %s, muốn EXHAUSTED", ev.Reason)
	}
	ev, ok := e.rec.last(dispatcher.EvLeadVacant)
	if !ok || ev.Until == nil || !ev.Until.Equal(reset) {
		t.Fatalf("LEAD_VACANT = %+v, muốn Until = cooldown_until của Claude", ev)
	}
	e.job("t1", dispatcher.JobSpec, dispatcher.KindClaude)
	code := e.job("t2", dispatcher.JobCode, dispatcher.KindAntigravity)
	if a := e.request("G", "G/antigravity"); a == nil || a.JobID != code {
		t.Fatal("trống lead: job CODE vẫn phải giao được")
	}
	e.advance(20*time.Minute, "C", "G")
	if e.rec.count(dispatcher.EvLeadVacant) != 1 {
		t.Fatalf("LEAD_VACANT phát %d lần, muốn đúng 1 dù Tick nhiều lần", e.rec.count(dispatcher.EvLeadVacant))
	}
	e.advance(11*time.Minute, "C", "G")
	l := e.lead()
	if l.AgentID != "C/claude" || l.Epoch != 2 {
		t.Fatalf("hết cooldown: lease = %+v, muốn C/claude epoch 2", l)
	}
	if e.rec.count(dispatcher.EvLeadRestored) != 1 {
		t.Fatal("phải phát đúng 1 LEAD_RESTORED")
	}
}

func TestSoftLimitCooldownAndLeadHandover(t *testing.T) { // TC-21, BR-09
	e := newEnv(t)
	e.machine("L", dispatcher.KindClaude)
	e.machine("M", dispatcher.KindClaude)
	e.exec(`UPDATE agents SET soft_max_jobs = 2, soft_window_hours = 5 WHERE id='L/claude'`)
	e.exec(`UPDATE agents SET lead_priority = 99 WHERE id='M/claude'`)
	for i := 1; i <= 2; i++ {
		e.job(fmt.Sprintf("t%d", i), dispatcher.JobSpec, dispatcher.KindClaude)
	}
	a := e.request("L", "L/claude")
	if err := e.svc.Submit("L", a.ID); err != nil {
		t.Fatal(err)
	}
	e.clk.Add(time.Minute)
	b := e.request("L", "L/claude")
	if b == nil {
		t.Fatal("chưa chạm hạn mềm phải nhận được job thứ 2")
	}
	if l := e.lead(); l.AgentID != "L/claude" {
		t.Fatal("đang làm job cuối trong hạn mềm vẫn giữ lease")
	}
	if err := e.svc.Submit("L", b.ID); err != nil {
		t.Fatal(err)
	}
	if s := e.str(`SELECT status || ':' || cooldown_reason FROM agents WHERE id='L/claude'`); s != "COOLDOWN:SOFT_LIMIT" {
		t.Fatalf("agent = %s, muốn COOLDOWN:SOFT_LIMIT", s)
	}
	first := e.clk.Now().Add(-time.Minute)
	ev, _ := e.rec.last(dispatcher.EvAgentCooldown)
	if ev.Until == nil || !ev.Until.Equal(first.Add(5*time.Hour)) {
		t.Fatalf("Until = %v, muốn lúc assignment cũ nhất ra khỏi cửa sổ 5h (%v)", ev.Until, first.Add(5*time.Hour))
	}
	if l := e.lead(); l.AgentID != "M/claude" {
		t.Fatalf("lead chạm hạn mềm phải nhường vai: %+v", l)
	}
}

func TestLogIdleCreatesUnblockOnce(t *testing.T) { // BR-16
	e := newEnv(t)
	e.machine("L", dispatcher.KindClaude)
	e.machine("G", dispatcher.KindAntigravity)
	e.job("t1", dispatcher.JobCode, dispatcher.KindAntigravity)
	a := e.request("G", "G/antigravity")
	e.advance(10*time.Minute, "L", "G")
	if err := e.svc.Log("G", a.ID); err != nil {
		t.Fatal(err)
	}
	e.advance(14*time.Minute, "L", "G")
	if n := e.num(`SELECT COUNT(*) FROM jobs WHERE kind='UNBLOCK'`); n != 0 {
		t.Fatalf("mới 14 phút kể từ log cuối đã có %d UNBLOCK", n)
	}
	e.advance(10*time.Minute, "L", "G")
	if n := e.num(`SELECT COUNT(*) FROM jobs WHERE kind='UNBLOCK' AND task_id='t1' AND lead_only=1`); n != 1 {
		t.Fatalf("UNBLOCK = %d, muốn đúng 1 dù Tick nhiều lần", n)
	}
	if s := e.str(`SELECT status FROM assignments WHERE id=?`, a.ID); s != "ACTIVE" {
		t.Fatalf("không có log không được thu hồi: assignment = %s", s)
	}
}
