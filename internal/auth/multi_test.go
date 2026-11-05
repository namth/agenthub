package auth_test

// Test nghiệm thu TASK-020 (phần auth) — do trưởng phòng viết, file bị khoá. DEC-17.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/auth"
	"github.com/namth/agenthub/internal/database"
)

type loginHook struct {
	members map[string]int // login → số dự án là thành viên
	calls   []string
}

func (h *loginHook) fn(_ context.Context, userID, login string) (int, error) {
	h.calls = append(h.calls, userID+"|"+login)
	return h.members[login], nil
}

func newMultiApp(t *testing.T, hook *loginHook) (*app, *httptest.Server) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a := &app{t: t, db: db, gh: newGitHub(t), now: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)}
	a.svc = auth.New(db, auth.Config{
		ClientID: "cid", ClientSecret: "csecret", BaseURL: "http://localhost:1407",
		JWTSecret: "test-secret-at-least-32-characters-long", ServerToken: "server-pat",
		GitHubWebURL: a.gh.srv.URL, GitHubAPIURL: a.gh.srv.URL, SessionTTL: time.Hour, Now: a.clock,
		Owner: "namth", OnLogin: hook.fn,
	})
	mux := http.NewServeMux()
	mux.Handle("/api/auth/", a.svc.Routes())
	a.srv = httptest.NewServer(mux)
	t.Cleanup(a.srv.Close)
	return a, a.srv
}

func TestMultiProjectLoginOwnerIsAdmin(t *testing.T) {
	hook := &loginHook{members: map[string]int{"namth": 0}}
	a, _ := newMultiApp(t, hook)
	res := a.login(client(), "good-code")
	res.Body.Close()
	if res.StatusCode != 302 || sessionCookie(res) == nil {
		t.Fatalf("owner luôn đăng nhập được (kể cả chưa có dự án): %d", res.StatusCode)
	}
	var role string
	a.db.QueryRow(`SELECT role FROM users WHERE github_id='42'`).Scan(&role)
	if role != "ADMIN" || len(hook.calls) != 1 || hook.calls[0][len(hook.calls[0])-6:] != "|namth" {
		t.Fatalf("owner → ADMIN hệ thống và gọi OnLogin: role=%s calls=%v", role, hook.calls)
	}
}

func TestMultiProjectLoginPartnerNeedsMembership(t *testing.T) {
	hook := &loginHook{members: map[string]int{"doi-tac": 0}}
	a, _ := newMultiApp(t, hook)
	a.gh.user = map[string]any{"id": 77, "login": "doi-tac", "avatar_url": "x"}
	res := a.login(client(), "good-code")
	if res.StatusCode != 403 || sessionCookie(res) != nil {
		t.Fatalf("partner không thuộc dự án nào: %d, muốn 403 không phiên", res.StatusCode)
	}
	res.Body.Close()
	hook.members["doi-tac"] = 2
	res = a.login(client(), "good-code")
	res.Body.Close()
	if res.StatusCode != 302 || sessionCookie(res) == nil {
		t.Fatalf("partner thuộc 2 dự án đăng nhập được: %d", res.StatusCode)
	}
	var role string
	a.db.QueryRow(`SELECT role FROM users WHERE github_id='77'`).Scan(&role)
	if role != "PARTNER" {
		t.Fatalf("không phải owner → PARTNER ở cấp hệ thống: %s", role)
	}
	a.gh.mu.Lock()
	_, permCalled := a.gh.lastAuth["permission"]
	a.gh.mu.Unlock()
	if permCalled {
		t.Fatal("chế độ đa dự án không tra quyền theo một repo cố định (OnLogin lo việc đó)")
	}
}
