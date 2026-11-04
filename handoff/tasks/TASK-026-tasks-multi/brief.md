# TASK-026 — Tasks, pipeline, webhook, gateway theo dự án

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-04: BR-24, DEC-15, DEC-17, TC-38, TC-40, TC-45

## 1. Mục tiêu
Mọi thao tác task (tạo, duyệt, merge, Kanban), pipeline (sub-task, bù job) và webhook làm đúng dự án/repo; runner được báo repo cần làm.

## 2. Tài liệu bắt buộc đọc
- `docs/features/multi-project.md` §2, §6, §7; `docs/14-decisions.md` DEC-17
- `internal/tasks/`, `internal/pipeline/`, `internal/gateway/hub.go` (cách làm `Option` sẵn có)
- `internal/tasks/multi_test.go`, `internal/pipeline/multi_test.go`, `internal/gateway/repo_test.go` (đặc tả chi tiết nhất)

## 3. Phạm vi file
**Được sửa:** các file `.go` (không phải `_test.go`) trong `internal/tasks/`, `internal/pipeline/`; trong `internal/gateway/` chỉ thêm option `WithRepo`.

**Không được sửa:** file trong `tests.lock` (mọi test cũ giữ nguyên hành vi); package khác; `cmd/`, `web/`, `go.mod`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**tasks**
```go
func NewMultiService(db *sql.DB, ghFor func(projectID string) (GitHub, error), d dispatcher.API, opts ...Option) *Service
func WithRoleIn(fn func(userID, projectID string) (role string, ok bool)) Option
func WithOwner(fn func(userID string) bool) Option
func WithMergerFor(fn func(projectID string) Merger) Option
func (s *Service) CreateIn(ctx context.Context, userID, projectID, title, description string) (Task, error)
func WebhookHandlerMulti(db *sql.DB, secret string, projectOf func(fullName string) (projectID string, ok bool), opts ...WebhookOption) http.Handler
```
- `NewService(db, gh, d, projectID, …)` cũ giữ nguyên (một dự án: `ghFor` luôn trả `gh`, mọi task thuộc `projectID`, không kiểm vai theo dự án).
- Chế độ đa dự án (`NewMultiService`): GitHub client và merger lấy theo `project_id` của task. Vai: `WithOwner` true → ADMIN mọi dự án; không thì `WithRoleIn`; không có vai → 403 `FORBIDDEN`.
- HTTP (`HandlerWithService` dùng chung): `POST /api/tasks` cần `project_id` (thiếu → 400) và vai bất kỳ trong dự án; `GET /api/tasks?project_id=` cần vai trong dự án; không truyền `project_id`: owner → mọi dự án, người khác → 400; `GET /api/tasks/{id}` cần vai trong dự án của task; `approve|reject|merge` cần vai **ADMIN trong dự án của task**. Thẻ Kanban và chi tiết có thêm `project_id`. Job SPEC tạo khi duyệt thuộc `project_id` của task.
- `WebhookHandlerMulti`: đọc `repository.full_name`; `projectOf` không có → 202, bỏ qua; có → xử lý như cũ với dự án đó (issues opened, workflow_run, pull_request merged).

**pipeline**
```go
func WithGitHubFor(fn func(projectID string) tasks.GitHub) Option
func (p *Pipeline) RepoFor(a dispatcher.Assignment) string   // github_repo của dự án chứa job
```
- Sub-task, issue con, job CODE/QA_REVIEW/REPLAN tạo theo `project_id` của task cha (không phải `projectID` khởi tạo); GitHub client theo `WithGitHubFor` (không có → `gh` khởi tạo).
- `Reconcile` xét mọi dự án `ACTIVE` khi `projectID` khởi tạo rỗng hoặc có `WithGitHubFor`; job bù thuộc dự án của task.

**gateway** — `func WithRepo(fn func(dispatcher.Assignment) string) Option` → `ASSIGN_TASK.payload.repo`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-026-tasks-multi/verify.sh` exit 0 (gồm `-race`)
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi test cũ của `tasks`, `pipeline`, `gateway`, `e2e` phải xanh.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
