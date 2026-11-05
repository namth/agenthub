package gateway_test

// Test nghiệm thu TASK-029 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/join-machine.md §6 (WS HANDSHAKE.data.version, HANDSHAKE_OK.server_version — DEC-19),
// §7 BR-32 (gỡ máy → gateway đóng kết nối).

import (
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/gateway"
)

func TestHandshakeServerVersion(t *testing.T) {
	r := newHookRig(t, gateway.WithServerVersion("v1.4.0"))
	c, _, err := r.dial("tok-a")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	send(t, c, map[string]any{"event": "HANDSHAKE", "data": map[string]any{"os": "darwin", "version": "v1.3.9", "agents": []map[string]string{{"kind": "CLAUDE"}}}})
	m := recv(t, c)
	if m["event"] != "HANDSHAKE_OK" || m["server_version"] != "v1.4.0" {
		t.Fatalf("HANDSHAKE_OK = %v, muốn server_version v1.4.0", m)
	}
}

func TestHandshakeWithoutServerVersionOption(t *testing.T) {
	r := newRig(t)
	c := r.join("tok-a", "CLAUDE") // join đã kiểm HANDSHAKE_OK
	_ = c
}

func TestVersionSinkReceivesRunnerVersion(t *testing.T) {
	var mu sync.Mutex
	got := map[string]string{}
	r := newHookRig(t, gateway.WithVersionSink(func(workerID, version string) {
		mu.Lock()
		got[workerID] = version
		mu.Unlock()
	}))
	c, _, err := r.dial("tok-b")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	send(t, c, map[string]any{"event": "HANDSHAKE", "data": map[string]any{"os": "linux", "version": "v2.0.1", "agents": []map[string]string{{"kind": "ANTIGRAVITY"}}}})
	if m := recv(t, c); m["event"] != "HANDSHAKE_OK" {
		t.Fatalf("%v", m)
	}
	mu.Lock()
	defer mu.Unlock()
	if got["b"] != "v2.0.1" {
		t.Fatalf("sink = %v, muốn b → v2.0.1 (gọi trước khi gửi HANDSHAKE_OK)", got)
	}
}

func TestVersionSinkSkipsEmptyVersion(t *testing.T) {
	called := false
	r := newHookRig(t, gateway.WithVersionSink(func(string, string) { called = true }))
	r.join("tok-a", "CLAUDE") // HANDSHAKE không có version
	if called {
		t.Fatal("không có version thì không gọi sink")
	}
}

func TestCloseWorker(t *testing.T) {
	r := newRig(t)
	c := r.join("tok-a", "CLAUDE")
	other := r.join("tok-b", "CLAUDE")
	if !r.hub.CloseWorker("a") {
		t.Fatal("CloseWorker(a) = false, muốn true")
	}
	select {
	case <-boxOf(c).closed:
	case <-time.After(2 * time.Second):
		t.Fatal("kết nối của a chưa bị đóng")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !r.api.has("Disconnect a") {
		if time.Now().After(deadline) {
			t.Fatal("chưa gọi Disconnect a")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r.hub.CloseWorker("khong-co") {
		t.Fatal("CloseWorker(không có) = true")
	}
	// Máy khác không bị ảnh hưởng.
	send(t, other, map[string]any{"event": "PING"})
	select {
	case <-boxOf(other).closed:
		t.Fatal("kết nối của b bị đóng nhầm")
	case <-time.After(200 * time.Millisecond):
	}
}
