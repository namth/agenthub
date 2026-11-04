# Báo cáo TASK-039

## Đã làm
- Hiện thực hoá luồng làm rõ và trả lời câu hỏi làm rõ trong service `internal/tasks`:
  - Thêm các kiểu dữ liệu `ClarifyQuestion`, `ClarifyRound`, `ClarifyView`.
  - Thêm `ErrForbidden = errors.New("FORBIDDEN")`.
  - Hiện thực hàm `Clarifications`: trả về các vòng làm rõ của task, kèm cờ `CanAnswer` (owner, Admin, người tạo task) và `CanAuto` (chỉ Admin).
  - Hiện thực hàm `Clarify`: kiểm tra quyền trả lời, trạng thái task `WAITING_USER` và vòng chờ `WAITING`, kiểm tra các câu hỏi có giá trị và độ dài (<= 1000 ký tự), hỗ trợ `_notes` tuỳ chọn; dùng transaction chốt vòng sang `ANSWERED`, task sang `PLANNING`, đưa job `SPEC` vào hàng đợi (agent Claude bất kỳ, không lead_only) và gọi hook `OnClarified`. Chống ghi đè đồng thời qua kiểm tra `RowsAffected`.
  - Hiện thực hàm `ClarifyAuto`: tự động chọn phương án `recommended` (hoặc câu dự phòng nếu rỗng) cho các câu hỏi, chuyển round sang `AUTO_DECIDED`, task sang `PLANNING`, đưa job `SPEC` vào hàng đợi và gọi hook `OnClarified`.
  - Hiện thực hàm `AnswerByLogin`: xác thực người dùng theo GitHub login và uỷ quyền cho `Clarify` với kênh `GITHUB`.
  - Hiện thực hàm `PickOption`: lưu dần các lựa chọn theo từng câu hỏi qua Telegram callback, khi đã chọn đủ các câu hỏi thì chốt vòng với kênh `TELEGRAM`.
  - Hiện thực hàm `ParseAnswerComment`: parse các dòng `/answer <id>: <giá trị>` và `_notes`.
  - Thêm option `WithOnClarified` và `WithOnIssueComment`.
- REST API trong `internal/tasks/handler.go`:
  - `GET /api/tasks/{id}/clarifications`
  - `POST /api/tasks/{id}/clarify`
  - `POST /api/tasks/{id}/clarify/auto`
  - Kanban `GET /api/tasks`: xếp task `WAITING_USER` vào cột `planning`, thêm thuộc tính `clarify_pending` đếm số câu hỏi của vòng chờ.
- Webhook GitHub trong `internal/tasks/webhook.go`:
  - Xử lý sự kiện `issue_comment` với action `created`, kích hoạt callback `WithOnIssueComment`.
- Telegram callback trong `internal/telegram/webhook.go` & `cmd/server/telegram_commander.go`:
  - Định nghĩa interface `Clarifier` với phương thức `PickOption`.
  - Bắt callback dạng `clarify:<taskID>:<round>:<questionID>:<idx>`, gọi `PickOption` và phản hồi callback ("Đã ghi", "Đã gửi cho trưởng phòng", hoặc lỗi).
  - Nối `PickOption` vào `TelegramCommander`.
- Nối webhook GitHub `issue_comment` trong `cmd/server/main.go` gọi `ParseAnswerComment` -> `AnswerByLogin`.

## File đã tạo/sửa
- `internal/tasks/service.go`: Thêm types làm rõ, `ErrForbidden`, `Clarifications`, `Clarify`, `ClarifyAuto`, `AnswerByLogin`, `PickOption`, `ParseAnswerComment`, `WithOnClarified`.
- `internal/tasks/handler.go`: Đăng ký endpoints clarifications/clarify/clarify/auto, xử lý mã lỗi `FORBIDDEN`, hiển thị task `WAITING_USER` ở cột planning trên Kanban kèm `clarify_pending`.
- `internal/tasks/webhook.go`: Thêm `WithOnIssueComment` và xử lý sự kiện GitHub `issue_comment`.
- `internal/telegram/webhook.go`: Thêm interface `Clarifier` và xử lý callback `clarify:`.
- `cmd/server/telegram_commander.go`: Cài đặt `telegram.Clarifier` bằng cách gọi `svc.PickOption`.
- `cmd/server/main.go`: Nối `WithOnIssueComment` sang `AnswerByLogin`, bổ sung `WithOnClarified` vào `taskOpts`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-039-clarify-api/verify.sh
ok  	github.com/namth/agenthub/internal/tasks	6.976s
ok  	github.com/namth/agenthub/internal/telegram	1.403s
TASK-039 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
