package tokens_test

// Test nghiệm thu TASK-046 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/remote-command.md §5, §6, BR-62; TC-92.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"database/sql"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/tokens"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func setup(t *testing.T) (*sql.DB, *clock, *tokens.Store) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-owner', '1', 'namth', 'ADMIN')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-p', '2', 'doi-tac', 'PARTNER')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	c := &clock{t: time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)}
	return db, c, tokens.NewStore(db, c.now)
}

func TestCreateAuthenticateRevoke(t *testing.T) { // TC-92
	db, c, s := setup(t)
	plain, tok, err := s.Create("u-owner", "macbook", 30)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, "ahp_") || len(plain) < 40 || tok.Last4 != plain[len(plain)-4:] {
		t.Fatalf("token = %q last4=%q", plain, tok.Last4)
	}
	if !tok.ExpiresAt.Equal(c.now().Add(30 * 24 * time.Hour)) {
		t.Fatalf("expires_at = %v", tok.ExpiresAt)
	}
	var stored string
	db.QueryRow(`SELECT token_hash FROM personal_tokens WHERE id = ?`, tok.ID).Scan(&stored)
	sum := sha256.Sum256([]byte(plain))
	if stored != hex.EncodeToString(sum[:]) {
		t.Fatal("DB phải lưu SHA-256 hex của token, không lưu token gốc")
	}
	var leak int
	db.QueryRow(`SELECT COUNT(*) FROM personal_tokens WHERE token_hash = ? OR name = ? OR last4 = ?`, plain, plain, plain).Scan(&leak)
	if leak != 0 {
		t.Fatal("token gốc không được nằm trong DB")
	}
	uid, err := s.Authenticate(plain)
	if err != nil || uid != "u-owner" {
		t.Fatalf("Authenticate = %q %v", uid, err)
	}
	for _, bad := range []string{"", "ahp_sai", plain + "x", strings.TrimPrefix(plain, "ahp_")} {
		if _, err := s.Authenticate(bad); !errors.Is(err, tokens.ErrUnauthorized) {
			t.Fatalf("Authenticate(%q) err = %v, muốn ErrUnauthorized", bad, err)
		}
	}
	if err := s.Revoke("u-p", tok.ID); !errors.Is(err, tokens.ErrForbidden) {
		t.Fatalf("thu hồi token người khác: %v", err)
	}
	if err := s.Revoke("u-owner", "khong-co"); !errors.Is(err, tokens.ErrNotFound) {
		t.Fatalf("thu hồi token không có: %v", err)
	}
	if err := s.Revoke("u-owner", tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Authenticate(plain); !errors.Is(err, tokens.ErrUnauthorized) {
		t.Fatal("token đã thu hồi phải bị từ chối")
	}
	// Hết hạn.
	p2, _, _ := s.Create("u-owner", "ngan-han", 30)
	c.add(31 * 24 * time.Hour)
	if _, err := s.Authenticate(p2); !errors.Is(err, tokens.ErrUnauthorized) {
		t.Fatal("token hết hạn phải bị từ chối")
	}
}

func TestCreateValidationAndLimits(t *testing.T) {
	_, _, s := setup(t)
	for _, d := range []int{0, 7, 400} {
		if _, _, err := s.Create("u-owner", "x", d); !errors.Is(err, tokens.ErrValidation) {
			t.Fatalf("days=%d: %v", d, err)
		}
	}
	if _, _, err := s.Create("u-owner", "", 30); !errors.Is(err, tokens.ErrValidation) {
		t.Fatal("tên rỗng phải ErrValidation")
	}
	if _, _, err := s.Create("u-owner", strings.Repeat("a", 41), 30); !errors.Is(err, tokens.ErrValidation) {
		t.Fatal("tên > 40 ký tự phải ErrValidation")
	}
	if _, _, err := s.Create("u-owner", "trung", 90); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Create("u-owner", "trung", 90); !errors.Is(err, tokens.ErrConflict) {
		t.Fatal("trùng tên phải ErrConflict")
	}
	for i := 0; i < 9; i++ {
		if _, _, err := s.Create("u-owner", "t"+string(rune('a'+i)), 365); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.Create("u-owner", "qua-gioi-han", 30); !errors.Is(err, tokens.ErrLimit) {
		t.Fatal("token thứ 11 phải ErrLimit")
	}
	list, _ := s.List("u-owner")
	if len(list) != 10 {
		t.Fatalf("List = %d", len(list))
	}
}

func TestLastUsedThrottled(t *testing.T) {
	db, c, s := setup(t)
	plain, tok, _ := s.Create("u-owner", "mcp", 30)
	s.Authenticate(plain)
	var first string
	db.QueryRow(`SELECT last_used_at FROM personal_tokens WHERE id=?`, tok.ID).Scan(&first)
	if first == "" {
		t.Fatal("phải ghi last_used_at")
	}
	c.add(20 * time.Second)
	s.Authenticate(plain)
	var second string
	db.QueryRow(`SELECT last_used_at FROM personal_tokens WHERE id=?`, tok.ID).Scan(&second)
	if second != first {
		t.Fatal("trong 1 phút chỉ ghi last_used_at một lần")
	}
}

func TestTokensHTTP(t *testing.T) {
	_, _, s := setup(t)
	h := tokens.Handler(s, func(r *http.Request) (string, string, bool) {
		u := r.Header.Get("X-Test-User")
		role := map[string]string{"u-owner": "ADMIN", "u-p": "PARTNER"}[u]
		return u, role, u != ""
	})
	do := func(method, path, user string, body any) (int, map[string]any) {
		var b []byte
		if body != nil {
			b, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(b))
		if user != "" {
			req.Header.Set("X-Test-User", user)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		var m map[string]any
		json.Unmarshal(rr.Body.Bytes(), &m)
		return rr.Code, m
	}
	if code, _ := do("POST", "/api/me/tokens", "", map[string]any{"name": "a", "expires_days": 30}); code != 401 {
		t.Fatalf("chưa đăng nhập: %d", code)
	}
	if code, _ := do("POST", "/api/me/tokens", "u-p", map[string]any{"name": "a", "expires_days": 30}); code != 403 {
		t.Fatalf("partner tạo token: %d, muốn 403", code)
	}
	code, m := do("POST", "/api/me/tokens", "u-owner", map[string]any{"name": "laptop", "expires_days": 90})
	if code != 201 {
		t.Fatalf("tạo = %d %v", code, m)
	}
	data := m["data"].(map[string]any)
	plain, _ := data["token"].(string)
	if !strings.HasPrefix(plain, "ahp_") || data["id"] == "" || data["last4"] != plain[len(plain)-4:] {
		t.Fatalf("data = %v", data)
	}
	code, m = do("GET", "/api/me/tokens", "u-owner", nil)
	raw, _ := json.Marshal(m)
	if code != 200 || strings.Contains(string(raw), plain) || !strings.Contains(string(raw), "laptop") {
		t.Fatalf("GET không được trả token gốc: %d %s", code, raw)
	}
	if code, _ := do("POST", "/api/me/tokens", "u-owner", map[string]any{"name": "laptop", "expires_days": 90}); code != 409 {
		t.Fatalf("trùng tên: %d, muốn 409", code)
	}
	if code, _ := do("POST", "/api/me/tokens", "u-owner", map[string]any{"name": "x", "expires_days": 5}); code != 400 {
		t.Fatalf("hạn sai: %d, muốn 400", code)
	}
	if code, _ := do("DELETE", "/api/me/tokens/"+data["id"].(string), "u-owner", nil); code != 204 && code != 200 {
		t.Fatalf("thu hồi: %d", code)
	}
	if code, _ := do("DELETE", "/api/me/tokens/khong-co", "u-owner", nil); code != 404 {
		t.Fatalf("thu hồi không có: %d, muốn 404", code)
	}
}
