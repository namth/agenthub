package runner_test

// Test nghiệm thu TASK-042 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/ops-readiness.md BR-57, DEC-35; TC-85.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/runner"
)

func testProbes() map[string][]string {
	return map[string][]string{"CLAUDE": {"claude-probe", "-p", "OK?"}, "ANTIGRAVITY": {"agy-probe", "-p", "OK?"}}
}

const oauthErr = "Failed to authenticate: OAuth session expired and could not be refreshed\n"

func TestParseProbeResult(t *testing.T) {
	cases := []struct {
		out  string
		exit int
		err  error
		want string
	}{
		{"OK\n", 0, nil, runner.AuthOK},
		{oauthErr, 1, nil, runner.AuthNeeds},
		{"Error: token expired, please log in again", 1, nil, runner.AuthNeeds},
		{"Please run /login", 1, nil, runner.AuthNeeds},
		{"network timeout", 2, nil, runner.AuthUnknown},
		{"", -1, context.DeadlineExceeded, runner.AuthUnknown},
		{"", -1, errors.New("exec: not found"), runner.AuthUnknown},
	}
	for i, c := range cases {
		got, detail := runner.ParseProbeResult(c.exit, c.out, c.err)
		if got != c.want {
			t.Errorf("case %d %q: %s, muốn %s", i, c.out, got, c.want)
		}
		if len([]rune(detail)) > 80 {
			t.Errorf("case %d: detail > 80 ký tự", i)
		}
		if got == runner.AuthNeeds && detail == "" {
			t.Errorf("case %d: NEEDS_AUTH phải có detail", i)
		}
	}
}

func TestDefaultsIncludeOAuthAndProbes(t *testing.T) {
	p := strings.ToLower(strings.Join(runner.DefaultAuthPatterns(), "|"))
	for _, want := range []string{"failed to authenticate", "oauth", "session expired", "token expired", "could not be refreshed", "not logged in", "please run /login"} {
		if !strings.Contains(p, want) {
			t.Errorf("DefaultAuthPatterns thiếu %q", want)
		}
	}
	pr := runner.DefaultAuthProbes()
	for kind, bin := range map[string]string{"CLAUDE": "claude", "ANTIGRAVITY": "agy"} {
		argv := pr[kind]
		if len(argv) < 3 || argv[0] != bin || argv[1] != "-p" {
			t.Fatalf("probe %s = %v, muốn %s -p <prompt ngắn>", kind, argv, bin)
		}
		for _, a := range argv {
			if strings.Contains(a, "dangerously") || strings.Contains(a, "bypass") {
				t.Fatalf("probe không được bỏ qua quyền: %v", argv)
			}
		}
	}
}

func TestProbeOverridesStatusCheck(t *testing.T) { // TC-85
	ex := &fakeExec{steps: map[string][]step{
		"claude-auth":  {loggedIn(true)},              // auth status nói đã đăng nhập…
		"claude-probe": {{output: oauthErr, exit: 1}}, // …nhưng gọi thật thì hết hạn OAuth
		"agy-probe":    {{output: "OK\n", exit: 0}},
	}}
	r := startAuth(t, ex, nil, func(c *runner.Config) { c.AuthProbes = testProbes(); c.ProbeEvery = time.Hour })
	m := r.gw.handshake("mac")
	ags, _ := hsAgents(t, m)
	if ags["CLAUDE"]["auth_status"] != "NEEDS_AUTH" {
		t.Fatalf("probe lỗi OAuth phải ghi đè loggedIn=true: %v", ags["CLAUDE"])
	}
	if ags["ANTIGRAVITY"]["auth_status"] != "OK" {
		t.Fatalf("agy probe OK phải thành OK (không còn UNKNOWN cố định): %v", ags["ANTIGRAVITY"])
	}
	r.gw.expect("REQUEST_TASK", agentIs("mac/antigravity"))
	r.gw.never("REQUEST_TASK", 300*time.Millisecond, agentIs("mac/claude"))
}

func TestProbeUnknownKeepsCheckResult(t *testing.T) {
	ex := &fakeExec{steps: map[string][]step{
		"claude-auth":  {loggedIn(true)},
		"claude-probe": {{block: true}}, // treo → hết giờ
	}}
	r := startAuth(t, ex, nil, func(c *runner.Config) {
		c.AuthProbes = map[string][]string{"CLAUDE": {"claude-probe", "-p", "OK?"}}
		c.ProbeTimeout = 150 * time.Millisecond
		c.ProbeEvery = time.Hour
	})
	m := r.gw.handshake("mac")
	ags, _ := hsAgents(t, m)
	if ags["CLAUDE"]["auth_status"] != "OK" {
		t.Fatalf("probe hết giờ (UNKNOWN) phải giữ kết quả lệnh kiểm OK, có %v", ags["CLAUDE"])
	}
}

func TestProbePeriodicOnlyOnChange(t *testing.T) {
	steps := []step{{output: "OK\n"}, {output: "OK\n"}, {output: "OK\n"}}
	for i := 0; i < 40; i++ {
		steps = append(steps, step{output: oauthErr, exit: 1})
	}
	ex := &fakeExec{steps: map[string][]step{"claude-auth": {loggedIn(true)}, "claude-probe": steps}}
	r := startAuth(t, ex, nil, func(c *runner.Config) {
		c.AuthProbes = map[string][]string{"CLAUDE": {"claude-probe", "-p", "OK?"}}
		c.ProbeEvery = 60 * time.Millisecond
	})
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
					t.Fatalf("AGENT_AUTH = %v", m)
				}
			}
		case <-deadline:
			break loop
		}
	}
	if n != 1 {
		t.Fatalf("probe định kỳ gửi %d AGENT_AUTH, muốn đúng 1", n)
	}
}

func TestJobOAuthErrorIsAuthNotFailure(t *testing.T) { // BR-39 với mẫu mới
	ex := &fakeExec{steps: map[string][]step{
		"claude-auth": {loggedIn(true)},
		"claude":      {{output: "đang chạy\n" + oauthErr, exit: 1}},
	}}
	r := startAuth(t, ex, nil, nil)
	r.gw.handshake("mac")
	r.gw.send(assign("asg_9", "mac/claude", "SPEC", "9", "", ""))
	m := r.gw.expect("AGENT_AUTH", authFor("mac/claude", "NEEDS_AUTH"))
	if m["assignment_id"] != "asg_9" {
		t.Fatalf("AGENT_AUTH phải kèm assignment_id: %v", m)
	}
	r.gw.never("TASK_SUBMITTED", 200*time.Millisecond, nil)
}

func TestRecheckRunsProbe(t *testing.T) {
	ex := &fakeExec{steps: map[string][]step{
		"claude-auth":  {loggedIn(true), loggedIn(true)},
		"claude-probe": {{output: oauthErr, exit: 1}, {output: "OK\n"}},
	}}
	r := startAuth(t, ex, nil, func(c *runner.Config) { c.AuthProbes = map[string][]string{"CLAUDE": {"claude-probe", "-p", "OK?"}}; c.ProbeEvery = time.Hour })
	r.gw.handshake("mac")
	r.gw.send(map[string]any{"event": "AUTH_RECHECK", "agent_id": "mac/claude"})
	r.gw.expect("AGENT_AUTH", authFor("mac/claude", "OK"))
	r.gw.expect("REQUEST_TASK", agentIs("mac/claude"))
}
