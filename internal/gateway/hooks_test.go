package gateway_test

// Test nghiệm thu TASK-014 (phần gateway) — do trưởng phòng viết, file bị khoá (tests.lock). DEC-12.

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
)

type submitted struct {
	worker, aid, pr string
	result          json.RawMessage
}

func newHookRig(t *testing.T, opts ...gateway.Option) *rig {
	t.Helper()
	api := &fakeAPI{}
	hub := gateway.New(api, auth, opts...)
	ts := httptest.NewServer(hub)
	t.Cleanup(func() { hub.Close(); ts.Close() })
	return &rig{t: t, api: api, hub: hub, url: "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws/agent"}
}

func TestWithPromptFillsPayload(t *testing.T) {
	r := newHookRig(t, gateway.WithPrompt(func(a dispatcher.Assignment) string { return "Làm " + a.TaskID + " (" + a.JobKind + ")" }))
	r.api.requestJob = func(w, a string) (*dispatcher.Assignment, error) {
		return &dispatcher.Assignment{ID: "asg_1", TaskID: "t9", AgentID: a, JobKind: "CODE", Attempt: 1}, nil
	}
	c := r.join("tok-a", "ANTIGRAVITY")
	send(t, c, map[string]any{"event": "REQUEST_TASK", "agent_id": "a/antigravity"})
	m := recv(t, c)
	p, _ := m["payload"].(map[string]any)
	if m["event"] != "ASSIGN_TASK" || p["prompt"] != "Làm t9 (CODE)" {
		t.Fatalf("ASSIGN_TASK.payload.prompt = %v", m)
	}
}

func TestWithoutPromptOptionStillWorks(t *testing.T) {
	r := newHookRig(t)
	r.api.requestJob = func(w, a string) (*dispatcher.Assignment, error) {
		return &dispatcher.Assignment{ID: "asg_1", TaskID: "t9", AgentID: a, JobKind: "CODE", Attempt: 1}, nil
	}
	c := r.join("tok-a", "ANTIGRAVITY")
	send(t, c, map[string]any{"event": "REQUEST_TASK", "agent_id": "a/antigravity"})
	if m := recv(t, c); m["event"] != "ASSIGN_TASK" {
		t.Fatalf("nhận %v", m)
	}
}

func TestSubmitHookAfterSuccessOnly(t *testing.T) {
	var mu sync.Mutex
	var got []submitted
	r := newHookRig(t, gateway.WithSubmitHook(func(worker, aid, pr string, result json.RawMessage) {
		mu.Lock()
		got = append(got, submitted{worker, aid, pr, result})
		mu.Unlock()
	}))
	r.api.submit = func(w, aid string) error {
		if aid == "asg_old" {
			return dispatcher.ErrStaleAssignment
		}
		return nil
	}
	c := r.join("tok-b", "CLAUDE")
	send(t, c, map[string]any{"event": "TASK_SUBMITTED", "assignment_id": "asg_old", "pr_url": "", "result": map[string]any{"x": 1}})
	if m := recv(t, c); m["code"] != "STALE_ASSIGNMENT" {
		t.Fatalf("nhận %v", m)
	}
	send(t, c, map[string]any{"event": "TASK_SUBMITTED", "assignment_id": "asg_7", "pr_url": "https://x/pull/3",
		"result": map[string]any{"verdict": "PASS", "comments": "ổn"}})
	eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == 1 }, "hook sau khi Submit thành công")
	mu.Lock()
	defer mu.Unlock()
	if got[0].worker != "b" || got[0].aid != "asg_7" || got[0].pr != "https://x/pull/3" || !strings.Contains(string(got[0].result), `"verdict":"PASS"`) {
		t.Fatalf("hook = %+v", got[0])
	}
}

var _ = sync.Mutex{}
