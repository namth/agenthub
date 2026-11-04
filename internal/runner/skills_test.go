package runner_test

// Test nghiệm thu TASK-037 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/clarification-and-shared-skills.md BR-50, BR-51, DEC-32; TC-81.

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/runner"
	"github.com/namth/agenthub/internal/skills"
)

// probeExec ghi lại dir/argv và nội dung file skill đọc được NGAY LÚC CLI chạy.
type probeExec struct {
	mu        sync.Mutex
	dir       string
	argv      []string
	skillPath string
	skillBody string
	gitignore string
	ran       chan struct{}
}

var skillLine = regexp.MustCompile(`AGENTHUB_SKILL=(\S+)`)

func (p *probeExec) Run(ctx context.Context, dir string, argv []string, out io.Writer) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if argv[0] != "claude" && argv[0] != "agy" {
		return 0, nil // git, gh…
	}
	p.dir, p.argv = dir, append([]string(nil), argv...)
	if m := skillLine.FindStringSubmatch(strings.Join(argv, "\n")); m != nil {
		p.skillPath = m[1]
		b, _ := os.ReadFile(m[1])
		p.skillBody = string(b)
		g, _ := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(m[1]))), ".gitignore"))
		p.gitignore = string(g)
	}
	select {
	case p.ran <- struct{}{}:
	default:
	}
	return 1, nil // không nộp: chỉ cần quan sát lúc chạy
}

// realDirGit: Prepare trả thư mục thật; Snapshot cũng thật (tuỳ test).
type realDirGit struct {
	fakeGit
	root     string
	snapshot bool
}

func (g *realDirGit) Prepare(_ context.Context, branch string) (string, error) {
	d := filepath.Join(g.root, "wt-"+strings.ReplaceAll(branch, "/", "-"))
	os.MkdirAll(d, 0o755)
	return d, nil
}

type snapGit struct{ realDirGit }

func (g *snapGit) Snapshot(_ context.Context, parent string) (string, error) {
	d := filepath.Join(parent, "repo")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func startProbe(t *testing.T, git runner.Git) (*rig, *probeExec) {
	t.Helper()
	gw := newGateway(t)
	pe := &probeExec{ran: make(chan struct{}, 4)}
	cfg := runner.Config{
		ServerURL: gw.url(), Token: "tok", OS: "darwin",
		Agents: []runner.AgentConfig{
			{Kind: "CLAUDE", Command: []string{"claude", "-p", "{prompt}"}},
			{Kind: "ANTIGRAVITY", Command: []string{"agy", "-p", "{prompt}"}},
		},
		HeartbeatEvery: 50 * time.Millisecond, RenewEvery: 50 * time.Millisecond,
		LogFlushEvery: 20 * time.Millisecond, LogFlushBytes: 1024,
		ReconnectMin: 20 * time.Millisecond, ReconnectMax: 100 * time.Millisecond,
	}
	r := runner.New(cfg, pe, git, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	})
	return &rig{gw: gw, stop: cancel, done: done}, pe
}

func waitProbe(t *testing.T, pe *probeExec) {
	t.Helper()
	select {
	case <-pe.ran:
	case <-time.After(3 * time.Second):
		t.Fatal("CLI không được gọi")
	}
}

func TestEmbeddedSkill(t *testing.T) {
	if !strings.Contains(skills.AgentHubSpec, "NEED_INFO") || !strings.Contains(skills.AgentHubSpec, "Không dùng AskUserQuestion") {
		t.Fatal("skill nhúng phải là bản agenthub-spec (có NEED_INFO, cấm AskUserQuestion)")
	}
	if skills.Files()["agenthub-spec/SKILL.md"] != skills.AgentHubSpec {
		t.Fatal("Files() phải chứa agenthub-spec/SKILL.md")
	}
}

func TestSpecGetsEmbeddedSkillInSnapshot(t *testing.T) { // TC-81
	g := &snapGit{realDirGit{fakeGit: fakeGit{sha: "x"}, root: t.TempDir()}}
	r, pe := startProbe(t, g)
	r.gw.handshake("mac")
	r.gw.send(assign("asg_s1", "mac/claude", "SPEC", "1", "", ""))
	waitProbe(t, pe)
	pe.mu.Lock()
	defer pe.mu.Unlock()
	if pe.dir == "" || filepath.Base(pe.dir) != "repo" {
		t.Fatalf("SPEC không nhánh phải chạy trong snapshot repo, dir = %q", pe.dir)
	}
	if pe.skillBody != skills.AgentHubSpec {
		t.Fatalf("AGENTHUB_SKILL (%s) phải trỏ file skill nhúng tồn tại lúc CLI chạy", pe.skillPath)
	}
	if !strings.HasPrefix(pe.skillPath, pe.dir) || !strings.Contains(pe.skillPath, filepath.Join(".agenthub", "skills", "agenthub-spec", "SKILL.md")) {
		t.Fatalf("đường skill = %q, muốn <dir>/.agenthub/skills/agenthub-spec/SKILL.md", pe.skillPath)
	}
	if strings.TrimSpace(pe.gitignore) != "*" {
		t.Fatalf(".agenthub/.gitignore phải là '*', có %q", pe.gitignore)
	}
	j := strings.Join(pe.argv, "\n")
	if !strings.Contains(j, "NEED_INFO") || strings.Index(j, "AGENTHUB_SKILL=") > strings.Index(j, "AGENTHUB_RESULT_FILE=") {
		t.Fatalf("prompt phải có AGENTHUB_SKILL trước AGENTHUB_RESULT_FILE và nhắc NEED_INFO: %s", j)
	}
}

func TestProjectSkillOverrides(t *testing.T) {
	g := &realDirGit{fakeGit: fakeGit{sha: "x"}, root: t.TempDir()}
	own := filepath.Join(g.root, "wt-feat-task-2", ".agents", "skills", "agenthub-spec")
	os.MkdirAll(own, 0o755)
	os.WriteFile(filepath.Join(own, "SKILL.md"), []byte("SKILL RIÊNG CỦA DỰ ÁN"), 0o644)
	r, pe := startProbe(t, g)
	r.gw.handshake("mac")
	r.gw.send(assign("asg_s2", "mac/claude", "REPLAN", "2", "feat/task-2", ""))
	waitProbe(t, pe)
	pe.mu.Lock()
	defer pe.mu.Unlock()
	if pe.skillBody != "SKILL RIÊNG CỦA DỰ ÁN" {
		t.Fatalf("dự án có .agents/skills/agenthub-spec → dùng bản dự án, đọc được %q từ %s", pe.skillBody, pe.skillPath)
	}
}

func TestCodeJobHasNoSkill(t *testing.T) {
	g := &realDirGit{fakeGit: fakeGit{sha: "x"}, root: t.TempDir()}
	r, pe := startProbe(t, g)
	r.gw.handshake("mac")
	r.gw.send(assign("asg_s3", "mac/antigravity", "CODE", "3", "feat/task-3", ""))
	waitProbe(t, pe)
	pe.mu.Lock()
	defer pe.mu.Unlock()
	if pe.skillPath != "" || strings.Contains(strings.Join(pe.argv, " "), "AGENTHUB_SKILL") {
		t.Fatalf("job CODE không được kèm skill: %v", pe.argv)
	}
	if _, err := os.Stat(filepath.Join(g.root, "wt-feat-task-3", ".agenthub")); err == nil {
		t.Fatal("job CODE không được tạo .agenthub/")
	}
}

func TestNoSnapshotterKeepsOldBehaviour(t *testing.T) {
	r, pe := startProbe(t, &fakeGit{sha: "x"}) // fakeGit cũ: không có Snapshot
	r.gw.handshake("mac")
	r.gw.send(assign("asg_s4", "mac/claude", "SPEC", "4", "", ""))
	waitProbe(t, pe)
	pe.mu.Lock()
	defer pe.mu.Unlock()
	if pe.dir != "" {
		t.Fatalf("không có Snapshotter → dir rỗng như cũ, có %q", pe.dir)
	}
	if pe.skillBody != skills.AgentHubSpec {
		t.Fatal("không có snapshot vẫn phải kèm skill (đặt trong thư mục tạm)")
	}
}
