package gateway_test

// Test nghiệm thu TASK-003 — do trưởng phòng viết, file bị khoá (tests.lock).
// Giao thức: docs/07 §2 và docs/features/lead-rotation.md §6.1–6.2.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
)

// ---------- dispatcher giả ----------

type fakeAPI struct {
	mu    sync.Mutex
	calls []string

	requestJob func(worker, agent string) (*dispatcher.Assignment, error)
	submit     func(worker, aid string) error
	renew      func(worker, agent string, epoch int64) (dispatcher.Lease, error)
	exhausted  func(worker, agent, aid string, resetsAt *time.Time) error
	lastReset  *time.Time
}

func (f *fakeAPI) rec(format string, args ...any) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
	f.mu.Unlock()
}

func (f *fakeAPI) has(call string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c == call {
			return true
		}
	}
	return false
}

func (f *fakeAPI) Connect(w string, kinds []string) ([]dispatcher.Agent, error) {
	f.rec("Connect %s %s", w, strings.Join(kinds, ","))
	var out []dispatcher.Agent
	for _, k := range kinds {
		out = append(out, dispatcher.Agent{ID: w + "/" + strings.ToLower(k), WorkerID: w, Kind: k, Status: "IDLE"})
	}
	return out, nil
}
func (f *fakeAPI) Heartbeat(w string) error  { f.rec("Heartbeat %s", w); return nil }
func (f *fakeAPI) Disconnect(w string) error { f.rec("Disconnect %s", w); return nil }
func (f *fakeAPI) Tick() error               { return nil }
func (f *fakeAPI) EnqueueJob(dispatcher.NewJob) (string, error) {
	return "", nil
}
func (f *fakeAPI) RequestJob(w, a string) (*dispatcher.Assignment, error) {
	f.rec("RequestJob %s %s", w, a)
	if f.requestJob != nil {
		return f.requestJob(w, a)
	}
	return nil, nil
}
func (f *fakeAPI) Log(w, aid string) error { f.rec("Log %s %s", w, aid); return nil }
func (f *fakeAPI) Submit(w, aid string) error {
	f.rec("Submit %s %s", w, aid)
	if f.submit != nil {
		return f.submit(w, aid)
	}
	return nil
}
func (f *fakeAPI) Exhausted(w, a, aid string, resetsAt *time.Time, reason, note, wip string) error {
	f.rec("Exhausted %s %s %s %s %s %s", w, a, aid, reason, note, wip)
	f.mu.Lock()
	f.lastReset = resetsAt
	f.mu.Unlock()
	if f.exhausted != nil {
		return f.exhausted(w, a, aid, resetsAt)
	}
	return nil
}
func (f *fakeAPI) Lead() (dispatcher.Lease, error) { return dispatcher.Lease{State: "VACANT"}, nil }
func (f *fakeAPI) Renew(w, a string, e int64) (dispatcher.Lease, error) {
	f.rec("Renew %s %s %d", w, a, e)
	if f.renew != nil {
		return f.renew(w, a, e)
	}
	return dispatcher.Lease{}, nil
}
func (f *fakeAPI) Release(w, a string, e int64) error { f.rec("Release %s %s %d", w, a, e); return nil }
func (f *fakeAPI) LeadNote(w, a string, e int64, task, entry string) error {
	f.rec("LeadNote %s %s %d %s %s", w, a, e, task, entry)
	return nil
}
func (f *fakeAPI) AdminAssign(string) (dispatcher.Lease, error) { return dispatcher.Lease{}, nil }
func (f *fakeAPI) AdminRevoke() (dispatcher.Lease, error)       { return dispatcher.Lease{}, nil }

var _ dispatcher.API = (*fakeAPI)(nil)

// ---------- hạ tầng ----------

var tokens = map[string]string{"tok-a": "a", "tok-b": "b", "tok-mac": "mac-mini"}

func auth(tok string) (string, error) {
	if w, ok := tokens[tok]; ok {
		return w, nil
	}
	return "", gateway.ErrUnauthorized
}

type rig struct {
	t   *testing.T
	api *fakeAPI
	hub *gateway.Hub
	url string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	api := &fakeAPI{}
	hub := gateway.New(api, auth)
	ts := httptest.NewServer(hub)
	t.Cleanup(func() { hub.Close(); ts.Close() })
	return &rig{t: t, api: api, hub: hub, url: "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/agent"}
}

func (r *rig) dial(token string) (*websocket.Conn, *http.Response, error) {
	h := http.Header{}
	if token != "" {
		h.Set("X-Runner-Token", token)
	}
	return websocket.DefaultDialer.Dial(r.url, h)
}

// join kết nối và bắt tay xong.
func (r *rig) join(token string, kinds ...string) *websocket.Conn {
	r.t.Helper()
	c, _, err := r.dial(token)
	if err != nil {
		r.t.Fatalf("dial: %v", err)
	}
	r.t.Cleanup(func() { c.Close() })
	agents := []map[string]string{}
	for _, k := range kinds {
		agents = append(agents, map[string]string{"kind": k})
	}
	send(r.t, c, map[string]any{"event": "HANDSHAKE", "data": map[string]any{"os": "darwin", "agents": agents}})
	if m := recv(r.t, c); m["event"] != "HANDSHAKE_OK" {
		r.t.Fatalf("muốn HANDSHAKE_OK, nhận %v", m)
	}
	return c
}

func send(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	if err := c.WriteJSON(v); err != nil {
		t.Fatalf("send: %v", err)
	}
}

func recv(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var m map[string]any
	if err := c.ReadJSON(&m); err != nil {
		t.Fatalf("recv: %v", err)
	}
	return m
}

func silent(t *testing.T, c *websocket.Conn) {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	var m map[string]any
	if err := c.ReadJSON(&m); err == nil {
		t.Fatalf("không được nhận gì, nhận %v", m)
	}
}

func eventually(t *testing.T, cond func() bool, what string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("hết giờ chờ: %s", what)
}

// ---------- xác thực & bắt tay ----------

func TestRejectsMissingOrBadToken(t *testing.T) {
	r := newRig(t)
	for _, tok := range []string{"", "sai"} {
		_, res, err := r.dial(tok)
		if err == nil {
			t.Fatalf("token %q: phải bị từ chối", tok)
		}
		if res == nil || res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("token %q: muốn HTTP 401, nhận %v", tok, res)
		}
	}
}

func TestHandshakeConnects(t *testing.T) {
	r := newRig(t)
	c, _, err := r.dial("tok-mac")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	send(t, c, map[string]any{"event": "HANDSHAKE", "data": map[string]any{"os": "darwin",
		"agents": []map[string]string{{"kind": "CLAUDE"}, {"kind": "ANTIGRAVITY"}}}})
	m := recv(t, c)
	if m["event"] != "HANDSHAKE_OK" {
		t.Fatalf("nhận %v", m)
	}
	agents, _ := m["agents"].([]any)
	if len(agents) != 2 || agents[0].(map[string]any)["id"] != "mac-mini/claude" || agents[0].(map[string]any)["status"] != "IDLE" {
		t.Fatalf("HANDSHAKE_OK.agents = %v", m["agents"])
	}
	if !r.api.has("Connect mac-mini CLAUDE,ANTIGRAVITY") {
		t.Fatalf("Connect phải nhận worker từ token và danh sách kind, calls = %v", r.api.calls)
	}
}

func TestMessageBeforeHandshakeRejected(t *testing.T) {
	r := newRig(t)
	c, _, err := r.dial("tok-a")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	send(t, c, map[string]any{"event": "PING", "timestamp": 1})
	if m := recv(t, c); m["event"] != "ERROR" || m["code"] != "NOT_HANDSHAKEN" {
		t.Fatalf("nhận %v, muốn ERROR NOT_HANDSHAKEN", m)
	}
	if r.api.has("Heartbeat a") {
		t.Fatal("chưa bắt tay không được gọi Heartbeat")
	}
}

// ---------- bản tin runner → server ----------

func TestPingPong(t *testing.T) {
	r := newRig(t)
	c := r.join("tok-a", "CLAUDE")
	send(t, c, map[string]any{"event": "PING", "timestamp": 1798982345})
	m := recv(t, c)
	if m["event"] != "PONG" || m["timestamp"] != float64(1798982345) {
		t.Fatalf("nhận %v, muốn PONG cùng timestamp", m)
	}
	if !r.api.has("Heartbeat a") {
		t.Fatal("PING phải gọi Heartbeat")
	}
}

func TestRequestTaskAssignOrNoTask(t *testing.T) {
	r := newRig(t)
	r.api.requestJob = func(w, a string) (*dispatcher.Assignment, error) {
		if a == "a/antigravity" {
			return &dispatcher.Assignment{ID: "asg_1", JobID: "j1", TaskID: "t102", AgentID: a, JobKind: "CODE",
				Attempt: 2, BranchName: "feat/task-102", HandoffNote: "3 files changed"}, nil
		}
		return nil, nil
	}
	c := r.join("tok-a", "CLAUDE", "ANTIGRAVITY")
	send(t, c, map[string]any{"event": "REQUEST_TASK", "agent_id": "a/antigravity"})
	m := recv(t, c)
	p, _ := m["payload"].(map[string]any)
	if m["event"] != "ASSIGN_TASK" || m["assignment_id"] != "asg_1" || m["agent_id"] != "a/antigravity" ||
		m["job_kind"] != "CODE" || m["attempt"] != float64(2) || p == nil ||
		p["task_id"] != "t102" || p["branch_name"] != "feat/task-102" || p["handoff_note"] != "3 files changed" {
		t.Fatalf("ASSIGN_TASK sai: %v", m)
	}
	send(t, c, map[string]any{"event": "REQUEST_TASK", "agent_id": "a/claude"})
	if m := recv(t, c); m["event"] != "NO_TASK" || m["agent_id"] != "a/claude" {
		t.Fatalf("không có job: nhận %v, muốn NO_TASK", m)
	}
	if !r.api.has("RequestJob a a/antigravity") {
		t.Fatalf("RequestJob phải mang worker từ token: %v", r.api.calls)
	}
}

func TestLogAndSubmitForwarded(t *testing.T) {
	r := newRig(t)
	c := r.join("tok-b", "ANTIGRAVITY")
	send(t, c, map[string]any{"event": "LOG_CHUNK", "assignment_id": "asg_9", "chunk": "\u001b[32mok\u001b[0m\n"})
	send(t, c, map[string]any{"event": "TASK_SUBMITTED", "assignment_id": "asg_9", "pr_url": "https://github.com/x/y/pull/1", "branch": "feat/task-9"})
	eventually(t, func() bool { return r.api.has("Log b asg_9") && r.api.has("Submit b asg_9") }, "Log và Submit")
}

func TestExhaustedParsesFields(t *testing.T) {
	r := newRig(t)
	c := r.join("tok-a", "CLAUDE")
	send(t, c, map[string]any{"event": "AGENT_EXHAUSTED", "agent_id": "a/claude", "assignment_id": "asg_2",
		"resets_at": "2026-11-03T21:40:00Z", "reason": "usage limit", "handoff_note": "ghi chú", "wip_commit": "a1b2c3d"})
	eventually(t, func() bool { return r.api.has("Exhausted a a/claude asg_2 usage limit ghi chú a1b2c3d") }, "Exhausted đủ trường")
	r.api.mu.Lock()
	got := r.api.lastReset
	r.api.mu.Unlock()
	if got == nil || !got.Equal(time.Date(2026, 11, 3, 21, 40, 0, 0, time.UTC)) {
		t.Fatalf("resets_at = %v", got)
	}
	send(t, c, map[string]any{"event": "AGENT_EXHAUSTED", "agent_id": "a/claude", "assignment_id": nil, "resets_at": nil, "reason": "rate limit"})
	eventually(t, func() bool { return r.api.has("Exhausted a a/claude  rate limit  ") }, "Exhausted với null")
	r.api.mu.Lock()
	got = r.api.lastReset
	r.api.mu.Unlock()
	if got != nil {
		t.Fatalf("resets_at null phải thành nil, nhận %v", got)
	}
}

func TestLeaseRenewRepliesGranted(t *testing.T) {
	r := newRig(t)
	exp := time.Date(2026, 11, 3, 21, 0, 0, 0, time.UTC)
	r.api.renew = func(w, a string, e int64) (dispatcher.Lease, error) {
		return dispatcher.Lease{State: "HELD", AgentID: a, Epoch: e, ExpiresAt: &exp}, nil
	}
	c := r.join("tok-a", "CLAUDE")
	send(t, c, map[string]any{"event": "LEASE_RENEW", "agent_id": "a/claude", "epoch": 8})
	m := recv(t, c)
	if m["event"] != "LEASE_GRANTED" || m["agent_id"] != "a/claude" || m["epoch"] != float64(8) || m["expires_at"] != "2026-11-03T21:00:00Z" {
		t.Fatalf("nhận %v, muốn LEASE_GRANTED epoch 8 expires_at RFC3339", m)
	}
}

func TestLeadReleaseAndActions(t *testing.T) {
	r := newRig(t)
	c := r.join("tok-a", "CLAUDE")
	send(t, c, map[string]any{"event": "LEAD_RELEASE", "agent_id": "a/claude", "epoch": 4, "reason": "SOFT_LIMIT"})
	send(t, c, map[string]any{"event": "LEAD_ACTION", "agent_id": "a/claude", "epoch": 4, "action": "NOTE",
		"data": map[string]any{"task_id": "t1", "note": "ưu tiên"}})
	eventually(t, func() bool { return r.api.has("Release a a/claude 4") && r.api.has("LeadNote a a/claude 4 t1 ưu tiên") }, "Release và LeadNote")
	send(t, c, map[string]any{"event": "LEAD_ACTION", "agent_id": "a/claude", "epoch": 4, "action": "REPRIORITIZE", "data": map[string]any{}})
	if m := recv(t, c); m["event"] != "ERROR" || m["code"] != "UNSUPPORTED" {
		t.Fatalf("nhận %v, muốn ERROR UNSUPPORTED", m)
	}
}

func TestDomainErrorsMapped(t *testing.T) {
	r := newRig(t)
	r.api.submit = func(w, aid string) error { return fmt.Errorf("submit: %w", dispatcher.ErrStaleAssignment) }
	r.api.renew = func(w, a string, e int64) (dispatcher.Lease, error) { return dispatcher.Lease{}, dispatcher.ErrStaleEpoch }
	r.api.exhausted = func(w, a, aid string, _ *time.Time) error {
		if a == "x/claude" {
			return dispatcher.ErrAgentNotOwned
		}
		return errors.New("disk full")
	}
	c := r.join("tok-a", "CLAUDE")

	send(t, c, map[string]any{"event": "TASK_SUBMITTED", "assignment_id": "asg_old"})
	m := recv(t, c)
	ref, _ := m["ref"].(map[string]any)
	if m["event"] != "ERROR" || m["code"] != "STALE_ASSIGNMENT" || ref == nil || ref["assignment_id"] != "asg_old" {
		t.Fatalf("nhận %v, muốn ERROR STALE_ASSIGNMENT ref.assignment_id", m)
	}
	send(t, c, map[string]any{"event": "LEASE_RENEW", "agent_id": "a/claude", "epoch": 7})
	m = recv(t, c)
	ref, _ = m["ref"].(map[string]any)
	if m["code"] != "STALE_EPOCH" || ref == nil || ref["epoch"] != float64(7) {
		t.Fatalf("nhận %v, muốn ERROR STALE_EPOCH ref.epoch", m)
	}
	send(t, c, map[string]any{"event": "AGENT_EXHAUSTED", "agent_id": "x/claude", "reason": "r"})
	if m := recv(t, c); m["code"] != "AGENT_NOT_OWNED" {
		t.Fatalf("nhận %v, muốn AGENT_NOT_OWNED", m)
	}
	send(t, c, map[string]any{"event": "AGENT_EXHAUSTED", "agent_id": "a/claude", "reason": "r"})
	m = recv(t, c)
	if m["code"] != "INTERNAL" || strings.Contains(fmt.Sprint(m), "disk full") {
		t.Fatalf("lỗi không thuộc nghiệp vụ: nhận %v, muốn INTERNAL và không lộ chi tiết", m)
	}
}

func TestBadMessagesKeepConnection(t *testing.T) {
	r := newRig(t)
	c := r.join("tok-a", "CLAUDE")
	if err := c.WriteMessage(websocket.TextMessage, []byte("{không phải json")); err != nil {
		t.Fatal(err)
	}
	if m := recv(t, c); m["event"] != "ERROR" || m["code"] != "BAD_REQUEST" {
		t.Fatalf("JSON hỏng: nhận %v", m)
	}
	send(t, c, map[string]any{"event": "LAM_GI_DO"})
	if m := recv(t, c); m["event"] != "ERROR" || m["code"] != "BAD_REQUEST" {
		t.Fatalf("event lạ: nhận %v", m)
	}
	send(t, c, map[string]any{"event": "PING", "timestamp": 5})
	if m := recv(t, c); m["event"] != "PONG" {
		t.Fatalf("kết nối phải còn dùng được, nhận %v", m)
	}
}

func TestCloseCallsDisconnect(t *testing.T) {
	r := newRig(t)
	c := r.join("tok-b", "ANTIGRAVITY")
	c.Close()
	eventually(t, func() bool { return r.api.has("Disconnect b") }, "Disconnect khi đóng kết nối")
}

func TestReconnectReplacesOldConnection(t *testing.T) {
	r := newRig(t)
	old := r.join("tok-a", "CLAUDE")
	_ = r.join("tok-a", "CLAUDE")
	old.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := old.ReadMessage(); err == nil {
		t.Fatal("kết nối cũ của cùng máy phải bị đóng")
	}
	if r.api.has("Disconnect a") {
		t.Fatal("thay kết nối cũ không được gọi Disconnect (máy vẫn đang online)")
	}
}

// ---------- sự kiện server → runner ----------

func TestNotifyRoutesToOwningWorker(t *testing.T) {
	r := newRig(t)
	a := r.join("tok-a", "CLAUDE")
	b := r.join("tok-b", "CLAUDE")
	exp := time.Date(2026, 11, 3, 21, 1, 30, 0, time.UTC)

	r.hub.Notify(dispatcher.Event{Type: dispatcher.EvLeaseGranted, AgentID: "b/claude", Epoch: 3, Until: &exp})
	m := recv(t, b)
	if m["event"] != "LEASE_GRANTED" || m["agent_id"] != "b/claude" || m["epoch"] != float64(3) || m["expires_at"] != "2026-11-03T21:01:30Z" {
		t.Fatalf("b nhận %v", m)
	}
	silent(t, a)

	r.hub.Notify(dispatcher.Event{Type: dispatcher.EvLeaseRevoked, AgentID: "a/claude", Epoch: 2, Reason: dispatcher.RevokeExhausted})
	if m := recv(t, a); m["event"] != "LEASE_REVOKED" || m["agent_id"] != "a/claude" || m["epoch"] != float64(2) || m["reason"] != "EXHAUSTED" {
		t.Fatalf("a nhận %v", m)
	}
	silent(t, b)

	r.hub.Notify(dispatcher.Event{Type: dispatcher.EvAbortTask, AgentID: "b/claude", AssignmentID: "asg_5", Reason: "Admin huỷ"})
	if m := recv(t, b); m["event"] != "ABORT_TASK" || m["assignment_id"] != "asg_5" || m["reason"] != "Admin huỷ" {
		t.Fatalf("b nhận %v", m)
	}

	r.hub.Notify(dispatcher.Event{Type: dispatcher.EvJobAvailable})
	for _, c := range []*websocket.Conn{a, b} {
		if m := recv(t, c); m["event"] != "JOB_AVAILABLE" {
			t.Fatalf("JOB_AVAILABLE phải phát cho mọi máy, nhận %v", m)
		}
	}

	// Sự kiện không dành cho runner thì không gửi.
	r.hub.Notify(dispatcher.Event{Type: dispatcher.EvWorkerOffline, WorkerID: "a"})
	silent(t, a)
}

func TestNotifyNeverBlocks(t *testing.T) {
	r := newRig(t)
	_ = r.join("tok-a", "CLAUDE") // không bao giờ đọc
	done := make(chan struct{})
	go func() {
		for i := 0; i < 2000; i++ {
			r.hub.Notify(dispatcher.Event{Type: dispatcher.EvLeaseGranted, AgentID: "a/claude", Epoch: int64(i)})
		}
		r.hub.Notify(dispatcher.Event{Type: dispatcher.EvLeaseGranted, AgentID: "khong-co/claude", Epoch: 1})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Notify bị chặn khi máy không đọc kịp")
	}
}

func TestEnvelopeIsFlatJSON(t *testing.T) {
	r := newRig(t)
	c := r.join("tok-a", "CLAUDE")
	send(t, c, map[string]any{"event": "PING", "timestamp": 1})
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m["event"] == nil {
		t.Fatalf("bản tin phải là object JSON phẳng có trường event: %s", raw)
	}
}
