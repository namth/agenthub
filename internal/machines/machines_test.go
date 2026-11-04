package machines_test

// Test nghiệm thu TASK-023 — do trưởng phòng viết, file bị khoá (tests.lock).
// FEAT-05: DEC-18, DEC-20, BR-30 → BR-32, BR-34, TC-46 → TC-52, TC-54.

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/gateway"
	"github.com/namth/agenthub/internal/machines"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

type rig struct {
	t       *testing.T
	db      *sql.DB
	clk     *clock
	srv     *httptest.Server
	removed []string
	runners string
}

// Người dùng giả qua header X-Test-User = "<user_id>" ; owner = u-owner.
func newRig(t *testing.T) *rig {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "j.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-owner', '1', 'namth', 'ADMIN')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-a', '2', 'doi-tac-a', 'PARTNER')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-b', '3', 'doi-tac-b', 'PARTNER')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'agenthub', 'namth/agenthub')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p2', 'edu', 'namth/edusystem')`,
		`INSERT INTO project_members (project_id, user_id, role) VALUES ('p1', 'u-a', 'PARTNER')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	r := &rig{t: t, db: db, clk: &clock{t: time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)}, runners: t.TempDir()}
	os.WriteFile(filepath.Join(r.runners, "agenthub-darwin-arm64"), []byte("BINARY-DARWIN"), 0o755)
	sum := sha256.Sum256([]byte("BINARY-DARWIN"))
	os.WriteFile(filepath.Join(r.runners, "agenthub-darwin-arm64.sha256"), []byte(hex.EncodeToString(sum[:])+"  agenthub-darwin-arm64\n"), 0o644)
	var base string
	h := machines.Handler(db, machines.Config{
		BaseURL:    func() string { return base },
		RunnersDir: r.runners,
		Now:        r.clk.now,
		IsOwner:    func(userID string) bool { return userID == "u-owner" },
		IsMember: func(userID, projectID string) bool {
			return userID == "u-owner" || (userID == "u-a" && projectID == "p1")
		},
		OnRemoved: func(workerID string) { r.removed = append(r.removed, workerID) },
	}, func(req *http.Request) (string, bool) {
		u := req.Header.Get("X-Test-User")
		return u, u != ""
	})
	r.srv = httptest.NewServer(h)
	base = r.srv.URL
	t.Cleanup(r.srv.Close)
	return r
}

type env struct {
	Success bool                   `json:"success"`
	Data    json.RawMessage        `json:"data"`
	Error   *struct{ Code string } `json:"error"`
}

func (r *rig) do(method, path, user string, body any, hdr ...string) (int, env) {
	r.t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, r.srv.URL+path, bytes.NewReader(b))
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer res.Body.Close()
	var e env
	json.NewDecoder(res.Body).Decode(&e)
	return res.StatusCode, e
}

type codeResp struct {
	Code        string `json:"code"`
	ExpiresAt   string `json:"expires_at"`
	Command     string `json:"command"`
	AgentPrompt string `json:"agent_prompt"`
}

func (r *rig) newCode(user, name string) codeResp {
	r.t.Helper()
	code, e := r.do("POST", "/api/fleet/join-codes", user, map[string]any{"name": name})
	if code != 201 {
		r.t.Fatalf("tạo mã = %d %+v", code, e.Error)
	}
	var c codeResp
	json.Unmarshal(e.Data, &c)
	return c
}

func (r *rig) join(code, name string, ip ...string) (int, env) {
	h := []string{}
	if len(ip) > 0 {
		h = []string{"X-Forwarded-For", ip[0]}
	}
	return r.do("POST", "/api/fleet/join", "", map[string]any{"code": code, "name": name, "os": "darwin", "arch": "arm64", "agents": []string{"CLAUDE", "ANTIGRAVITY"}}, h...)
}

// ---------- mã tham gia ----------

func TestJoinCodeShapeAndPrompt(t *testing.T) {
	r := newRig(t)
	c := r.newCode("u-a", "macbook-a")
	if !regexp.MustCompile(`^[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}$`).MatchString(c.Code) {
		t.Fatalf("mã = %q, muốn XXXX-XXXX không có ký tự dễ nhầm (0/O/1/I)", c.Code)
	}
	exp, _ := time.Parse(time.RFC3339, c.ExpiresAt)
	if !exp.Equal(r.clk.now().Add(15 * time.Minute)) {
		t.Fatalf("hết hạn = %s, muốn sau 15 phút", c.ExpiresAt)
	}
	if c.Command != "curl -fsSL "+r.srv.URL+"/install.sh | sh -s -- --join "+c.Code+" --name macbook-a" {
		t.Fatalf("command = %q", c.Command)
	}
	for _, want := range []string{c.Code, r.srv.URL, "install.sh", "gh auth login", "claude", "/login", "--status", "dangerously-skip-permissions", "token"} {
		if !strings.Contains(c.AgentPrompt, want) {
			t.Fatalf("agent_prompt thiếu %q:\n%s", want, c.AgentPrompt)
		}
	}
	var hash string
	r.db.QueryRow(`SELECT code_hash FROM join_codes`).Scan(&hash)
	if hash == c.Code || hash != gateway.HashToken(c.Code) {
		t.Fatal("chỉ lưu SHA-256 của mã")
	}
	for _, bad := range []string{"", "a", "co dau cach", strings.Repeat("x", 41)} {
		if code, _ := r.do("POST", "/api/fleet/join-codes", "u-a", map[string]any{"name": bad}); code != 400 {
			t.Errorf("tên %q: %d, muốn 400", bad, code)
		}
	}
	if code, _ := r.do("POST", "/api/fleet/join-codes", "", map[string]any{"name": "x1"}); code != 401 {
		t.Fatal("chưa đăng nhập phải 401")
	}
}

func TestJoinOnceAndOwnership(t *testing.T) { // TC-46, TC-48
	r := newRig(t)
	c := r.newCode("u-a", "macbook-a")
	code, e := r.join(c.Code, "macbook-a")
	var j struct {
		WorkerID    string `json:"worker_id"`
		RunnerToken string `json:"runner_token"`
	}
	json.Unmarshal(e.Data, &j)
	if code != 200 || j.WorkerID != "macbook-a" || len(j.RunnerToken) < 32 {
		t.Fatalf("join = %d %s", code, e.Data)
	}
	var owner, hash, os_ string
	r.db.QueryRow(`SELECT owner_user_id, token_hash, os FROM workers WHERE id='macbook-a'`).Scan(&owner, &hash, &os_)
	if owner != "u-a" || hash != gateway.HashToken(j.RunnerToken) || os_ != "darwin" {
		t.Fatalf("máy = %s %s %s", owner, hash, os_)
	}
	if w, err := gateway.DBAuthenticator(r.db)(j.RunnerToken); err != nil || w != "macbook-a" {
		t.Fatal("token mới phải xác thực được với gateway")
	}
	var le int
	r.db.QueryRow(`SELECT lead_eligible FROM agents WHERE id='macbook-a/claude'`).Scan(&le)
	if le != 0 {
		t.Fatal("máy của partner: agent không được làm trưởng phòng mặc định (DEC-20)")
	}
	if code, _ := r.join(c.Code, "macbook-a"); code != 401 {
		t.Fatalf("dùng lại mã: %d, muốn 401", code)
	}
	o := r.newCode("u-owner", "server-owner")
	r.join(o.Code, "server-owner")
	r.db.QueryRow(`SELECT lead_eligible FROM agents WHERE id='server-owner/claude'`).Scan(&le)
	if le != 1 {
		t.Fatal("máy của owner: agent Claude được làm trưởng phòng")
	}
}

func TestJoinExpiredAndRateLimit(t *testing.T) { // TC-46, TC-47
	r := newRig(t)
	c := r.newCode("u-a", "m-exp")
	r.clk.add(16 * time.Minute)
	if code, _ := r.join(c.Code, "m-exp"); code != 401 {
		t.Fatalf("mã hết hạn: %d", code)
	}
	for i := 0; i < 5; i++ {
		if code, _ := r.join("AAAA-BBBB", "x", "10.0.0.9"); code != 401 {
			t.Fatalf("lần %d: %d", i+1, code)
		}
	}
	if code, e := r.join("AAAA-BBBB", "x", "10.0.0.9"); code != 429 || e.Error.Code != "TOO_MANY_REQUESTS" {
		t.Fatalf("lần 6 cùng IP: %d, muốn 429", code)
	}
	if code, _ := r.join("AAAA-BBBB", "x", "10.0.0.10"); code != 401 {
		t.Fatal("IP khác không bị chặn")
	}
}

func TestJoinNameTakenByOther(t *testing.T) {
	r := newRig(t)
	c := r.newCode("u-a", "dung-chung")
	r.join(c.Code, "dung-chung")
	if code, e := r.do("POST", "/api/fleet/join-codes", "u-b", map[string]any{"name": "dung-chung"}); code != 409 || e.Error.Code != "CONFLICT" {
		t.Fatalf("tên trùng máy người khác: %d", code)
	}
}

// ---------- máy của tôi ----------

func (r *rig) joinAs(user, name string) {
	r.t.Helper()
	c := r.newCode(user, name)
	if code, _ := r.join(c.Code, name); code != 200 {
		r.t.Fatalf("join %s", name)
	}
}

func TestMyMachinesScope(t *testing.T) { // TC-49
	r := newRig(t)
	r.joinAs("u-a", "may-a")
	r.joinAs("u-b", "may-b")
	_, e := r.do("GET", "/api/my/machines", "u-a", nil)
	var ms []map[string]any
	json.Unmarshal(e.Data, &ms)
	if len(ms) != 1 || ms[0]["id"] != "may-a" {
		t.Fatalf("u-a chỉ thấy máy mình: %s", e.Data)
	}
	if code, e := r.do("PATCH", "/api/my/machines/may-b", "u-a", map[string]any{"paused": true}); code != 403 || e.Error.Code != "FORBIDDEN" {
		t.Fatalf("sửa máy người khác: %d", code)
	}
	if code, _ := r.do("DELETE", "/api/my/machines/may-b", "u-a", nil); code != 403 {
		t.Fatal("gỡ máy người khác phải 403")
	}
}

func TestPauseAndProjects(t *testing.T) { // TC-50, BR-34
	r := newRig(t)
	r.joinAs("u-a", "may-a")
	if code, _ := r.do("PATCH", "/api/my/machines/may-a", "u-a", map[string]any{"paused": true}); code != 200 {
		t.Fatalf("tạm dừng: %d", code)
	}
	var paused int
	r.db.QueryRow(`SELECT paused FROM workers WHERE id='may-a'`).Scan(&paused)
	if paused != 1 {
		t.Fatal("paused phải = 1")
	}
	if code, _ := r.do("PATCH", "/api/my/machines/may-a", "u-a", map[string]any{"projects": []string{"p2"}}); code != 403 {
		t.Fatalf("gán dự án không là thành viên: %d, muốn 403", code)
	}
	if code, _ := r.do("PATCH", "/api/my/machines/may-a", "u-a", map[string]any{"projects": []string{"p1"}}); code != 200 {
		t.Fatalf("gán dự án là thành viên: %d", code)
	}
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM agent_projects WHERE agent_id LIKE 'may-a/%' AND project_id='p1'`).Scan(&n)
	if n != 2 {
		t.Fatalf("gán dự án cho mọi agent của máy: %d dòng, muốn 2", n)
	}
	r.do("PATCH", "/api/my/machines/may-a", "u-a", map[string]any{"projects": nil})
	r.db.QueryRow(`SELECT COUNT(*) FROM agent_projects WHERE agent_id LIKE 'may-a/%'`).Scan(&n)
	if n != 0 {
		t.Fatal("projects null = mọi dự án (xoá danh sách)")
	}
}

func TestRemoveMachine(t *testing.T) { // TC-52
	r := newRig(t)
	r.joinAs("u-a", "may-a")
	var oldHash string
	r.db.QueryRow(`SELECT token_hash FROM workers WHERE id='may-a'`).Scan(&oldHash)
	if code, _ := r.do("DELETE", "/api/my/machines/may-a", "u-a", nil); code != 204 && code != 200 {
		t.Fatalf("gỡ máy: %d", code)
	}
	var h string
	err := r.db.QueryRow(`SELECT token_hash FROM workers WHERE id='may-a'`).Scan(&h)
	if err == nil && h == oldHash {
		t.Fatal("gỡ máy phải vô hiệu token")
	}
	if len(r.removed) != 1 || r.removed[0] != "may-a" {
		t.Fatalf("OnRemoved = %v (để main đóng kết nối, thu hồi việc)", r.removed)
	}
	r.joinAs("u-b", "may-b")
	if code, _ := r.do("DELETE", "/api/my/machines/may-b", "u-owner", nil); code != 204 && code != 200 {
		t.Fatal("owner gỡ được máy của người khác")
	}
}

// ---------- tải binary & install.sh ----------

func TestDownload(t *testing.T) {
	r := newRig(t)
	res, _ := http.Get(r.srv.URL + "/download/agenthub-darwin-arm64")
	b := new(bytes.Buffer)
	b.ReadFrom(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || b.String() != "BINARY-DARWIN" {
		t.Fatalf("download = %d %q", res.StatusCode, b.String())
	}
	for _, bad := range []string{"/download/agenthub-plan9-mips", "/download/..%2Fetc%2Fpasswd", "/download/agenthub-darwin-arm64%2F..%2F..%2Fx"} {
		res, _ := http.Get(r.srv.URL + bad)
		res.Body.Close()
		if res.StatusCode != 404 && res.StatusCode != 400 {
			t.Fatalf("%s: %d, muốn 404/400", bad, res.StatusCode)
		}
	}
}

func TestInstallScriptEndToEnd(t *testing.T) { // TC-54
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("không có sh")
	}
	r := newRig(t)
	res, _ := http.Get(r.srv.URL + "/install.sh")
	script := new(bytes.Buffer)
	script.ReadFrom(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(script.String(), "#!/bin/sh") || !strings.Contains(script.String(), r.srv.URL) {
		t.Fatalf("install.sh = %d, phải bắt đầu #!/bin/sh và chứa BASE_URL", res.StatusCode)
	}
	if out, err := exec.Command("sh", "-n", "-c", script.String()).CombinedOutput(); err != nil {
		t.Fatalf("install.sh sai cú pháp sh: %v %s", err, out)
	}
	run := func(code string, extra ...string) (string, error) {
		home := filepath.Join(t.TempDir(), "home")
		args := append([]string{"-s", "--", "--join", code, "--name", "may-test", "--no-service"}, extra...)
		cmd := exec.Command("sh", args...)
		cmd.Stdin = strings.NewReader(script.String())
		cmd.Env = append(os.Environ(), "AGENTHUB_HOME="+home, "AGENTHUB_UNAME_S=Darwin", "AGENTHUB_UNAME_M=arm64")
		out, err := cmd.CombinedOutput()
		return home + "\n" + string(out), err
	}
	c := r.newCode("u-a", "may-test")
	out, err := run(c.Code)
	home := strings.SplitN(out, "\n", 2)[0]
	if err != nil {
		t.Fatalf("install.sh lỗi: %v\n%s", err, out)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "agenthub")); string(b) != "BINARY-DARWIN" {
		t.Fatalf("phải tải đúng binary darwin-arm64 vào $AGENTHUB_HOME/agenthub\n%s", out)
	}
	st, err := os.Stat(filepath.Join(home, "token"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("token phải lưu ở $AGENTHUB_HOME/token quyền 600: %v %v", err, st)
	}
	tok, _ := os.ReadFile(filepath.Join(home, "token"))
	if strings.Contains(out, strings.TrimSpace(string(tok))) {
		t.Fatal("install.sh không được in token")
	}
	for _, want := range []string{"git", "gh", "claude", "agy"} {
		if !strings.Contains(out, want) {
			t.Fatalf("phải in bảng kiểm công cụ (thiếu %q):\n%s", want, out)
		}
	}
	// Sai checksum → dừng, không có token.
	os.WriteFile(filepath.Join(r.runners, "agenthub-darwin-arm64"), []byte("BI-DOI"), 0o755)
	c2 := r.newCode("u-a", "may-test2")
	out, err = run(c2.Code)
	home = strings.SplitN(out, "\n", 2)[0]
	if err == nil {
		t.Fatalf("sai SHA-256 phải thoát lỗi:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(home, "token")); err == nil {
		t.Fatal("sai SHA-256 thì không được đổi mã lấy token")
	}
}
