package runner_test

// Test nghiệm thu TASK-021 — do trưởng phòng viết, file bị khoá (tests.lock).
// DEC-15 (đa repo), DEC-19 + BR-35 (tự cập nhật), FEAT-05 §4 (--status).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/namth/agenthub/internal/runner"
)

func baseCfg(url string) runner.Config {
	return runner.Config{
		ServerURL: url, Token: "t", OS: "darwin",
		Agents:         []runner.AgentConfig{{Kind: "ANTIGRAVITY", Command: []string{"agy", "-p", "{prompt}"}}},
		HeartbeatEvery: 50 * time.Millisecond, RenewEvery: time.Second, LogFlushEvery: 20 * time.Millisecond, LogFlushBytes: 1024,
		ReconnectMin: 20 * time.Millisecond, ReconnectMax: 100 * time.Millisecond,
	}
}

func runIt(t *testing.T, r *runner.Runner) (context.CancelFunc, chan error) {
	t.Helper()
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
	return cancel, done
}

// ---------- đa repo ----------

func TestGitPerRepo(t *testing.T) { // TC-44
	gw := newGateway(t)
	var mu sync.Mutex
	gits := map[string]*fakeGit{}
	cfg := baseCfg(gw.url())
	cfg.GitFor = func(repo string) (runner.Git, error) {
		mu.Lock()
		defer mu.Unlock()
		if repo == "namth/hong" {
			return nil, errors.New("clone thất bại")
		}
		g, ok := gits[repo]
		if !ok {
			g = &fakeGit{sha: "abc"}
			gits[repo] = g
		}
		return g, nil
	}
	def := &fakeGit{sha: "def"}
	ex := &fakeExec{steps: map[string][]step{"agy": {{exit: 0}, {exit: 0}}}}
	runIt(t, runner.New(cfg, ex, def, time.Now))
	gw.handshake("mac")

	a := assign("asg_1", "mac/antigravity", "CODE", "7", "feat/task-7", "")
	a["payload"].(map[string]any)["repo"] = "namth/edusystem"
	gw.send(a)
	gw.expect("TASK_SUBMITTED", nil)
	mu.Lock()
	edu := gits["namth/edusystem"]
	mu.Unlock()
	if edu == nil || !edu.has("Prepare feat/task-7") || !edu.has("Push") {
		t.Fatalf("job repo namth/edusystem phải dùng git của repo đó: %v", gits)
	}
	if def.has("Prepare") {
		t.Fatal("có payload.repo thì không dùng git mặc định")
	}

	b := assign("asg_2", "mac/antigravity", "CODE", "8", "feat/task-8", "")
	b["payload"].(map[string]any)["repo"] = "namth/hong"
	gw.send(b)
	gw.expect("LOG_CHUNK", func(m map[string]any) bool {
		return strings.Contains(m["chunk"].(string), "[runner]") && strings.Contains(m["chunk"].(string), "namth/hong")
	})
	gw.never("TASK_SUBMITTED", 300*time.Millisecond, nil)
}

func TestRepoCacheClonesOnce(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	git(t, root, "init", "-q", "--bare", "-b", "main", remote)
	seed := filepath.Join(root, "seed")
	git(t, root, "clone", "-q", remote, seed)
	os.WriteFile(filepath.Join(seed, "a.txt"), []byte("x"), 0o644)
	git(t, seed, "add", ".")
	git(t, seed, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init")
	git(t, seed, "push", "-q", "origin", "main")

	home := filepath.Join(root, "home")
	gitFor := runner.NewRepoCache(home, func(repo string) string { return remote }, runner.ExecExecutor{})
	g, err := gitFor("namth/demo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "repos", "namth", "demo", ".git")); err != nil {
		t.Fatal("phải clone vào <home>/repos/namth/demo")
	}
	dir, err := g.Prepare(context.Background(), "feat/task-1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dir, filepath.Join(home, "worktrees")) {
		t.Fatalf("worktree phải nằm trong <home>/worktrees: %s", dir)
	}
	if _, err := gitFor("namth/demo"); err != nil {
		t.Fatalf("lần hai dùng lại bản clone: %v", err)
	}
	if _, err := gitFor("../thoat"); err == nil {
		t.Fatal("tên repo không hợp lệ (path traversal) phải bị từ chối")
	}
}

// ---------- phiên bản & tự cập nhật ----------

type updGateway struct {
	srv      *httptest.Server
	mu       sync.Mutex
	versions []string
	conn     *websocket.Conn
}

func newUpdGateway(t *testing.T, serverVersion string, binary, sumOf []byte) *updGateway {
	g := &updGateway{}
	sum := sha256.Sum256(sumOf)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/agent", func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		g.mu.Lock()
		g.conn = c
		g.mu.Unlock()
		for {
			var m map[string]any
			if c.ReadJSON(&m) != nil {
				return
			}
			if m["event"] == "HANDSHAKE" {
				data, _ := m["data"].(map[string]any)
				g.mu.Lock()
				g.versions = append(g.versions, data["version"].(string))
				g.mu.Unlock()
				c.WriteJSON(map[string]any{"event": "HANDSHAKE_OK", "server_version": serverVersion,
					"agents": []any{map[string]any{"id": "mac/antigravity", "status": "IDLE"}}})
			}
		}
	})
	mux.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			w.Write([]byte(hex.EncodeToString(sum[:]) + "  agenthub\n"))
			return
		}
		w.Write(binary)
	})
	g.srv = httptest.NewServer(mux)
	t.Cleanup(g.srv.Close)
	return g
}

func (g *updGateway) url() string { return "ws" + strings.TrimPrefix(g.srv.URL, "http") + "/ws/agent" }

func TestHandshakeSendsVersion(t *testing.T) {
	g := newUpdGateway(t, "1.0.0", []byte("x"), []byte("x"))
	runner.Version = "1.0.0"
	cfg := baseCfg(g.url())
	cfg.SelfPath = filepath.Join(t.TempDir(), "agenthub")
	os.WriteFile(cfg.SelfPath, []byte("cũ"), 0o755)
	runIt(t, runner.New(cfg, &fakeExec{steps: map[string][]step{}}, &fakeGit{}, time.Now))
	deadline := time.After(3 * time.Second)
	for {
		g.mu.Lock()
		n := len(g.versions)
		g.mu.Unlock()
		if n > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("không nhận HANDSHAKE")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if g.versions[0] != "1.0.0" {
		t.Fatalf("HANDSHAKE.data.version = %q", g.versions[0])
	}
	time.Sleep(200 * time.Millisecond)
	if b, _ := os.ReadFile(cfg.SelfPath); string(b) != "cũ" {
		t.Fatal("cùng phiên bản thì không cập nhật")
	}
}

func TestSelfUpdateWhenIdle(t *testing.T) { // TC-55
	newBin := []byte("binary-moi-2.0.0")
	g := newUpdGateway(t, "2.0.0", newBin, newBin)
	runner.Version = "1.0.0"
	cfg := baseCfg(g.url())
	cfg.SelfPath = filepath.Join(t.TempDir(), "agenthub")
	os.WriteFile(cfg.SelfPath, []byte("cũ"), 0o755)
	_, done := runIt(t, runner.New(cfg, &fakeExec{steps: map[string][]step{}}, &fakeGit{}, time.Now))
	select {
	case err := <-done:
		if !errors.Is(err, runner.ErrUpdated) {
			t.Fatalf("Run phải trả ErrUpdated sau khi thay binary, nhận %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("không tự cập nhật")
	}
	b, _ := os.ReadFile(cfg.SelfPath)
	st, _ := os.Stat(cfg.SelfPath)
	if string(b) != string(newBin) || st.Mode()&0o100 == 0 {
		t.Fatalf("binary mới phải thay file và giữ quyền chạy: %q %v", b, st.Mode())
	}
}

func TestSelfUpdateRejectsBadChecksum(t *testing.T) {
	g := newUpdGateway(t, "2.0.0", []byte("doc-hai"), []byte("ban-dung")) // .sha256 không khớp binary
	runner.Version = "1.0.0"
	cfg := baseCfg(g.url())
	cfg.SelfPath = filepath.Join(t.TempDir(), "agenthub")
	os.WriteFile(cfg.SelfPath, []byte("cũ"), 0o755)
	_, done := runIt(t, runner.New(cfg, &fakeExec{steps: map[string][]step{}}, &fakeGit{}, time.Now))
	select {
	case err := <-done:
		t.Fatalf("sai checksum không được thoát để cập nhật: %v", err)
	case <-time.After(800 * time.Millisecond):
	}
	if b, _ := os.ReadFile(cfg.SelfPath); string(b) != "cũ" {
		t.Fatal("sai checksum phải giữ binary cũ")
	}
}

func TestNoUpdateFlag(t *testing.T) {
	g := newUpdGateway(t, "9.9.9", []byte("moi"), []byte("moi"))
	runner.Version = "1.0.0"
	cfg := baseCfg(g.url())
	cfg.NoUpdate = true
	cfg.SelfPath = filepath.Join(t.TempDir(), "agenthub")
	os.WriteFile(cfg.SelfPath, []byte("cũ"), 0o755)
	_, done := runIt(t, runner.New(cfg, &fakeExec{steps: map[string][]step{}}, &fakeGit{}, time.Now))
	select {
	case err := <-done:
		t.Fatalf("--no-update không được cập nhật: %v", err)
	case <-time.After(600 * time.Millisecond):
	}
}

// ---------- --status ----------

func TestStatusFile(t *testing.T) {
	gw := newGateway(t)
	cfg := baseCfg(gw.url())
	cfg.StatusFile = filepath.Join(t.TempDir(), "status.json")
	runIt(t, runner.New(cfg, &fakeExec{steps: map[string][]step{}}, &fakeGit{}, time.Now))
	gw.handshake("mac")
	var st map[string]any
	deadline := time.After(3 * time.Second)
	for {
		b, err := os.ReadFile(cfg.StatusFile)
		if err == nil && json.Unmarshal(b, &st) == nil && st["connected"] == true {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("status.json chưa báo connected: %s", b)
		case <-time.After(30 * time.Millisecond):
		}
	}
	if st["server"] == nil || st["updated_at"] == nil || st["agents"] == nil || st["version"] == nil {
		t.Fatalf("status.json thiếu trường: %v", st)
	}
	if strings.Contains(mustRead(t, cfg.StatusFile), "\"t\"") {
		t.Fatal("status.json không được chứa token")
	}
	if code := runner.Main([]string{"--status", "--status-file", cfg.StatusFile}); code != 0 {
		t.Fatalf("--status khi đang kết nối: exit %d, muốn 0", code)
	}
	old := filepath.Join(t.TempDir(), "old.json")
	os.WriteFile(old, []byte(`{"connected":true,"updated_at":"2020-01-01T00:00:00Z"}`), 0o600)
	if code := runner.Main([]string{"--status", "--status-file", old}); code == 0 {
		t.Fatal("status cũ quá 2 phút phải exit ≠ 0")
	}
}

func mustRead(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
