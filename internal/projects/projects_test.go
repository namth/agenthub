package projects_test

// Test nghiệm thu TASK-020 — do trưởng phòng viết, file bị khoá (tests.lock).
// DEC-14, DEC-17, BR-26, BR-27, TC-38, TC-39.

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/projects"
)

// ---------- GitHub giả ----------

type fakeGH struct {
	mu      sync.Mutex
	repos   []projects.Repo
	perms   map[string]string // "repo|login" → permission; thiếu = không phải collaborator
	hookErr error
	hooks   []string
	listErr error
}

func (g *fakeGH) ListRepos(context.Context) ([]projects.Repo, error) {
	return g.repos, g.listErr
}
func (g *fakeGH) CreateWebhook(_ context.Context, full, url, secret string) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.hookErr != nil {
		return 0, g.hookErr
	}
	g.hooks = append(g.hooks, full+"|"+url+"|"+secret)
	return int64(len(g.hooks)), nil
}
func (g *fakeGH) Permission(_ context.Context, full, login string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p, ok := g.perms[full+"|"+login]
	if !ok {
		return "", projects.ErrNotCollaborator
	}
	return p, nil
}

type rig struct {
	t   *testing.T
	db  *sql.DB
	gh  *fakeGH
	svc *projects.Service
	h   http.Handler
}

func newRig(t *testing.T) *rig {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	gh := &fakeGH{
		repos: []projects.Repo{{FullName: "namth/agenthub", DefaultBranch: "main"}, {FullName: "namth/edusystem", DefaultBranch: "develop"}, {FullName: "namth/zalo", DefaultBranch: "main"}},
		perms: map[string]string{"namth/agenthub|namth": "admin", "namth/edusystem|namth": "admin", "namth/zalo|namth": "admin",
			"namth/agenthub|doi-tac": "write"},
	}
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-owner', '1', 'namth', 'ADMIN')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-partner', '2', 'doi-tac', 'PARTNER')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'agenthub', 'namth/agenthub')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	svc := projects.New(db, gh, projects.Config{Owner: "namth", BaseURL: "https://agent.oa.io.vn", WebhookSecret: "whsec"})
	r := &rig{t: t, db: db, gh: gh, svc: svc}
	r.h = projects.Handler(svc, func(req *http.Request) (string, string, bool) {
		v := req.Header.Get("X-Test-User") // "<user_id>:<login>"
		i := strings.Index(v, ":")
		if i < 0 {
			return "", "", false
		}
		return v[:i], v[i+1:], true
	})
	return r
}

type env struct {
	Success bool                   `json:"success"`
	Data    json.RawMessage        `json:"data"`
	Error   *struct{ Code string } `json:"error"`
}

func (r *rig) do(method, path, user string, body any) (int, env) {
	r.t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	rr := httptest.NewRecorder()
	r.h.ServeHTTP(rr, req)
	var e env
	if err := json.Unmarshal(rr.Body.Bytes(), &e); err != nil {
		r.t.Fatalf("%s %s: không phải JSON: %q", method, path, rr.Body.String())
	}
	return rr.Code, e
}

const owner = "u-owner:namth"
const partner = "u-partner:doi-tac"

// ---------- đồng bộ vai ----------

func TestSyncUserRolesPerProject(t *testing.T) { // BR-27
	r := newRig(t)
	ctx := context.Background()
	if _, err := r.svc.AddProject(ctx, "namth/edusystem"); err != nil {
		t.Fatal(err)
	}
	n, err := r.svc.SyncUser(ctx, "u-partner", "doi-tac")
	if err != nil || n != 1 {
		t.Fatalf("partner là collaborator đúng 1 dự án: n=%d err=%v", n, err)
	}
	role, ok := r.svc.RoleIn(ctx, "u-partner", "p1")
	if !ok || role != "PARTNER" {
		t.Fatalf("vai partner ở p1 = %q %v, muốn PARTNER (write → PARTNER, DEC-09)", role, ok)
	}
	var pid string
	r.db.QueryRow(`SELECT id FROM projects WHERE github_repo='namth/edusystem'`).Scan(&pid)
	if _, ok := r.svc.RoleIn(ctx, "u-partner", pid); ok {
		t.Fatal("không là collaborator thì không có vai trong dự án")
	}
	// Mất quyền → bị xoá khỏi dự án.
	delete(r.gh.perms, "namth/agenthub|doi-tac")
	if n, _ := r.svc.SyncUser(ctx, "u-partner", "doi-tac"); n != 0 {
		t.Fatalf("mất quyền: n=%d", n)
	}
	if _, ok := r.svc.RoleIn(ctx, "u-partner", "p1"); ok {
		t.Fatal("mất quyền collaborator phải bị xoá khỏi project_members")
	}
	if !r.svc.IsOwner("namth") || !r.svc.IsOwner("NamTh") || r.svc.IsOwner("doi-tac") || r.svc.IsOwner("") {
		t.Fatal("IsOwner: so login với owner, không phân biệt hoa thường (login GitHub không phân biệt hoa thường)")
	}
}

// ---------- thêm dự án ----------

func TestAddProjectCreatesWebhookAndMembers(t *testing.T) { // TC-39
	r := newRig(t)
	code, e := r.do("POST", "/api/admin/projects", owner, map[string]any{"github_repo": "namth/edusystem"})
	if code != 201 {
		t.Fatalf("thêm dự án = %d %+v", code, e.Error)
	}
	var p struct {
		ID, GitHubRepo, DefaultBranch string
		WebhookOK                     bool `json:"webhook_ok"`
	}
	var raw map[string]any
	json.Unmarshal(e.Data, &raw)
	p.ID, _ = raw["id"].(string)
	p.GitHubRepo, _ = raw["github_repo"].(string)
	p.DefaultBranch, _ = raw["default_branch"].(string)
	p.WebhookOK, _ = raw["webhook_ok"].(bool)
	if p.ID == "" || p.GitHubRepo != "namth/edusystem" || p.DefaultBranch != "develop" || !p.WebhookOK {
		t.Fatalf("dự án = %s", e.Data)
	}
	if len(r.gh.hooks) != 1 || r.gh.hooks[0] != "namth/edusystem|https://agent.oa.io.vn/api/webhooks/github|whsec" {
		t.Fatalf("webhook = %v", r.gh.hooks)
	}
	var wid sql.NullInt64
	r.db.QueryRow(`SELECT webhook_id FROM projects WHERE id=?`, p.ID).Scan(&wid)
	if !wid.Valid {
		t.Fatal("phải lưu webhook_id")
	}
	var role string
	r.db.QueryRow(`SELECT role FROM project_members WHERE project_id=? AND user_id='u-owner'`, p.ID).Scan(&role)
	if role != "ADMIN" {
		t.Fatalf("thêm dự án phải đồng bộ vai cho user đã biết: owner = %q", role)
	}
}

func TestAddProjectWebhookFailureStillSaves(t *testing.T) { // BR-26
	r := newRig(t)
	r.gh.hookErr = errors.New("403 Resource not accessible")
	code, e := r.do("POST", "/api/admin/projects", owner, map[string]any{"github_repo": "namth/zalo"})
	if code != 201 || !bytes.Contains(e.Data, []byte(`"webhook_ok":false`)) {
		t.Fatalf("webhook lỗi vẫn lưu dự án, webhook_ok=false: %d %s", code, e.Data)
	}
}

func TestAddProjectErrors(t *testing.T) {
	r := newRig(t)
	if code, e := r.do("POST", "/api/admin/projects", owner, map[string]any{"github_repo": "namth/agenthub"}); code != 409 || e.Error.Code != "CONFLICT" {
		t.Fatalf("đã có: %d", code)
	}
	if code, e := r.do("POST", "/api/admin/projects", owner, map[string]any{"github_repo": "nguoi-khac/repo"}); code != 404 || e.Error.Code != "NOT_FOUND" {
		t.Fatalf("PAT không thấy repo: %d", code)
	}
	if code, e := r.do("POST", "/api/admin/projects", owner, map[string]any{"github_repo": "khong co dau gach"}); code != 400 || e.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("sai định dạng: %d", code)
	}
}

// ---------- phân quyền & API ----------

func TestAdminEndpointsOwnerOnly(t *testing.T) {
	r := newRig(t)
	for _, q := range []struct{ m, p string }{{"GET", "/api/admin/github/repos"}, {"POST", "/api/admin/projects"}, {"PATCH", "/api/admin/projects/p1"}} {
		if code, e := r.do(q.m, q.p, partner, map[string]any{}); code != 403 || e.Error.Code != "FORBIDDEN" {
			t.Errorf("%s %s bởi partner: %d", q.m, q.p, code)
		}
		if code, _ := r.do(q.m, q.p, "", map[string]any{}); code != 401 {
			t.Errorf("%s %s chưa đăng nhập: %d", q.m, q.p, code)
		}
	}
}

func TestListReposMarksAdded(t *testing.T) {
	r := newRig(t)
	code, e := r.do("GET", "/api/admin/github/repos", owner, nil)
	var repos []map[string]any
	json.Unmarshal(e.Data, &repos)
	if code != 200 || len(repos) != 3 {
		t.Fatalf("repos = %d %s", code, e.Data)
	}
	added := map[string]bool{}
	for _, x := range repos {
		added[x["full_name"].(string)], _ = x["added"].(bool)
	}
	if !added["namth/agenthub"] || added["namth/edusystem"] {
		t.Fatalf("added = %v", added)
	}
	r.gh.listErr = errors.New("401 Bad credentials")
	if code, e := r.do("GET", "/api/admin/github/repos", owner, nil); code != 502 || e.Error.Code != "GITHUB_ERROR" {
		t.Fatalf("PAT lỗi: %d", code)
	}
}

func TestListProjectsByMembership(t *testing.T) { // TC-38 phần API
	r := newRig(t)
	ctx := context.Background()
	r.svc.AddProject(ctx, "namth/edusystem")
	r.svc.SyncUser(ctx, "u-partner", "doi-tac")
	_, e := r.do("GET", "/api/projects", partner, nil)
	var ps []map[string]any
	json.Unmarshal(e.Data, &ps)
	if len(ps) != 1 || ps[0]["github_repo"] != "namth/agenthub" || ps[0]["role"] != "PARTNER" {
		t.Fatalf("partner chỉ thấy dự án mình là thành viên: %s", e.Data)
	}
	_, e = r.do("GET", "/api/projects", owner, nil)
	json.Unmarshal(e.Data, &ps)
	if len(ps) != 2 {
		t.Fatalf("owner thấy mọi dự án: %s", e.Data)
	}
}

func TestPatchProject(t *testing.T) {
	r := newRig(t)
	if code, _ := r.do("PATCH", "/api/admin/projects/p1", owner, map[string]any{"weight": 3, "max_parallel": 2}); code != 200 {
		t.Fatalf("patch = %d", code)
	}
	var w, m int
	r.db.QueryRow(`SELECT weight, max_parallel FROM projects WHERE id='p1'`).Scan(&w, &m)
	if w != 3 || m != 2 {
		t.Fatalf("DB = %d %d", w, m)
	}
	r.do("PATCH", "/api/admin/projects/p1", owner, map[string]any{"max_parallel": nil, "status": "ARCHIVED"})
	var mp sql.NullInt64
	var st string
	r.db.QueryRow(`SELECT max_parallel, status FROM projects WHERE id='p1'`).Scan(&mp, &st)
	if mp.Valid || st != "ARCHIVED" {
		t.Fatalf("null xoá max_parallel, lưu trữ: %v %s", mp, st)
	}
	for _, bad := range []map[string]any{{"weight": 0}, {"weight": 6}, {"max_parallel": 51}, {"status": "DELETED"}, {"la": 1}} {
		if code, _ := r.do("PATCH", "/api/admin/projects/p1", owner, bad); code != 400 {
			t.Errorf("%v: %d, muốn 400", bad, code)
		}
	}
	if code, _ := r.do("PATCH", "/api/admin/projects/khong-co", owner, map[string]any{"weight": 2}); code != 404 {
		t.Fatalf("không có: %d", code)
	}
}

// ---------- GitHub client thật ----------

func TestGitHubClientRequests(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery+" "+r.Header.Get("Authorization")+" "+string(b))
		mu.Unlock()
		switch {
		case r.URL.Path == "/user/repos" && r.URL.Query().Get("page") == "1":
			json.NewEncoder(w).Encode([]map[string]any{{"full_name": "namth/a", "default_branch": "main", "private": true}})
		case r.URL.Path == "/user/repos":
			w.Write([]byte("[]"))
		case strings.HasSuffix(r.URL.Path, "/hooks"):
			w.WriteHeader(201)
			w.Write([]byte(`{"id": 77}`))
		case strings.HasSuffix(r.URL.Path, "/permission"):
			if strings.Contains(r.URL.Path, "/la/") {
				w.WriteHeader(404)
				return
			}
			w.Write([]byte(`{"permission":"maintain"}`))
		}
	}))
	defer srv.Close()
	gh := projects.NewGitHub(srv.URL, "pat", nil)
	ctx := context.Background()
	repos, err := gh.ListRepos(ctx)
	if err != nil || len(repos) != 1 || repos[0].FullName != "namth/a" || repos[0].DefaultBranch != "main" {
		t.Fatalf("ListRepos = %+v %v", repos, err)
	}
	id, err := gh.CreateWebhook(ctx, "namth/a", "https://x/api/webhooks/github", "s")
	if err != nil || id != 77 {
		t.Fatalf("CreateWebhook = %d %v", id, err)
	}
	if p, err := gh.Permission(ctx, "namth/a", "namth"); err != nil || p != "maintain" {
		t.Fatalf("Permission = %q %v", p, err)
	}
	if _, err := gh.Permission(ctx, "namth/a", "la"); !errors.Is(err, projects.ErrNotCollaborator) {
		t.Fatalf("404 → ErrNotCollaborator, nhận %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(calls, "\n")
	for _, want := range []string{"Bearer pat", `"events":["issues","pull_request","workflow_run"]`, `"content_type":"json"`, `"secret":"s"`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("thiếu %q trong:\n%s", want, joined)
		}
	}
}
