package office_test

// Test nghiệm thu TASK-010 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/virtual-office.md §2–8 (BR-19 → BR-23, TC-28 → TC-32).

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"database/sql"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/office"
)

var ict = time.FixedZone("ICT", 7*3600)

func fixedNow() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC) }

func newBus() *office.Bus { return office.NewBus(400, fixedNow, ict) }

func last(t *testing.T, b *office.Bus) office.Event {
	t.Helper()
	evs, _ := b.Since(0)
	if len(evs) == 0 {
		t.Fatal("bus rỗng")
	}
	return evs[len(evs)-1]
}

// ---------- chuyển sự kiện (TC-28) ----------

func TestMapping(t *testing.T) {
	b := newBus()
	until := time.Date(2026, 11, 4, 14, 40, 0, 0, time.UTC)

	b.Notify(dispatcher.Event{Type: dispatcher.EvLeaseGranted, AgentID: "mac/claude", Epoch: 1})
	if e := last(t, b); e.Who != "system" || e.Type != "handover" || e.To != "mac/claude" {
		t.Fatalf("trao lead lần đầu = %+v", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvJobAssigned, AgentID: "mac/antigravity", TaskID: "t1", Reason: "CODE"})
	if e := last(t, b); e.Who != "mac/claude" || e.Type != "assign" || e.To != "mac/antigravity" || e.Task != "t1" {
		t.Fatalf("giao việc phải do lead hiện tại mang tới: %+v", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvJobSubmitted, AgentID: "mac/antigravity", JobID: "j1"})
	if e := last(t, b); e.Who != "mac/antigravity" || e.Type != "submit" {
		t.Fatalf("nộp = %+v", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvAgentCooldown, AgentID: "mac/claude", Until: &until, Reason: "EXHAUSTED"})
	if e := last(t, b); e.Who != "mac/claude" || e.Type != "cooldown" || !strings.Contains(e.Text, "21:40") {
		t.Fatalf("nghỉ = %+v (giờ theo múi giờ cấu hình)", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvLeaseRevoked, AgentID: "mac/claude", Epoch: 1, Reason: "EXHAUSTED"})
	b.Notify(dispatcher.Event{Type: dispatcher.EvLeaseGranted, AgentID: "lap/claude", Epoch: 2})
	if e := last(t, b); e.Who != "mac/claude" || e.Type != "handover" || e.To != "lap/claude" || !strings.Contains(e.Text, "epoch 2") {
		t.Fatalf("chuyển giao = %+v", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvLeadVacant, Until: &until})
	if e := last(t, b); e.Who != "system" || e.Type != "vacant" || !strings.Contains(e.Text, "21:40") {
		t.Fatalf("trống lead = %+v", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvJobAssigned, AgentID: "x/antigravity", TaskID: "t2"})
	if e := last(t, b); e.Who != "system" {
		t.Fatalf("trống lead thì việc giao từ khay (system): %+v", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvAgentIdle, AgentID: "mac/claude"})
	if e := last(t, b); e.Who != "mac/claude" || e.Type != "idle" {
		t.Fatalf("hết nghỉ = %+v", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvWorkerOffline, WorkerID: "server"})
	if e := last(t, b); e.Who != "server" || e.Type != "offline" {
		t.Fatalf("offline = %+v", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvWorkerOnline, WorkerID: "server"})
	if e := last(t, b); e.Type != "online" {
		t.Fatalf("online = %+v", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvJobFailed, TaskID: "t9"})
	if e := last(t, b); e.Type != "error" || e.Task != "t9" {
		t.Fatalf("thất bại = %+v", e)
	}
	n := len(mustSince(b, 0))
	b.Notify(dispatcher.Event{Type: dispatcher.EvJobAvailable})
	b.Notify(dispatcher.Event{Type: dispatcher.EvJobReclaimed})
	if len(mustSince(b, 0)) != n {
		t.Fatal("JOB_AVAILABLE/RECLAIMED không vẽ lên văn phòng")
	}
}

func mustSince(b *office.Bus, id int64) []office.Event {
	evs, _ := b.Since(id)
	return evs
}

func TestRedactAndTruncate(t *testing.T) { // TC-29, BR-20
	b := newBus()
	b.Emit(office.Event{Who: "admin", Type: "approve", Text: "token ghp_abcdefghijklmnopqrstuvwxyz0123456789 và sk-ant-xyz123456 rồi " + strings.Repeat("ạ", 200)})
	e := last(t, b)
	if strings.Contains(e.Text, "ghp_") || strings.Contains(e.Text, "sk-ant") {
		t.Fatalf("chưa che bí mật: %q", e.Text)
	}
	if n := len([]rune(e.Text)); n > 80 {
		t.Fatalf("text %d ký tự, tối đa 80", n)
	}
	b.Emit(office.Event{Who: "admin", Type: "approve", Text: "khoá ABCDEF0123456789abcdef0123456789XYZ cuối"})
	if strings.Contains(last(t, b).Text, "ABCDEF0123456789abcdef0123456789XYZ") {
		t.Fatal("chuỗi liên tục ≥ 32 ký tự phải bị che")
	}
}

func TestRingBufferAndIDs(t *testing.T) {
	b := office.NewBus(5, fixedNow, ict)
	for i := 0; i < 8; i++ {
		b.Emit(office.Event{Who: "admin", Type: "approve", Text: "x"})
	}
	evs, ok := b.Since(0)
	if len(evs) != 5 || evs[0].ID != 4 || evs[4].ID != 8 {
		t.Fatalf("bộ đệm vòng giữ 5 sự kiện mới nhất, id tăng dần: %+v", evs)
	}
	if ok {
		t.Fatal("Since(0) khi đã mất sự kiện cũ phải báo ok=false")
	}
	if evs, ok := b.Since(6); !ok || len(evs) != 2 || evs[0].ID != 7 {
		t.Fatalf("Since(6) = %+v %v", evs, ok)
	}
	if evs[0].TS.IsZero() {
		t.Fatal("sự kiện phải có ts")
	}
}

// ---------- HTTP ----------

type rig struct {
	t   *testing.T
	db  *sql.DB
	bus *office.Bus
	srv *httptest.Server
}

func authz(r *http.Request) (string, bool) {
	role := r.Header.Get("X-Test-Role")
	return role, role != ""
}

func newRig(t *testing.T) *rig {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "o.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	bus := newBus()
	snap := func() (any, error) {
		return map[string]any{"lead": map[string]any{"state": "VACANT"}, "workers": []any{}}, nil
	}
	h := office.Handler(db, bus, snap, authz, office.Options{PingEvery: 50 * time.Millisecond, MaxStreams: 5})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &rig{t: t, db: db, bus: bus, srv: srv}
}

func (r *rig) req(method, path, role string, body any) (*http.Response, map[string]any) {
	r.t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, r.srv.URL+path, bytes.NewReader(b))
	if role != "" {
		req.Header.Set("X-Test-Role", role)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	json.NewDecoder(res.Body).Decode(&m)
	return res, m
}

func (r *rig) enable(on bool) {
	r.t.Helper()
	if res, m := r.req("PATCH", "/api/admin/settings", "ADMIN", map[string]any{"office_enabled": on}); res.StatusCode != 200 {
		r.t.Fatalf("PATCH settings = %d %v", res.StatusCode, m)
	}
}

type sse struct {
	res  *http.Response
	sc   *bufio.Scanner
	stop context.CancelFunc
}

type frame struct{ id, event, data string }

func (r *rig) stream(role, lastID string) (*sse, int) {
	r.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", r.srv.URL+"/api/admin/office/stream", nil)
	if role != "" {
		req.Header.Set("X-Test-Role", role)
	}
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		r.t.Fatal(err)
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		cancel()
		return nil, res.StatusCode
	}
	s := &sse{res: res, sc: bufio.NewScanner(res.Body), stop: cancel}
	r.t.Cleanup(func() { cancel(); res.Body.Close() })
	return s, 200
}

// next đọc frame kế tiếp (bỏ qua comment ": ping"); trả ok=false nếu luồng đóng.
func (s *sse) next(t *testing.T) (frame, bool) {
	t.Helper()
	var f frame
	got := make(chan bool, 1)
	go func() {
		for s.sc.Scan() {
			line := s.sc.Text()
			switch {
			case line == "":
				if f.event != "" || f.data != "" {
					got <- true
					return
				}
			case strings.HasPrefix(line, ":"):
			case strings.HasPrefix(line, "id: "):
				f.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				f.event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				f.data += strings.TrimPrefix(line, "data: ")
			}
		}
		got <- false
	}()
	select {
	case ok := <-got:
		return f, ok
	case <-time.After(3 * time.Second):
		t.Fatal("hết giờ chờ frame SSE")
	}
	return f, false
}

func TestRBACAndDisabled(t *testing.T) { // TC-30, TC-31
	r := newRig(t)
	if res, _ := r.req("GET", "/api/admin/settings", "", nil); res.StatusCode != 401 {
		t.Fatalf("chưa đăng nhập: %d", res.StatusCode)
	}
	if res, m := r.req("GET", "/api/admin/settings", "PARTNER", nil); res.StatusCode != 403 || m["error"].(map[string]any)["code"] != "FORBIDDEN" {
		t.Fatalf("partner: %d", res.StatusCode)
	}
	// DEC-27 (TASK-033): partner được mở stream; văn phòng đang tắt → 404.
	if _, code := r.stream("PARTNER", ""); code != 404 {
		t.Fatalf("partner mở stream khi tắt: %d, muốn 404", code)
	}
	res, m := r.req("GET", "/api/admin/settings", "ADMIN", nil)
	if res.StatusCode != 200 || m["data"].(map[string]any)["office_enabled"] != false {
		t.Fatalf("mặc định tắt: %d %v", res.StatusCode, m)
	}
	if _, code := r.stream("ADMIN", ""); code != 404 {
		t.Fatalf("tắt văn phòng: stream = %d, muốn 404", code)
	}
	for _, bad := range []any{map[string]any{"office_enabled": "yes"}, map[string]any{"la_gi": true}, map[string]any{}} {
		if res, _ := r.req("PATCH", "/api/admin/settings", "ADMIN", bad); res.StatusCode != 400 {
			t.Fatalf("PATCH %v: %d, muốn 400", bad, res.StatusCode)
		}
	}
	r.enable(true)
	var v string
	r.db.QueryRow(`SELECT value FROM settings WHERE key='office_enabled'`).Scan(&v)
	if v != "true" {
		t.Fatalf("settings DB = %q", v)
	}
	if _, code := r.stream("ADMIN", ""); code != 200 {
		t.Fatalf("bật rồi: stream = %d", code)
	}
}

func TestStreamSnapshotThenLive(t *testing.T) {
	r := newRig(t)
	r.enable(true)
	r.bus.Emit(office.Event{Who: "admin", Type: "approve", Task: "t0"})
	s, _ := r.stream("ADMIN", "")
	if ct := s.res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}
	f, _ := s.next(t)
	if f.event != "snapshot" {
		t.Fatalf("frame đầu = %+v, muốn snapshot", f)
	}
	var snap map[string]any
	json.Unmarshal([]byte(f.data), &snap)
	if snap["lead"] == nil || snap["workers"] == nil {
		t.Fatalf("snapshot thiếu lead/workers: %s", f.data)
	}
	recent, _ := snap["recent"].([]any)
	if len(recent) != 1 {
		t.Fatalf("snapshot.recent = %v, muốn 1 sự kiện cũ", snap["recent"])
	}
	r.bus.Emit(office.Event{Who: "mac/claude", Type: "work", Task: "t1"})
	f, _ = s.next(t)
	var ev map[string]any
	json.Unmarshal([]byte(f.data), &ev)
	if f.event != "office" || f.id != "2" || ev["who"] != "mac/claude" || ev["type"] != "work" || ev["task"] != "t1" {
		t.Fatalf("frame sống = %+v", f)
	}
}

func TestLastEventIDResumes(t *testing.T) { // TC-32
	r := newRig(t)
	r.enable(true)
	for i := 0; i < 4; i++ {
		r.bus.Emit(office.Event{Who: "admin", Type: "approve"})
	}
	s, _ := r.stream("ADMIN", "2")
	f1, _ := s.next(t)
	f2, _ := s.next(t)
	if f1.event != "office" || f1.id != "3" || f2.id != "4" {
		t.Fatalf("nối lại với Last-Event-ID 2 phải gửi bù 3, 4 (không snapshot): %+v %+v", f1, f2)
	}
}

func TestMaxStreams(t *testing.T) { // BR-23
	r := newRig(t)
	r.enable(true)
	for i := 0; i < 5; i++ {
		if _, code := r.stream("ADMIN", ""); code != 200 {
			t.Fatalf("kết nối %d: %d", i+1, code)
		}
	}
	if _, code := r.stream("ADMIN", ""); code != 429 {
		t.Fatalf("kết nối thứ 6: %d, muốn 429", code)
	}
}

func TestDisableClosesStreams(t *testing.T) { // BR-22
	r := newRig(t)
	r.enable(true)
	s, _ := r.stream("ADMIN", "")
	s.next(t) // snapshot
	r.enable(false)
	if _, ok := s.next(t); ok {
		t.Fatal("tắt văn phòng phải đóng các luồng đang mở")
	}
}
