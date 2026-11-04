# TASK-045 — Telegram: lệnh owner `/new`, `/tasks`, `/status` và nút Merge

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-10: DEC-39, BR-61 · TC-90, TC-91
> Phạm vi đợt này: lệnh + Merge. Nút trả lời làm rõ (FEAT-07) và đề xuất huỷ (FEAT-10) làm ở task sau.

## 1. Mục tiêu
Owner tạo yêu cầu, xem danh sách/tiến độ và merge ngay trên Telegram, cùng RBAC/validate với web.

## 2. Tài liệu bắt buộc đọc
- `docs/features/remote-command.md` §6 (đoạn Telegram), §7 BR-61
- Test đặc tả: `internal/telegram/commands_test.go`; code: `internal/telegram/webhook.go`, `internal/telegram/notifier.go`, `cmd/server/main.go`

## 3. Phạm vi file
**Được tạo/sửa:** file không phải `_test.go` trong `internal/telegram/`; `cmd/server/main.go` và file mới `cmd/server/telegram_commander.go` (cài `Commander` trên `tasks.Service`).
**Không được sửa:** file trong `tests.lock` và mọi `_test.go`; package khác; `web/`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
type TaskBrief struct { ID, Title, Status, Project, PRURL string; Subtasks []TaskBrief }
type Commander interface {
    CreateTask(ctx context.Context, project, title, description string) (string, error) // project = tên dự án hoặc "owner/repo"
    ListTasks(ctx context.Context, project string, limit int) ([]TaskBrief, error)      // project rỗng = mọi dự án
    TaskStatus(ctx context.Context, id string) (TaskBrief, error)
    Merge(ctx context.Context, id string) error
}
var ErrNotFound = errors.New("NOT_FOUND")   // Commander trả lỗi này khi không có dự án/task
func WebhookHandlerWith(s Sender, a Approver, c Commander, adminChatID, secret string) http.Handler
```
- `WebhookHandler(s, a, chat, secret)` giữ nguyên chữ ký = `WebhookHandlerWith(s, a, nil, …)`.
- **Tin nhắn** (`update.message.text`) chỉ xử lý khi `message.chat.id == adminChatID` (chat khác: bỏ qua im lặng, trả 200):
  - `/new <dự án> | <tiêu đề> | <mô tả>` → `CreateTask`; trả "✅ Đã tạo task <id>: <tiêu đề>". Thiếu phần / tiêu đề < 6 hoặc > 120 ký tự / mô tả < 20 ký tự → tin hướng dẫn chứa "Cú pháp: /new <dự án> | <tiêu đề> | <mô tả>", không gọi `CreateTask`.
  - `/tasks [dự án]` → tối đa 10 task: mỗi dòng `<id> · <trạng thái> · <tiêu đề>`; rỗng → "Không có task nào".
  - `/status <id>` → tiêu đề, trạng thái, PR (nếu có), sub-task; task `QA_PASSED` có PR → kèm nút **Merge** (`task_merge:<id>`). `ErrNotFound` → "Không tìm thấy task <id>".
  - Lệnh khác bắt đầu bằng `/` → danh sách lệnh hỗ trợ.
- **Callback** `task_merge:<id>` (chat admin) → `Merge`; thành công answer "Đã merge", lỗi "Không thực hiện được: <lỗi>".
- **Notifier:** `func (n *Notifier) NotifyQAPassed(taskID, title, prURL string)` gửi chat admin tin có "QA đạt", tiêu đề, PR và nút Merge. `main.go` gọi khi task chuyển `QA_PASSED` (dùng điểm phát sự kiện sẵn có của pipeline, hoặc thêm hook nhỏ trong `cmd/server`).
- `cmd/server/telegram_commander.go`: cài `Commander` bằng `tasks.Service` sẵn có với danh tính owner (`GITHUB_OWNER`), tìm dự án theo `projects.name` hoặc `github_repo` (không phân biệt hoa thường); merge qua cùng đường với nút Merge trên web.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-045-telegram-owner-commands/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không có đường tắt bỏ RBAC/validate: tạo task phải đi qua service như web.
- Mọi test telegram cũ phải xanh với `-race`.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc` — mỗi lần một lệnh, không ghép `||`, `&&`, `|`; không `which`, `test`, `cp`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
