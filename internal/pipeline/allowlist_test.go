package pipeline_test

// Test nghiệm thu TASK-041 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/ops-readiness.md §6, BR-56, BR-59.

import (
	"reflect"
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/pipeline"
)

func TestDefaultAllowedCommands(t *testing.T) {
	want := []string{"git", "gh pr create", "gh pr view", "ls", "cat", "grep", "find", "head", "tail", "wc"}
	if got := pipeline.DefaultAllowedCommands(); !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultAllowedCommands = %v, muốn %v", got, want)
	}
}

func TestPayloadForMergesProjectCommands(t *testing.T) {
	r := newRig(t)
	r.exec(`UPDATE projects SET allowed_commands = '["go test","git","gofmt"]', needs_ci_for_integration = 1 WHERE id = 'p1'`)
	jobID, err := r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: dispatcher.JobCode, RequiredAgent: dispatcher.KindAntigravity})
	if err != nil {
		t.Fatal(err)
	}
	got := r.p.PayloadFor(dispatcher.Assignment{JobID: jobID, TaskID: "t1"})
	cmds, _ := got["allowed_commands"].([]string)
	want := append(pipeline.DefaultAllowedCommands(), "go test", "gofmt")
	if !reflect.DeepEqual(cmds, want) {
		t.Fatalf("allowed_commands = %v, muốn %v (mặc định trước, bỏ trùng git)", cmds, want)
	}
	if got["needs_ci"] != true {
		t.Fatalf("needs_ci = %v", got["needs_ci"])
	}
	r.exec(`UPDATE projects SET allowed_commands = NULL, needs_ci_for_integration = 0 WHERE id = 'p1'`)
	got = r.p.PayloadFor(dispatcher.Assignment{JobID: jobID, TaskID: "t1"})
	if cmds, _ := got["allowed_commands"].([]string); !reflect.DeepEqual(cmds, pipeline.DefaultAllowedCommands()) || got["needs_ci"] != false {
		t.Fatalf("không khai thì chỉ có mặc định, needs_ci=false: %v", got)
	}
}
