package machines_test

// Test nghiệm thu TASK-030 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-auth.md §6 (auth-recheck, trường auth trong /api/my/machines), TC-63; DEC-29 (join).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/machines"
)

type authRig struct {
	t       *testing.T
	srv     *httptest.Server
	mu      sync.Mutex
	calls   []string
	offline map[string]bool
	exec    func(q string, args ...any)
	str     func(q string, args ...any) string
}

func newAuthRig(t *testing.T) *authRig {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	r := &authRig{t: t, offline: map[string]bool{}}
	r.exec = func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	r.str = func(q string, args ...any) string {
		t.Helper()
		var s string
		if err := db.QueryRow(q, args...).Scan(&s); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return s
	}
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-owner', '1', 'namth', 'ADMIN')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-a', '2', 'doi-tac-a', 'PARTNER')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-b', '3', 'doi-tac-b', 'PARTNER')`,
		`INSERT INTO workers (id, name, os, token_hash, owner_user_id, conn_status, gh_auth) VALUES ('may-a', 'may-a', 'darwin', 'h', 'u-a', 'ONLINE', 'NEEDS_AUTH')`,
		`INSERT INTO agents (id, worker_id, kind, status, auth_status, auth_detail, auth_checked_at) VALUES ('may-a/claude', 'may-a', 'CLAUDE', 'IDLE', 'NEEDS_AUTH', 'claude: loggedIn=false', '2026-11-04 08:00:00')`,
		`INSERT INTO agents (id, worker_id, kind, status) VALUES ('may-a/antigravity', 'may-a', 'ANTIGRAVITY', 'IDLE')`,
	} {
		r.exec(q)
	}
	h := machines.Handler(db, machines.Config{
		BaseURL:    func() string { return "http://x" },
		RunnersDir: t.TempDir(),
		Now:        func() time.Time { return time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC) },
		IsOwner:    func(u string) bool { return u == "u-owner" },
		IsMember:   func(u, p string) bool { return u == "u-owner" },
		Recheck: func(workerID, agentID string) bool {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.calls = append(r.calls, workerID+"|"+agentID)
			return !r.offline[workerID]
		},
	}, func(req *http.Request) (string, bool) {
		u := req.Header.Get("X-Test-User")
		return u, u != ""
	})
	r.srv = httptest.NewServer(h)
	t.Cleanup(r.srv.Close)
	return r
}

func (r *authRig) do(method, path, user string, body any) (int, env) {
	r.t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, r.srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("X-Test-User", user)
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

func (r *authRig) recheckCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func TestAuthRecheckPermissions(t *testing.T) { // TC-63
	r := newAuthRig(t)
	if code, _ := r.do("POST", "/api/my/machines/may-a/auth-recheck", "u-a", map[string]any{"agent_id": "may-a/antigravity"}); code != 202 {
		t.Fatalf("chủ máy kiểm lại = %d, muốn 202", code)
	}
	if code, _ := r.do("POST", "/api/my/machines/may-a/auth-recheck", "u-owner", map[string]any{}); code != 202 {
		t.Fatalf("owner kiểm lại cả máy = %d, muốn 202", code)
	}
	if got := r.recheckCalls(); len(got) != 2 || got[0] != "may-a|may-a/antigravity" || got[1] != "may-a|" {
		t.Fatalf("Recheck calls = %v", got)
	}
	code, e := r.do("POST", "/api/my/machines/may-a/auth-recheck", "u-b", map[string]any{})
	if code != 403 || e.Error == nil || e.Error.Code != "FORBIDDEN" {
		t.Fatalf("partner khác = %d %+v, muốn 403 FORBIDDEN", code, e.Error)
	}
	if code, _ := r.do("POST", "/api/my/machines/may-a/auth-recheck", "", map[string]any{}); code != 401 {
		t.Fatalf("chưa đăng nhập = %d, muốn 401", code)
	}
	if code, _ := r.do("POST", "/api/my/machines/khong-co/auth-recheck", "u-owner", map[string]any{}); code != 404 {
		t.Fatalf("máy không tồn tại = %d, muốn 404", code)
	}
	code, e = r.do("POST", "/api/my/machines/may-a/auth-recheck", "u-a", map[string]any{"agent_id": "may-khac/claude"})
	if code != 400 || e.Error == nil || e.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("agent không thuộc máy = %d %+v, muốn 400 VALIDATION_ERROR", code, e.Error)
	}
	if len(r.recheckCalls()) != 2 {
		t.Fatal("yêu cầu bị từ chối không được gọi Recheck")
	}
	r.offline["may-a"] = true
	code, e = r.do("POST", "/api/my/machines/may-a/auth-recheck", "u-a", map[string]any{})
	if code != 409 || e.Error == nil || e.Error.Code != "WORKER_OFFLINE" {
		t.Fatalf("máy offline = %d %+v, muốn 409 WORKER_OFFLINE", code, e.Error)
	}
}

func TestMyMachinesAuthFields(t *testing.T) {
	r := newAuthRig(t)
	code, e := r.do("GET", "/api/my/machines", "u-a", nil)
	if code != 200 {
		t.Fatalf("GET /api/my/machines = %d", code)
	}
	var ms []struct {
		ID     string `json:"id"`
		GhAuth string `json:"gh_auth"`
		Agents []struct {
			ID            string  `json:"id"`
			AuthStatus    string  `json:"auth_status"`
			AuthDetail    *string `json:"auth_detail"`
			AuthCheckedAt *string `json:"auth_checked_at"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(e.Data, &ms); err != nil || len(ms) != 1 {
		t.Fatalf("data = %s (%v)", e.Data, err)
	}
	if ms[0].GhAuth != "NEEDS_AUTH" {
		t.Fatalf("gh_auth = %q", ms[0].GhAuth)
	}
	seen := 0
	for _, a := range ms[0].Agents {
		switch a.ID {
		case "may-a/claude":
			seen++
			if a.AuthStatus != "NEEDS_AUTH" || a.AuthDetail == nil || *a.AuthDetail != "claude: loggedIn=false" || a.AuthCheckedAt == nil || *a.AuthCheckedAt == "" {
				t.Fatalf("agent claude = %+v", a)
			}
		case "may-a/antigravity":
			seen++
			if a.AuthStatus != "UNKNOWN" || a.AuthDetail != nil {
				t.Fatalf("agent agy = %+v, muốn UNKNOWN, auth_detail null", a)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("thiếu agent: %+v", ms[0].Agents)
	}
}

func TestJoinAntigravityNotLeadEligible(t *testing.T) { // DEC-29
	r := newRig(t)
	c := r.newCode("u-owner", "may-chu")
	code, e := r.do("POST", "/api/fleet/join", "", map[string]any{"code": c.Code, "name": "may-chu", "os": "darwin", "arch": "arm64", "agents": []string{"CLAUDE", "ANTIGRAVITY"}})
	if code != 200 && code != 201 {
		t.Fatalf("join = %d %+v", code, e.Error)
	}
	var claude, agy int
	r.db.QueryRow(`SELECT lead_eligible FROM agents WHERE kind='CLAUDE' AND worker_id IN (SELECT id FROM workers WHERE name='may-chu')`).Scan(&claude)
	if err := r.db.QueryRow(`SELECT lead_eligible FROM agents WHERE kind='ANTIGRAVITY' AND worker_id IN (SELECT id FROM workers WHERE name='may-chu')`).Scan(&agy); err != nil {
		t.Fatal(err)
	}
	if claude != 1 || agy != 0 {
		t.Fatalf("máy owner: claude lead_eligible=%d (muốn 1), antigravity=%d (muốn 0)", claude, agy)
	}
}
