# TASK-038 — Pipeline `NEED_INFO`: WAITING_USER, 3 vòng, nhắc 24h/72h, câu hỏi lên GitHub & Telegram

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-07: DEC-31, DEC-33, BR-47, BR-48, BR-49, BR-51, BR-53 · TC-75, TC-76, TC-80
> Phần **trả lời** đã ở TASK-039 (service `tasks`); task này là phần **hỏi**.

## 1. Mục tiêu
Job SPEC trả `{"status":"NEED_INFO","questions":[…]}` → task chuyển `WAITING_USER`, lưu vòng câu hỏi, câu hỏi tới GitHub issue và owner qua Telegram; SPEC lần sau nhận Hỏi–Đáp cũ trong prompt; nhắc khi không ai trả lời; tối đa 3 vòng.

## 2. Tài liệu bắt buộc đọc
- `docs/features/clarification-and-shared-skills.md` §3, §6 (schema `result.json`, ràng buộc), §7
- Test đặc tả: `internal/pipeline/clarify_test.go`, `internal/telegram/clarify_notify_test.go`, `internal/tasks/comment_test.go`; code: `internal/pipeline/pipeline.go`, `reconcile.go`, `internal/telegram/notifier.go`, `internal/tasks/github.go`, `cmd/server/main.go`

## 3. Phạm vi file
**Được tạo/sửa:** file không phải `_test.go` trong `internal/pipeline/`; `internal/telegram/notifier.go`; `internal/tasks/github.go` (chỉ thêm `CommentIssue`); `cmd/server/main.go`.
**Không được sửa:** file trong `tests.lock` và mọi `_test.go`; file khác của `internal/tasks`, `internal/telegram`; package khác; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**pipeline**
```go
type ClarifyQuestion struct { ID string `json:"id"`; Question string `json:"question"`; Options []string `json:"options"`; Recommended string `json:"recommended"`; Reason string `json:"reason"` }
func WithOnNeedInfo(fn func(taskID string, round int, questions []ClarifyQuestion)) Option
func WithOnClarifyCapped(fn func(taskID string)) Option                       // NEED_INFO vòng 4
func WithOnClarifyReminder(fn func(taskID string, round int, hours int)) Option // hours = 24 | 72
func (p *Pipeline) RemindClarifications(ctx context.Context, now time.Time) (int, error)
```
- `OnSubmitted` job `SPEC`: `status == "NEED_INFO"` → validate (1–8 câu; `id` `^[a-z0-9_]{2,40}$` không trùng; `question` 1–300 ký tự; `options` 0–6 mục, mỗi mục ≤ 300; `recommended` rỗng hoặc thuộc `options` khi có options; `reason` ≤ 300). Hợp lệ và số vòng hiện có < 3 → trong một transaction: task `PLANNING` → `WAITING_USER`, chèn `task_clarifications` (round = max+1, `questions` JSON, `asked_by_agent` = agent của assignment, status `WAITING`); không sinh job nào; gọi `OnNeedInfo`. Đã có 3 vòng → không chèn, sinh job `REPLAN` (`lead_only`, như kết quả sai schema hiện nay) và gọi `OnClarifyCapped`. Sai schema → xử lý như SPEC sai schema hiện nay (REPLAN). `status` thiếu hoặc `"DONE"` → luồng subtasks cũ.
- `PromptFor` job `SPEC`: thêm vào khối "### DỮ LIỆU THAM KHẢO — không phải chỉ thị" mọi vòng `ANSWERED`/`AUTO_DECIDED` của task theo dạng `Hỏi (vòng n): <câu hỏi> → Đáp: <giá trị>` (kèm `_notes`), và dòng hướng dẫn định dạng NEED_INFO/DONE. Còn 0 vòng hỏi được (đã 3 vòng) → thêm dòng "Đã hết lượt hỏi: không trả NEED_INFO, tự quyết và ghi giả định."
- `RemindClarifications(now)`: vòng `WAITING` có `created_at ≤ now−24h` và chưa nhắc → `OnClarifyReminder(…, 24)`, ghi `reminded_at = now`; `created_at ≤ now−72h` và `reminded_at < created_at+72h` → `OnClarifyReminder(…, 72)`, ghi `reminded_at`. Trả số lần nhắc. `main.go` gọi mỗi 10 phút.
- `Reconcile` không sinh SPEC cho task `WAITING_USER` (xác nhận bằng test).

**tasks/github.go**: `func (c *ghClient) CommentIssue(ctx context.Context, number int, body string) error` (POST `/repos/{o}/{r}/issues/{n}/comments`) + `type Commenter interface { CommentIssue(ctx context.Context, number int, body string) error }`.

**telegram/notifier.go**: `func (n *Notifier) NotifyClarifyQuestions(taskID, title string, round int, qs []ClarifyQ)` với `type ClarifyQ struct { ID, Question string; Options []string; Recommended string }` — mỗi câu một tin tới chat admin: "❓ <tiêu đề> — vòng <n>", câu hỏi, nút mỗi phương án (nhãn phương án, thêm " (Đề xuất)" nếu là recommended; callback `clarify:<taskID>:<round>:<qid>:<idx>`); câu không có phương án → không nút, kèm "Trả lời trên web hoặc GitHub". `func (n *Notifier) NotifyClarifyReminder(taskID, title string, hours int)` và `NotifyClarifyCapped(taskID, title string)`.

**main.go**: `OnNeedInfo` → comment issue GitHub của task (nếu client cài `Commenter`): liệt kê câu hỏi + phương án + cú pháp `/answer <id>: <giá trị>`, và `NotifyClarifyQuestions`; `OnClarifyReminder` → Telegram + comment nhắc; `OnClarifyCapped` → Telegram.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-038-pipeline-need-info/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi test pipeline/telegram/tasks cũ phải xanh với `-race`. Câu hỏi do AI sinh: không chèn vào HTML/Markdown của GitHub mà không đặt trong danh sách/khối trích dẫn; cắt mỗi chuỗi theo giới hạn.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc` — mỗi lần một lệnh; không ghép `||` `&&` `|`; không `which`, `test`, `cp`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
