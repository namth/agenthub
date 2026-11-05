package runner_test

// Test nghiệm thu TASK-031 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-auth.md §3, §6, BR-38, BR-39; TC-57, TC-59.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/runner"
)

// Lệnh kiểm dùng argv[0] riêng để không ăn bước giả của "claude"/"agy".
func testChecks() map[string][]string {
	return map[string][]string{"CLAUDE": {"claude-auth", "status"}, "GH": {"gh", "auth", "status"}}
}

func startAuth(t *testing.T, exec *fakeExec, git *fakeGit, mut func(*runner.Config)) *rig {
	t.Helper()
	gw := newGateway(t)
	if exec == nil {
		exec = &fakeExec{steps: map[string][]step{}}
	}
	if git == nil {
		git = &fakeGit{sha: "a1b2c3d"}
	}
	cfg := runner.Config{
		ServerURL: gw.url(),
		Token:     "tok-secret",
		OS:        "darwin",
		Agents: []runner.AgentConfig{
			{Kind: "CLAUDE", Command: []string{"claude", "-p", "{prompt}"}},
			{Kind: "ANTIGRAVITY", Command: []string{"agy", "-p", "{prompt}"}},
		},
		HeartbeatEvery:   50 * time.Millisecond,
		RenewEvery:       50 * time.Millisecond,
		LogFlushEvery:    20 * time.Millisecond,
		LogFlushBytes:    1024,
		ReconnectMin:     20 * time.Millisecond,
		ReconnectMax:     100 * time.Millisecond,
		AuthChecks:       testChecks(),
		AuthCheckEvery:   time.Hour,
		AuthFreshFor:     time.Hour,
		AuthCheckTimeout: 2 * time.Second,
	}
	if mut != nil {
		mut(&cfg)
	}
	r := runner.New(cfg, exec, git, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	rg := &rig{gw: gw, exec: exec, git: git, stop: cancel, done: done}
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("Run không dừng sau khi huỷ ctx")
		}
	})
	return rg
}

func loggedIn(v bool) step {
	return step{output: fmt.Sprintf(`{"loggedIn": %v, "authMethod": "claude.ai"}`+"\n", v), exit: 0}
}

func hsAgents(t *testing.T, m map[string]any) (map[string]map[string]any, string) {
	t.Helper()
	data, _ := m["data"].(map[string]any)
	out := map[string]map[string]any{}
	agents, _ := data["agents"].([]any)
	for _, a := range agents {
		am := a.(map[string]any)
		out[fmt.Sprint(am["kind"])] = am
	}
	return out, fmt.Sprint(data["gh_auth"])
}

func authFor(id, status string) func(map[string]any) bool {
	return func(m map[string]any) bool {
		return m["scope"] == "agent" && m["agent_id"] == id && (status == "" || m["auth_status"] == status)
	}
}

func TestParseAuthResult(t *testing.T) { // TC-57
	cases := []struct {
		kind, out string
		exit      int
		err       error
		want      string
	}{
		{"CLAUDE", `{"loggedIn": false}`, 1, nil, runner.AuthNeeds},
		{"CLAUDE", "some banner\n{\n  \"loggedIn\": true,\n  \"authMethod\": \"claude.ai\"\n}", 0, nil, runner.AuthOK},
		{"CLAUDE", "not json at all", 0, nil, runner.AuthUnknown},
		{"CLAUDE", "", -1, context.DeadlineExceeded, runner.AuthUnknown},
		{runner.AuthKindGH, "✓ Logged in to github.com account namth (keyring)", 0, nil, runner.AuthOK},
		{runner.AuthKindGH, "You are not logged into any GitHub hosts. To log in, run: gh auth login", 1, nil, runner.AuthNeeds},
		{runner.AuthKindGH, "network unreachable", 1, nil, runner.AuthUnknown},
		{"ANTIGRAVITY", "Error: Please run /login first", 1, nil, runner.AuthNeeds},
		{"ANTIGRAVITY", "ok", 0, nil, runner.AuthOK},
		{"ANTIGRAVITY", "segfault", 2, errors.New("boom"), runner.AuthUnknown},
	}
	for i, c := range cases {
		got, detail := runner.ParseAuthResult(c.kind, c.exit, c.out, c.err)
		if got != c.want {
			t.Errorf("case %d (%s %q): %s, muốn %s", i, c.kind, c.out, got, c.want)
		}
		if len([]rune(detail)) > 80 {
			t.Errorf("case %d: detail dài %d > 80", i, len([]rune(detail)))
		}
		if got == runner.AuthOK && detail != "" {
			t.Errorf("case %d: OK thì detail rỗng, có %q", i, detail)
		}
	}
}

func TestDefaultAuthChecksAndPatterns(t *testing.T) {
	c := runner.DefaultAuthChecks()
	if strings.Join(c["CLAUDE"], " ") != "claude auth status" || strings.Join(c["GH"], " ") != "gh auth status" {
		t.Fatalf("DefaultAuthChecks = %v", c)
	}
	if _, ok := c["ANTIGRAVITY"]; ok {
		t.Fatal("agy chưa có lệnh kiểm không tương tác: không được khai báo")
	}
	p := strings.ToLower(strings.Join(runner.DefaultAuthPatterns(), "|"))
	for _, want := range []string{"not logged in", "please run /login", "authentication required", "invalid api key", "gh auth login"} {
		if !strings.Contains(p, want) {
			t.Errorf("DefaultAuthPatterns thiếu %q", want)
		}
	}
}

func TestHandshakeIncludesAuthAndBlocksRequests(t *testing.T) { // BR-38
	ex := &fakeExec{steps: map[string][]step{"claude-auth": {loggedIn(false)}, "gh": {{output: "Logged in\n", exit: 0}}}}
	r := startAuth(t, ex, nil, nil)
	m := r.gw.handshake("mac")
	ags, gh := hsAgents(t, m)
	if ags["CLAUDE"]["auth_status"] != "NEEDS_AUTH" || ags["ANTIGRAVITY"]["auth_status"] != "UNKNOWN" || gh != "OK" {
		t.Fatalf("HANDSHAKE agents=%v gh_auth=%s", ags, gh)
	}
	if d := fmt.Sprint(ags["CLAUDE"]["auth_detail"]); !strings.Contains(d, "loggedIn") {
		t.Fatalf("auth_detail = %q", d)
	}
	r.gw.expect("REQUEST_TASK", agentIs("mac/antigravity"))
	r.gw.never("REQUEST_TASK", 300*time.Millisecond, agentIs("mac/claude"))
	r.gw.send(map[string]any{"event": "JOB_AVAILABLE"})
	r.gw.never("REQUEST_TASK", 300*time.Millisecond, agentIs("mac/claude"))
}

func TestAuthCheckTimeoutIsUnknown(t *testing.T) {
	ex := &fakeExec{steps: map[string][]step{"claude-auth": {{block: true}}}}
	r := startAuth(t, ex, nil, func(c *runner.Config) { c.AuthCheckTimeout = 150 * time.Millisecond })
	m := r.gw.handshake("mac")
	ags, _ := hsAgents(t, m)
	if ags["CLAUDE"]["auth_status"] != "UNKNOWN" {
		t.Fatalf("lệnh kiểm treo phải ra UNKNOWN, có %v", ags["CLAUDE"])
	}
	r.gw.expect("REQUEST_TASK", agentIs("mac/claude")) // UNKNOWN vẫn xin việc
}

func TestGhNeedsAuthBlocksAllRequests(t *testing.T) {
	ex := &fakeExec{steps: map[string][]step{"claude-auth": {loggedIn(true)}, "gh": {{output: "You are not logged into any GitHub hosts.\n", exit: 1}}}}
	r := startAuth(t, ex, nil, nil)
	m := r.gw.handshake("mac")
	if _, gh := hsAgents(t, m); gh != "NEEDS_AUTH" {
		t.Fatalf("gh_auth = %s", gh)
	}
	r.gw.never("REQUEST_TASK", 400*time.Millisecond, nil)
}

func TestRecheckClearsNeedsAuth(t *testing.T) {
	ex := &fakeExec{steps: map[string][]step{"claude-auth": {loggedIn(false), loggedIn(true)}}}
	r := startAuth(t, ex, nil, nil)
	r.gw.handshake("mac")
	r.gw.expect("REQUEST_TASK", agentIs("mac/antigravity"))
	r.gw.send(map[string]any{"event": "AUTH_RECHECK", "agent_id": "mac/claude"})
	r.gw.expect("AGENT_AUTH", authFor("mac/claude", "OK"))
	r.gw.expect("REQUEST_TASK", agentIs("mac/claude"))
}

func TestRecheckWithoutCommandReturnsUnknown(t *testing.T) {
	ex := &fakeExec{steps: map[string][]step{"claude-auth": {loggedIn(true), loggedIn(true)}, "gh": {{exit: 0}, {exit: 0}}}}
	r := startAuth(t, ex, nil, nil)
	r.gw.handshake("mac")
	r.gw.send(map[string]any{"event": "AUTH_RECHECK", "agent_id": "mac/antigravity"})
	r.gw.expect("AGENT_AUTH", authFor("mac/antigravity", "UNKNOWN"))
	// Kiểm cả máy: luôn trả lời cho GH và mọi agent, kể cả không đổi.
	r.gw.send(map[string]any{"event": "AUTH_RECHECK"})
	got := map[string]bool{}
	deadline := time.After(3 * time.Second)
	for len(got) < 3 {
		select {
		case m := <-r.gw.inbox:
			if m["event"] != "AGENT_AUTH" {
				continue
			}
			if m["scope"] == "gh" {
				got["gh"] = true
			} else {
				got[fmt.Sprint(m["agent_id"])] = true
			}
		case <-deadline:
			t.Fatalf("AUTH_RECHECK cả máy chỉ trả %v, muốn gh + mac/claude + mac/antigravity", got)
		}
	}
}

func TestAuthPatternOnJobFailure(t *testing.T) { // TC-59, BR-39
	ex := &fakeExec{steps: map[string][]step{
		"claude-auth": {loggedIn(true)},
		"agy":         {{output: "đang khởi động\nError: Please run /login to authenticate\n", exit: 1}},
	}}
	r := startAuth(t, ex, nil, nil)
	r.gw.handshake("mac")
	r.gw.send(assign("asg_7", "mac/antigravity", "CODE", "107", "feat/task-107", ""))
	m := r.gw.expect("AGENT_AUTH", authFor("mac/antigravity", "NEEDS_AUTH"))
	if m["assignment_id"] != "asg_7" || m["wip_commit"] != "a1b2c3d" {
		t.Fatalf("AGENT_AUTH = %v, muốn assignment_id asg_7, wip_commit a1b2c3d", m)
	}
	if note := fmt.Sprint(m["handoff_note"]); !strings.Contains(note, "1 file changed") {
		t.Fatalf("handoff_note = %q", note)
	}
	if d := fmt.Sprint(m["detail"]); !strings.Contains(strings.ToLower(d), "login") || len([]rune(d)) > 80 {
		t.Fatalf("detail = %q", d)
	}
	if !r.git.has("CommitWIP /wt/feat/task-107") || !r.git.has("Push /wt/feat/task-107 feat/task-107") {
		t.Fatalf("phải commit WIP + push trước khi báo: %v", r.git.calls)
	}
	r.gw.never("TASK_SUBMITTED", 200*time.Millisecond, nil)
	r.gw.never("AGENT_EXHAUSTED", 100*time.Millisecond, nil)
	r.gw.drain()
	r.gw.send(map[string]any{"event": "JOB_AVAILABLE"})
	r.gw.expect("REQUEST_TASK", agentIs("mac/claude"))
	r.gw.never("REQUEST_TASK", 300*time.Millisecond, agentIs("mac/antigravity"))
}

func TestExhaustBeatsAuthPattern(t *testing.T) {
	ex := &fakeExec{steps: map[string][]step{
		"claude-auth": {loggedIn(true)},
		"agy":         {{output: "usage limit reached; not logged in\n", exit: 1}},
	}}
	r := startAuth(t, ex, nil, nil)
	r.gw.handshake("mac")
	r.gw.send(assign("asg_8", "mac/antigravity", "CODE", "108", "feat/task-108", ""))
	r.gw.expect("AGENT_EXHAUSTED", agentIs("mac/antigravity"))
	r.gw.never("AGENT_AUTH", 200*time.Millisecond, authFor("mac/antigravity", "NEEDS_AUTH"))
}

func TestPeriodicCheckSendsOnlyChanges(t *testing.T) {
	steps := []step{loggedIn(true), loggedIn(true), loggedIn(true)}
	for i := 0; i < 40; i++ {
		steps = append(steps, loggedIn(false))
	}
	ex := &fakeExec{steps: map[string][]step{"claude-auth": steps}}
	r := startAuth(t, ex, nil, func(c *runner.Config) { c.AuthCheckEvery = 60 * time.Millisecond })
	r.gw.handshake("mac")
	n := 0
	deadline := time.After(1500 * time.Millisecond)
loop:
	for {
		select {
		case m := <-r.gw.inbox:
			if m["event"] == "AGENT_AUTH" && m["agent_id"] == "mac/claude" {
				n++
				if m["auth_status"] != "NEEDS_AUTH" {
					t.Fatalf("AGENT_AUTH = %v, muốn NEEDS_AUTH", m)
				}
			}
		case <-deadline:
			break loop
		}
	}
	if n != 1 {
		t.Fatalf("kiểm định kỳ gửi %d AGENT_AUTH, muốn đúng 1 (chỉ khi đổi)", n)
	}
}
