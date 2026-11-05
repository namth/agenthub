package auth_test

// Test nghiệm thu TASK-005 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/07 §1.1, docs/08 §1.1, docs/02 §2–3, DEC-09.

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"database/sql"

	"github.com/namth/agenthub/internal/auth"
	"github.com/namth/agenthub/internal/database"
)

// ---------- GitHub giả ----------

type fakeGitHub struct {
	srv        *httptest.Server
	mu         sync.Mutex
	permission map[string]string // login → admin|maintain|write|triage|read; thiếu = 404
	user       map[string]any
	lastAuth   map[string]string // path → header Authorization
}

func newGitHub(t *testing.T) *fakeGitHub {
	g := &fakeGitHub{
		permission: map[string]string{"namth": "admin"},
		user:       map[string]any{"id": 42, "login": "namth", "avatar_url": "https://avatars/42"},
		lastAuth:   map[string]string{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Method != http.MethodPost || r.Form.Get("client_id") != "cid" || r.Form.Get("client_secret") != "csecret" || r.Form.Get("code") != "good-code" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"error": "bad_verification_code"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "user-token", "token_type": "bearer", "scope": "read:user"})
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.lastAuth["/user"] = r.Header.Get("Authorization")
		u := g.user
		g.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer user-token" {
			w.WriteHeader(401)
			return
		}
		json.NewEncoder(w).Encode(u)
	})
	mux.HandleFunc("/repos/namth/agenthub/collaborators/", func(w http.ResponseWriter, r *http.Request) {
		login := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/repos/namth/agenthub/collaborators/"), "/permission")
		g.mu.Lock()
		g.lastAuth["permission"] = r.Header.Get("Authorization")
		p, ok := g.permission[login]
		g.mu.Unlock()
		if !ok {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]any{"message": "Not Found"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"permission": p})
	})
	g.srv = httptest.NewServer(mux)
	t.Cleanup(g.srv.Close)
	return g
}

// ---------- dựng app ----------

type app struct {
	t   *testing.T
	db  *sql.DB
	gh  *fakeGitHub
	svc *auth.Service
	srv *httptest.Server
	now time.Time
	mu  sync.Mutex
}

func (a *app) clock() time.Time { a.mu.Lock(); defer a.mu.Unlock(); return a.now }
func (a *app) advance(d time.Duration) {
	a.mu.Lock()
	a.now = a.now.Add(d)
	a.mu.Unlock()
}

func newApp(t *testing.T, baseURL string) *app {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a := &app{t: t, db: db, gh: newGitHub(t), now: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)}
	a.svc = auth.New(db, auth.Config{
		ClientID: "cid", ClientSecret: "csecret",
		BaseURL:   baseURL,
		JWTSecret: "test-secret-at-least-32-characters-long",
		RepoOwner: "namth", RepoName: "agenthub",
		ServerToken:  "server-pat",
		GitHubWebURL: a.gh.srv.URL, GitHubAPIURL: a.gh.srv.URL,
		SessionTTL: 24 * time.Hour,
		Now:        a.clock,
	})
	mux := http.NewServeMux()
	mux.Handle("/api/auth/", a.svc.Routes())
	protected := a.svc.Middleware(auth.RequireRole("ADMIN")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.UserFrom(r.Context())
		if !ok {
			t.Error("RequireRole cho qua nhưng không có user trong context")
		}
		w.Write([]byte("admin-ok:" + u.Username))
	})))
	mux.Handle("/api/admin/ping", protected)
	mux.Handle("/api/any/ping", a.svc.Middleware(auth.RequireRole("ADMIN", "PARTNER")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("any-ok"))
	}))))
	a.srv = httptest.NewServer(mux)
	t.Cleanup(a.srv.Close)
	return a
}

func client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func readEnv(t *testing.T, res *http.Response) envelope {
	t.Helper()
	defer res.Body.Close()
	var e envelope
	if err := json.NewDecoder(res.Body).Decode(&e); err != nil {
		t.Fatalf("body không phải JSON chuẩn docs/07: %v", err)
	}
	return e
}

// login chạy trọn luồng OAuth bằng client c, trả response của callback.
func (a *app) login(c *http.Client, code string) *http.Response {
	a.t.Helper()
	res, err := c.Get(a.srv.URL + "/api/auth/github")
	if err != nil {
		a.t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound {
		a.t.Fatalf("/api/auth/github: status %d, muốn 302", res.StatusCode)
	}
	loc, _ := url.Parse(res.Header.Get("Location"))
	state := loc.Query().Get("state")
	res, err = c.Get(a.srv.URL + "/api/auth/callback?code=" + code + "&state=" + url.QueryEscape(state))
	if err != nil {
		a.t.Fatal(err)
	}
	return res
}

func sessionCookie(res *http.Response) *http.Cookie {
	for _, c := range res.Cookies() {
		if c.Name == "agenthub_session" {
			return c
		}
	}
	return nil
}

// ---------- test ----------

func TestLoginRedirectsToGitHub(t *testing.T) {
	a := newApp(t, "http://localhost:8080")
	res, err := client().Get(a.srv.URL + "/api/auth/github")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	loc, _ := url.Parse(res.Header.Get("Location"))
	q := loc.Query()
	if res.StatusCode != 302 || !strings.HasPrefix(res.Header.Get("Location"), a.gh.srv.URL+"/login/oauth/authorize") {
		t.Fatalf("redirect = %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	if q.Get("client_id") != "cid" || q.Get("redirect_uri") != "http://localhost:8080/api/auth/callback" || len(q.Get("state")) < 16 {
		t.Fatalf("query authorize sai: %v", q)
	}
	if q.Get("scope") != "read:user" {
		t.Fatalf("scope = %q, chỉ xin read:user (quyền repo đọc bằng token server)", q.Get("scope"))
	}
	var stateCookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == "agenthub_oauth_state" {
			stateCookie = c
		}
	}
	if stateCookie == nil || !stateCookie.HttpOnly {
		t.Fatal("phải đặt cookie agenthub_oauth_state HttpOnly")
	}
}

func TestCallbackAdminCreatesUserAndSession(t *testing.T) {
	a := newApp(t, "http://localhost:8080")
	c := client()
	res := a.login(c, "good-code")
	res.Body.Close()
	if res.StatusCode != 302 || res.Header.Get("Location") != "/" {
		t.Fatalf("callback = %d %s, muốn 302 về /", res.StatusCode, res.Header.Get("Location"))
	}
	ck := sessionCookie(res)
	if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie phiên phải HttpOnly + SameSite=Lax: %+v", ck)
	}
	var role, username, avatar string
	if err := a.db.QueryRow(`SELECT role, username, avatar_url FROM users WHERE github_id='42'`).Scan(&role, &username, &avatar); err != nil {
		t.Fatal(err)
	}
	if role != "ADMIN" || username != "namth" || avatar != "https://avatars/42" {
		t.Fatalf("users = %s %s %s", role, username, avatar)
	}
	a.gh.mu.Lock()
	permAuth := a.gh.lastAuth["permission"]
	a.gh.mu.Unlock()
	if permAuth != "Bearer server-pat" && permAuth != "token server-pat" {
		t.Fatalf("tra quyền repo phải dùng token server, nhận %q", permAuth)
	}
	me, err := c.Get(a.srv.URL + "/api/auth/me")
	if err != nil {
		t.Fatal(err)
	}
	e := readEnv(t, me)
	var u struct{ ID, Username, AvatarURL, Role string }
	var raw map[string]any
	json.Unmarshal(e.Data, &raw)
	u.Username, _ = raw["username"].(string)
	u.Role, _ = raw["role"].(string)
	u.ID, _ = raw["id"].(string)
	u.AvatarURL, _ = raw["avatar_url"].(string)
	if me.StatusCode != 200 || !e.Success || u.Username != "namth" || u.Role != "ADMIN" || u.ID == "" || u.AvatarURL == "" {
		t.Fatalf("/api/auth/me = %d %s", me.StatusCode, e.Data)
	}
}

func TestRoleMappingDEC09(t *testing.T) {
	cases := map[string]string{"admin": "ADMIN", "maintain": "ADMIN", "write": "PARTNER", "triage": "PARTNER", "read": "PARTNER"}
	for perm, want := range cases {
		a := newApp(t, "http://localhost:8080")
		a.gh.permission["namth"] = perm
		c := client()
		a.login(c, "good-code").Body.Close()
		var role string
		if err := a.db.QueryRow(`SELECT role FROM users WHERE github_id='42'`).Scan(&role); err != nil || role != want {
			t.Fatalf("permission %s → role %q (err %v), muốn %s", perm, role, err, want)
		}
	}
}

func TestRoleUpdatedOnNextLogin(t *testing.T) {
	a := newApp(t, "http://localhost:8080")
	a.login(client(), "good-code").Body.Close()
	a.gh.permission["namth"] = "write"
	a.login(client(), "good-code").Body.Close()
	var n int
	var role string
	a.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	a.db.QueryRow(`SELECT role FROM users WHERE github_id='42'`).Scan(&role)
	if n != 1 || role != "PARTNER" {
		t.Fatalf("users = %d, role = %s: phải cập nhật bản ghi cũ, không tạo mới", n, role)
	}
}

func TestNonCollaboratorRejected(t *testing.T) {
	a := newApp(t, "http://localhost:8080")
	delete(a.gh.permission, "namth")
	res := a.login(client(), "good-code")
	if res.StatusCode != 403 || sessionCookie(res) != nil {
		t.Fatalf("không phải collaborator: %d, cookie %v, muốn 403 và không có phiên", res.StatusCode, sessionCookie(res))
	}
	if e := readEnv(t, res); e.Error == nil || e.Error.Code != "FORBIDDEN" {
		t.Fatalf("body = %+v", e)
	}
	var n int
	a.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	if n != 0 {
		t.Fatal("không được tạo user cho người ngoài repo")
	}
}

func TestCallbackRejectsBadStateOrCode(t *testing.T) {
	a := newApp(t, "http://localhost:8080")
	// Không đi qua /api/auth/github → không có cookie state.
	res, _ := client().Get(a.srv.URL + "/api/auth/callback?code=good-code&state=gia-mao")
	if res.StatusCode != 400 || sessionCookie(res) != nil {
		t.Fatalf("state giả: %d, muốn 400 không phiên", res.StatusCode)
	}
	if e := readEnv(t, res); e.Error == nil || e.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("body = %+v", e)
	}
	// Code sai.
	res = a.login(client(), "bad-code")
	if res.StatusCode != 401 || sessionCookie(res) != nil {
		t.Fatalf("code sai: %d, muốn 401 không phiên", res.StatusCode)
	}
	res.Body.Close()
}

func TestSecureCookieOnHTTPS(t *testing.T) {
	a := newApp(t, "https://agenthub.example.com")
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, _ := c.Get(a.srv.URL + "/api/auth/github")
	res.Body.Close()
	var state string
	for _, ck := range res.Cookies() {
		if ck.Name == "agenthub_oauth_state" {
			state = ck.Value
			if !ck.Secure {
				t.Fatal("BASE_URL https → cookie state phải Secure")
			}
		}
	}
	loc, _ := url.Parse(res.Header.Get("Location"))
	req, _ := http.NewRequest("GET", a.srv.URL+"/api/auth/callback?code=good-code&state="+url.QueryEscape(loc.Query().Get("state")), nil)
	req.AddCookie(&http.Cookie{Name: "agenthub_oauth_state", Value: state})
	res, _ = c.Do(req)
	res.Body.Close()
	if ck := sessionCookie(res); ck == nil || !ck.Secure {
		t.Fatalf("BASE_URL https → cookie phiên phải Secure: %+v", ck)
	}
}

func TestMeRejectsMissingTamperedExpired(t *testing.T) {
	a := newApp(t, "http://localhost:8080")
	res, _ := client().Get(a.srv.URL + "/api/auth/me")
	if e := readEnv(t, res); res.StatusCode != 401 || e.Error == nil || e.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("không cookie: %d %+v", res.StatusCode, e)
	}
	c := client()
	cb := a.login(c, "good-code")
	ck := sessionCookie(cb)
	cb.Body.Close()

	// Sửa payload (đổi role) → chữ ký sai.
	parts := strings.Split(ck.Value, ".")
	if len(parts) != 3 {
		t.Fatalf("cookie phiên phải là JWT 3 phần, nhận %q", ck.Value)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	forged := strings.Replace(string(payload), `"ADMIN"`, `"PARTNER"`, 1)
	bad := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(forged)) + "." + parts[2]
	req, _ := http.NewRequest("GET", a.srv.URL+"/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "agenthub_session", Value: bad})
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != 401 {
		t.Fatalf("JWT bị sửa: %d, muốn 401", res.StatusCode)
	}
	res.Body.Close()

	// alg=none.
	none := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + parts[1] + "."
	req, _ = http.NewRequest("GET", a.srv.URL+"/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "agenthub_session", Value: none})
	res, _ = http.DefaultClient.Do(req)
	if res.StatusCode != 401 {
		t.Fatalf("JWT alg=none: %d, muốn 401", res.StatusCode)
	}
	res.Body.Close()

	// Hết hạn.
	a.advance(25 * time.Hour)
	res, _ = c.Get(a.srv.URL + "/api/auth/me")
	if res.StatusCode != 401 {
		t.Fatalf("phiên quá TTL: %d, muốn 401", res.StatusCode)
	}
	res.Body.Close()
}

func TestLogoutClearsSession(t *testing.T) {
	a := newApp(t, "http://localhost:8080")
	c := client()
	a.login(c, "good-code").Body.Close()
	req, _ := http.NewRequest("POST", a.srv.URL+"/api/auth/logout", nil)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if e := readEnv(t, res); res.StatusCode != 200 || !e.Success {
		t.Fatalf("logout = %d %+v", res.StatusCode, e)
	}
	cleared := false
	for _, ck := range res.Cookies() {
		if ck.Name == "agenthub_session" && (ck.MaxAge < 0 || ck.Value == "") {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("logout phải xoá cookie agenthub_session")
	}
	me, _ := c.Get(a.srv.URL + "/api/auth/me")
	if me.StatusCode != 401 {
		t.Fatalf("sau logout /me = %d, muốn 401", me.StatusCode)
	}
	me.Body.Close()
}

func TestRequireRole(t *testing.T) { // TC-09 dạng tổng quát
	a := newApp(t, "http://localhost:8080")
	anon := client()
	res, _ := anon.Get(a.srv.URL + "/api/admin/ping")
	if e := readEnv(t, res); res.StatusCode != 401 || e.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("chưa đăng nhập: %d", res.StatusCode)
	}

	admin := client()
	a.login(admin, "good-code").Body.Close()
	res, _ = admin.Get(a.srv.URL + "/api/admin/ping")
	buf := make([]byte, 64)
	n, _ := res.Body.Read(buf)
	res.Body.Close()
	if res.StatusCode != 200 || string(buf[:n]) != "admin-ok:namth" {
		t.Fatalf("admin: %d %q", res.StatusCode, buf[:n])
	}

	a.gh.mu.Lock()
	a.gh.permission["doi-tac"] = "read"
	a.gh.user = map[string]any{"id": 77, "login": "doi-tac", "avatar_url": "x"}
	a.gh.mu.Unlock()
	partner := client()
	a.login(partner, "good-code").Body.Close()
	res, _ = partner.Get(a.srv.URL + "/api/admin/ping")
	if e := readEnv(t, res); res.StatusCode != 403 || e.Error.Code != "FORBIDDEN" {
		t.Fatalf("partner gọi API admin: %d, muốn 403 FORBIDDEN", res.StatusCode)
	}
	res, _ = partner.Get(a.srv.URL + "/api/any/ping")
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("partner gọi API cho cả hai vai: %d, muốn 200", res.StatusCode)
	}
}
