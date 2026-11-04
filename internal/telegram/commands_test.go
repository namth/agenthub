package telegram_test

// Test nghiệm thu TASK-045 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/remote-command.md §6 (Telegram), BR-61; TC-90, TC-91.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/namth/agenthub/internal/telegram"
)

type fakeCommander struct {
	mu       sync.Mutex
	created  []string
	merged   []string
	tasks    map[string]telegram.TaskBrief
	mergeErr error
}

func (f *fakeCommander) CreateTask(_ context.Context, project, title, desc string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if project == "khong-co" {
		return "", telegram.ErrNotFound
	}
	f.created = append(f.created, project+"|"+title+"|"+desc)
	return "t-new", nil
}
func (f *fakeCommander) ListTasks(_ context.Context, project string, limit int) ([]telegram.TaskBrief, error) {
	out := []telegram.TaskBrief{}
	for _, t := range f.tasks {
		if project == "" || t.Project == project {
			out = append(out, t)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (f *fakeCommander) TaskStatus(_ context.Context, id string) (telegram.TaskBrief, error) {
	t, ok := f.tasks[id]
	if !ok {
		return telegram.TaskBrief{}, telegram.ErrNotFound
	}
	return t, nil
}
func (f *fakeCommander) Merge(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mergeErr != nil {
		return f.mergeErr
	}
	f.merged = append(f.merged, id)
	return nil
}

func message(chatID int64, text string, h http.Handler) int {
	body, _ := json.Marshal(map[string]any{"update_id": 2, "message": map[string]any{
		"message_id": 5, "chat": map[string]any{"id": chatID}, "from": map[string]any{"id": chatID}, "text": text}})
	req := httptest.NewRequest("POST", "/api/webhooks/telegram", bytes.NewReader(body))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "hook-secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code
}

func newCmdRig() (*fakeSender, *fakeCommander, http.Handler) {
	s := &fakeSender{}
	c := &fakeCommander{tasks: map[string]telegram.TaskBrief{
		"t1": {ID: "t1", Title: "Xuất báo cáo PDF", Status: "QA_PASSED", Project: "agenthub", PRURL: "https://github.com/namth/agenthub/pull/9",
			Subtasks: []telegram.TaskBrief{{ID: "s1", Title: "Schema", Status: "DONE"}}},
		"t2": {ID: "t2", Title: "Thêm 2FA", Status: "IN_PROGRESS", Project: "agenthub"},
	}}
	return s, c, telegram.WebhookHandlerWith(s, &fakeApprover{}, c, "987654321", "hook-secret")
}

func TestNewCommand(t *testing.T) { // TC-90
	s, c, h := newCmdRig()
	if code := message(987654321, "/new agenthub | Xuất báo cáo Excel | Khách cần tải bảng tiến độ dạng Excel", h); code != 200 {
		t.Fatalf("status %d", code)
	}
	m := waitMsgs(t, s, 1)
	if len(c.created) != 1 || c.created[0] != "agenthub|Xuất báo cáo Excel|Khách cần tải bảng tiến độ dạng Excel" {
		t.Fatalf("CreateTask = %v", c.created)
	}
	if !strings.Contains(m[0].text, "t-new") || m[0].chat != "987654321" {
		t.Fatalf("tin xác nhận = %+v", m[0])
	}
	for i, bad := range []string{"/new agenthub | Ngắn", "/new agenthub | Tiêu | mô tả đủ dài để qua kiểm tra nhé", "/new agenthub | Tiêu đề ổn | ngắn quá"} {
		message(987654321, bad, h)
		m = waitMsgs(t, s, 2+i)
		if !strings.Contains(m[1+i].text, "Cú pháp: /new") {
			t.Fatalf("sai cú pháp %q → phải trả hướng dẫn, có %q", bad, m[1+i].text)
		}
	}
	if len(c.created) != 1 {
		t.Fatal("sai cú pháp không được tạo task")
	}
	message(987654321, "/new khong-co | Xuất báo cáo Excel | Khách cần tải bảng tiến độ dạng Excel", h)
	m = waitMsgs(t, s, 5)
	if !strings.Contains(strings.ToLower(m[4].text), "không tìm thấy") {
		t.Fatalf("dự án không có → báo không tìm thấy: %q", m[4].text)
	}
}

func TestCommandsIgnoreOtherChats(t *testing.T) { // BR-61
	s, c, h := newCmdRig()
	if code := message(111, "/new agenthub | Xuất báo cáo Excel | Khách cần tải bảng tiến độ dạng Excel", h); code != 200 {
		t.Fatalf("chat lạ phải trả 200, có %d", code)
	}
	message(111, "/tasks", h)
	if len(c.created) != 0 || len(s.all()) != 0 {
		t.Fatalf("chat lạ phải bị bỏ qua im lặng: created=%v msgs=%v", c.created, s.all())
	}
}

func TestTasksAndStatus(t *testing.T) {
	s, _, h := newCmdRig()
	message(987654321, "/tasks agenthub", h)
	m := waitMsgs(t, s, 1)
	if !strings.Contains(m[0].text, "t1") || !strings.Contains(m[0].text, "t2") || !strings.Contains(m[0].text, "IN_PROGRESS") {
		t.Fatalf("/tasks = %q", m[0].text)
	}
	message(987654321, "/tasks du-an-trong", h)
	m = waitMsgs(t, s, 2)
	if !strings.Contains(m[1].text, "Không có task nào") {
		t.Fatalf("/tasks rỗng = %q", m[1].text)
	}
	message(987654321, "/status t1", h)
	m = waitMsgs(t, s, 3)
	st := m[2]
	if !strings.Contains(st.text, "Xuất báo cáo PDF") || !strings.Contains(st.text, "QA_PASSED") || !strings.Contains(st.text, "pull/9") || !strings.Contains(st.text, "Schema") {
		t.Fatalf("/status = %q", st.text)
	}
	hasMerge := false
	for _, row := range st.buttons {
		for _, b := range row {
			if b.CallbackData == "task_merge:t1" {
				hasMerge = true
			}
		}
	}
	if !hasMerge {
		t.Fatalf("task QA_PASSED có PR phải kèm nút Merge: %+v", st.buttons)
	}
	message(987654321, "/status t2", h)
	m = waitMsgs(t, s, 4)
	for _, row := range m[3].buttons {
		for _, b := range row {
			if strings.HasPrefix(b.CallbackData, "task_merge:") {
				t.Fatal("task chưa QA_PASSED không được có nút Merge")
			}
		}
	}
	message(987654321, "/status khong-co", h)
	m = waitMsgs(t, s, 5)
	if !strings.Contains(m[4].text, "Không tìm thấy task khong-co") {
		t.Fatalf("/status không có = %q", m[4].text)
	}
	message(987654321, "/abc", h)
	m = waitMsgs(t, s, 6)
	for _, cmd := range []string{"/new", "/tasks", "/status"} {
		if !strings.Contains(m[5].text, cmd) {
			t.Fatalf("lệnh lạ phải liệt kê lệnh hỗ trợ: %q", m[5].text)
		}
	}
}

func TestMergeCallback(t *testing.T) { // TC-91
	s, c, h := newCmdRig()
	if code := callback(987654321, "task_merge:t1", "hook-secret", h); code != 200 {
		t.Fatalf("status %d", code)
	}
	if len(c.merged) != 1 || c.merged[0] != "t1" {
		t.Fatalf("Merge = %v", c.merged)
	}
	if len(s.acks) != 1 || !strings.Contains(s.acks[0], "Đã merge") {
		t.Fatalf("ack = %v", s.acks)
	}
	c.mergeErr = errors.New("INVALID_TRANSITION")
	callback(987654321, "task_merge:t2", "hook-secret", h)
	if len(s.acks) != 2 || !strings.Contains(s.acks[1], "Không thực hiện được") {
		t.Fatalf("merge lỗi phải báo: %v", s.acks)
	}
	callback(111, "task_merge:t1", "hook-secret", h)
	if len(c.merged) != 1 {
		t.Fatal("chat lạ không được merge")
	}
	// Không có Commander → callback merge bị bỏ qua, không panic.
	h2 := telegram.WebhookHandler(s, &fakeApprover{}, "987654321", "hook-secret")
	if code := callback(987654321, "task_merge:t1", "hook-secret", h2); code != 200 {
		t.Fatalf("không có Commander: %d", code)
	}
}

func TestNotifyQAPassedHasMergeButton(t *testing.T) {
	s := &fakeSender{}
	n := newNotifier(t, s)
	n.NotifyQAPassed("t1", "Xuất báo cáo PDF", "https://github.com/namth/agenthub/pull/9")
	m := waitMsgs(t, s, 1)
	if !strings.Contains(m[0].text, "QA đạt") || !strings.Contains(m[0].text, "pull/9") {
		t.Fatalf("tin = %q", m[0].text)
	}
	found := false
	for _, row := range m[0].buttons {
		for _, b := range row {
			if b.CallbackData == "task_merge:t1" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("thiếu nút Merge: %+v", m[0].buttons)
	}
}
