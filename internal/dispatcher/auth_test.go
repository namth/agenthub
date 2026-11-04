package dispatcher_test

// Test nghiệm thu TASK-030 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-auth.md §5–§7 (BR-37 → BR-40), TC-58, TC-60, TC-61.

import (
	"errors"
	"strings"
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
)

var _ dispatcher.AuthAPI = (*dispatcher.Service)(nil)

func (e *env) bare(worker string) {
	e.t.Helper()
	e.exec(`INSERT INTO workers (id, name, os, token_hash) VALUES (?, ?, 'darwin', 'h')`, worker, worker)
}

func (e *env) connectAuth(worker, gh string, hello ...dispatcher.AgentHello) []dispatcher.Agent {
	e.t.Helper()
	ags, err := e.svc.ConnectAuth(worker, gh, hello)
	if err != nil {
		e.t.Fatalf("ConnectAuth(%s): %v", worker, err)
	}
	return ags
}

func (e *env) setAuth(worker, agent, status string) {
	e.t.Helper()
	if err := e.svc.SetAuth(worker, agent, status, ""); err != nil {
		e.t.Fatalf("SetAuth(%s, %s): %v", agent, status, err)
	}
}

func TestConnectAuthStoresStatusBeforeLease(t *testing.T) {
	e := newEnv(t)
	e.bare("m")
	ags := e.connectAuth("m", dispatcher.AuthOK,
		dispatcher.AgentHello{Kind: dispatcher.KindClaude, AuthStatus: dispatcher.AuthNeeds, AuthDetail: "claude: loggedIn=false"},
		dispatcher.AgentHello{Kind: dispatcher.KindAntigravity, AuthStatus: dispatcher.AuthUnknown})
	if len(ags) != 2 {
		t.Fatalf("ConnectAuth trả %d agent", len(ags))
	}
	for _, a := range ags {
		want := map[string]string{"m/claude": dispatcher.AuthNeeds, "m/antigravity": dispatcher.AuthUnknown}[a.ID]
		if a.AuthStatus != want {
			t.Fatalf("Agent %s AuthStatus = %q, muốn %q", a.ID, a.AuthStatus, want)
		}
	}
	if s := e.str(`SELECT auth_status FROM agents WHERE id='m/claude'`); s != "NEEDS_AUTH" {
		t.Fatalf("auth_status = %s", s)
	}
	if s := e.str(`SELECT auth_detail FROM agents WHERE id='m/claude'`); s != "claude: loggedIn=false" {
		t.Fatalf("auth_detail = %q", s)
	}
	if s := e.str(`SELECT auth_checked_at FROM agents WHERE id='m/claude'`); s == "" {
		t.Fatal("auth_checked_at phải được ghi")
	}
	if s := e.str(`SELECT gh_auth FROM workers WHERE id='m'`); s != "OK" {
		t.Fatalf("gh_auth = %s", s)
	}
	if l := e.lead(); l.State != "VACANT" {
		t.Fatalf("agent NEEDS_AUTH không được nhận lease lúc bắt tay: %+v", l)
	}
	if e.rec.count(dispatcher.EvLeaseGranted) != 0 {
		t.Fatal("không được phát LEASE_GRANTED rồi thu hồi ngay (phải ghi auth trước khi trao lease)")
	}
	// Runner cũ (Connect không có auth) không xoá trạng thái đã biết.
	if _, err := e.svc.Connect("m", []string{dispatcher.KindClaude, dispatcher.KindAntigravity}); err != nil {
		t.Fatal(err)
	}
	if s := e.str(`SELECT auth_status FROM agents WHERE id='m/claude'`); s != "NEEDS_AUTH" {
		t.Fatalf("Connect không auth phải giữ NEEDS_AUTH, có %s", s)
	}
	// Agent mới mặc định UNKNOWN.
	e.machine("n", dispatcher.KindClaude)
	if s := e.str(`SELECT auth_status FROM agents WHERE id='n/claude'`); s != "UNKNOWN" {
		t.Fatalf("agent mới auth_status = %s, muốn UNKNOWN", s)
	}
}

func TestAuthGuardRequestJob(t *testing.T) { // TC-58, BR-37
	e := newEnv(t)
	e.bare("A")
	e.connectAuth("A", "", dispatcher.AgentHello{Kind: dispatcher.KindClaude, AuthStatus: dispatcher.AuthNeeds})
	e.machine("B", dispatcher.KindClaude) // UNKNOWN vẫn được nhận việc
	job := e.job("t1", dispatcher.JobSpec, dispatcher.KindClaude)
	if a := e.request("A", "A/claude"); a != nil {
		t.Fatalf("agent NEEDS_AUTH nhận job: %+v", a)
	}
	a := e.request("B", "B/claude")
	if a == nil || a.JobID != job {
		t.Fatalf("agent UNKNOWN phải nhận job, có %+v", a)
	}
	// gh chưa đăng nhập → mọi agent của máy không nhận việc.
	e.bare("C")
	e.connectAuth("C", dispatcher.AuthNeeds, dispatcher.AgentHello{Kind: dispatcher.KindAntigravity, AuthStatus: dispatcher.AuthOK})
	e.job("t2", dispatcher.JobCode, dispatcher.KindAntigravity)
	if a := e.request("C", "C/antigravity"); a != nil {
		t.Fatalf("máy gh_auth=NEEDS_AUTH nhận job: %+v", a)
	}
	if err := e.svc.SetGhAuth("C", dispatcher.AuthOK, ""); err != nil {
		t.Fatal(err)
	}
	if a := e.request("C", "C/antigravity"); a == nil {
		t.Fatal("gh đã OK phải nhận được job")
	}
	// Đăng nhập lại → nhận việc.
	e.setAuth("A", "A/claude", dispatcher.AuthOK)
	e.job("t3", dispatcher.JobSpec, dispatcher.KindClaude)
	if a := e.request("A", "A/claude"); a == nil {
		t.Fatal("agent đã OK phải nhận được job")
	}
}

func TestSetAuthValidationAndEvents(t *testing.T) {
	e := newEnv(t)
	e.machine("m", dispatcher.KindAntigravity)
	e.machine("x", dispatcher.KindAntigravity)
	if err := e.svc.SetAuth("x", "m/antigravity", dispatcher.AuthOK, ""); !errors.Is(err, dispatcher.ErrAgentNotOwned) {
		t.Fatalf("SetAuth agent máy khác: err = %v, muốn ErrAgentNotOwned", err)
	}
	if err := e.svc.SetAuth("m", "m/antigravity", "LOGGED", ""); !errors.Is(err, dispatcher.ErrInvalidAuth) {
		t.Fatalf("SetAuth giá trị sai: err = %v, muốn ErrInvalidAuth", err)
	}
	if err := e.svc.SetGhAuth("m", "maybe", ""); !errors.Is(err, dispatcher.ErrInvalidAuth) {
		t.Fatalf("SetGhAuth giá trị sai: err = %v, muốn ErrInvalidAuth", err)
	}
	e.setAuth("m", "m/antigravity", dispatcher.AuthNeeds)
	e.setAuth("m", "m/antigravity", dispatcher.AuthNeeds) // không đổi → không phát
	e.setAuth("m", "m/antigravity", dispatcher.AuthOK)
	if n := e.rec.count(dispatcher.EvAgentAuth); n != 2 {
		t.Fatalf("AGENT_AUTH phát %d lần, muốn 2 (chỉ khi đổi giá trị)", n)
	}
	ev, _ := e.rec.last(dispatcher.EvAgentAuth)
	if ev.AgentID != "m/antigravity" || ev.WorkerID != "m" || ev.AuthStatus != "OK" || ev.PrevAuthStatus != "NEEDS_AUTH" {
		t.Fatalf("AGENT_AUTH = %+v", ev)
	}
	if err := e.svc.SetGhAuth("m", dispatcher.AuthNeeds, "gh: not logged in"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetGhAuth("m", dispatcher.AuthNeeds, "gh: not logged in"); err != nil {
		t.Fatal(err)
	}
	if n := e.rec.count(dispatcher.EvWorkerAuth); n != 1 {
		t.Fatalf("WORKER_AUTH phát %d lần, muốn 1", n)
	}
	wev, _ := e.rec.last(dispatcher.EvWorkerAuth)
	if wev.WorkerID != "m" || wev.AuthStatus != "NEEDS_AUTH" || wev.PrevAuthStatus != "UNKNOWN" || wev.Detail != "gh: not logged in" {
		t.Fatalf("WORKER_AUTH = %+v", wev)
	}
}

func TestAuthDetailSanitized(t *testing.T) { // BR-41 / BR-20
	e := newEnv(t)
	e.machine("m", dispatcher.KindClaude)
	secret := "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	detail := "lỗi token " + secret + " " + strings.Repeat("x", 200)
	if err := e.svc.SetAuth("m", "m/claude", dispatcher.AuthNeeds, detail); err != nil {
		t.Fatal(err)
	}
	got := e.str(`SELECT auth_detail FROM agents WHERE id='m/claude'`)
	if strings.Contains(got, "ghp_ABCDEF") || strings.Contains(got, strings.Repeat("x", 40)) {
		t.Fatalf("auth_detail chưa che secret: %q", got)
	}
	if n := len([]rune(got)); n > 80 || n == 0 {
		t.Fatalf("auth_detail dài %d ký tự, muốn 1–80", n)
	}
	ev, _ := e.rec.last(dispatcher.EvAgentAuth)
	if strings.Contains(ev.Detail, "ghp_ABCDEF") || len([]rune(ev.Detail)) > 80 {
		t.Fatalf("Event.Detail chưa che: %q", ev.Detail)
	}
}

func TestAuthRequiredRequeuesWithoutAttemptPenalty(t *testing.T) { // TC-60, BR-39
	e := newEnv(t)
	e.machine("lead", dispatcher.KindClaude)
	ws := []string{"A", "B", "C", "D", "E"}
	for _, w := range ws {
		e.machine(w, dispatcher.KindAntigravity)
	}
	job := e.job("t1", dispatcher.JobCode, dispatcher.KindAntigravity)
	a1 := e.request("A", "A/antigravity")
	if a1 == nil {
		t.Fatal("không giao được job")
	}
	if err := e.svc.AuthRequired("B", "A/antigravity", a1.ID, "x", "", ""); !errors.Is(err, dispatcher.ErrAgentNotOwned) {
		t.Fatalf("AuthRequired từ máy khác: err = %v, muốn ErrAgentNotOwned", err)
	}
	if err := e.svc.AuthRequired("A", "A/antigravity", a1.ID, "Please run /login", "2 files changed", "wip1"); err != nil {
		t.Fatal(err)
	}
	if s := e.str(`SELECT auth_status FROM agents WHERE id='A/antigravity'`); s != "NEEDS_AUTH" {
		t.Fatalf("agent = %s, muốn NEEDS_AUTH", s)
	}
	if s := e.str(`SELECT status FROM agents WHERE id='A/antigravity'`); s == "BUSY" {
		t.Fatal("agent không còn BUSY sau khi nhả job")
	}
	if s := e.str(`SELECT status FROM assignments WHERE id=?`, a1.ID); s != "RECLAIMED" {
		t.Fatalf("assignment = %s, muốn RECLAIMED", s)
	}
	if s := e.str(`SELECT wip_commit FROM assignments WHERE id=?`, a1.ID); s != "wip1" {
		t.Fatalf("wip_commit = %q", s)
	}
	if s := e.str(`SELECT status FROM jobs WHERE id=?`, job); s != "QUEUED" {
		t.Fatalf("job = %s, muốn QUEUED", s)
	}
	if err := e.svc.AuthRequired("A", "A/antigravity", a1.ID, "x", "", ""); !errors.Is(err, dispatcher.ErrStaleAssignment) {
		t.Fatalf("AuthRequired lặp lại: err = %v, muốn ErrStaleAssignment", err)
	}
	if a := e.request("A", "A/antigravity"); a != nil {
		t.Fatal("agent NEEDS_AUTH không được nhận lại job")
	}
	prev := a1
	for _, w := range ws[1:] { // thêm 4 lần mất đăng nhập: vẫn không vượt MaxAttempts
		a := e.request(w, w+"/antigravity")
		if a == nil || a.JobID != job || a.Attempt != a1.Attempt || a.ID == prev.ID {
			t.Fatalf("giao lại cho %s = %+v, muốn cùng job, attempt %d, id mới", w, a, a1.Attempt)
		}
		if w == "B" && a.HandoffNote != "2 files changed" {
			t.Fatalf("handoff_note = %q", a.HandoffNote)
		}
		if err := e.svc.AuthRequired(w, w+"/antigravity", a.ID, "not logged in", "", ""); err != nil {
			t.Fatal(err)
		}
		prev = a
	}
	if s := e.str(`SELECT status FROM jobs WHERE id=?`, job); s != "QUEUED" {
		t.Fatalf("job = %s sau 5 lần mất đăng nhập, muốn vẫn QUEUED", s)
	}
	if e.rec.count(dispatcher.EvJobFailed) != 0 {
		t.Fatal("mất đăng nhập không được tính vào MaxAttempts")
	}
	if n := e.num(`SELECT COUNT(*) FROM jobs WHERE kind='REPLAN'`); n != 0 {
		t.Fatalf("có %d job REPLAN, muốn 0", n)
	}
	if e.rec.count(dispatcher.EvJobAvailable) == 0 {
		t.Fatal("job về hàng đợi phải phát JOB_AVAILABLE")
	}
}

func TestLeadLosesAuth(t *testing.T) { // TC-61, BR-40
	e := newEnv(t)
	e.machine("m1", dispatcher.KindClaude)
	e.machine("m2", dispatcher.KindClaude)
	if l := e.lead(); l.AgentID != "m1/claude" || l.Epoch != 1 {
		t.Fatalf("lead ban đầu = %+v", l)
	}
	e.setAuth("m1", "m1/claude", dispatcher.AuthNeeds)
	l := e.lead()
	if l.State != "HELD" || l.AgentID != "m2/claude" || l.Epoch != 2 {
		t.Fatalf("sau khi lead mất đăng nhập: %+v, muốn m2/claude epoch 2", l)
	}
	ev, _ := e.rec.last(dispatcher.EvLeaseRevoked)
	if ev.AgentID != "m1/claude" || ev.Reason != dispatcher.RevokeAuth {
		t.Fatalf("LEASE_REVOKED = %+v, muốn m1/claude reason AUTH", ev)
	}
	if err := e.svc.SetGhAuth("m2", dispatcher.AuthNeeds, ""); err != nil {
		t.Fatal(err)
	}
	if l := e.lead(); l.State != "VACANT" {
		t.Fatalf("lead có gh NEEDS_AUTH cũng phải bị thu hồi: %+v", l)
	}
	if _, err := e.svc.AdminAssign("m1/claude"); !errors.Is(err, dispatcher.ErrAgentNotEligible) {
		t.Fatalf("AdminAssign agent NEEDS_AUTH: err = %v, muốn ErrAgentNotEligible", err)
	}
	if _, err := e.svc.AdminAssign("m2/claude"); !errors.Is(err, dispatcher.ErrAgentNotEligible) {
		t.Fatalf("AdminAssign agent máy gh NEEDS_AUTH: err = %v, muốn ErrAgentNotEligible", err)
	}
	e.setAuth("m1", "m1/claude", dispatcher.AuthOK)
	if l := e.lead(); l.AgentID != "m1/claude" || l.Epoch != 3 {
		t.Fatalf("đăng nhập lại khi VACANT phải nhận lease: %+v", l)
	}
}
