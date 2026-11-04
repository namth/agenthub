# TASK-007 — Tasks API (Kanban, tạo yêu cầu, duyệt/từ chối) + GitHub Issues + webhook

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Phần backend của TASK-05 (webhook) + TASK-06 trong `docs/11-tasks.md`.

## 1. Mục tiêu
Đối tác/Admin tạo yêu cầu → tạo GitHub Issue; Admin duyệt (→ PLANNING + job SPEC cho Claude) hoặc từ chối (đóng issue); Kanban đọc theo 5 cột; issue tạo thẳng trên GitHub được đồng bộ về qua webhook.

## 2. Tài liệu bắt buộc đọc
- `docs/04-user-flows.md` §1, `docs/07-api-contracts.md` §1, §1.2, §1.5, `docs/08-integrations.md` §1.2
- `docs/12-business-rules.md` §1 (máy trạng thái), `docs/03-screens-and-ui.md` §4 (validate), `docs/14-decisions.md` DEC-11
- `internal/dispatcher/types.go` (`EnqueueJob`), `internal/tasks/tasks_test.go` (đặc tả chi tiết nhất)

## 3. Phạm vi file
**Được tạo/sửa:** các file `.go` (không phải `_test.go`) trong `internal/tasks/`.

**Không được sửa:** file trong `tests.lock`; package khác trong `internal/`, `cmd/`, `web/`, `go.mod`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
type GitHub interface {
    CreateIssue(ctx context.Context, title, body string, labels []string) (number int, url string, err error)
    SetLabels(ctx context.Context, number int, labels []string) error
    CloseIssue(ctx context.Context, number int, comment string) error
}
func NewGitHubClient(apiURL, owner, repo, token string, hc *http.Client) GitHub // REST v3; apiURL rỗng → https://api.github.com
type Authz func(r *http.Request) (userID, role string, ok bool)
func Handler(db *sql.DB, gh GitHub, d dispatcher.API, projectID string, authz Authz) http.Handler
func WebhookHandler(db *sql.DB, secret, projectID string) http.Handler
```
- Response chuẩn `docs/07` §1. Chưa đăng nhập → 401 `UNAUTHORIZED`; sai vai → 403 `FORBIDDEN`.
- `POST /api/tasks` (ADMIN, PARTNER): `title` sau khi trim 6–120 ký tự (đếm rune), `description` ≥ 20 ký tự → 400 `VALIDATION_ERROR`. Gọi `CreateIssue(title, description, ["status:pending-approval","type:feature"])` **trước**; lỗi → 502 `GITHUB_ERROR`, không lưu. Thành công: lưu task (`id` UUID, `project_id`, `github_issue_id`, `status=PENDING_APPROVAL`, `created_by`=userID), trả **201** `data = task`.
- `POST /api/tasks/{id}/approve` (ADMIN): chỉ từ `PENDING_APPROVAL`, khác → 409 `INVALID_TRANSITION`; không có → 404. Đặt `PLANNING`, `SetLabels(["status:planning","type:feature"])`, `d.EnqueueJob({ProjectID, TaskID, Kind:"SPEC", RequiredAgent:"CLAUDE"})`.
- `POST /api/tasks/{id}/reject {reason}` (ADMIN): `reason` trim rỗng → 400; chỉ từ `PENDING_APPROVAL`; đặt `REJECTED`, `CloseIssue(number, "Từ chối: <reason>")`.
- `GET /api/tasks` (ADMIN, PARTNER): `data = {columns:[{key,title,tasks:[…]}]}` đúng 5 cột theo DEC-11 (thứ tự `backlog, planning, in_progress, review, done`), REJECTED bỏ ra; mỗi thẻ có `id, title, status, github_issue_id, parent_id, created_at`. ADMIN thêm `branch_name, pr_url, assigned_worker_id`; PARTNER chỉ thấy task gốc và **không** có ba trường nội bộ đó (bỏ hẳn khoá). Hỗ trợ lọc `?status=`.
- `GET /api/tasks/{id}` (ADMIN, PARTNER — partner không xem được sub-task): task + `subtasks[]`.
- Cập nhật `updated_at` mỗi lần đổi trạng thái.
- **Webhook** `POST /api/webhooks/github`: xác minh `X-Hub-Signature-256` (`sha256=<hex HMAC>`, so bằng `hmac.Equal`) → sai/thiếu 401. `issues` + `opened`: nếu chưa có task với `github_issue_id` đó thì tạo task PENDING_APPROVAL (`created_by` = user hệ thống `system-github`, tạo nếu chưa có, role `PARTNER`); idempotent khi GitHub gửi lại. Sự kiện khác → 202, bỏ qua. Đọc body tối đa 1 MB.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-007-tasks-github/verify.sh` exit 0 (gồm `-race`)
- [ ] Không test nào bị skip; `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không đổi trạng thái nhảy cóc; mọi chuyển trạng thái kiểm trạng thái nguồn trong cùng câu `UPDATE … WHERE status = ?`.
- Không log token GitHub hay secret webhook.
- `ls`/`cat`/`grep` không được phép qua run_command — dùng công cụ đọc/liệt kê file có sẵn. Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
