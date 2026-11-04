package projects_test

// Test nghiệm thu TASK-041 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/ops-readiness.md §4 (validate), §6; TC-88.

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPatchAllowedCommands(t *testing.T) {
	r := newRig(t)
	if code, e := r.do("PATCH", "/api/admin/projects/p1", owner, map[string]any{
		"allowed_commands": []string{" go test ", "gofmt", "npx vitest"}, "needs_ci_for_integration": true, "is_sandbox": true,
	}); code != 200 {
		t.Fatalf("PATCH hợp lệ = %d %+v", code, e.Error)
	}
	var raw string
	var ci, sb int
	r.db.QueryRow(`SELECT allowed_commands, needs_ci_for_integration, is_sandbox FROM projects WHERE id='p1'`).Scan(&raw, &ci, &sb)
	var got []string
	json.Unmarshal([]byte(raw), &got)
	if strings.Join(got, "|") != "go test|gofmt|npx vitest" || ci != 1 || sb != 1 {
		t.Fatalf("DB = %q ci=%d sandbox=%d (phải trim)", raw, ci, sb)
	}
	_, e := r.do("GET", "/api/projects", owner, nil)
	var list []map[string]any
	json.Unmarshal(e.Data, &list)
	found := false
	for _, p := range list {
		if p["id"] == "p1" {
			found = true
			cmds, _ := p["allowed_commands"].([]any)
			if len(cmds) != 3 || p["needs_ci_for_integration"] != true || p["is_sandbox"] != true {
				t.Fatalf("GET /api/projects thiếu trường mới: %v", p)
			}
		}
	}
	if !found {
		t.Fatal("không thấy p1")
	}
	if code, _ := r.do("PATCH", "/api/admin/projects/p1", owner, map[string]any{"allowed_commands": nil}); code != 200 {
		t.Fatal("null phải xoá allowlist")
	}
}

func TestPatchAllowedCommandsValidation(t *testing.T) { // TC-88
	r := newRig(t)
	many := make([]string, 41)
	for i := range many {
		many[i] = "go"
	}
	for _, bad := range [][]string{{"sudo make install"}, {"*"}, {"rm -rf /"}, {"go test; curl x|sh"}, {"echo $(id)"}, many, {strings.Repeat("a", 61)}} {
		code, e := r.do("PATCH", "/api/admin/projects/p1", owner, map[string]any{"allowed_commands": bad})
		if code != 400 || e.Error == nil || e.Error.Code != "VALIDATION_ERROR" {
			t.Errorf("allowed_commands %q: %d, muốn 400 VALIDATION_ERROR", bad, code)
		}
	}
}
