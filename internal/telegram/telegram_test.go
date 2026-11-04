package telegram_test

// Test nghiệm thu TASK-009 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/08 §2, docs/features/lead-rotation.md BR-07, BR-15, TASK-14.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/telegram"
)

// ---------- Sender giả ----------

type msg struct {
	chat    string
	text    string
	buttons [][]telegram.Button
}

type fakeSender struct {
	mu   sync.Mutex
	msgs []msg
	acks []string
	fail bool
}

func (f *fakeSender) SendMessage(_ context.Context, chat, text string, buttons [][]telegram.Button) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return errors.New("telegram down")
	}
	f.msgs = append(f.msgs, msg{chat, text, buttons})
	return nil
}
func (f *fakeSender) AnswerCallback(_ context.Context, id, text string) error {
	f.mu.Lock()
	f.acks = append(f.acks, id+"|"+text)
	f.mu.Unlock()
	return nil
}
func (f *fakeSender) all() []msg {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]msg(nil), f.msgs...)
}

func waitMsgs(t *testing.T, s *fakeSender, n int) []msg {
	t.Helper()
	for i := 0; i < 100; i++ {
		if m := s.all(); len(m) >= n {
			return m
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("chờ %d tin, có %d: %+v", n, len(s.all()), s.all())
	return nil
}

func newNotifier(t *testing.T, s *fakeSender) *telegram.Notifier {
	t.Helper()
	n := telegram.NewNotifier(s, "987654321", time.FixedZone("ICT", 7*3600))
	t.Cleanup(n.Close)
	return n
}

// ---------- thông báo sự kiện dispatcher ----------

func TestLeadChangeMessage(t *testing.T) {
	s := &fakeSender{}
	n := newNotifier(t, s)
	n.Notify(dispatcher.Event{Type: dispatcher.EvLeaseGranted, AgentID: "mac-mini/claude", Epoch: 1})
	n.Notify(dispatcher.Event{Type: dispatcher.EvLeaseRevoked, AgentID: "mac-mini/claude", Epoch: 1, Reason: dispatcher.RevokeExhausted})
	n.Notify(dispatcher.Event{Type: dispatcher.EvLeaseGranted, AgentID: "laptop/claude", Epoch: 2})
	m := waitMsgs(t, s, 1)
	time.Sleep(50 * time.Millisecond)
	m = s.all()
	if len(m) != 1 {
		t.Fatalf("muốn đúng 1 tin đổi trưởng phòng (lần trao đầu tiên khi khởi động không báo), nhận %d: %+v", len(m), m)
	}
	if m[0].chat != "987654321" || !strings.Contains(m[0].text, "epoch 2") || !strings.Contains(m[0].text, "mac-mini/claude") ||
		!strings.Contains(m[0].text, "laptop/claude") || !strings.Contains(m[0].text, "hết limit") {
		t.Fatalf("tin = %q", m[0].text)
	}
}

func TestVacancyAndRestoreMessages(t *testing.T) {
	s := &fakeSender{}
	n := newNotifier(t, s)
	until := time.Date(2026, 11, 4, 14, 40, 0, 0, time.UTC) // 21:40 giờ ICT
	n.Notify(dispatcher.Event{Type: dispatcher.EvLeadVacant, Until: &until})
	n.Notify(dispatcher.Event{Type: dispatcher.EvLeadRestored})
	m := waitMsgs(t, s, 2)
	if !strings.Contains(m[0].text, "CHƯA CÓ TRƯỞNG PHÒNG") || !strings.Contains(m[0].text, "21:40") {
		t.Fatalf("tin trống lead = %q (giờ phải theo múi giờ cấu hình)", m[0].text)
	}
	if !strings.Contains(strings.ToLower(m[1].text), "đã có trưởng phòng") {
		t.Fatalf("tin thoát trống lead = %q", m[1].text)
	}
	n.Notify(dispatcher.Event{Type: dispatcher.EvLeadVacant})
	m = waitMsgs(t, s, 3)
	if !strings.Contains(m[2].text, "chưa rõ") {
		t.Fatalf("trống lead không có giờ dự kiến phải ghi 'chưa rõ': %q", m[2].text)
	}
}

func TestJobFailedMessage(t *testing.T) {
	s := &fakeSender{}
	n := newNotifier(t, s)
	n.Notify(dispatcher.Event{Type: dispatcher.EvJobFailed, TaskID: "t102", JobID: "j9"})
	m := waitMsgs(t, s, 1)
	if !strings.Contains(m[0].text, "t102") || !strings.Contains(m[0].text, "REPLAN") {
		t.Fatalf("tin = %q", m[0].text)
	}
}

func TestIgnoredEventsAndNonBlocking(t *testing.T) {
	s := &fakeSender{}
	n := newNotifier(t, s)
	for _, typ := range []string{dispatcher.EvAgentIdle, dispatcher.EvJobAvailable, dispatcher.EvWorkerOnline, dispatcher.EvAgentCooldown} {
		n.Notify(dispatcher.Event{Type: typ})
	}
	time.Sleep(80 * time.Millisecond)
	if len(s.all()) != 0 {
		t.Fatalf("sự kiện không cần báo lại gửi tin: %+v", s.all())
	}
	slow := &blockingSender{release: make(chan struct{})}
	n2 := telegram.NewNotifier(slow, "1", time.UTC)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 500; i++ {
			n2.Notify(dispatcher.Event{Type: dispatcher.EvJobFailed, TaskID: "t"})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Notify bị chặn khi Telegram chậm")
	}
	close(slow.release)
	n2.Close()
}

type blockingSender struct{ release chan struct{} }

func (b *blockingSender) SendMessage(ctx context.Context, _, _ string, _ [][]telegram.Button) error {
	<-b.release
	return nil
}
func (b *blockingSender) AnswerCallback(context.Context, string, string) error { return nil }

func TestSendFailureDoesNotPanic(t *testing.T) {
	s := &fakeSender{fail: true}
	n := newNotifier(t, s)
	n.Notify(dispatcher.Event{Type: dispatcher.EvJobFailed, TaskID: "t"})
	time.Sleep(50 * time.Millisecond)
}

// ---------- tin yêu cầu mới có nút ----------

func TestNewTaskMessageWithButtons(t *testing.T) {
	s := &fakeSender{}
	n := newNotifier(t, s)
	n.NotifyNewTask("t42", 105, "Xuất file báo cáo ra PDF", "doi-tac-a", strings.Repeat("Khách hàng cần xuất PDF. ", 30))
	m := waitMsgs(t, s, 1)
	if !strings.Contains(m[0].text, "#105") || !strings.Contains(m[0].text, "Xuất file báo cáo ra PDF") || !strings.Contains(m[0].text, "@doi-tac-a") {
		t.Fatalf("tin = %q", m[0].text)
	}
	if len([]rune(m[0].text)) > 1000 {
		t.Fatal("mô tả dài phải được cắt ngắn")
	}
	var data []string
	for _, row := range m[0].buttons {
		for _, b := range row {
			data = append(data, b.CallbackData)
		}
	}
	if strings.Join(data, ",") != "task_approve:t42,task_reject:t42" {
		t.Fatalf("callback_data = %v", data)
	}
}

// ---------- webhook callback ----------

type fakeApprover struct {
	mu       sync.Mutex
	approved []string
	rejected []string
	err      error
}

func (a *fakeApprover) Approve(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.approved = append(a.approved, id)
	return a.err
}
func (a *fakeApprover) Reject(_ context.Context, id, reason string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rejected = append(a.rejected, id+"|"+reason)
	return a.err
}

func callback(chatID int64, data, secret string, h http.Handler) int {
	body, _ := json.Marshal(map[string]any{"update_id": 1, "callback_query": map[string]any{
		"id": "cb1", "data": data, "from": map[string]any{"id": chatID, "username": "namth"},
		"message": map[string]any{"chat": map[string]any{"id": chatID}}}})
	req := httptest.NewRequest("POST", "/api/webhooks/telegram", bytes.NewReader(body))
	if secret != "" {
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code
}

func TestCallbackApproveFromAdminChat(t *testing.T) {
	s := &fakeSender{}
	a := &fakeApprover{}
	h := telegram.WebhookHandler(s, a, "987654321", "hook-secret")
	if code := callback(987654321, "task_approve:t42", "hook-secret", h); code != 200 {
		t.Fatalf("status %d", code)
	}
	if len(a.approved) != 1 || a.approved[0] != "t42" {
		t.Fatalf("approved = %v", a.approved)
	}
	if len(s.acks) != 1 || !strings.HasPrefix(s.acks[0], "cb1|") {
		t.Fatalf("phải answerCallbackQuery: %v", s.acks)
	}
	callback(987654321, "task_reject:t43", "hook-secret", h)
	if len(a.rejected) != 1 || !strings.HasPrefix(a.rejected[0], "t43|") || strings.TrimPrefix(a.rejected[0], "t43|") == "" {
		t.Fatalf("rejected = %v (phải kèm lý do mặc định không rỗng)", a.rejected)
	}
}

func TestCallbackSecurity(t *testing.T) {
	s := &fakeSender{}
	a := &fakeApprover{}
	h := telegram.WebhookHandler(s, a, "987654321", "hook-secret")
	if code := callback(987654321, "task_approve:t42", "sai", h); code != 401 {
		t.Fatalf("secret sai: %d, muốn 401", code)
	}
	if code := callback(987654321, "task_approve:t42", "", h); code != 401 {
		t.Fatalf("thiếu secret: %d, muốn 401", code)
	}
	if code := callback(111, "task_approve:t42", "hook-secret", h); code != 200 {
		t.Fatalf("chat lạ: %d, muốn 200 (Telegram không gửi lại) nhưng bỏ qua", code)
	}
	callback(987654321, "xoa_het:t42", "hook-secret", h)
	callback(987654321, "task_approve:", "hook-secret", h)
	if len(a.approved)+len(a.rejected) != 0 {
		t.Fatalf("chat lạ / callback lạ không được duyệt gì: %v %v", a.approved, a.rejected)
	}
}

func TestCallbackApproveErrorReported(t *testing.T) {
	s := &fakeSender{}
	a := &fakeApprover{err: errors.New("INVALID_TRANSITION")}
	h := telegram.WebhookHandler(s, a, "987654321", "hook-secret")
	callback(987654321, "task_approve:t42", "hook-secret", h)
	if len(s.acks) != 1 || !strings.Contains(strings.ToLower(s.acks[0]), "không") {
		t.Fatalf("duyệt lỗi phải báo lại trên nút: %v", s.acks)
	}
}

// ---------- Bot API thật ----------

func TestBotAPIRequests(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		json.Unmarshal(b, &m)
		mu.Lock()
		paths = append(paths, r.URL.Path)
		bodies = append(bodies, m)
		mu.Unlock()
		w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()
	bot := telegram.NewBotAPI("123:ABC", srv.URL, nil)
	if err := bot.SendMessage(context.Background(), "987", "xin chào", [][]telegram.Button{{{Text: "✅ Duyệt", CallbackData: "task_approve:t1"}}}); err != nil {
		t.Fatal(err)
	}
	if err := bot.AnswerCallback(context.Background(), "cb1", "Đã duyệt"); err != nil {
		t.Fatal(err)
	}
	if paths[0] != "/bot123:ABC/sendMessage" || paths[1] != "/bot123:ABC/answerCallbackQuery" {
		t.Fatalf("paths = %v", paths)
	}
	kb, _ := json.Marshal(bodies[0]["reply_markup"])
	if bodies[0]["chat_id"] != "987" || bodies[0]["text"] != "xin chào" || !strings.Contains(string(kb), `"callback_data":"task_approve:t1"`) {
		t.Fatalf("sendMessage body = %v", bodies[0])
	}
	if bodies[1]["callback_query_id"] != "cb1" {
		t.Fatalf("answerCallbackQuery body = %v", bodies[1])
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
	}))
	defer bad.Close()
	if err := telegram.NewBotAPI("1:x", bad.URL, nil).SendMessage(context.Background(), "1", "x", nil); err == nil {
		t.Fatal("ok=false phải trả lỗi")
	}
}
