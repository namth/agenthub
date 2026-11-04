package gateway_test

// Test nghiệm thu TASK-016 (phần gateway) — do trưởng phòng viết, file bị khoá (tests.lock).

import (
	"sync"
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
)

func TestWithLogSink(t *testing.T) {
	var mu sync.Mutex
	var got []string
	r := newHookRig(t, gateway.WithLogSink(func(worker, aid, chunk string) {
		mu.Lock()
		got = append(got, worker+"|"+aid+"|"+chunk)
		mu.Unlock()
	}))
	c := r.join("tok-a", "ANTIGRAVITY")
	send(t, c, map[string]any{"event": "LOG_CHUNK", "assignment_id": "asg_1", "chunk": "xin chào\n"})
	eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == 1 }, "log sink")
	mu.Lock()
	if got[0] != "a|asg_1|xin chào\n" {
		t.Fatalf("sink = %q", got[0])
	}
	mu.Unlock()
	_ = dispatcher.ErrStaleAssignment
}
