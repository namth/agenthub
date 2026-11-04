package telegram_test

// Test nghiệm thu TASK-030 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-auth.md §7 BR-41, TC-62.

import (
	"strings"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/dispatcher"
)

func authEv(agent, prev, cur string) dispatcher.Event {
	w := strings.SplitN(agent, "/", 2)[0]
	return dispatcher.Event{Type: dispatcher.EvAgentAuth, WorkerID: w, AgentID: agent, PrevAuthStatus: prev, AuthStatus: cur, Detail: "chi tiết"}
}

func TestAuthNotifyTransitions(t *testing.T) { // TC-62
	s := &fakeSender{}
	n := newNotifier(t, s)
	// Dispatcher chỉ phát khi giá trị đổi: OK→NEEDS, NEEDS→UNKNOWN, UNKNOWN→NEEDS, NEEDS→OK, UNKNOWN→OK, OK→UNKNOWN.
	n.Notify(authEv("iMac-8GB/claude", "OK", "NEEDS_AUTH"))      // gửi (vào)
	n.Notify(authEv("iMac-8GB/claude", "NEEDS_AUTH", "UNKNOWN")) // không
	n.Notify(authEv("iMac-8GB/claude", "UNKNOWN", "NEEDS_AUTH")) // gửi (vào)
	n.Notify(authEv("iMac-8GB/claude", "NEEDS_AUTH", "OK"))      // gửi (ra)
	n.Notify(authEv("laptop/claude", "UNKNOWN", "OK"))           // không
	n.Notify(authEv("laptop/claude", "OK", "UNKNOWN"))           // không
	m := waitMsgs(t, s, 3)
	time.Sleep(80 * time.Millisecond)
	m = s.all()
	if len(m) != 3 {
		t.Fatalf("muốn đúng 3 tin, nhận %d: %+v", len(m), m)
	}
	for i, want := range []string{"🔑", "🔑", "✅"} {
		if !strings.Contains(m[i].text, want) || !strings.Contains(m[i].text, "iMac-8GB/claude") || m[i].chat != "987654321" {
			t.Fatalf("tin %d = %q, muốn chứa %s và tên agent", i, m[i].text, want)
		}
	}
	if !strings.Contains(m[0].text, "/login") {
		t.Fatalf("tin cần đăng nhập Claude phải gợi ý /login: %q", m[0].text)
	}
}

func TestAuthNotifyCommandHints(t *testing.T) {
	s := &fakeSender{}
	n := newNotifier(t, s)
	n.Notify(authEv("mac/antigravity", "UNKNOWN", "NEEDS_AUTH"))
	n.Notify(dispatcher.Event{Type: dispatcher.EvWorkerAuth, WorkerID: "mac", PrevAuthStatus: "OK", AuthStatus: "NEEDS_AUTH", Detail: "gh: not logged in"})
	n.Notify(dispatcher.Event{Type: dispatcher.EvWorkerAuth, WorkerID: "mac", PrevAuthStatus: "NEEDS_AUTH", AuthStatus: "OK"})
	m := waitMsgs(t, s, 3)
	if !strings.Contains(m[0].text, "agy") || !strings.Contains(m[0].text, "mac/antigravity") {
		t.Fatalf("tin agy = %q", m[0].text)
	}
	if !strings.Contains(m[1].text, "gh auth login") || !strings.Contains(m[1].text, "mac") || !strings.Contains(m[1].text, "🔑") {
		t.Fatalf("tin gh = %q", m[1].text)
	}
	if !strings.Contains(m[2].text, "✅") || !strings.Contains(m[2].text, "mac") {
		t.Fatalf("tin gh khôi phục = %q", m[2].text)
	}
}
