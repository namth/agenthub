package runner_test

// Test nghiệm thu TASK-004 — do trưởng phòng viết, file bị khoá (tests.lock).
// Runner thật ↔ gateway thật ↔ dispatcher thật (SQLite), CLI và git giả;
// cùng test cho Git và Executor cài đặt thật.

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
	"github.com/namth/agenthub/internal/runner"
	"net/http/httptest"
)

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }

type hubNotifier struct{ hub *gateway.Hub }

func (h *hubNotifier) Notify(e dispatcher.Event) {
	if h.hub != nil {
		h.hub.Notify(e)
	}
}

func waitDB(t *testing.T, db *sql.DB, q, want string, args ...any) {
	t.Helper()
	var got string
	for i := 0; i < 250; i++ {
		if err := db.QueryRow(q, args...).Scan(&got); err == nil && got == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s = %q, muốn %q", q, got, want)
}

func TestEndToEndWithRealServer(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "e2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', 'gh1', 'namth', 'ADMIN')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'AgentHub', 'namth/agenthub')`,
		`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t1', 'p1', 'A', 'B', 'u1', 'READY')`,
		`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t2', 'p1', 'C', 'D', 'u1', 'READY')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO workers (id, name, os, token_hash) VALUES ('mac', 'mac', 'darwin', ?)`, gateway.HashToken("tok-e2e")); err != nil {
		t.Fatal(err)
	}

	n := &hubNotifier{}
	disp := dispatcher.New(db, wallClock{}, dispatcher.DefaultConfig(), n)
	hub := gateway.New(disp, gateway.DBAuthenticator(db))
	n.hub = hub
	srv := httptest.NewServer(hub)
	defer srv.Close()
	defer hub.Close()

	ex := &fakeExec{steps: map[string][]step{"agy": {
		{output: "ok\n", exit: 0},
		{output: "error: Individual quota reached. Resets in 1h0m0s.\n", exit: 3},
	}}}
	r := runner.New(runner.Config{
		ServerURL: "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/agent",
		Token:     "tok-e2e", OS: "darwin",
		Agents:         []runner.AgentConfig{{Kind: "CLAUDE", Command: []string{"claude", "-p", "{prompt}"}}, {Kind: "ANTIGRAVITY", Command: []string{"agy", "-p", "{prompt}"}}},
		HeartbeatEvery: 100 * time.Millisecond, RenewEvery: 100 * time.Millisecond,
		LogFlushEvery: 20 * time.Millisecond, LogFlushBytes: 1024,
		ReconnectMin: 20 * time.Millisecond, ReconnectMax: 100 * time.Millisecond,
	}, ex, &fakeGit{sha: "deadbee"}, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	waitDB(t, db, `SELECT conn_status FROM workers WHERE id='mac'`, "ONLINE")
	waitDB(t, db, `SELECT state || ':' || agent_id FROM lead_lease WHERE id=1`, "HELD:mac/claude")

	// Job CODE thành công: runner nhận qua JOB_AVAILABLE, chạy, nộp.
	j1, err := disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "CODE", RequiredAgent: "ANTIGRAVITY", BranchName: "feat/task-t1"})
	if err != nil {
		t.Fatal(err)
	}
	waitDB(t, db, `SELECT status FROM jobs WHERE id=?`, "DONE", j1)
	waitDB(t, db, `SELECT status FROM agents WHERE id='mac/antigravity'`, "IDLE")

	// Job thứ hai hết limit: agent nghỉ, job về hàng đợi, có wip_commit.
	j2, err := disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t2", Kind: "CODE", RequiredAgent: "ANTIGRAVITY", BranchName: "feat/task-t2"})
	if err != nil {
		t.Fatal(err)
	}
	waitDB(t, db, `SELECT status FROM agents WHERE id='mac/antigravity'`, "COOLDOWN")
	waitDB(t, db, `SELECT status FROM jobs WHERE id=?`, "QUEUED", j2)
	waitDB(t, db, `SELECT wip_commit FROM assignments WHERE job_id=? AND status='EXHAUSTED'`, "deadbee", j2)

	// Lease được gia hạn đều (không hết hạn sau > TTL nhờ LEASE_RENEW).
	var exp1, exp2 string
	db.QueryRow(`SELECT expires_at FROM lead_lease WHERE id=1`).Scan(&exp1)
	time.Sleep(1200 * time.Millisecond)
	db.QueryRow(`SELECT expires_at FROM lead_lease WHERE id=1`).Scan(&exp2)
	if exp2 <= exp1 {
		t.Fatalf("expires_at không tăng (%s → %s): runner phải gửi LEASE_RENEW", exp1, exp2)
	}
}

// ---------- Git thật ----------

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestRealGitWorktreeLifecycle(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	git(t, root, "init", "-q", "--bare", "-b", "main", remote)
	seed := filepath.Join(root, "seed")
	git(t, root, "clone", "-q", remote, seed)
	os.WriteFile(filepath.Join(seed, "README.md"), []byte("x\n"), 0o644)
	git(t, seed, "add", ".")
	git(t, seed, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init")
	git(t, seed, "push", "-q", "origin", "main")

	// Máy A: tạo nhánh mới, sửa, commit WIP, push.
	cloneA := filepath.Join(root, "a")
	git(t, root, "clone", "-q", remote, cloneA)
	ga := runner.NewGit(cloneA, filepath.Join(cloneA, ".worktrees"))
	ctx := context.Background()
	dir, err := ga.Prepare(ctx, "feat/task-1")
	if err != nil {
		t.Fatal(err)
	}
	if git(t, dir, "rev-parse", "--abbrev-ref", "HEAD") != "feat/task-1" {
		t.Fatal("worktree phải ở nhánh feat/task-1")
	}
	if sha, err := ga.CommitWIP(ctx, dir, "wip(task-1): attempt 1 handoff [by Runner]"); err != nil || sha != "" {
		t.Fatalf("không có thay đổi: CommitWIP = %q, %v, muốn rỗng", sha, err)
	}
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o644)
	if st := ga.DiffStat(ctx, dir); !strings.Contains(st, "a.go") {
		t.Fatalf("DiffStat phải tính cả file mới chưa track: %q", st)
	}
	sha, err := ga.CommitWIP(ctx, dir, "wip(task-1): attempt 1 handoff [by Runner]")
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(sha) {
		t.Fatalf("CommitWIP = %q, %v, muốn SHA 40 ký tự", sha, err)
	}
	if err := ga.Push(ctx, dir, "feat/task-1"); err != nil {
		t.Fatal(err)
	}
	if err := ga.Cleanup(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("Cleanup phải xoá thư mục worktree")
	}

	// Máy B: nhận lại cùng nhánh, phải thấy commit của máy A (làm tiếp từ nhánh).
	cloneB := filepath.Join(root, "b")
	git(t, root, "clone", "-q", remote, cloneB)
	gb := runner.NewGit(cloneB, filepath.Join(cloneB, ".worktrees"))
	dirB, err := gb.Prepare(ctx, "feat/task-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dirB, "a.go")); err != nil {
		t.Fatal("máy kế tiếp phải checkout nhánh đã push, thấy a.go")
	}
	if git(t, dirB, "rev-parse", "HEAD") != sha {
		t.Fatal("HEAD của máy B phải là commit WIP của máy A")
	}
}

func TestExecExecutor(t *testing.T) {
	var out strings.Builder
	code, err := runner.ExecExecutor{}.Run(context.Background(), t.TempDir(), []string{"sh", "-c", "echo hi; echo err >&2; exit 3"}, &out)
	if err != nil || code != 3 || !strings.Contains(out.String(), "hi") || !strings.Contains(out.String(), "err") {
		t.Fatalf("code=%d err=%v out=%q", code, err, out.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _ = runner.ExecExecutor{}.Run(ctx, t.TempDir(), []string{"sleep", "10"}, &out)
	if time.Since(start) > 3*time.Second {
		t.Fatal("huỷ ctx phải dừng tiến trình CLI")
	}
}
