package dispatcher_test

// Test nghiệm thu TASK-043 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/ops-readiness.md BR-58, DEC-37; TC-86, TC-87.

import (
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
)

// partnerSetup: owner u1 (ADMIN, có sẵn trong newEnv), partner u2 thành viên p1, dự án p2 partner không tham gia.
func partnerSetup(e *env) {
	e.t.Helper()
	e.exec(`INSERT INTO users (id, github_id, username, role) VALUES ('u2', 'gh2', 'doi-tac', 'PARTNER')`)
	e.exec(`INSERT INTO projects (id, name, github_repo) VALUES ('p2', 'Bi mat', 'namth/bi-mat')`)
	e.exec(`INSERT INTO project_members (project_id, user_id, role) VALUES ('p1', 'u2', 'PARTNER')`)
}

func (e *env) ownedMachine(worker, owner string, kinds ...string) {
	e.t.Helper()
	e.exec(`INSERT INTO workers (id, name, os, token_hash, owner_user_id) VALUES (?, ?, 'darwin', 'h', ?)`, worker, worker, owner)
	if _, err := e.svc.Connect(worker, kinds); err != nil {
		e.t.Fatalf("Connect(%s): %v", worker, err)
	}
}

func (e *env) projJob(project, task, kind, required string, leadOnly bool) string {
	e.t.Helper()
	e.exec(`INSERT OR IGNORE INTO tasks (id, project_id, title, description, created_by) VALUES (?, ?, 't', 'd', 'u1')`, task, project)
	id, err := e.svc.EnqueueJob(dispatcher.NewJob{ProjectID: project, TaskID: task, Kind: kind, RequiredAgent: required, LeadOnly: leadOnly, BranchName: "feat/" + task})
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func TestPartnerMachineOnlyMemberProjects(t *testing.T) { // TC-86
	e := newEnv(t)
	partnerSetup(e)
	e.ownedMachine("may-dt", "u2", dispatcher.KindAntigravity)
	secret := e.projJob("p2", "s1", dispatcher.JobCode, dispatcher.KindAntigravity, false)
	if a := e.request("may-dt", "may-dt/antigravity"); a != nil {
		t.Fatalf("máy partner nhận job dự án mình không tham gia: %+v", a)
	}
	mine := e.projJob("p1", "m1", dispatcher.JobCode, dispatcher.KindAntigravity, false)
	a := e.request("may-dt", "may-dt/antigravity")
	if a == nil || a.JobID != mine {
		t.Fatalf("máy partner phải nhận job dự án mình là thành viên, có %+v", a)
	}
	// Máy owner và máy hệ thống (không chủ) vẫn nhận mọi dự án.
	e.ownedMachine("may-owner", "u1", dispatcher.KindAntigravity)
	if a := e.request("may-owner", "may-owner/antigravity"); a == nil || a.JobID != secret {
		t.Fatalf("máy owner phải nhận được job p2: %+v", a)
	}
	e.projJob("p2", "s2", dispatcher.JobCode, dispatcher.KindAntigravity, false)
	e.machine("he-thong", dispatcher.KindAntigravity)
	if a := e.request("he-thong", "he-thong/antigravity"); a == nil {
		t.Fatal("máy hệ thống (không có chủ) phải nhận mọi dự án")
	}
}

func TestPartnerFilterCombinesWithAllowlist(t *testing.T) {
	e := newEnv(t)
	partnerSetup(e)
	e.exec(`INSERT INTO projects (id, name, github_repo) VALUES ('p3', 'Chung', 'namth/chung')`)
	e.exec(`INSERT INTO project_members (project_id, user_id, role) VALUES ('p3', 'u2', 'PARTNER')`)
	e.ownedMachine("may-dt", "u2", dispatcher.KindAntigravity)
	e.exec(`INSERT INTO agent_projects (agent_id, project_id) VALUES ('may-dt/antigravity', 'p3')`)
	e.exec(`INSERT INTO agent_projects (agent_id, project_id) VALUES ('may-dt/antigravity', 'p2')`) // dòng sai lệch: vẫn không được nhận p2
	e.projJob("p1", "a1", dispatcher.JobCode, dispatcher.KindAntigravity, false)
	e.projJob("p2", "b1", dispatcher.JobCode, dispatcher.KindAntigravity, false)
	j3 := e.projJob("p3", "c1", dispatcher.JobCode, dispatcher.KindAntigravity, false)
	a := e.request("may-dt", "may-dt/antigravity")
	if a == nil || a.JobID != j3 {
		t.Fatalf("chỉ được nhận dự án vừa thuộc agent_projects vừa là thành viên (p3): %+v", a)
	}
	if a := e.request("may-dt", "may-dt/antigravity"); a != nil {
		t.Fatalf("không còn job hợp lệ, nhận %+v", a)
	}
}

func TestPartnerLeadOnlyJobsFiltered(t *testing.T) {
	e := newEnv(t)
	partnerSetup(e)
	e.exec(`INSERT INTO workers (id, name, os, token_hash, owner_user_id) VALUES ('may-dt', 'may-dt', 'darwin', 'h', 'u2')`)
	e.exec(`INSERT INTO agents (id, worker_id, kind, lead_eligible) VALUES ('may-dt/claude', 'may-dt', 'CLAUDE', 1)`)
	if _, err := e.svc.Connect("may-dt", []string{dispatcher.KindClaude}); err != nil {
		t.Fatal(err)
	}
	if l := e.lead(); l.AgentID != "may-dt/claude" {
		t.Fatalf("partner lead_eligible làm lead: %+v", l)
	}
	e.projJob("p2", "r2", dispatcher.JobReplan, dispatcher.KindClaude, true)
	if a := e.request("may-dt", "may-dt/claude"); a != nil {
		t.Fatalf("lead là máy partner không được nhận job lead_only của dự án mình không tham gia: %+v", a)
	}
	r1 := e.projJob("p1", "r1", dispatcher.JobReplan, dispatcher.KindClaude, true)
	if a := e.request("may-dt", "may-dt/claude"); a == nil || a.JobID != r1 {
		t.Fatalf("lead partner nhận job lead_only dự án mình: %+v", a)
	}
}

func TestReclaimWhenMembershipLost(t *testing.T) { // TC-87
	e := newEnv(t)
	partnerSetup(e)
	e.ownedMachine("may-dt", "u2", dispatcher.KindAntigravity)
	job := e.projJob("p1", "m1", dispatcher.JobCode, dispatcher.KindAntigravity, false)
	a := e.request("may-dt", "may-dt/antigravity")
	if a == nil {
		t.Fatal("không giao được")
	}
	attemptBefore := e.num(`SELECT attempt FROM jobs WHERE id=?`, job)
	e.advance(e.cfg.OfflineAfter/3, "may-dt") // vẫn còn quyền → không thu hồi
	if s := e.str(`SELECT status FROM assignments WHERE id=?`, a.ID); s != "ACTIVE" {
		t.Fatalf("còn là thành viên mà bị thu hồi: %s", s)
	}
	e.exec(`DELETE FROM project_members WHERE project_id='p1' AND user_id='u2'`)
	e.advance(e.cfg.OfflineAfter/3, "may-dt")
	if s := e.str(`SELECT status FROM assignments WHERE id=?`, a.ID); s != "RECLAIMED" {
		t.Fatalf("mất tư cách thành viên → assignment = %s, muốn RECLAIMED", s)
	}
	if s := e.str(`SELECT status FROM jobs WHERE id=?`, job); s != "QUEUED" {
		t.Fatalf("job = %s, muốn QUEUED", s)
	}
	if n := e.num(`SELECT attempt FROM jobs WHERE id=?`, job); n != attemptBefore {
		t.Fatalf("attempt %d → %d, không được tăng", attemptBefore, n)
	}
	if s := e.str(`SELECT status FROM agents WHERE id='may-dt/antigravity'`); s != "IDLE" {
		t.Fatalf("agent = %s, muốn IDLE", s)
	}
	ev, ok := e.rec.last(dispatcher.EvAbortTask)
	if !ok || ev.AssignmentID != a.ID {
		t.Fatalf("phải phát ABORT_TASK cho assignment bị thu hồi: %+v", ev)
	}
	if a := e.request("may-dt", "may-dt/antigravity"); a != nil {
		t.Fatalf("không còn là thành viên thì không nhận lại: %+v", a)
	}
}
