package gateway_test

// Test nghiệm thu TASK-049 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-accounts.md §6 (WS server→runner AGENT_REMOVE {agent_id}); BR-69.

import (
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
)

func TestAgentRemovedRoutesToWorker(t *testing.T) {
	r := newRig(t)
	a := r.join("tok-a", "CLAUDE")
	b := r.join("tok-b", "CLAUDE")

	r.hub.Notify(dispatcher.Event{Type: dispatcher.EvAgentRemoved, WorkerID: "b", AgentID: "b/claude-2"})
	m := recv(t, b)
	if m["event"] != "AGENT_REMOVE" || m["agent_id"] != "b/claude-2" {
		t.Fatalf("b phải nhận AGENT_REMOVE b/claude-2, có %v", m)
	}
	silent(t, a)
}
