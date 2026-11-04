package telegram_test

// Test nghiệm thu TASK-039 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/clarification-and-shared-skills.md §6 (callback clarify:<task>:<round>:<qid>:<idx>).

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/namth/agenthub/internal/telegram"
)

type clarifyCommander struct {
	fakeCommander
	picks []string
	n     int
}

var _ telegram.Clarifier = (*clarifyCommander)(nil)

func (c *clarifyCommander) PickOption(_ context.Context, taskID string, round int, qid string, idx int) (bool, error) {
	if idx > 5 {
		return false, errors.New("VALIDATION_ERROR")
	}
	c.picks = append(c.picks, fmt.Sprintf("%s|%d|%s|%d", taskID, round, qid, idx))
	c.n++
	return c.n >= 2, nil
}

func TestClarifyCallback(t *testing.T) {
	s := &fakeSender{}
	c := &clarifyCommander{fakeCommander: fakeCommander{tasks: map[string]telegram.TaskBrief{}}}
	h := telegram.WebhookHandlerWith(s, &fakeApprover{}, c, "987654321", "hook-secret")
	callback(987654321, "clarify:t-c:2:payment_gateway:1", "hook-secret", h)
	callback(987654321, "clarify:t-c:2:scope:0", "hook-secret", h)
	callback(987654321, "clarify:t-c:2:scope:9", "hook-secret", h)
	callback(111, "clarify:t-c:2:scope:0", "hook-secret", h)
	if strings.Join(c.picks, ",") != "t-c|2|payment_gateway|1,t-c|2|scope|0" {
		t.Fatalf("PickOption = %v (chat lạ phải bị bỏ qua)", c.picks)
	}
	if len(s.acks) != 3 || !strings.Contains(s.acks[0], "Đã ghi") || !strings.Contains(s.acks[1], "Đã gửi cho trưởng phòng") || !strings.Contains(s.acks[2], "Không thực hiện được") {
		t.Fatalf("acks = %v", s.acks)
	}
	// Commander không cài Clarifier → callback clarify bị bỏ qua, không panic.
	h2 := telegram.WebhookHandlerWith(s, &fakeApprover{}, &fakeCommander{}, "987654321", "hook-secret")
	if code := callback(987654321, "clarify:t-c:2:scope:0", "hook-secret", h2); code != 200 {
		t.Fatalf("không có Clarifier: %d", code)
	}
	if code := callback(987654321, "clarify:sai-dinh-dang", "hook-secret", h); code != 200 {
		t.Fatalf("callback sai định dạng: %d", code)
	}
}
