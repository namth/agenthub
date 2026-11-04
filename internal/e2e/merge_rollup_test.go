package e2e_test

// Test do trưởng phòng viết: merge qua tasks.Service (đường gọi thật của nút Merge) phải gộp task cha.

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/pipeline"
	"github.com/namth/agenthub/internal/tasks"
)

type okMerger struct{}

func (okMerger) MergePR(context.Context, int) error { return nil }

func TestMergeViaServiceRollsUpParent(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', 'g1', 'namth', 'ADMIN')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'A', 'namth/agenthub')`,
		`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t1', 'p1', 'Cha', 'x', 'u1', 'READY')`,
		`INSERT INTO tasks (id, project_id, parent_id, title, description, created_by, status, pr_url) VALUES ('s1', 'p1', 't1', 'A', 'x', 'u1', 'QA_PASSED', 'https://x/pull/1')`,
		`INSERT INTO tasks (id, project_id, parent_id, title, description, created_by, status, pr_url) VALUES ('s2', 'p1', 't1', 'B', 'x', 'u1', 'QA_PASSED', 'https://x/pull/2')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	disp := dispatcher.New(db, wall{}, dispatcher.DefaultConfig(), nil)
	pl := pipeline.New(db, disp, &gh{}, "p1")
	svc := tasks.NewService(db, &gh{}, disp, "p1", tasks.WithMerger(okMerger{}),
		tasks.WithOnMerged(func(pr string) { pl.OnMerged(context.Background(), pr) }))
	ctx := context.Background()
	if err := svc.Merge(ctx, "s1"); err != nil {
		t.Fatal(err)
	}
	var st string
	db.QueryRow(`SELECT status FROM tasks WHERE id='t1'`).Scan(&st)
	if st == "DONE" {
		t.Fatal("còn s2 chưa merge thì cha chưa DONE")
	}
	if err := svc.Merge(ctx, "s2"); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(`SELECT status FROM tasks WHERE id='t1'`).Scan(&st)
	if st != "DONE" {
		t.Fatalf("merge hết sub-task qua nút Merge: cha = %s, muốn DONE", st)
	}
}
