package dispatcher_test

// Test nghiệm thu TASK-027 — do trưởng phòng viết, file bị khoá (tests.lock).
// DEC-16, BR-25 (chia việc), BR-32 (paused), BR-33 (lead_eligible), DEC-15 (agent_projects).

import (
	"fmt"
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
)

func (e *env) project2(weight int) {
	e.t.Helper()
	e.exec(`INSERT INTO projects (id, name, github_repo, weight) VALUES ('p2', 'Edu', 'namth/edusystem', ?)`, weight)
	e.exec(`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('q1', 'p2', 'Q', 'mô tả', 'u1', 'READY')`)
}

func (e *env) jobs(project, task string, n int) {
	e.t.Helper()
	for i := 0; i < n; i++ {
		if _, err := e.svc.EnqueueJob(dispatcher.NewJob{ProjectID: project, TaskID: task, Kind: "CODE", RequiredAgent: "ANTIGRAVITY", BranchName: fmt.Sprintf("feat/task-%s-%d", task, i)}); err != nil {
			e.t.Fatal(err)
		}
		e.clk.Add(1)
	}
}

func (e *env) running(project string) int64 {
	return e.num(`SELECT COUNT(*) FROM jobs WHERE project_id=? AND status='RUNNING'`, project)
}

func TestFairShareByWeight(t *testing.T) { // TC-41
	e := newEnv(t)
	e.project2(2)
	e.jobs("p1", "t1", 10) // p1 tạo trước: FIFO cũ sẽ giao hết cho p1
	e.jobs("p2", "q1", 10)
	for i := 0; i < 6; i++ {
		w := fmt.Sprintf("m%d", i)
		e.machine(w, dispatcher.KindAntigravity)
		if a := e.request(w, w+"/antigravity"); a == nil {
			t.Fatalf("máy %s không nhận được việc", w)
		}
	}
	if p1, p2 := e.running("p1"), e.running("p2"); p1 != 2 || p2 != 4 {
		t.Fatalf("RUNNING p1=%d p2=%d, muốn 2 : 4 theo weight 1 : 2", p1, p2)
	}
}

func TestPriorityStillBeatsFairness(t *testing.T) {
	e := newEnv(t)
	e.project2(5)
	e.jobs("p2", "q1", 3)
	urgent, _ := e.svc.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "CODE", RequiredAgent: "ANTIGRAVITY", Priority: 5})
	e.machine("m", dispatcher.KindAntigravity)
	if a := e.request("m", "m/antigravity"); a == nil || a.JobID != urgent {
		t.Fatalf("priority nhỏ hơn vẫn đi trước dù khác dự án: %+v", a)
	}
}

func TestMaxParallel(t *testing.T) { // TC-42
	e := newEnv(t)
	e.project2(1)
	e.exec(`UPDATE projects SET max_parallel = 1 WHERE id = 'p2'`)
	e.jobs("p2", "q1", 5)
	got := 0
	for i := 0; i < 3; i++ {
		w := fmt.Sprintf("m%d", i)
		e.machine(w, dispatcher.KindAntigravity)
		if a := e.request(w, w+"/antigravity"); a != nil {
			got++
		}
	}
	if got != 1 || e.running("p2") != 1 {
		t.Fatalf("max_parallel=1: giao %d job, RUNNING %d", got, e.running("p2"))
	}
}

func TestArchivedProjectSkipped(t *testing.T) {
	e := newEnv(t)
	e.project2(1)
	e.exec(`UPDATE projects SET status = 'ARCHIVED' WHERE id = 'p2'`)
	e.jobs("p2", "q1", 2)
	e.machine("m", dispatcher.KindAntigravity)
	if a := e.request("m", "m/antigravity"); a != nil {
		t.Fatalf("dự án ARCHIVED không được giao việc: %+v", a)
	}
}

func TestAgentProjectAllowlist(t *testing.T) { // TC-43
	e := newEnv(t)
	e.project2(1)
	e.jobs("p2", "q1", 2)
	e.machine("m", dispatcher.KindAntigravity)
	e.exec(`INSERT INTO agent_projects (agent_id, project_id) VALUES ('m/antigravity', 'p1')`)
	if a := e.request("m", "m/antigravity"); a != nil {
		t.Fatalf("agent chỉ nhận p1 không được nhận job p2: %+v", a)
	}
	e.jobs("p1", "t1", 1)
	if a := e.request("m", "m/antigravity"); a == nil {
		t.Fatal("agent phải nhận job của dự án trong danh sách")
	}
}

func TestLeadOnlyIgnoresAllowlist(t *testing.T) { // BR-29
	e := newEnv(t)
	e.project2(1)
	e.machine("L", dispatcher.KindClaude)
	e.exec(`INSERT INTO agent_projects (agent_id, project_id) VALUES ('L/claude', 'p1')`)
	replan, _ := e.svc.EnqueueJob(dispatcher.NewJob{ProjectID: "p2", TaskID: "q1", Kind: "REPLAN", RequiredAgent: "CLAUDE", LeadOnly: true})
	if a := e.request("L", "L/claude"); a == nil || a.JobID != replan {
		t.Fatalf("trưởng phòng chung xử lý job lead của mọi dự án: %+v", a)
	}
}

func TestPausedWorker(t *testing.T) { // TC-51, BR-32
	e := newEnv(t)
	e.machine("m", dispatcher.KindAntigravity, dispatcher.KindClaude)
	e.exec(`UPDATE workers SET paused = 1 WHERE id = 'm'`)
	e.jobs("p1", "t1", 1)
	if a := e.request("m", "m/antigravity"); a != nil {
		t.Fatalf("máy tạm dừng không được nhận việc: %+v", a)
	}
	// Máy đang giữ lease từ lúc Connect: Tick phải thu hồi và không trao lại cho máy tạm dừng.
	if err := e.svc.Tick(); err != nil {
		t.Fatal(err)
	}
	if l := e.lead(); l.AgentID == "m/claude" {
		t.Fatalf("máy tạm dừng không được giữ vai trưởng phòng: %+v", l)
	}
}

func TestLeadEligible(t *testing.T) { // TC-53, BR-33
	e := newEnv(t)
	e.exec(`INSERT INTO workers (id, name, os, token_hash) VALUES ('p', 'p', 'darwin', 'h')`)
	e.exec(`INSERT INTO agents (id, worker_id, kind, lead_eligible) VALUES ('p/claude', 'p', 'CLAUDE', 0)`)
	if _, err := e.svc.Connect("p", []string{dispatcher.KindClaude}); err != nil {
		t.Fatal(err)
	}
	if l := e.lead(); l.State != "VACANT" {
		t.Fatalf("agent lead_eligible=0 không được nhận lease: %+v", l)
	}
	if _, err := e.svc.AdminAssign("p/claude"); err == nil {
		t.Fatal("Admin trao vai cho agent lead_eligible=0 phải bị từ chối (ErrAgentNotEligible)")
	}
	e.machine("o", dispatcher.KindClaude)
	if l := e.lead(); l.AgentID != "o/claude" {
		t.Fatalf("agent đủ điều kiện nhận lease: %+v", l)
	}
}
