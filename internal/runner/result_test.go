package runner_test

// Test nghiệm thu TASK-013 — do trưởng phòng viết, file bị khoá (tests.lock). DEC-12.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/runner"
)

// resultExec ghi nội dung vào đường dẫn AGENTHUB_RESULT_FILE tìm thấy trong prompt.
type resultExec struct {
	mu      sync.Mutex
	content string // "" = không ghi file
	exit    int
	prompts []string
}

var resultLine = regexp.MustCompile(`AGENTHUB_RESULT_FILE=(\S+)`)

func (e *resultExec) Run(_ context.Context, dir string, argv []string, out io.Writer) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := strings.Join(argv, " ")
	e.prompts = append(e.prompts, p)
	m := resultLine.FindStringSubmatch(p)
	if m != nil && e.content != "" {
		os.WriteFile(m[1], []byte(e.content), 0o600)
	}
	io.WriteString(out, "xong\n")
	return e.exit, nil
}

type prCall struct{ dir, branch, title, body string }

type prRig struct {
	gw    *fakeGateway
	git   *fakeGit
	mu    sync.Mutex
	calls []prCall
}

func startWithPR(t *testing.T, ex runner.Executor, prURL string, prErr error) *prRig {
	t.Helper()
	gw := newGateway(t)
	rg := &prRig{gw: gw, git: &fakeGit{sha: "abc1234"}}
	cfg := runner.Config{
		ServerURL: gw.url(), Token: "t", OS: "darwin",
		Agents:         []runner.AgentConfig{{Kind: "CLAUDE", Command: []string{"claude", "-p", "{prompt}"}}, {Kind: "ANTIGRAVITY", Command: []string{"agy", "-p", "{prompt}"}}},
		HeartbeatEvery: time.Second, RenewEvery: time.Second, LogFlushEvery: 20 * time.Millisecond, LogFlushBytes: 1024,
		ReconnectMin: 20 * time.Millisecond, ReconnectMax: 100 * time.Millisecond,
		OpenPR: func(_ context.Context, dir, branch, title, body string) (string, error) {
			rg.mu.Lock()
			rg.calls = append(rg.calls, prCall{dir, branch, title, body})
			rg.mu.Unlock()
			return prURL, prErr
		},
	}
	r := runner.New(cfg, ex, rg.git, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	gw.handshake("mac")
	return rg
}

func TestSpecResultSubmitted(t *testing.T) {
	ex := &resultExec{content: `{"subtasks":[{"title":"Schema","description":"Tạo bảng","dod":"go test xanh"}]}`}
	r := startWithPR(t, ex, "", nil)
	r.gw.send(assign("asg_s", "mac/claude", "SPEC", "t1", "", ""))
	m := r.gw.expect("TASK_SUBMITTED", nil)
	res, _ := m["result"].(map[string]any)
	subs, _ := res["subtasks"].([]any)
	if m["assignment_id"] != "asg_s" || len(subs) != 1 || subs[0].(map[string]any)["title"] != "Schema" {
		t.Fatalf("TASK_SUBMITTED = %v", m)
	}
	p := ex.prompts[0]
	path := resultLine.FindStringSubmatch(p)
	if path == nil || !filepath.IsAbs(path[1]) {
		t.Fatalf("prompt phải có dòng AGENTHUB_RESULT_FILE=<đường dẫn tuyệt đối>: %q", p)
	}
	if _, err := os.Stat(path[1]); !os.IsNotExist(err) {
		t.Fatal("xong việc phải dọn file kết quả tạm")
	}
}

func TestEachAssignmentGetsOwnResultFile(t *testing.T) {
	ex := &resultExec{content: `{"subtasks":[{"title":"A","description":"B","dod":"C"}]}`}
	r := startWithPR(t, ex, "", nil)
	r.gw.send(assign("asg_1", "mac/claude", "SPEC", "t1", "", ""))
	r.gw.expect("TASK_SUBMITTED", nil)
	r.gw.send(assign("asg_2", "mac/claude", "SPEC", "t2", "", ""))
	r.gw.expect("TASK_SUBMITTED", nil)
	a := resultLine.FindStringSubmatch(ex.prompts[0])[1]
	b := resultLine.FindStringSubmatch(ex.prompts[1])[1]
	if a == b {
		t.Fatal("mỗi assignment phải có file kết quả riêng")
	}
}

func TestRequiredResultMissingOrInvalid(t *testing.T) {
	for name, content := range map[string]string{
		"thiếu file":        "",
		"không phải JSON":   "subtasks: 1",
		"không phải object": `[1,2]`,
		"quá 64KB":          `{"x":"` + strings.Repeat("a", 70*1024) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			r := startWithPR(t, &resultExec{content: content}, "", nil)
			r.gw.send(assign("asg_x", "mac/claude", "QA_REVIEW", "t1", "", ""))
			r.gw.expect("LOG_CHUNK", func(m map[string]any) bool {
				c := fmt.Sprint(m["chunk"])
				return strings.Contains(c, "[runner]") && strings.Contains(c, "result.json")
			})
			r.gw.never("TASK_SUBMITTED", 300*time.Millisecond, nil)
		})
	}
}

func TestCodeJobOpensPR(t *testing.T) {
	ex := &resultExec{content: `{"summary":"Thêm bảng workers"}`}
	r := startWithPR(t, ex, "https://github.com/namth/agenthub/pull/15", nil)
	r.gw.send(assign("asg_c", "mac/antigravity", "CODE", "102", "feat/task-102", ""))
	m := r.gw.expect("TASK_SUBMITTED", nil)
	if m["pr_url"] != "https://github.com/namth/agenthub/pull/15" || m["branch"] != "feat/task-102" {
		t.Fatalf("TASK_SUBMITTED = %v", m)
	}
	res, _ := m["result"].(map[string]any)
	if res["summary"] != "Thêm bảng workers" {
		t.Fatalf("result = %v", m["result"])
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) != 1 || r.calls[0].branch != "feat/task-102" || r.calls[0].dir != "/wt/feat/task-102" ||
		!strings.Contains(r.calls[0].title, "102") || !strings.Contains(r.calls[0].body, "Thêm bảng workers") {
		t.Fatalf("OpenPR = %+v", r.calls)
	}
	if !r.git.has("Push /wt/feat/task-102 feat/task-102") {
		t.Fatal("phải push trước khi mở PR")
	}
}

func TestCodeJobWithoutResultStillSubmits(t *testing.T) {
	r := startWithPR(t, &resultExec{}, "https://github.com/x/y/pull/2", nil)
	r.gw.send(assign("asg_c2", "mac/antigravity", "CODE", "7", "feat/task-7", ""))
	m := r.gw.expect("TASK_SUBMITTED", nil)
	if m["pr_url"] != "https://github.com/x/y/pull/2" || (m["result"] != nil && fmt.Sprint(m["result"]) != "map[]") {
		t.Fatalf("CODE không có result.json vẫn nộp, result null: %v", m)
	}
}

func TestOpenPRFailureStillSubmits(t *testing.T) {
	r := startWithPR(t, &resultExec{}, "", errors.New("gh: not logged in"))
	r.gw.send(assign("asg_c3", "mac/antigravity", "CODE", "8", "feat/task-8", ""))
	r.gw.expect("LOG_CHUNK", func(m map[string]any) bool { return strings.Contains(fmt.Sprint(m["chunk"]), "PR") })
	m := r.gw.expect("TASK_SUBMITTED", nil)
	if m["pr_url"] != "" {
		t.Fatalf("mở PR lỗi → pr_url rỗng: %v", m["pr_url"])
	}
}

// ---------- gh pr create ----------

type argvExec struct {
	argv []string
	dir  string
	out  string
	code int
}

func (a *argvExec) Run(_ context.Context, dir string, argv []string, out io.Writer) (int, error) {
	a.argv, a.dir = argv, dir
	io.WriteString(out, a.out)
	return a.code, nil
}

func TestGhPROpener(t *testing.T) {
	ex := &argvExec{out: "Creating pull request...\nhttps://github.com/namth/agenthub/pull/42\n"}
	open := runner.GhPROpener(ex, "main")
	url, err := open(context.Background(), "/wt/feat/task-1", "feat/task-1", "feat(task-1): CODE", "Tóm tắt")
	if err != nil || url != "https://github.com/namth/agenthub/pull/42" {
		t.Fatalf("url = %q, err = %v", url, err)
	}
	joined := strings.Join(ex.argv, " ")
	for _, want := range []string{"gh pr create", "--head feat/task-1", "--base main", "--title feat(task-1): CODE", "--body Tóm tắt"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("argv = %q, thiếu %q", joined, want)
		}
	}
	if ex.dir != "/wt/feat/task-1" {
		t.Fatalf("chạy gh trong worktree, dir = %q", ex.dir)
	}
	if _, err := runner.GhPROpener(&argvExec{out: "a pull request already exists", code: 1}, "main")(context.Background(), "/d", "b", "t", "x"); err == nil {
		t.Fatal("gh thoát mã ≠ 0 phải trả lỗi")
	}
	var _ = json.Marshal
}
