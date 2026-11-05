# TASK-044 — Dự án sandbox: Chạy thử dây chuyền + giao diện SCR-09 (allowlist, sandbox)

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-09: DEC-36, BR-56, BR-60 · TC-89
> API allowlist/sandbox (`PATCH /api/admin/projects/{id}` với `allowed_commands`, `needs_ci_for_integration`, `is_sandbox`) đã có từ TASK-041; task này làm **giao diện** cho chúng và **chạy thử**.

## 1. Mục tiêu
Owner bấm một nút trên dự án sandbox → hệ thống tự tạo yêu cầu mẫu, tự duyệt, theo dõi SPEC → CODE → QA, báo từng bước kèm thời gian, đóng PR/issue mẫu khi xong. Dùng sau mỗi lần triển khai hoặc thêm máy.

## 2. Tài liệu bắt buộc đọc
- `docs/features/ops-readiness.md` §3, §4 (SCR-09, 4 trạng thái chạy thử), §5 (`smoke_runs` — bảng đã có), §6, §7 BR-60
- Test đặc tả: `internal/smoke/smoke_test.go`, `web/src/pages/projects_ops.test.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** package mới `internal/smoke/` (không phải `_test.go`); `cmd/server/main.go`; `web/src/pages/Projects.tsx`.
**Không được sửa:** file trong `tests.lock` và mọi file test; package Go khác (dùng qua API công khai của chúng); `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**`internal/smoke`**
```go
var ErrNotSandbox, ErrConflict, ErrNotFound error
type Step struct { Step string `json:"step"`; AgentID string `json:"agent_id"`; StartedAt *time.Time `json:"started_at"`; EndedAt *time.Time `json:"ended_at"`; OK *bool `json:"ok"`; Note string `json:"note"` }
type Run struct { ID string `json:"id"`; ProjectID string `json:"project_id"`; TaskID string `json:"task_id"`; Status string `json:"status"`; Steps []Step `json:"steps"`; StartedAt time.Time `json:"started_at"`; EndedAt *time.Time `json:"ended_at"` }
type TaskOps interface {
    CreateIn(ctx context.Context, userID, projectID, title, description string) (taskID string, err error)
    Approve(ctx context.Context, taskID string) error
    Cleanup(ctx context.Context, taskID string) error   // đóng PR (không merge) + issue mẫu
}
func New(db *sql.DB, ops TaskOps, now func() time.Time) *Smoke
func (s *Smoke) Start(ctx context.Context, projectID, userID string) (runID string, err error)
func (s *Smoke) Track(ctx context.Context) error            // main gọi mỗi 10 giây
func (s *Smoke) List(projectID string, limit int) ([]Run, error) // mới nhất trước
func (s *Smoke) Get(projectID, runID string) (Run, error)
func Handler(s *Smoke, authz func(*http.Request) (userID, role string, ok bool)) http.Handler
```
- `Start`: dự án không `is_sandbox` → `ErrNotSandbox` (HTTP 400); đã có lần chạy `RUNNING` → `ErrConflict` (409). Tạo task tiêu đề "Smoke test: hàm Add(a, b) + test" (mô tả cố định: thêm hàm cộng hai số và test bảng), `Approve`, ghi `smoke_runs` `RUNNING` với 3 bước `SPEC`, `CODE`, `QA` chưa xong.
- `Track` cho mỗi lần chạy `RUNNING`: nâng `priority = 50` cho job `QUEUED` của task và sub-task; bước `SPEC` xong khi job SPEC của task `DONE` (ghi `agent_id` assignment `SUBMITTED`, thời điểm); `CODE` xong khi mọi job CODE của sub-task `DONE` (note = PR); `QA` xong khi task `QA_PASSED` → run `PASSED`. Task `REJECTED` hoặc job `FAILED` → `FAILED` (note = 20 dòng `task_logs` cuối của job lỗi). Quá 30 phút → `TIMEOUT`. Kết thúc (mọi trạng thái) → `ended_at`, gọi `Cleanup` đúng một lần.
- **HTTP** (chỉ `role == "ADMIN"`): `POST /api/admin/projects/{id}/smoke-run` → `202 {run_id}`; `GET /api/admin/projects/{id}/smoke-runs` → 5 lần gần nhất; `GET /api/admin/projects/{id}/smoke-runs/{run}`. `main.go`: cài `TaskOps` trên `tasks.Service` + GitHub (đóng PR/issue), gắn route, ticker `Track`.

**SCR-09 (`Projects.tsx`)** — mỗi dự án có thêm khung `data-testid="project-ops-<id>"`:
- Textarea `aria-label="Lệnh được phép"` (mỗi dòng một lệnh), công tắc `aria-label="Cần CI cho test tích hợp"`, công tắc `aria-label="Dự án sandbox"`; nút **Lưu cấu hình** → `PATCH /api/admin/projects/{id}` `{allowed_commands: [dòng không rỗng, đã trim], needs_ci_for_integration, is_sandbox}`; lỗi 400 → `role="alert"` trong khung. Hiện danh sách mặc định chỉ đọc "Mặc định: git, gh pr create, gh pr view, ls, cat, grep, find, head, tail, wc".
- Dự án `is_sandbox`: nút **Chạy thử dây chuyền** → `POST …/smoke-run`; đang có lần `RUNNING` → nút `disabled`, thăm dò `GET …/smoke-runs` mỗi 3 giây tới khi xong; bảng lần chạy `data-testid="smoke-runs-<id>"`: trạng thái, từng bước ✓/✗/…, thời gian, note. Chưa chạy lần nào → "Chưa chạy thử lần nào".

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-044-smoke-run/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không merge PR mẫu. Không chạy thử trên dự án thường. `Cleanup` lỗi chỉ ghi log, không làm hỏng run.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`, `node` — mỗi lần một lệnh; không ghép `||` `&&` `|`; không `which`, `test`, `cp`, `npm`, `npx` (web chạy qua verify.sh).
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
