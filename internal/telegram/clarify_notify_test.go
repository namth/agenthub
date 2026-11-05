package telegram_test

// Test nghiệm thu TASK-038 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/clarification-and-shared-skills.md §3, §6 (câu hỏi tới owner qua Telegram).

import (
	"strings"
	"testing"

	"github.com/namth/agenthub/internal/telegram"
)

func TestNotifyClarifyQuestions(t *testing.T) {
	s := &fakeSender{}
	n := newNotifier(t, s)
	n.NotifyClarifyQuestions("t-c", "Tích hợp thanh toán", 2, []telegram.ClarifyQ{
		{ID: "payment_gateway", Question: "Cổng nào?", Options: []string{"VNPay", "MoMo"}, Recommended: "VNPay"},
		{ID: "refund", Question: "Hoàn tiền?"},
	})
	m := waitMsgs(t, s, 2)
	if m[0].chat != "987654321" || !strings.Contains(m[0].text, "Tích hợp thanh toán") || !strings.Contains(m[0].text, "Cổng nào?") || !strings.Contains(m[0].text, "2") {
		t.Fatalf("tin 1 = %+v", m[0])
	}
	got := map[string]string{}
	for _, row := range m[0].buttons {
		for _, b := range row {
			got[b.CallbackData] = b.Text
		}
	}
	if got["clarify:t-c:2:payment_gateway:0"] != "VNPay (Đề xuất)" || got["clarify:t-c:2:payment_gateway:1"] != "MoMo" {
		t.Fatalf("nút = %v", got)
	}
	if len(m[1].buttons) != 0 || !strings.Contains(m[1].text, "Trả lời trên web hoặc GitHub") {
		t.Fatalf("câu không phương án = %+v", m[1])
	}
	n.NotifyClarifyReminder("t-c", "Tích hợp thanh toán", 72)
	n.NotifyClarifyCapped("t-c", "Tích hợp thanh toán")
	m = waitMsgs(t, s, 4)
	if !strings.Contains(m[2].text, "72") || !strings.Contains(m[3].text, "Tích hợp thanh toán") {
		t.Fatalf("nhắc/cap = %q | %q", m[2].text, m[3].text)
	}
}
