package office_test

// Test nghiệm thu TASK-033 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/virtual-office.md bản 2 §2, §3, §5–§7 (BR-23, BR-44, BR-45); TC-30, TC-68 → TC-70.

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"database/sql"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/office"
)

type mclock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *mclock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *mclock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func seedUsers(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "v2.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, avatar_url, role) VALUES ('u-owner', '1', 'namth', 'https://a/1', 'ADMIN')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-c', '4', 'cuong', 'PARTNER')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-a', '2', 'an', 'PARTNER')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-b', '3', 'binh', 'PARTNER')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func isOwner(id string) bool { return id == "u-owner" }

type v2rig struct {
	t   *testing.T
	db  *sql.DB
	bus *office.Bus
	clk *mclock
	p   *office.Presence
	srv *httptest.Server
}

func newV2Rig(t *testing.T, opt office.Options) *v2rig {
	t.Helper()
	db := seedUsers(t)
	clk := &mclock{t: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)}
	bus := office.NewBus(400, clk.now, ict)
	p := office.NewPresence(db, clk.now, bus, isOwner)
	snap := func() (any, error) { return office.BuildSnapshot(db, dispatcher.Lease{State: "VACANT"}, p) }
	opt.PingEvery = 50 * time.Millisecond
	opt.User = func(r *http.Request) (string, bool) { u := r.Header.Get("X-Test-User"); return u, u != "" }
	opt.Presence = p
	h := office.Handler(db, bus, snap, authz, opt)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	r := &v2rig{t: t, db: db, bus: bus, clk: clk, p: p, srv: srv}
	if _, err := db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES ('office_enabled', 'true')`); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *v2rig) open(path, role, user string) (*sse, int) {
	r.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", r.srv.URL+path, nil)
	if role != "" {
		req.Header.Set("X-Test-Role", role)
	}
	if user != "" {
		req.Header.Set("X-Test-User", user)
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
	s := &sse{res: res, stop: cancel}
	s.sc = newScanner(res)
	r.t.Cleanup(func() { cancel(); res.Body.Close() })
	return s, 200
}

func TestPartnerStreamsNewPath(t *testing.T) { // TC-30 (sửa)
	r := newV2Rig(t, office.Options{})
	if _, code := r.open("/api/office/stream", "", ""); code != 401 {
		t.Fatalf("chưa đăng nhập: %d, muốn 401", code)
	}
	s, code := r.open("/api/office/stream", "PARTNER", "u-a")
	if code != 200 {
		t.Fatalf("partner mở stream: %d, muốn 200", code)
	}
	f, ok := s.next(t)
	if !ok || f.event != "snapshot" {
		t.Fatalf("frame đầu = %+v", f)
	}
	var snap struct {
		Users []office.UserView `json:"users"`
	}
	if err := json.Unmarshal([]byte(f.data), &snap); err != nil || len(snap.Users) != 4 {
		t.Fatalf("snapshot.users = %s (%v)", f.data, err)
	}
}

func TestOldPathRedirects(t *testing.T) {
	r := newV2Rig(t, office.Options{})
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest("GET", r.srv.URL+"/api/admin/office/stream", nil)
	req.Header.Set("X-Test-Role", "ADMIN")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusPermanentRedirect || res.Header.Get("Location") != "/api/office/stream" {
		t.Fatalf("đường cũ = %d Location=%q, muốn 308 → /api/office/stream", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestPerUserAndTotalLimits(t *testing.T) { // TC-70, BR-23
	r := newV2Rig(t, office.Options{MaxPerUser: 3, MaxStreams: 4})
	for i := 0; i < 3; i++ {
		if _, code := r.open("/api/office/stream", "PARTNER", "u-a"); code != 200 {
			t.Fatalf("u-a kết nối %d: %d", i+1, code)
		}
	}
	if _, code := r.open("/api/office/stream", "PARTNER", "u-a"); code != 429 {
		t.Fatalf("u-a kết nối thứ 4: %d, muốn 429", code)
	}
	if _, code := r.open("/api/office/stream", "PARTNER", "u-b"); code != 200 {
		t.Fatalf("u-b kết nối đầu: %d, muốn 200", code)
	}
	if _, code := r.open("/api/office/stream", "ADMIN", "u-owner"); code != 429 {
		t.Fatalf("vượt tổng 4: %d, muốn 429", code)
	}
}

func TestDefaultLimits(t *testing.T) {
	r := newV2Rig(t, office.Options{})
	for i := 0; i < 3; i++ {
		if _, code := r.open("/api/office/stream", "PARTNER", "u-c"); code != 200 {
			t.Fatalf("kết nối %d: %d", i+1, code)
		}
	}
	if _, code := r.open("/api/office/stream", "PARTNER", "u-c"); code != 429 {
		t.Fatalf("mặc định 3 kết nối/người: lần 4 = %d", code)
	}
}

func TestSettingsTelegramEnabled(t *testing.T) {
	r := newV2Rig(t, office.Options{TelegramEnabled: true})
	req, _ := http.NewRequest("GET", r.srv.URL+"/api/admin/settings", nil)
	req.Header.Set("X-Test-Role", "ADMIN")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var m struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(res.Body).Decode(&m)
	if m.Data["telegram_enabled"] != true || m.Data["office_enabled"] != true {
		t.Fatalf("settings = %v", m.Data)
	}
}

func TestAuthEventMapping(t *testing.T) { // TC-28 (bản 2), BR-45
	b := newBus()
	b.Notify(dispatcher.Event{Type: dispatcher.EvAgentAuth, WorkerID: "mac", AgentID: "mac/claude", PrevAuthStatus: "OK", AuthStatus: "NEEDS_AUTH"})
	if e := last(t, b); e.Who != "mac/claude" || e.Type != "auth" || e.Text != "NEEDS_AUTH" {
		t.Fatalf("vào NEEDS_AUTH = %+v", e)
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvAgentAuth, WorkerID: "mac", AgentID: "mac/claude", PrevAuthStatus: "NEEDS_AUTH", AuthStatus: "OK"})
	if e := last(t, b); e.Type != "auth" || e.Text != "OK" {
		t.Fatalf("ra NEEDS_AUTH = %+v", e)
	}
	n, _ := b.Since(0)
	b.Notify(dispatcher.Event{Type: dispatcher.EvAgentAuth, WorkerID: "mac", AgentID: "mac/claude", PrevAuthStatus: "UNKNOWN", AuthStatus: "OK"})
	if m, _ := b.Since(0); len(m) != len(n) {
		t.Fatal("UNKNOWN → OK không được vẽ")
	}
	b.Notify(dispatcher.Event{Type: dispatcher.EvWorkerAuth, WorkerID: "mac", PrevAuthStatus: "UNKNOWN", AuthStatus: "NEEDS_AUTH"})
	if e := last(t, b); e.Who != "mac" || e.Type != "auth" || e.Text != "NEEDS_AUTH" {
		t.Fatalf("gh máy = %+v", e)
	}
}

func TestPresenceTouchThrottled(t *testing.T) { // TC-69
	db := seedUsers(t)
	clk := &mclock{t: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)}
	p := office.NewPresence(db, clk.now, office.NewBus(400, clk.now, ict), isOwner)
	seen := func() string {
		var s sql.NullString
		db.QueryRow(`SELECT last_seen_at FROM users WHERE id='u-a'`).Scan(&s)
		return s.String
	}
	p.Touch("u-a")
	first := seen()
	if first == "" {
		t.Fatal("Touch đầu tiên phải ghi last_seen_at")
	}
	for i := 0; i < 10; i++ {
		clk.add(3 * time.Second)
		p.Touch("u-a")
	}
	if seen() != first {
		t.Fatalf("trong 30 giây chỉ ghi 1 lần: %q → %q", first, seen())
	}
	clk.add(31 * time.Second)
	p.Touch("u-a")
	if seen() == first {
		t.Fatal("quá 60 giây phải ghi lại")
	}
}

func TestPresenceOrderingAndEvents(t *testing.T) { // TC-68, BR-44
	db := seedUsers(t)
	clk := &mclock{t: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)}
	bus := office.NewBus(400, clk.now, ict)
	p := office.NewPresence(db, clk.now, bus, isOwner)
	p.Touch("u-c")
	p.StreamOpened("u-b")
	p.Tick()
	us, err := p.Users()
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, u := range us {
		got = append(got, u.Login)
	}
	want := []string{"namth", "binh", "cuong", "an"} // owner → online (binh, cuong) → vắng (an)
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("thứ tự = %v, muốn %v", got, want)
		}
	}
	if !us[0].IsOwner || us[0].Online || us[0].AvatarURL != "https://a/1" || !us[1].Online || us[3].Online {
		t.Fatalf("users = %+v", us)
	}
	evs, _ := bus.Since(0)
	online := map[string]bool{}
	for _, e := range evs {
		if e.Type == "presence" && e.Text == "online" {
			online[e.Who] = true
		}
	}
	if !online["user:binh"] || !online["user:cuong"] || online["user:an"] {
		t.Fatalf("sự kiện presence online = %v", online)
	}
	before := len(evs)
	p.Tick() // không đổi → không phát
	if evs, _ = bus.Since(0); len(evs) != before {
		t.Fatal("Tick không đổi trạng thái không được phát sự kiện")
	}
	clk.add(6 * time.Minute)
	p.Tick()
	evs, _ = bus.Since(0)
	offline := map[string]bool{}
	for _, e := range evs[before:] {
		if e.Type == "presence" && e.Text == "offline" {
			offline[e.Who] = true
		}
	}
	if !offline["user:cuong"] || offline["user:binh"] {
		t.Fatalf("sau 6 phút: cuong vắng, binh còn stream mở → online; có offline=%v", offline)
	}
	p.StreamClosed("u-b")
	if p.Online("u-b") {
		t.Fatal("đóng stream và last_seen cũ → vắng")
	}
}

func TestPresenceMiddleware(t *testing.T) {
	db := seedUsers(t)
	clk := &mclock{t: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)}
	p := office.NewPresence(db, clk.now, office.NewBus(400, clk.now, ict), isOwner)
	called := false
	h := p.Middleware(func(r *http.Request) (string, bool) { return "u-a", true })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/tasks", nil))
	if !called || !p.Online("u-a") {
		t.Fatalf("middleware phải chuyển tiếp và Touch: called=%v online=%v", called, p.Online("u-a"))
	}
}

func TestBuildSnapshot(t *testing.T) {
	db := seedUsers(t)
	for _, q := range []string{
		`INSERT INTO workers (id, name, os, token_hash, conn_status, gh_auth, owner_user_id) VALUES ('mac', 'mac', 'darwin', 'h', 'ONLINE', 'NEEDS_AUTH', 'u-a')`,
		`INSERT INTO workers (id, name, os, token_hash) VALUES ('box', 'box', 'linux', 'h')`,
		`INSERT INTO agents (id, worker_id, kind, status, auth_status) VALUES ('mac/claude', 'mac', 'CLAUDE', 'IDLE', 'NEEDS_AUTH')`,
		`INSERT INTO agents (id, worker_id, kind, status, cooldown_until) VALUES ('box/antigravity', 'box', 'ANTIGRAVITY', 'COOLDOWN', '2026-11-04 10:00:00')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	clk := &mclock{t: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)}
	p := office.NewPresence(db, clk.now, office.NewBus(400, clk.now, ict), isOwner)
	m, err := office.BuildSnapshot(db, dispatcher.Lease{State: "HELD", AgentID: "mac/claude", Epoch: 4}, p)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(m)
	var s struct {
		Lead struct {
			State   string  `json:"state"`
			AgentID *string `json:"agent_id"`
			Epoch   int64   `json:"epoch"`
		} `json:"lead"`
		Workers []struct {
			ID         string  `json:"id"`
			ConnStatus string  `json:"conn_status"`
			GhAuth     string  `json:"gh_auth"`
			Owner      *string `json:"owner"`
			Agents     []struct {
				ID            string  `json:"id"`
				Kind          string  `json:"kind"`
				Status        string  `json:"status"`
				AuthStatus    string  `json:"auth_status"`
				CooldownUntil *string `json:"cooldown_until"`
			} `json:"agents"`
		} `json:"workers"`
		Users []office.UserView `json:"users"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.Lead.State != "HELD" || s.Lead.AgentID == nil || *s.Lead.AgentID != "mac/claude" || s.Lead.Epoch != 4 {
		t.Fatalf("lead = %+v", s.Lead)
	}
	if len(s.Workers) != 2 || s.Workers[0].ID != "box" || s.Workers[1].ID != "mac" {
		t.Fatalf("workers (sắp theo id) = %s", b)
	}
	mac, box := s.Workers[1], s.Workers[0]
	if mac.GhAuth != "NEEDS_AUTH" || mac.Owner == nil || *mac.Owner != "an" || box.Owner != nil || box.GhAuth != "UNKNOWN" {
		t.Fatalf("worker = %+v / %+v", mac, box)
	}
	if mac.Agents[0].AuthStatus != "NEEDS_AUTH" || box.Agents[0].AuthStatus != "UNKNOWN" {
		t.Fatalf("auth_status agent = %s", b)
	}
	if box.Agents[0].CooldownUntil == nil || *box.Agents[0].CooldownUntil != "2026-11-04T10:00:00Z" {
		t.Fatalf("cooldown_until = %v", box.Agents[0].CooldownUntil)
	}
	if len(s.Users) != 4 || s.Users[0].Login != "namth" {
		t.Fatalf("users = %+v", s.Users)
	}
	vacant, _ := office.BuildSnapshot(db, dispatcher.Lease{State: "VACANT"}, nil)
	vb, _ := json.Marshal(vacant)
	var v struct {
		Lead  map[string]any `json:"lead"`
		Users []any          `json:"users"`
	}
	json.Unmarshal(vb, &v)
	if v.Lead["agent_id"] != nil || v.Users == nil || len(v.Users) != 0 {
		t.Fatalf("VACANT: agent_id null, users rỗng (mảng); có %s", vb)
	}
}

func newScanner(res *http.Response) *bufio.Scanner { return bufio.NewScanner(res.Body) }

func TestOfficeStatusForAnyUser(t *testing.T) {
	r := newV2Rig(t, office.Options{})
	get := func(role string) (int, map[string]any) {
		req, _ := http.NewRequest("GET", r.srv.URL+"/api/office/status", nil)
		if role != "" {
			req.Header.Set("X-Test-Role", role)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m struct {
			Data map[string]any `json:"data"`
		}
		json.NewDecoder(res.Body).Decode(&m)
		return res.StatusCode, m.Data
	}
	if code, _ := get(""); code != 401 {
		t.Fatalf("chưa đăng nhập: %d", code)
	}
	if code, d := get("PARTNER"); code != 200 || d["enabled"] != true {
		t.Fatalf("partner: %d %v, muốn 200 enabled=true", code, d)
	}
	r.db.Exec(`UPDATE settings SET value='false' WHERE key='office_enabled'`)
	if code, d := get("PARTNER"); code != 200 || d["enabled"] != false {
		t.Fatalf("tắt: %d %v", code, d)
	}
}
