package telegram_test

// Test nghiệm thu TASK-048 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/remote-command.md §6 (Telegram xác nhận đề xuất huỷ).

import (
	"context"
	"strings"
	"testing"

	"github.com/namth/agenthub/internal/telegram"
)

type proposalCommander struct {
	fakeCommander
	decided []string
}

var _ telegram.ProposalDecider = (*proposalCommander)(nil)

func (p *proposalCommander) DecideProposal(_ context.Context, taskID, pid string, accept bool) error {
	v := "reject"
	if accept {
		v = "accept"
	}
	p.decided = append(p.decided, taskID+"|"+pid+"|"+v)
	return nil
}

func TestProposalNotifyAndCallback(t *testing.T) {
	s := &fakeSender{}
	n := newNotifier(t, s)
	n.NotifyProposal("t-p", "Báo cáo trùng", "lp1", "Trùng #98")
	m := waitMsgs(t, s, 1)
	if !strings.Contains(m[0].text, "Trùng #98") || !strings.Contains(m[0].text, "Báo cáo trùng") {
		t.Fatalf("tin = %q", m[0].text)
	}
	cb := map[string]bool{}
	for _, row := range m[0].buttons {
		for _, b := range row {
			cb[b.CallbackData] = true
		}
	}
	if !cb["proposal_accept:t-p:lp1"] || !cb["proposal_reject:t-p:lp1"] {
		t.Fatalf("nút = %v", cb)
	}
	c := &proposalCommander{fakeCommander: fakeCommander{tasks: map[string]telegram.TaskBrief{}}}
	h := telegram.WebhookHandlerWith(s, &fakeApprover{}, c, "987654321", "hook-secret")
	callback(987654321, "proposal_accept:t-p:lp1", "hook-secret", h)
	callback(987654321, "proposal_reject:t-p:lp2", "hook-secret", h)
	callback(111, "proposal_accept:t-p:lp1", "hook-secret", h)
	if strings.Join(c.decided, ",") != "t-p|lp1|accept,t-p|lp2|reject" {
		t.Fatalf("DecideProposal = %v", c.decided)
	}
}
