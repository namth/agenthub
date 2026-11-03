package gateway_test

// Test nghiệm thu TASK-003 — do trưởng phòng viết, file bị khoá (tests.lock).

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/gateway"
)

func openDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestHashTokenIsSHA256Hex(t *testing.T) {
	if got := gateway.HashToken("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("HashToken(abc) = %s", got)
	}
}

func TestDBAuthenticator(t *testing.T) {
	db := openDB(t)
	if _, err := db.Exec(`INSERT INTO workers (id, name, os, token_hash) VALUES ('mac-mini', 'mac-mini', 'darwin', ?)`, gateway.HashToken("s3cret")); err != nil {
		t.Fatal(err)
	}
	auth := gateway.DBAuthenticator(db)
	if w, err := auth("s3cret"); err != nil || w != "mac-mini" {
		t.Fatalf("auth(s3cret) = %q, %v", w, err)
	}
	for _, bad := range []string{"", "sai", gateway.HashToken("s3cret")} {
		if _, err := auth(bad); !errors.Is(err, gateway.ErrUnauthorized) {
			t.Fatalf("auth(%q): err = %v, muốn ErrUnauthorized", bad, err)
		}
	}
}

type regResp struct {
	Success bool `json:"success"`
	Data    struct {
		WorkerID    string `json:"worker_id"`
		RunnerToken string `json:"runner_token"`
	} `json:"data"`
	Error *struct {
		Code string `json:"code"`
	} `json:"error"`
}

func postRegister(t *testing.T, h http.Handler, key string, body any) (int, regResp) {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/fleet/register", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Registration-Key", key)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	var r regResp
	_ = json.Unmarshal(rr.Body.Bytes(), &r)
	return rr.Code, r
}

func TestRegisterWorker(t *testing.T) {
	db := openDB(t)
	h := gateway.RegisterHandler(db, "master-key")
	ok := map[string]any{"name": "Macbook-M2", "os": "darwin", "agents": []string{"CLAUDE", "ANTIGRAVITY"}}

	if code, r := postRegister(t, h, "", ok); code != 401 || r.Error == nil || r.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("thiếu khoá: %d %+v", code, r)
	}
	if code, r := postRegister(t, h, "sai", ok); code != 401 || r.Error == nil || r.Error.Code != "UNAUTHORIZED" {
		t.Fatalf("sai khoá: %d %+v", code, r)
	}
	for _, bad := range []map[string]any{
		{"name": "mac mini", "os": "darwin", "agents": []string{"CLAUDE"}},
		{"name": "", "os": "darwin", "agents": []string{"CLAUDE"}},
		{"name": "ok-name", "os": "darwin", "agents": []string{}},
		{"name": "ok-name", "os": "darwin", "agents": []string{"GPT"}},
		{"name": "ok-name", "os": "", "agents": []string{"CLAUDE"}},
	} {
		if code, r := postRegister(t, h, "master-key", bad); code != 400 || r.Error == nil || r.Error.Code != "VALIDATION_ERROR" {
			t.Fatalf("body %v: %d %+v, muốn 400 VALIDATION_ERROR", bad, code, r)
		}
	}

	code, r := postRegister(t, h, "master-key", ok)
	if code != 200 || !r.Success || r.Data.WorkerID != "Macbook-M2" || len(r.Data.RunnerToken) < 32 {
		t.Fatalf("đăng ký: %d %+v", code, r)
	}
	var hash, osName string
	if err := db.QueryRow(`SELECT token_hash, os FROM workers WHERE id = 'Macbook-M2'`).Scan(&hash, &osName); err != nil {
		t.Fatal(err)
	}
	if hash != gateway.HashToken(r.Data.RunnerToken) || hash == r.Data.RunnerToken || osName != "darwin" {
		t.Fatalf("DB phải lưu SHA-256 của token, không lưu bản rõ (hash=%s os=%s)", hash, osName)
	}
	if w, err := gateway.DBAuthenticator(db)(r.Data.RunnerToken); err != nil || w != "Macbook-M2" {
		t.Fatalf("token vừa cấp phải xác thực được: %q %v", w, err)
	}

	_, r2 := postRegister(t, h, "master-key", map[string]any{"name": "other", "os": "linux", "agents": []string{"CLAUDE"}})
	if r2.Data.RunnerToken == r.Data.RunnerToken {
		t.Fatal("mỗi máy phải có token ngẫu nhiên riêng")
	}
	if code, r := postRegister(t, h, "master-key", ok); code != 409 || r.Error == nil || r.Error.Code != "CONFLICT" {
		t.Fatalf("trùng tên: %d %+v, muốn 409 CONFLICT", code, r)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/fleet/register", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d, muốn 405", rr.Code)
	}
}
