# TASK-009 — Telegram: thông báo trưởng phòng / trống lead / task thất bại, nút duyệt nhanh

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Ứng với TASK-08 + TASK-14 trong `docs/11-tasks.md`.

## 1. Mục tiêu
Admin nhận tin Telegram khi đổi trưởng phòng, khi không còn trưởng phòng, khi task thất bại quá số lần; có tin yêu cầu mới kèm nút Duyệt/Từ chối bấm được từ điện thoại.

## 2. Tài liệu bắt buộc đọc
- `docs/08-integrations.md` §2 (định dạng tin, tin 1–5); `docs/features/lead-rotation.md` BR-07, BR-15
- `internal/dispatcher/types.go` (sự kiện `Ev*`); `internal/telegram/telegram_test.go` (đặc tả chi tiết nhất)

## 3. Phạm vi file
**Được tạo/sửa:** các file `.go` (không phải `_test.go`) trong `internal/telegram/`.

**Không được sửa:** file trong `tests.lock`; package khác, `cmd/`, `web/`, `go.mod`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
type Button struct { Text, CallbackData string }
type Sender interface {
    SendMessage(ctx context.Context, chatID, text string, buttons [][]Button) error
    AnswerCallback(ctx context.Context, callbackID, text string) error
}
func NewBotAPI(token, apiURL string, hc *http.Client) Sender // apiURL rỗng → https://api.telegram.org; POST {api}/bot<token>/<method> JSON; ok=false → lỗi
type Notifier struct{ … }
func NewNotifier(s Sender, adminChatID string, loc *time.Location) *Notifier // gửi bất đồng bộ qua hàng đợi có giới hạn (đầy → bỏ tin, không chặn)
func (n *Notifier) Notify(e dispatcher.Event)                              // thoả dispatcher.Notifier
func (n *Notifier) NotifyNewTask(taskID string, issue int, title, requester, description string)
func (n *Notifier) Close()                                                 // dừng goroutine gửi
type Approver interface { Approve(ctx context.Context, taskID string) error; Reject(ctx context.Context, taskID, reason string) error }
func WebhookHandler(s Sender, a Approver, adminChatID, secret string) http.Handler
```
- **Đổi trưởng phòng:** nhớ `LEASE_REVOKED` gần nhất (agent, lý do); `LEASE_GRANTED` sau một lần thu hồi → tin `👑 ĐỔI TRƯỞNG PHÒNG (epoch N)` + `<cũ> ➡️ <mới> · Lý do: <lý do>`. Lý do: EXHAUSTED "hết limit", EXPIRED "hết hạn lease", OFFLINE "mất kết nối", ADMIN "Admin thu hồi", RELEASED "tự nhường vai". `LEASE_GRANTED` không có thu hồi trước (lần trao đầu khi khởi động) → không gửi.
- `LEAD_VACANT` → `⚠️ CHƯA CÓ TRƯỞNG PHÒNG` + "Việc code vẫn chạy; việc lập kế hoạch/review đang chờ." + "Dự kiến có trưởng phòng lại lúc HH:MM" (theo `loc`), không có `Until` → "chưa rõ". `LEAD_RESTORED` → `✅ Đã có trưởng phòng trở lại`. `JOB_FAILED` → `🔁 TASK <task_id> THẤT BẠI NHIỀU LẦN` + "Đã chuyển về lập lại kế hoạch (REPLAN)". Sự kiện khác bỏ qua.
- **Tin yêu cầu mới:** `🔔 YÊU CẦU TÍNH NĂNG MỚI #<issue>`, `Người yêu cầu: @<requester>`, `Tiêu đề: …`, `Mô tả: …` (cắt còn ≤ 300 ký tự + "…"); một hàng hai nút `✅ Phê duyệt & Lập kế hoạch` (`task_approve:<id>`) và `❌ Từ chối` (`task_reject:<id>`).
- **Webhook** `POST /api/webhooks/telegram`: header `X-Telegram-Bot-Api-Secret-Token` khác `secret` (so hằng thời gian) → 401. `callback_query` từ chat khác `adminChatID` hoặc `data` không đúng mẫu `task_(approve|reject):<id không rỗng>` → bỏ qua, 200. Approve/Reject (lý do mặc định "Từ chối qua Telegram") rồi `AnswerCallback` ("Đã duyệt" / "Đã từ chối"; lỗi → "Không thực hiện được: …"). Luôn trả 200 cho update hợp lệ.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-009-telegram/verify.sh` exit 0 (gồm `-race`)
- [ ] Không test nào bị skip; `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không log bot token (nằm trong URL — log lỗi phải che).
- `ls`/`cat`/`grep` không được phép qua run_command — dùng công cụ đọc/liệt kê file có sẵn. Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
