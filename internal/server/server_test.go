package server_test

// Test nghiệm thu TASK-001 — do trưởng phòng viết, file bị khoá (tests.lock).

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/server"
)

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	static := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>AgentHub</title><div id=root></div>")},
		"assets/app.js": {Data: []byte("console.log('app')")},
	}
	ts := httptest.NewServer(server.New(db, static))
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func TestHealth(t *testing.T) {
	ts := newServer(t)
	res, body := get(t, ts.URL+"/api/health")
	if res.StatusCode != 200 {
		t.Fatalf("status = %d, muốn 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type = %q, muốn application/json", ct)
	}
	var e envelope
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("body không phải JSON: %q", body)
	}
	var data struct{ Status string }
	_ = json.Unmarshal(e.Data, &data)
	if !e.Success || data.Status != "ok" || e.Error != nil {
		t.Fatalf("body = %s, muốn success=true, data.status=ok, error=null", body)
	}
	if !strings.Contains(body, `"error":null`) {
		t.Fatalf("body phải có trường error: null theo docs/07, nhận %s", body)
	}
}

func TestUnknownAPIReturnsJSON404(t *testing.T) {
	ts := newServer(t)
	for _, p := range []string{"/api/khong-co", "/api/tasks/xyz/khong-co"} {
		res, body := get(t, ts.URL+p)
		if res.StatusCode != 404 {
			t.Fatalf("%s: status = %d, muốn 404", p, res.StatusCode)
		}
		var e envelope
		if err := json.Unmarshal([]byte(body), &e); err != nil {
			t.Fatalf("%s: phải trả JSON, không phải index.html: %q", p, body)
		}
		if e.Success || e.Error == nil || e.Error.Code != "NOT_FOUND" {
			t.Fatalf("%s: body = %s, muốn success=false, error.code=NOT_FOUND", p, body)
		}
	}
}

func TestStaticFileServed(t *testing.T) {
	ts := newServer(t)
	res, body := get(t, ts.URL+"/assets/app.js")
	if res.StatusCode != 200 || body != "console.log('app')" {
		t.Fatalf("/assets/app.js: status %d body %q", res.StatusCode, body)
	}
	res, body = get(t, ts.URL+"/")
	if res.StatusCode != 200 || !strings.Contains(body, "AgentHub") {
		t.Fatalf("/: status %d body %q", res.StatusCode, body)
	}
}

func TestSPAFallback(t *testing.T) {
	ts := newServer(t)
	for _, p := range []string{"/kanban", "/tasks/123", "/fleet/mac-mini/terminal"} {
		res, body := get(t, ts.URL+p)
		if res.StatusCode != 200 || !strings.Contains(body, "AgentHub") {
			t.Fatalf("%s: SPA fallback phải trả index.html 200, nhận %d %q", p, res.StatusCode, body)
		}
	}
}
