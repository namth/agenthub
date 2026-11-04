package gateway_test

// Test nghiệm thu TASK-026 (phần gateway) — do trưởng phòng viết, file bị khoá. DEC-15.

import (
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
)

func TestWithRepoFillsPayload(t *testing.T) {
	r := newHookRig(t, gateway.WithRepo(func(a dispatcher.Assignment) string { return "namth/" + a.TaskID }))
	r.api.requestJob = func(w, a string) (*dispatcher.Assignment, error) {
		return &dispatcher.Assignment{ID: "asg_1", TaskID: "edusystem", AgentID: a, JobKind: "CODE", Attempt: 1}, nil
	}
	c := r.join("tok-a", "ANTIGRAVITY")
	send(t, c, map[string]any{"event": "REQUEST_TASK", "agent_id": "a/antigravity"})
	m := recv(t, c)
	if p, _ := m["payload"].(map[string]any); p["repo"] != "namth/edusystem" {
		t.Fatalf("ASSIGN_TASK.payload.repo = %v", m)
	}
}
