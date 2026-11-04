package gateway_test

// Test nghiệm thu TASK-030 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-auth.md §6 (HANDSHAKE mở rộng, AGENT_AUTH, AUTH_RECHECK), TC-63.

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
)

type fakeAuthAPI struct {
	*fakeAPI
	setAuthErr error
}

var _ dispatcher.AuthAPI = (*fakeAuthAPI)(nil)

func (f *fakeAuthAPI) ConnectAuth(w, gh string, hello []dispatcher.AgentHello) ([]dispatcher.Agent, error) {
	parts := []string{}
	var out []dispatcher.Agent
	for _, h := range hello {
		parts = append(parts, fmt.Sprintf("%s:%s:%s", h.Kind, h.AuthStatus, h.AuthDetail))
		out = append(out, dispatcher.Agent{ID: w + "/" + strings.ToLower(h.Kind), WorkerID: w, Kind: h.Kind, Status: "IDLE", AuthStatus: h.AuthStatus})
	}
	f.rec("ConnectAuth %s gh=%s %s", w, gh, strings.Join(parts, ","))
	return out, nil
}
func (f *fakeAuthAPI) SetAuth(w, a, status, detail string) error {
	f.rec("SetAuth %s %s %s %s", w, a, status, detail)
	return f.setAuthErr
}
func (f *fakeAuthAPI) SetGhAuth(w, status, detail string) error {
	f.rec("SetGhAuth %s %s %s", w, status, detail)
	return nil
}
func (f *fakeAuthAPI) AuthRequired(w, a, aid, detail, note, wip string) error {
	f.rec("AuthRequired %s %s %s %s %s %s", w, a, aid, detail, note, wip)
	return nil
}

func newAuthRig(t *testing.T) (*rig, *fakeAuthAPI) {
	t.Helper()
	api := &fakeAuthAPI{fakeAPI: &fakeAPI{}}
	hub := gateway.New(api, auth)
	ts := httptest.NewServer(hub)
	t.Cleanup(func() { hub.Close(); ts.Close() })
	return &rig{t: t, api: api.fakeAPI, hub: hub, url: "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/agent"}, api
}

func TestHandshakeCarriesAuth(t *testing.T) {
	r, api := newAuthRig(t)
	c, _, err := r.dial("tok-a")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	send(t, c, map[string]any{"event": "HANDSHAKE", "data": map[string]any{
		"os": "darwin", "gh_auth": "OK",
		"agents": []map[string]string{
			{"kind": "CLAUDE", "auth_status": "NEEDS_AUTH", "auth_detail": "claude: loggedIn=false"},
			{"kind": "ANTIGRAVITY"},
		},
	}})
	if m := recv(t, c); m["event"] != "HANDSHAKE_OK" {
		t.Fatalf("%v", m)
	}
	if !api.has("ConnectAuth a gh=OK CLAUDE:NEEDS_AUTH:claude: loggedIn=false,ANTIGRAVITY::") {
		t.Fatal("phải gọi ConnectAuth với trạng thái từ HANDSHAKE")
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	for _, call := range api.calls {
		if strings.HasPrefix(call, "Connect ") {
			t.Fatalf("api có AuthAPI thì không gọi Connect cũ: %v", api.calls)
		}
	}
}

func TestAgentAuthMessageRouting(t *testing.T) {
	r, api := newAuthRig(t)
	c := r.join("tok-a", "CLAUDE", "ANTIGRAVITY")
	send(t, c, map[string]any{"event": "AGENT_AUTH", "scope": "agent", "agent_id": "a/claude", "auth_status": "OK"})
	send(t, c, map[string]any{"event": "AGENT_AUTH", "scope": "gh", "auth_status": "NEEDS_AUTH", "detail": "gh: not logged in"})
	send(t, c, map[string]any{"event": "AGENT_AUTH", "scope": "agent", "agent_id": "a/antigravity", "auth_status": "NEEDS_AUTH",
		"assignment_id": "asg_9", "detail": "please run /login", "handoff_note": "ghi chú", "wip_commit": "abc"})
	eventually(t, func() bool {
		return api.has("SetAuth a a/claude OK ") &&
			api.has("SetGhAuth a NEEDS_AUTH gh: not logged in") &&
			api.has("AuthRequired a a/antigravity asg_9 please run /login ghi chú abc")
	}, "định tuyến AGENT_AUTH tới SetAuth / SetGhAuth / AuthRequired")
}

func TestAgentAuthErrorReported(t *testing.T) {
	r, api := newAuthRig(t)
	api.setAuthErr = dispatcher.ErrInvalidAuth
	c := r.join("tok-a", "CLAUDE")
	send(t, c, map[string]any{"event": "AGENT_AUTH", "scope": "agent", "agent_id": "a/claude", "auth_status": "BAD"})
	m := recv(t, c)
	if m["event"] != "ERROR" || m["code"] != "VALIDATION_ERROR" {
		t.Fatalf("muốn ERROR VALIDATION_ERROR, nhận %v", m)
	}
	api.setAuthErr = dispatcher.ErrAgentNotOwned
	send(t, c, map[string]any{"event": "AGENT_AUTH", "scope": "agent", "agent_id": "b/claude", "auth_status": "OK"})
	m = recv(t, c)
	if m["event"] != "ERROR" || m["code"] != "AGENT_NOT_OWNED" {
		t.Fatalf("muốn ERROR AGENT_NOT_OWNED, nhận %v", m)
	}
}

func TestAgentAuthIgnoredWithoutAuthAPI(t *testing.T) {
	r := newRig(t) // fakeAPI không cài AuthAPI
	c := r.join("tok-a", "CLAUDE")
	send(t, c, map[string]any{"event": "AGENT_AUTH", "scope": "agent", "agent_id": "a/claude", "auth_status": "OK"})
	silent(t, c)
}

func TestSendAuthRecheck(t *testing.T) {
	r, _ := newAuthRig(t)
	c := r.join("tok-a", "CLAUDE", "ANTIGRAVITY")
	if !r.hub.SendAuthRecheck("a", "a/antigravity") {
		t.Fatal("SendAuthRecheck máy đang kết nối = false")
	}
	m := recv(t, c)
	if m["event"] != "AUTH_RECHECK" || m["agent_id"] != "a/antigravity" {
		t.Fatalf("muốn AUTH_RECHECK a/antigravity, nhận %v", m)
	}
	if !r.hub.SendAuthRecheck("a", "") {
		t.Fatal("SendAuthRecheck cả máy = false")
	}
	m = recv(t, c)
	if _, has := m["agent_id"]; m["event"] != "AUTH_RECHECK" || has {
		t.Fatalf("kiểm cả máy: không có agent_id, nhận %v", m)
	}
	if r.hub.SendAuthRecheck("b", "") {
		t.Fatal("máy không kết nối phải trả false")
	}
}
