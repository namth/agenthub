package runner_test

// Test nghiệm thu TASK-041 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/ops-readiness.md BR-56, BR-59, DEC-34; TC-83, TC-84.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func assignWith(id, agent, kind, task string, extras map[string]any) map[string]any {
	m := assign(id, agent, kind, task, "feat/task-"+task, "")
	p := m["payload"].(map[string]any)
	for k, v := range extras {
		p[k] = v
	}
	return m
}

func waitRun(t *testing.T, ex *fakeExec, bin string) run {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ex.mu.Lock()
		for _, r := range ex.runs {
			if len(r.argv) > 0 && r.argv[0] == bin {
				ex.mu.Unlock()
				return r
			}
		}
		ex.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("CLI %s không được gọi", bin)
	return run{}
}

func noBypass(t *testing.T, argv []string) {
	t.Helper()
	j := strings.Join(argv, " ")
	for _, bad := range []string{"dangerously", "bypassPermissions", `"*"`} {
		if strings.Contains(j, bad) {
			t.Fatalf("không được bỏ qua quyền (%s): %v", bad, argv)
		}
	}
}

func TestClaudeGetsPermissionFlags(t *testing.T) { // TC-83
	r := start(t, nil, nil)
	r.gw.handshake("mac")
	r.gw.send(assignWith("asg_1", "mac/claude", "SPEC", "1", map[string]any{"allowed_commands": []string{"git", "gh pr create", "go test"}}))
	got := waitRun(t, r.exec, "claude").argv
	noBypass(t, got)
	idx := func(s string) int {
		for i, a := range got {
			if a == s {
				return i
			}
		}
		return -1
	}
	pm := idx("--permission-mode")
	if pm < 0 || pm+1 >= len(got) || got[pm+1] != "acceptEdits" {
		t.Fatalf("thiếu --permission-mode acceptEdits: %v", got)
	}
	at := idx("--allowedTools")
	if at < 0 {
		t.Fatalf("thiếu --allowedTools: %v", got)
	}
	tools := map[string]bool{}
	for _, a := range got[at+1:] {
		if strings.HasPrefix(a, "--") {
			break
		}
		tools[a] = true
	}
	for _, want := range []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash(git:*)", "Bash(gh pr create:*)", "Bash(go test:*)"} {
		if !tools[want] {
			t.Fatalf("--allowedTools thiếu %s: %v", want, got)
		}
	}
	if got[1] != "-p" || !strings.Contains(got[2], "Làm task 1") {
		t.Fatalf("giữ nguyên -p <prompt> ở đầu: %v", got)
	}
}

func TestAgyGetsSessionSettings(t *testing.T) { // TC-84
	r := start(t, nil, nil)
	r.gw.handshake("mac")
	r.gw.send(assignWith("asg_2", "mac/antigravity", "CODE", "2", map[string]any{"allowed_commands": []string{"git", "go test", "gofmt", "go vet"}}))
	got := waitRun(t, r.exec, "agy").argv
	noBypass(t, got)
	i := -1
	for k, a := range got {
		if a == "--settings" {
			i = k
		}
	}
	if i < 0 || i+1 >= len(got) {
		t.Fatalf("thiếu --settings <JSON>: %v", got)
	}
	var s struct {
		Permissions struct {
			Allow []string `json:"allow"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(got[i+1]), &s); err != nil {
		t.Fatalf("--settings không phải JSON: %q", got[i+1])
	}
	set := map[string]int{}
	for _, a := range s.Permissions.Allow {
		set[a]++
	}
	for _, want := range []string{"command(git)", "command(go)", "command(gofmt)"} {
		if set[want] != 1 {
			t.Fatalf("allow phải có đúng một %s: %v", want, s.Permissions.Allow)
		}
	}
	if set["*"] > 0 {
		t.Fatal("không được allow *")
	}
}

func TestNoAllowlistKeepsOldArgv(t *testing.T) {
	r := start(t, nil, nil)
	r.gw.handshake("mac")
	r.gw.send(assign("asg_3", "mac/claude", "SPEC", "3", "", ""))
	got := waitRun(t, r.exec, "claude").argv
	if len(got) != 3 || got[0] != "claude" || got[1] != "-p" {
		t.Fatalf("payload không có allowed_commands → argv giữ nguyên [claude -p <prompt>], có %v", got)
	}
}

func TestNeedsCINoteInPrompt(t *testing.T) { // BR-59
	r := start(t, nil, nil)
	r.gw.handshake("mac")
	r.gw.send(assignWith("asg_4", "mac/antigravity", "CODE", "4", map[string]any{"allowed_commands": []string{}, "needs_ci": true}))
	got := waitRun(t, r.exec, "agy").argv
	if !strings.Contains(strings.Join(got, " "), "không chạy test cần secret") {
		t.Fatalf("needs_ci phải thêm ghi chú vào prompt: %v", got)
	}
}
