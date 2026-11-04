# TASK-011 — Tách `tasks.Service` dùng chung cho HTTP và Telegram

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity

## 1. Mục tiêu
Nút Duyệt/Từ chối trên Telegram (`telegram.Approver`) và tin báo "yêu cầu mới" cần gọi cùng nghiệp vụ với API HTTP. Tách nghiệp vụ ra `Service`, HTTP handler chỉ còn lo phân quyền + JSON.

## 2. Tài liệu bắt buộc đọc
- `internal/tasks/` hiện có (handler.go, github.go, webhook.go) và `internal/tasks/tasks_test.go` (hành vi HTTP phải giữ nguyên)
- `internal/telegram/` (`Approver` interface), `internal/tasks/service_test.go` (đặc tả mới)

## 3. Phạm vi file
**Được tạo/sửa:** các file `.go` (không phải `_test.go`) trong `internal/tasks/`.

**Không được sửa:** file trong `tests.lock` (gồm `tasks_test.go`, `service_test.go`); package khác; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
var ErrNotFound, ErrInvalidTransition, ErrValidation, ErrGitHub error // dùng errors.Is
type Task struct { ID, Title, Description, Status string; GitHubIssueID int; ParentID *string; CreatedBy string }
type Created struct { TaskID string; Issue int; Title, Requester, Description string } // Requester = users.username của người tạo
type Option func(*Service)
func WithOnCreated(fn func(Created)) Option
func NewService(db *sql.DB, gh GitHub, d dispatcher.API, projectID string, opts ...Option) *Service
func (s *Service) Create(ctx context.Context, userID, title, description string) (Task, error)
func (s *Service) Approve(ctx context.Context, taskID string) error   // thoả telegram.Approver
func (s *Service) Reject(ctx context.Context, taskID, reason string) error
func HandlerWithService(s *Service, authz Authz) http.Handler
```
- `Handler(db, gh, d, projectID, authz)` giữ nguyên chữ ký, bên trong = `HandlerWithService(NewService(db, gh, d, projectID), authz)`.
- Quy tắc như TASK-007 (validate, GitHub trước rồi mới lưu, chỉ chuyển từ `PENDING_APPROVAL`, approve → job SPEC). Lỗi `EnqueueJob` khi duyệt **không được nuốt**: trả lỗi (task vẫn PLANNING; ghi rõ trong report cách xử lý).
- `OnCreated` gọi **sau khi** lưu DB thành công, đồng bộ, đúng một lần cho mỗi task tạo qua `Create` (HTTP cũng đi qua `Create`).
- Ánh xạ lỗi HTTP giữ nguyên: ErrValidation→400, ErrNotFound→404, ErrInvalidTransition→409, ErrGitHub→502.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-011-tasks-service/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không đổi hành vi HTTP đã khoá ở `tasks_test.go`.
- `ls`/`cat`/`grep` không được phép qua run_command — dùng công cụ đọc/liệt kê file có sẵn. Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
