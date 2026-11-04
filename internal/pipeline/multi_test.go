package pipeline_test

// Test nghiệm thu TASK-026 (phần pipeline) — do trưởng phòng viết, file bị khoá (tests.lock). BR-24.

import (
	"context"
	"strings"
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/pipeline"
	"github.com/namth/agenthub/internal/tasks"
)

func TestSubtasksUseProjectRepoAndRepoFor(t *testing.T) {
	r := newRig(t)
	r.exec(`INSERT INTO projects (id, name, github_repo) VALUES ('p2', 'Edu', 'namth/edusystem')`)
	r.exec(`INSERT INTO tasks (id, project_id, github_issue_id, title, description, created_by, status) VALUES ('q1', 'p2', 300, 'Edu', 'Mô tả edu', 'u1', 'PLANNING')`)
	gh2 := &fakeGH{next: 50}
	r.p = pipeline.New(r.db, r.disp, r.gh, "p1", pipeline.WithGitHubFor(func(projectID string) tasks.GitHub {
		if projectID == "p2" {
			return gh2
		}
		return r.gh
	}))
	r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p2", TaskID: "q1", Kind: "SPEC", RequiredAgent: "CLAUDE"})
	r.run("m/claude", "", spec3())
	if len(gh2.issues) != 3 || len(r.gh.issues) != 0 || !strings.Contains(gh2.issues[0], "parent:#300") {
		t.Fatalf("issue con phải tạo trên repo của dự án cha: p1=%v p2=%v", r.gh.issues, gh2.issues)
	}
	if n := r.num(`SELECT COUNT(*) FROM tasks WHERE parent_id='q1' AND project_id='p2'`); n != 3 {
		t.Fatalf("sub-task thuộc dự án p2: %d", n)
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE project_id='p2' AND kind='CODE'`); n != 3 {
		t.Fatalf("job CODE thuộc dự án p2: %d", n)
	}
	a, _ := r.disp.RequestJob("m", "m/antigravity")
	if got := r.p.RepoFor(*a); got != "namth/edusystem" {
		t.Fatalf("RepoFor = %q, muốn namth/edusystem", got)
	}
}

func TestReconcileAllProjects(t *testing.T) {
	r := newRig(t)
	r.exec(`INSERT INTO projects (id, name, github_repo) VALUES ('p2', 'Edu', 'namth/edusystem')`)
	r.exec(`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('q1', 'p2', 'Edu', 'x', 'u1', 'PLANNING')`)
	p := pipeline.New(r.db, r.disp, r.gh, "", pipeline.WithGitHubFor(func(string) tasks.GitHub { return r.gh }))
	n, err := p.Reconcile(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("bù SPEC cho task PLANNING của mọi dự án (t1, q1): %d %v", n, err)
	}
	if c := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id='q1' AND project_id='p2' AND kind='SPEC'`); c != 1 {
		t.Fatalf("job bù phải thuộc đúng dự án của task: %d", c)
	}
}
