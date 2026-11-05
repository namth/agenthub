package runner_test

// Test nghiệm thu TASK-050 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-accounts.md §5, §6, §7 (BR-66, BR-67, BR-69); TC-101, TC-102.

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/runner"
)

// envExec ghi lại argv + env của mỗi lần chạy (runner phải dùng RunEnv khi agent có ConfigDir).
type envExec struct {
	mu   sync.Mutex
	runs []envRun
	out  map[string]string // theo argv[0]
}

type envRun struct {
	argv []string
	env  []string
}

func (f *envExec) Run(ctx context.Context, dir string, argv []string, out io.Writer) (int, error) {
	return f.RunEnv(ctx, dir, argv, nil, out)
}

func (f *envExec) RunEnv(ctx context.Context, dir string, argv []string, env []string, out io.Writer) (int, error) {
	f.mu.Lock()
	f.runs = append(f.runs, envRun{append([]string(nil), argv...), append([]string(nil), env...)})
	o := f.out[argv[0]]
	f.mu.Unlock()
	io.WriteString(out, o)
	return 0, nil
}

func (f *envExec) find(bin string) []envRun {
	f.mu.Lock()
	defer f.mu.Unlock()
	var r []envRun
	for _, x := range f.runs {
		if x.argv[0] == bin {
			r = append(r, x)
		}
	}
	return r
}

func hasEnv(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func yesB() *bool { b := true; return &b }
func noB() *bool  { b := false; return &b }

func startAccounts(t *testing.T, ex runner.Executor, agents []runner.AgentConfig, accountsPath string) (*fakeGateway, context.CancelFunc) {
	t.Helper()
	gw := newGateway(t)
	cfg := runner.Config{
		ServerURL:      gw.url(),
		Token:          "tok-secret",
		OS:             "darwin",
		Agents:         agents,
		AccountsPath:   accountsPath,
		HeartbeatEvery: 50 * time.Millisecond,
		RenewEvery:     50 * time.Millisecond,
		LogFlushEvery:  20 * time.Millisecond,
		LogFlushBytes:  1024,
		ReconnectMin:   20 * time.Millisecond,
		ReconnectMax:   100 * time.Millisecond,
	}
	r := runner.New(cfg, ex, &fakeGit{sha: "a1b2c3d"}, time.Now)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("Run không dừng sau khi huỷ ctx")
		}
	})
	return gw, cancel
}

// handshakeNamed trả HANDSHAKE_OK theo name (id = worker/name) và trả danh sách agent trong HANDSHAKE.
func handshakeNamed(gw *fakeGateway, worker string) []map[string]any {
	gw.t.Helper()
	m := gw.expect("HANDSHAKE", nil)
	data, _ := m["data"].(map[string]any)
	raw, _ := data["agents"].([]any)
	var list []map[string]any
	var ok []map[string]any
	for _, a := range raw {
		am := a.(map[string]any)
		list = append(list, am)
		name, _ := am["name"].(string)
		if name == "" {
			name = strings.ToLower(am["kind"].(string))
		}
		ok = append(ok, map[string]any{"id": worker + "/" + name, "status": "IDLE"})
	}
	gw.send(map[string]any{"event": "HANDSHAKE_OK", "agents": ok})
	return list
}

func TestAccountsFileRoundTrip(t *testing.T) { // TC-101 (agents.json)
	p := filepath.Join(t.TempDir(), "agents.json")
	f, err := runner.LoadAccounts(p)
	if err != nil || len(f.Agents) != 0 {
		t.Fatalf("file chưa có → rỗng, không lỗi: %+v %v", f, err)
	}
	f.Agents = append(f.Agents,
		runner.AccountEntry{Name: "claude", Kind: "CLAUDE", Command: []string{"claude", "-p", "{prompt}"}, EverOK: true},
	)
	if n := f.NextName("CLAUDE"); n != "claude-2" {
		t.Fatalf("NextName = %q, muốn claude-2", n)
	}
	if n := f.NextName("ANTIGRAVITY"); n != "antigravity" {
		t.Fatalf("chưa có tài khoản agy → tên mặc định antigravity, có %q", n)
	}
	f.Removed = []string{"claude-2"}
	if n := f.NextName("CLAUDE"); n != "claude-3" {
		t.Fatalf("không tái dùng tên đã xoá: %q, muốn claude-3", n)
	}
	f.Agents = append(f.Agents, runner.AccountEntry{Name: "claude-3", Kind: "CLAUDE", Command: []string{"claude", "-p", "{prompt}"}, ConfigDir: "/x/claude-3"})
	if err := runner.SaveAccounts(p, f); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("agents.json phải 0600, có %v", st.Mode().Perm())
	}
	g, err := runner.LoadAccounts(p)
	if err != nil || len(g.Agents) != 2 || g.Agents[1].ConfigDir != "/x/claude-3" || !g.Agents[0].EverOK || g.Agents[1].EverOK || len(g.Removed) != 1 {
		t.Fatalf("đọc lại sai: %+v %v", g, err)
	}
	cfgs := g.AgentConfigs()
	if len(cfgs) != 2 || cfgs[1].Name != "claude-3" || cfgs[1].ConfigDir != "/x/claude-3" || cfgs[1].EverOK == nil || *cfgs[1].EverOK {
		t.Fatalf("AgentConfigs = %+v", cfgs)
	}
}

func TestAccountEnvOnJobAndProbe(t *testing.T) { // TC-101
	ex := &envExec{out: map[string]string{}}
	gw, _ := startAccounts(t, ex, []runner.AgentConfig{
		{Kind: "CLAUDE", Name: "claude", Command: []string{"claude", "-p", "{prompt}"}, EverOK: yesB()},
		{Kind: "CLAUDE", Name: "claude-2", Command: []string{"claude", "-p", "{prompt}"}, ConfigDir: "/acc/claude-2", EverOK: yesB()},
		{Kind: "ANTIGRAVITY", Name: "agy-2", Command: []string{"agy", "-p", "{prompt}"}, ConfigDir: "/acc/agy-2", EverOK: yesB()},
	}, "")
	list := handshakeNamed(gw, "mac")
	if len(list) != 3 {
		t.Fatalf("HELLO phải có 3 agent, có %v", list)
	}
	names := map[string]map[string]any{}
	for _, a := range list {
		names[a["name"].(string)] = a
	}
	if names["claude-2"]["kind"] != "CLAUDE" || names["claude-2"]["ever_ok"] != true {
		t.Fatalf("HELLO thiếu name/ever_ok: %v", names["claude-2"])
	}

	gw.send(assign("asg_1", "mac/claude-2", "CODE", "t1", "feat/t1", ""))
	gw.expect("TASK_SUBMITTED", nil)
	gw.send(assign("asg_2", "mac/agy-2", "CODE", "t2", "feat/t2", ""))
	gw.expect("TASK_SUBMITTED", nil)

	var c2, agy bool
	for _, r := range ex.find("claude") {
		if strings.Contains(strings.Join(r.argv, " "), "Làm task t1") {
			c2 = hasEnv(r.env, "CLAUDE_CONFIG_DIR=/acc/claude-2")
		}
	}
	for _, r := range ex.find("agy") {
		if strings.Contains(strings.Join(r.argv, " "), "Làm task t2") {
			agy = hasEnv(r.env, "HOME=/acc/agy-2")
		}
	}
	if !c2 {
		t.Fatalf("job của claude-2 phải chạy với CLAUDE_CONFIG_DIR=/acc/claude-2: %+v", ex.find("claude"))
	}
	if !agy {
		t.Fatalf("job của agy-2 phải chạy với HOME=/acc/agy-2: %+v", ex.find("agy"))
	}
	for _, r := range ex.find("claude") {
		if !hasEnv(r.env, "CLAUDE_CONFIG_DIR=/acc/claude-2") && strings.Contains(strings.Join(r.argv, " "), "Làm task t1") {
			t.Fatal("lẫn env giữa các tài khoản")
		}
	}
}

func TestHelloOnlyEverOK(t *testing.T) { // BR-67
	ex := &envExec{out: map[string]string{}}
	gw, _ := startAccounts(t, ex, []runner.AgentConfig{
		{Kind: "CLAUDE", Name: "claude", Command: []string{"claude", "-p", "{prompt}"}, EverOK: yesB()},
		{Kind: "CLAUDE", Name: "claude-2", Command: []string{"claude", "-p", "{prompt}"}, ConfigDir: "/acc/claude-2", EverOK: noB()},
		{Kind: "ANTIGRAVITY", Command: []string{"agy", "-p", "{prompt}"}}, // cờ --agent cũ: EverOK nil
	}, "")
	list := handshakeNamed(gw, "mac")
	got := map[string]map[string]any{}
	for _, a := range list {
		n, _ := a["name"].(string)
		if n == "" {
			n = strings.ToLower(a["kind"].(string))
		}
		got[n] = a
	}
	if _, ok := got["claude-2"]; ok {
		t.Fatalf("agent ever_ok=false không được báo trong HELLO: %v", list)
	}
	if _, ok := got["claude"]; !ok {
		t.Fatalf("thiếu agent claude: %v", list)
	}
	ag, ok := got["antigravity"]
	if !ok {
		t.Fatalf("agent từ cờ cũ (ever_ok chưa biết) vẫn phải được báo: %v", list)
	}
	if _, has := ag["ever_ok"]; has {
		t.Fatalf("agent từ cờ cũ không gửi ever_ok (để server tự suy từ auth_status): %v", ag)
	}
}

func TestAgentRemoveRewritesAccounts(t *testing.T) { // TC-102
	dir := t.TempDir()
	p := filepath.Join(dir, "agents.json")
	f := runner.AccountFile{Agents: []runner.AccountEntry{
		{Name: "claude", Kind: "CLAUDE", Command: []string{"claude", "-p", "{prompt}"}, EverOK: true},
		{Name: "claude-2", Kind: "CLAUDE", Command: []string{"claude", "-p", "{prompt}"}, ConfigDir: filepath.Join(dir, "claude-2"), EverOK: true},
	}}
	if err := runner.SaveAccounts(p, f); err != nil {
		t.Fatal(err)
	}
	ex := &envExec{out: map[string]string{}}
	gw, _ := startAccounts(t, ex, f.AgentConfigs(), p)
	if n := len(handshakeNamed(gw, "mac")); n != 2 {
		t.Fatalf("HELLO đầu có 2 agent, có %d", n)
	}
	gw.send(map[string]any{"event": "AGENT_REMOVE", "agent_id": "mac/claude-2"})

	deadline := time.Now().Add(2 * time.Second)
	for {
		g, _ := runner.LoadAccounts(p)
		if len(g.Agents) == 1 && g.Agents[0].Name == "claude" {
			if len(g.Removed) != 1 || g.Removed[0] != "claude-2" {
				t.Fatalf("phải ghi tên đã xoá vào removed: %+v", g)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("agents.json chưa gỡ claude-2: %+v", g)
		}
		time.Sleep(20 * time.Millisecond)
	}
	gw.never("REQUEST_TASK", 300*time.Millisecond, agentIs("mac/claude-2"))

	gw.dropConn()
	list := handshakeNamed(gw, "mac")
	for _, a := range list {
		if a["name"] == "claude-2" {
			t.Fatalf("HELLO sau khi xoá vẫn báo claude-2: %v", list)
		}
	}
}
