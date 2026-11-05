package e2e_test

// Test trọn dây chuyền do trưởng phòng viết: dispatcher + pipeline + gateway + runner + tasks.Service
// chạy thật trong một tiến trình; chỉ CLI (claude/agy), git và GitHub là giả.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
	"github.com/namth/agenthub/internal/pipeline"
	"github.com/namth/agenthub/internal/runner"
	"github.com/namth/agenthub/internal/tasks"
)

type wall struct{}

func (wall) Now() time.Time { return time.Now() }

type fanout struct {
	mu sync.Mutex
	ns []dispatcher.Notifier
}

func (f *fanout) add(n dispatcher.Notifier) { f.mu.Lock(); f.ns = append(f.ns, n); f.mu.Unlock() }
func (f *fanout) Notify(e dispatcher.Event) {
	f.mu.Lock()
	ns := append([]dispatcher.Notifier(nil), f.ns...)
	f.mu.Unlock()
	for _, n := range ns {
		n.Notify(e)
	}
}

type gh struct {
	mu   sync.Mutex
	next int
}

func (g *gh) CreateIssue(context.Context, string, string, []string) (int, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return 100 + g.next, "", nil
}
func (g *gh) SetLabels(context.Context, int, []string) error { return nil }
func (g *gh) CloseIssue(context.Context, int, string) error  { return nil }

// cli giả: đọc job kind trong prompt rồi ghi result.json phù hợp.
type cli struct {
	mu      sync.Mutex
	reviews int
}

var resultFile = regexp.MustCompile(`AGENTHUB_RESULT_FILE=(\S+)`)

func (c *cli) Run(_ context.Context, _ string, argv []string, out io.Writer) (int, error) {
	prompt := strings.Join(argv, " ")
	m := resultFile.FindStringSubmatch(prompt)
	var res any
	switch {
	case strings.Contains(prompt, "subtasks"):
		res = map[string]any{"subtasks": []any{
			map[string]any{"title": "Schema DB", "description": "Tạo bảng báo cáo", "dod": "go test xanh"},
			map[string]any{"title": "API xuất PDF", "description": "Endpoint xuất", "dod": "curl trả PDF"},
		}}
	case strings.Contains(prompt, "verdict"):
		c.mu.Lock()
		c.reviews++
		n := c.reviews
		c.mu.Unlock()
		if n == 1 {
			res = map[string]any{"verdict": "CHANGES", "comments": "Thiếu test rỗng"}
		} else {
			res = map[string]any{"verdict": "PASS", "comments": "Ổn"}
		}
	default:
		res = map[string]any{"summary": "Đã code"}
	}
	if m != nil {
		b, _ := json.Marshal(res)
		os.WriteFile(m[1], b, 0o600)
	}
	io.WriteString(out, "xong\n")
	return 0, nil
}

type git struct{}

func (git) Prepare(_ context.Context, branch string) (string, error) { return "/wt/" + branch, nil }
func (git) CommitWIP(context.Context, string, string) (string, error) {
	return "deadbeef", nil
}
func (git) Push(context.Context, string, string) error { return nil }
func (git) DiffStat(context.Context, string) string    { return "" }
func (git) Cleanup(context.Context, string) error      { return nil }

func waitFor(t *testing.T, db *sql.DB, what, q, want string, args ...any) {
	t.Helper()
	var got string
	for i := 0; i < 400; i++ {
		var s sql.NullString
		if db.QueryRow(q, args...).Scan(&s); s.String == want {
			return
		}
		got = s.String
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("%s: %q, muốn %q", what, got, want)
}

func TestFullPipeline(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "e2e.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', 'g1', 'namth', 'ADMIN')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'AgentHub', 'namth/agenthub')`,
	} {
		db.Exec(q)
	}
	db.Exec(`INSERT INTO workers (id, name, os, token_hash) VALUES ('mac', 'mac', 'darwin', ?)`, gateway.HashToken("tok"))

	fo := &fanout{}
	disp := dispatcher.New(db, wall{}, dispatcher.DefaultConfig(), fo)
	ghc := &gh{}
	pl := pipeline.New(db, disp, ghc, "p1")
	fo.add(pl)
	hub := gateway.New(disp, gateway.DBAuthenticator(db), gateway.WithPrompt(pl.PromptFor),
		gateway.WithSubmitHook(func(w, aid, pr string, res json.RawMessage) {
			if err := pl.OnSubmitted(context.Background(), w, aid, pr, res); err != nil {
				t.Errorf("OnSubmitted: %v", err)
			}
		}))
	fo.add(hub)
	srv := httptest.NewServer(hub)
	defer srv.Close()
	defer hub.Close()

	var prs int
	var prMu sync.Mutex
	r := runner.New(runner.Config{
		ServerURL: "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/agent", Token: "tok", OS: "darwin",
		Agents:         []runner.AgentConfig{{Kind: "CLAUDE", Command: []string{"claude", "-p", "{prompt}"}}, {Kind: "ANTIGRAVITY", Command: []string{"agy", "-p", "{prompt}"}}},
		HeartbeatEvery: 200 * time.Millisecond, RenewEvery: 200 * time.Millisecond, LogFlushEvery: 20 * time.Millisecond, LogFlushBytes: 1024,
		ReconnectMin: 20 * time.Millisecond, ReconnectMax: 100 * time.Millisecond,
		OpenPR: func(_ context.Context, _, branch, _, _ string) (string, error) {
			prMu.Lock()
			prs++
			n := prs
			prMu.Unlock()
			return fmt.Sprintf("https://github.com/namth/agenthub/pull/%d", n), nil
		},
	}, &cli{}, git{}, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	waitFor(t, db, "runner kết nối", `SELECT conn_status FROM workers WHERE id='mac'`, "ONLINE")

	svc := tasks.NewService(db, ghc, disp, "p1")
	task, err := svc.Create(context.Background(), "u1", "Xuất báo cáo PDF", "Khách cần xuất bảng tiến độ thành PDF gửi ban giám đốc")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Approve(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}

	waitFor(t, db, "task cha sau SPEC", `SELECT status FROM tasks WHERE id=?`, "READY", task.ID)
	waitFor(t, db, "số sub-task", `SELECT CAST(COUNT(*) AS TEXT) FROM tasks WHERE parent_id=?`, "2", task.ID)
	waitFor(t, db, "mọi sub-task QA_PASSED",
		`SELECT CAST(COUNT(*) AS TEXT) FROM tasks WHERE parent_id=? AND status='QA_PASSED'`, "2", task.ID)

	var withPR int
	db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE parent_id=? AND pr_url LIKE 'https://github.com/namth/agenthub/pull/%'`, task.ID).Scan(&withPR)
	if withPR != 2 {
		t.Fatalf("sub-task có PR = %d, muốn 2", withPR)
	}
	var notes int
	db.QueryRow(`SELECT COUNT(*) FROM lead_journal WHERE entry LIKE '%Thiếu test rỗng%'`).Scan(&notes)
	if notes != 1 {
		t.Fatalf("nhận xét QA lần đầu phải vào lead_journal: %d", notes)
	}
	var codeJobs int
	db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE kind='CODE' AND status='DONE'`).Scan(&codeJobs)
	if codeJobs != 3 {
		t.Fatalf("job CODE xong = %d, muốn 3 (2 sub-task + 1 lần sửa)", codeJobs)
	}
}
