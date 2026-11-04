# Báo cáo TASK-009

## Đã làm
- Cài đặt `Sender` interface và `NewBotAPI` để gửi tin nhắn Telegram (`sendMessage`) và phản hồi inline callback (`answerCallbackQuery`) qua HTTP POST JSON tới Telegram Bot API. Đảm bảo che token bí mật khi có lỗi phát sinh.
- Cài đặt `Notifier` gửi thông báo bất đồng bộ qua hàng đợi có giới hạn, đảm bảo không chặn tiến trình xử lý sự kiện:
  - Ghi nhận `LEASE_REVOKED` và gửi thông báo `👑 ĐỔI TRƯỞNG PHÒNG` khi nhận `LEASE_GRANTED` kế tiếp với lý do được dịch sang tiếng Việt (hết limit, hết hạn lease, mất kết nối, Admin thu hồi, tự nhường vai).
  - Gửi thông báo `⚠️ CHƯA CÓ TRƯỞNG PHÒNG` khi nhận `LEAD_VACANT`, hiển thị giờ dự kiến theo múi giờ chỉ định hoặc "chưa rõ".
  - Gửi thông báo `✅ Đã có trưởng phòng trở lại` khi nhận `LEAD_RESTORED`.
  - Gửi thông báo `🔁 TASK <task_id> THẤT BẠI NHIỀU LẦN` kèm ghi chú `REPLAN` khi nhận `JOB_FAILED`.
  - Bỏ qua các sự kiện khác theo đúng đặc tả.
  - Cung cấp hàm `NotifyNewTask` gửi thông báo yêu cầu tính năng mới với mô tả rút gọn (≤ 300 ký tự) và hai nút inline `✅ Phê duyệt & Lập kế hoạch` / `❌ Từ chối`.
- Cài đặt `WebhookHandler` xử lý webhook từ Telegram Bot:
  - Xác thực header `X-Telegram-Bot-Api-Secret-Token` sử dụng `subtle.ConstantTimeCompare`, trả HTTP 401 nếu sai/thiếu secret.
  - Kiểm tra `adminChatID`, bỏ qua và trả 200 cho callback từ chat khác hoặc dữ liệu callback không đúng định dạng.
  - Gọi `Approver.Approve` hoặc `Approver.Reject` (kèm lý do mặc định "Từ chối qua Telegram"), phản hồi lại bằng `AnswerCallback` tương ứng và xử lý báo lỗi nếu thất bại.

## File đã tạo/sửa
- `internal/telegram/bot.go`: Cài đặt `Button`, interface `Sender`, `botAPI` và hàm `NewBotAPI`.
- `internal/telegram/notifier.go`: Cài đặt `Notifier`, hàng đợi bất đồng bộ, các định dạng thông báo sự kiện dispatcher và yêu cầu task mới.
- `internal/telegram/webhook.go`: Cài đặt `Approver` và `WebhookHandler` xác thực secret hằng thời gian và tiếp nhận phê duyệt/từ chối task qua callback query.

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-009-telegram/verify.sh
== 1. gofmt + vet
== 2. So secret hằng thời gian
== 3. Test telegram (-race)
--- PASS: TestLeadChangeMessage (0.06s)
--- PASS: TestVacancyAndRestoreMessages (0.02s)
--- PASS: TestJobFailedMessage (0.01s)
--- PASS: TestIgnoredEventsAndNonBlocking (0.08s)
--- PASS: TestSendFailureDoesNotPanic (0.05s)
--- PASS: TestNewTaskMessageWithButtons (0.01s)
--- PASS: TestCallbackApproveFromAdminChat (0.00s)
--- PASS: TestCallbackSecurity (0.00s)
--- PASS: TestCallbackApproveErrorReported (0.00s)
--- PASS: TestBotAPIRequests (0.01s)
ok  	github.com/namth/agenthub/internal/telegram	1.562s
== 4. Không phá test cũ
ok  	github.com/namth/agenthub/internal/database	0.655s
ok  	github.com/namth/agenthub/internal/dispatcher	1.231s
ok  	github.com/namth/agenthub/internal/gateway	1.762s
ok  	github.com/namth/agenthub/internal/runner	6.173s
ok  	github.com/namth/agenthub/internal/server	1.389s
ok  	github.com/namth/agenthub/internal/auth	1.225s
ok  	github.com/namth/agenthub/web	1.600s
TASK-009 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
