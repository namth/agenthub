package dispatcher_test

// Test nghiệm thu TASK-032 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: DEC-29, docs/features/agent-auth.md §7 BR-42; TC-64, TC-65.

import (
	"errors"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/dispatcher"
)

// eligible tạo sẵn agent với lead_eligible / lead_priority trước khi máy kết nối.
func (e *env) eligible(worker, kind string, prio any) {
	e.t.Helper()
	e.exec(`INSERT INTO workers (id, name, os, token_hash) VALUES (?, ?, 'darwin', 'h')`, worker, worker)
	id := worker + "/" + map[string]string{dispatcher.KindClaude: "claude", dispatcher.KindAntigravity: "antigravity"}[kind]
	e.exec(`INSERT INTO agents (id, worker_id, kind, lead_eligible, lead_priority) VALUES (?, ?, ?, 1, ?)`, id, worker, kind, prio)
	if _, err := e.svc.Connect(worker, []string{kind}); err != nil {
		e.t.Fatalf("Connect(%s): %v", worker, err)
	}
}

func TestNewAntigravityNotLeadEligible(t *testing.T) {
	e := newEnv(t)
	e.machine("m", dispatcher.KindClaude, dispatcher.KindAntigravity)
	if n := e.num(`SELECT lead_eligible FROM agents WHERE id='m/antigravity'`); n != 0 {
		t.Fatalf("agent Antigravity mới lead_eligible = %d, muốn 0", n)
	}
	if n := e.num(`SELECT lead_eligible FROM agents WHERE id='m/claude'`); n != 1 {
		t.Fatalf("agent Claude mới lead_eligible = %d, muốn 1", n)
	}
	// Admin đã bật thì kết nối lại không ghi đè.
	e.exec(`UPDATE agents SET lead_eligible = 1 WHERE id='m/antigravity'`)
	if _, err := e.svc.Connect("m", []string{dispatcher.KindClaude, dispatcher.KindAntigravity}); err != nil {
		t.Fatal(err)
	}
	if n := e.num(`SELECT lead_eligible FROM agents WHERE id='m/antigravity'`); n != 1 {
		t.Fatal("kết nối lại không được tắt lead_eligible Admin đã bật")
	}
}

func TestLeadAnyKind(t *testing.T) { // TC-64
	e := newEnv(t)
	e.eligible("g", dispatcher.KindAntigravity, nil)
	l := e.lead()
	if l.State != "HELD" || l.AgentID != "g/antigravity" || l.Epoch != 1 {
		t.Fatalf("Antigravity lead_eligible phải nhận lease: %+v", l)
	}
	replan, err := e.svc.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: dispatcher.JobReplan, RequiredAgent: dispatcher.KindClaude, LeadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if a := e.request("g", "g/antigravity"); a == nil || a.JobID != replan {
		t.Fatalf("lead Antigravity phải nhận job lead_only (required CLAUDE): %+v", a)
	}
	// Job thường required CLAUDE vẫn không giao cho Antigravity.
	e.job("t2", dispatcher.JobSpec, dispatcher.KindClaude)
	if err := e.svc.Submit("g", e.str(`SELECT id FROM assignments WHERE job_id=?`, replan)); err != nil {
		t.Fatal(err)
	}
	if a := e.request("g", "g/antigravity"); a != nil {
		t.Fatalf("job SPEC (required CLAUDE, không lead_only) không được giao cho Antigravity: %+v", a)
	}
}

func TestCandidateOrderingAcrossKinds(t *testing.T) { // BR-06 sửa theo DEC-29
	e := newEnv(t)
	e.machine("L", dispatcher.KindClaude)
	e.eligible("c", dispatcher.KindClaude, 5)
	e.eligible("g", dispatcher.KindAntigravity, 1)
	e.machine("x", dispatcher.KindAntigravity) // lead_eligible = 0: không bao giờ là ứng viên
	if err := e.svc.Release("L", "L/claude", 1); err != nil {
		t.Fatal(err)
	}
	if l := e.lead(); l.AgentID != "g/antigravity" || l.Epoch != 2 {
		t.Fatalf("lead_priority nhỏ nhất (Antigravity, 1) phải thắng: %+v", l)
	}
	if _, err := e.svc.AdminAssign("x/antigravity"); !errors.Is(err, dispatcher.ErrAgentNotEligible) {
		t.Fatalf("AdminAssign agent lead_eligible=0: err = %v", err)
	}
	if l, err := e.svc.AdminAssign("c/claude"); err != nil || l.AgentID != "c/claude" {
		t.Fatalf("AdminAssign Claude: %+v %v", l, err)
	}
	if l, err := e.svc.AdminAssign("g/antigravity"); err != nil || l.AgentID != "g/antigravity" {
		t.Fatalf("AdminAssign Antigravity lead_eligible: %+v %v", l, err)
	}
}

func TestVacantWhenNoEligibleAndEtaAnyKind(t *testing.T) { // BR-07 sửa
	e := newEnv(t)
	e.eligible("g", dispatcher.KindAntigravity, nil)
	reset := e.clk.Now().Add(40 * time.Minute)
	if err := e.svc.Exhausted("g", "g/antigravity", "", ptr(reset), "usage limit", "", ""); err != nil {
		t.Fatal(err)
	}
	if l := e.lead(); l.State != "VACANT" {
		t.Fatalf("không còn ứng viên → VACANT: %+v", l)
	}
	ev, ok := e.rec.last(dispatcher.EvLeadVacant)
	if !ok || ev.Until == nil || !ev.Until.Equal(reset) {
		t.Fatalf("LEAD_VACANT.Until = %+v, muốn cooldown_until của ứng viên Antigravity", ev)
	}
	e.advance(41*time.Minute, "g")
	if l := e.lead(); l.AgentID != "g/antigravity" {
		t.Fatalf("hết cooldown, ứng viên Antigravity tự nhận lease: %+v", l)
	}
}

func TestQAIndependent(t *testing.T) { // TC-65
	e := newEnv(t)
	e.machine("c1", dispatcher.KindClaude)
	e.machine("c2", dispatcher.KindClaude)
	code := e.job("t1", dispatcher.JobCode, dispatcher.KindClaude)
	a := e.request("c1", "c1/claude")
	if a == nil || a.JobID != code {
		t.Fatalf("c1 nhận CODE: %+v", a)
	}
	if err := e.svc.Submit("c1", a.ID); err != nil {
		t.Fatal(err)
	}
	qa := e.job("t1", dispatcher.JobQAReview, dispatcher.KindClaude)
	if got := e.request("c1", "c1/claude"); got != nil {
		t.Fatalf("tác giả CODE không được tự review: %+v", got)
	}
	if got := e.request("c2", "c2/claude"); got == nil || got.JobID != qa {
		t.Fatalf("agent khác phải nhận QA_REVIEW: %+v", got)
	}
	// QA của task khác thì tác giả t1 vẫn nhận được.
	other := e.job("t2", dispatcher.JobQAReview, dispatcher.KindClaude)
	if got := e.request("c1", "c1/claude"); got == nil || got.JobID != other {
		t.Fatalf("QA task khác giao cho c1: %+v", got)
	}
}
