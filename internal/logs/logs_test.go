package logs_test

// Test nghiệm thu TASK-016 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/06 §3 (giữ ≤ 5000 dòng/task), docs/02 (chỉ Admin xem log), SCR-06.

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/logs"
)

type clk struct{}

func (clk) Now() time.Time { return time.Date(2026, 11, 4, 14, 0, 0, 0, time.UTC) }

func setup(t *testing.T, keep int) (*sql.DB, *logs.Store, string) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "l.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u1', 'g1', 'namth', 'ADMIN')`,
		`INSERT INTO projects (id, name, github_repo) VALUES ('p1', 'A', 'namth/agenthub')`,
		`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('t1', 'p1', 'A', 'B', 'u1', 'READY')`,
		`INSERT INTO workers (id, name, os, token_hash) VALUES ('mac', 'mac', 'darwin', 'h')`,
	} {
		db.Exec(q)
	}
	d := dispatcher.New(db, clk{}, dispatcher.DefaultConfig(), nil)
	d.Connect("mac", []string{"ANTIGRAVITY"})
	d.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "CODE", RequiredAgent: "ANTIGRAVITY"})
	a, _ := d.RequestJob("mac", "mac/antigravity")
	return db, logs.NewStore(db, time.Now, keep), a.ID
}

func TestAppendStoresWithTask(t *testing.T) {
	db, s, aid := setup(t, 5000)
	if err := s.Append("mac", aid, "\x1b[32mok\x1b[0m\n"); err != nil {
		t.Fatal(err)
	}
	var task, worker, chunk, asg string
	db.QueryRow(`SELECT task_id, worker_id, assignment_id, log_chunk FROM task_logs`).Scan(&task, &worker, &asg, &chunk)
	if task != "t1" || worker != "mac" || asg != aid || chunk != "\x1b[32mok\x1b[0m\n" {
		t.Fatalf("task_logs = %s %s %s %q", task, worker, asg, chunk)
	}
	if err := s.Append("mac", "khong-co", "x"); err == nil {
		t.Fatal("assignment không tồn tại phải lỗi")
	}
	if err := s.Append("mac", aid, strings.Repeat("a", 70*1024)); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow(`SELECT MAX(LENGTH(log_chunk)) FROM task_logs`).Scan(&n)
	if n > 64*1024 {
		t.Fatalf("chunk %d byte, phải cắt ≤ 64 KB", n)
	}
}

func TestRetentionPerTask(t *testing.T) {
	db, s, aid := setup(t, 10)
	for i := 0; i < 25; i++ {
		s.Append("mac", aid, "dòng\n")
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM task_logs WHERE task_id='t1'`).Scan(&n)
	if n != 10 {
		t.Fatalf("giữ %d dòng, muốn 10 (giới hạn cấu hình)", n)
	}
}

func TestRecentAndSubscribe(t *testing.T) {
	_, s, aid := setup(t, 5000)
	for i := 0; i < 3; i++ {
		s.Append("mac", aid, "cũ\n")
	}
	ch, cancel := s.Subscribe("mac")
	defer cancel()
	s.Append("mac", aid, "mới\n")
	select {
	case l := <-ch:
		if l.Chunk != "mới\n" || l.AssignmentID != aid || l.TaskID != "t1" || l.ID == 0 {
			t.Fatalf("live = %+v", l)
		}
	case <-time.After(time.Second):
		t.Fatal("không nhận được log sống")
	}
	recent, err := s.Recent("mac", 2)
	if err != nil || len(recent) != 2 || recent[1].Chunk != "mới\n" || recent[0].ID >= recent[1].ID {
		t.Fatalf("Recent = %+v %v (2 dòng mới nhất, thứ tự tăng dần)", recent, err)
	}
	other, _ := s.Subscribe("may-khac")
	s.Append("mac", aid, "x\n")
	select {
	case l := <-other:
		t.Fatalf("máy khác không được nhận log của mac: %+v", l)
	case <-time.After(100 * time.Millisecond):
	}
}

func authz(r *http.Request) (string, bool) {
	role := r.Header.Get("X-Test-Role")
	return role, role != ""
}

func TestStreamHTTP(t *testing.T) {
	db, s, aid := setup(t, 5000)
	s.Append("mac", aid, "lịch sử\n")
	srv := httptest.NewServer(logs.Handler(db, s, authz))
	defer srv.Close()

	get := func(role, path string) *http.Response {
		req, _ := http.NewRequest("GET", srv.URL+path, nil)
		if role != "" {
			req.Header.Set("X-Test-Role", role)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	for role, want := range map[string]int{"": 401, "PARTNER": 403} {
		if res := get(role, "/api/admin/workers/mac/logs/stream"); res.StatusCode != want {
			t.Fatalf("vai %q: %d, muốn %d", role, res.StatusCode, want)
		}
	}
	if res := get("ADMIN", "/api/admin/workers/khong-co/logs/stream"); res.StatusCode != 404 {
		t.Fatalf("máy không tồn tại: %d", res.StatusCode)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/admin/workers/mac/logs/stream", nil)
	req.Header.Set("X-Test-Role", "ADMIN")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("Content-Type = %q", res.Header.Get("Content-Type"))
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	frames := make(chan [2]string, 10)
	go func() {
		var ev, data string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				data = strings.TrimPrefix(line, "data: ")
			case line == "" && ev != "":
				frames <- [2]string{ev, data}
				ev, data = "", ""
			}
		}
	}()
	next := func() [2]string {
		select {
		case f := <-frames:
			return f
		case <-time.After(2 * time.Second):
			t.Fatal("hết giờ chờ frame")
		}
		return [2]string{}
	}
	f := next()
	var hist []map[string]any
	json.Unmarshal([]byte(f[1]), &hist)
	if f[0] != "history" || len(hist) != 1 || hist[0]["chunk"] != "lịch sử\n" || hist[0]["task_id"] != "t1" {
		t.Fatalf("frame đầu = %v", f)
	}
	time.Sleep(50 * time.Millisecond)
	s.Append("mac", aid, "sống\n")
	f = next()
	var live map[string]any
	json.Unmarshal([]byte(f[1]), &live)
	if f[0] != "log" || live["chunk"] != "sống\n" || live["assignment_id"] != aid {
		t.Fatalf("frame sống = %v", f)
	}
}
