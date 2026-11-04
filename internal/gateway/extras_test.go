package gateway_test

// Test nghiệm thu TASK-041 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/ops-readiness.md §6 (ASSIGN_TASK.payload.allowed_commands, needs_ci).

import (
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
)

func TestPayloadExtrasMerged(t *testing.T) {
	r := newHookRig(t,
		gateway.WithPrompt(func(a dispatcher.Assignment) string { return "prompt gốc" }),
		gateway.WithPayloadExtras(func(a dispatcher.Assignment) map[string]any {
			return map[string]any{"allowed_commands": []string{"git", "go test"}, "needs_ci": true, "prompt": "GHI ĐÈ", "task_id": "x"}
		}))
	r.api.requestJob = func(w, a string) (*dispatcher.Assignment, error) {
		return &dispatcher.Assignment{ID: "asg_1", TaskID: "t9", AgentID: a, JobKind: "CODE", Attempt: 1}, nil
	}
	c := r.join("tok-a", "ANTIGRAVITY")
	send(t, c, map[string]any{"event": "REQUEST_TASK", "agent_id": "a/antigravity"})
	m := recv(t, c)
	for m["event"] != "ASSIGN_TASK" {
		m = recv(t, c)
	}
	p := m["payload"].(map[string]any)
	if p["prompt"] != "prompt gốc" || p["task_id"] != "t9" {
		t.Fatalf("extras không được ghi đè khoá có sẵn: %v", p)
	}
	cmds, _ := p["allowed_commands"].([]any)
	if len(cmds) != 2 || cmds[1] != "go test" || p["needs_ci"] != true {
		t.Fatalf("payload thiếu extras: %v", p)
	}
}
