# TASK-039 — API làm rõ: clarify, tự quyết, `/answer` trên GitHub, nút Telegram

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-07: DEC-33, BR-49, BR-52, BR-54, BR-55 · TC-77 → TC-79
> Phần pipeline sinh câu hỏi (`NEED_INFO` → `WAITING_USER`) là TASK-038; task này xử lý **trả lời**. Test tự seed `task_clarifications`.

## 1. Mục tiêu
Một đường nghiệp vụ duy nhất (service `tasks`) để trả lời câu hỏi làm rõ từ web, GitHub comment, Telegram (owner) và sau này MCP.

## 2. Tài liệu bắt buộc đọc
- `docs/features/clarification-and-shared-skills.md` §2, §5, §6, §7 (BR-49, BR-52 → BR-55)
- Test đặc tả: `internal/tasks/clarify_test.go`, `internal/telegram/clarify_test.go`; code: `internal/tasks/service.go`, `handler.go`, `webhook.go`, `internal/telegram/webhook.go`

## 3. Phạm vi file
**Được tạo/sửa:** file không phải `_test.go` trong `internal/tasks/` và `internal/telegram/`; `cmd/server/main.go`, `cmd/server/telegram_commander.go`, `cmd/server/mcp_backend.go` nếu đã tồn tại (nối `AnswerClarification`).
**Không được sửa:** file trong `tests.lock` và mọi `_test.go`; package khác; migration; `web/`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**Service (`internal/tasks`)**
```go
var ErrForbidden = errors.New("FORBIDDEN") // nếu chưa có
type ClarifyQuestion struct { ID, Question string; Options []string; Recommended, Reason string }
type ClarifyRound struct { Round int; Status string; Questions []ClarifyQuestion; Answers map[string]string; AnsweredBy, AnsweredVia string; AnsweredAt *time.Time }
type ClarifyView struct { Rounds []ClarifyRound; CanAnswer, CanAuto bool }
func (s *Service) Clarifications(ctx context.Context, userID, role, taskID string) (ClarifyView, error)
func (s *Service) Clarify(ctx context.Context, userID, role, taskID string, answers map[string]string, via string) error
func (s *Service) ClarifyAuto(ctx context.Context, userID, role, taskID string) error
func (s *Service) AnswerByLogin(ctx context.Context, login, taskID string, answers map[string]string) error // via GITHUB
func (s *Service) PickOption(ctx context.Context, userID, taskID string, round int, questionID string, optionIdx int) (done bool, err error) // via TELEGRAM, lưu từng câu
func ParseAnswerComment(body string) (map[string]string, bool) // dòng "/answer <id>: <giá trị>"; "_notes" là ghi chú
func WithOnClarified(fn func(taskID string, round int, byLogin, via string)) Option
func WithOnIssueComment(fn func(projectID string, issue int, login, body string)) WebhookOption
```
- **Quyền trả lời** (BR-54): `role == "ADMIN"` (owner/Admin ở chế độ một dự án), vai `ADMIN` trong dự án (`UserRoleIn`), hoặc `tasks.created_by == userID`. `CanAuto`: chỉ hai nhóm ADMIN.
- **`Clarify`**: vòng `WAITING` mới nhất của task đang `WAITING_USER`; mỗi `questions[].id` phải có giá trị không rỗng (≤ 1000 ký tự), `_notes` tuỳ chọn; khoá khác bỏ qua. Trong **một transaction**: round → `ANSWERED`, `answers` (JSON), `answered_by`, `answered_via`, `answered_at`; task → `PLANNING`; enqueue job `SPEC` (`required_agent = CLAUDE`, không `lead_only`). Không `WAITING_USER`/không có vòng chờ → `ErrInvalidTransition` (409); thiếu câu → `ErrValidation` (400); không có quyền → `ErrForbidden` (403). Thành công → gọi hook `OnClarified`.
- **`ClarifyAuto`**: như trên với `answers[qid] = recommended` (rỗng → "Trưởng phòng tự chọn phương án hợp lý nhất, ghi rõ giả định"), status `AUTO_DECIDED`, via `AUTO`.
- **`PickOption`**: owner chọn phương án `optionIdx` cho một câu; lưu dần vào `answers`; khi đủ mọi câu → chốt như `Clarify` với via `TELEGRAM`, trả `done = true`.
- **REST**: `GET /api/tasks/{id}/clarifications` → `{rounds:[{round,status,questions,answers,answered_by (login),answered_via,answered_at}], can_answer, can_auto}`; `POST /api/tasks/{id}/clarify` `{answers}` → `200 {status:"PLANNING"}`; `POST /api/tasks/{id}/clarify/auto` → `200 {status:"PLANNING"}`. Mã lỗi theo trên.
- **Kanban**: `GET /api/tasks` đặt `WAITING_USER` vào cột `planning`, thẻ thêm `clarify_pending` = số câu của vòng `WAITING` (0 nếu không có).
- **Webhook GitHub** `issue_comment` (action `created`): gọi `OnIssueComment(projectID, issue.number, comment.user.login, comment.body)`; trả 200. `main.go` nối: tìm task theo `project_id` + `github_issue_id`, `ParseAnswerComment` → `AnswerByLogin` (lỗi bỏ qua, không trả lỗi cho GitHub).
- **Telegram**: nếu `Commander` truyền vào `WebhookHandlerWith` cài thêm `telegram.Clarifier { PickOption(ctx, taskID string, round int, questionID string, optionIdx int) (bool, error) }` thì callback `clarify:<taskID>:<round>:<questionID>:<idx>` (chat admin) gọi nó; answer "Đã ghi" (chưa đủ) / "Đã gửi cho trưởng phòng" (đủ) / "Không thực hiện được: <lỗi>".

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-039-clarify-api/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi test tasks/telegram cũ phải xanh với `-race`. Chốt vòng phải chống ghi đè đồng thời (`UPDATE … WHERE status='WAITING'` rồi kiểm `RowsAffected`).
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc` — mỗi lần một lệnh; không ghép `||` `&&` `|`; không `which`, `test`, `cp`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
