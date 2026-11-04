# Báo cáo TASK-026

## Đã làm
- `internal/tasks`:
  - Hiện thực `NewMultiService`, `WithRoleIn`, `WithOwner`, `WithMergerFor`, `CreateIn`.
  - Quản lý GitHub client và Merger theo `project_id` của từng task.
  - Phân quyền theo dự án: Owner là ADMIN mọi dự án; thành viên theo `WithRoleIn` (ADMIN/PARTNER); không thuộc dự án trả về 403 `FORBIDDEN`.
  - Cập nhật HTTP Handlers: `POST /api/tasks` yêu cầu `project_id` (400 nếu thiếu) và quyền thành viên trong dự án; `GET /api/tasks` yêu cầu quyền trong dự án (owner không truyền trả về mọi dự án, người khác 400); `GET /api/tasks/{id}` yêu cầu quyền trong dự án của task; `approve`, `reject`, `merge` yêu cầu quyền ADMIN trong dự án của task; thẻ Kanban và chi tiết có thêm `project_id`.
  - Hiện thực `WebhookHandlerMulti`: đọc `repository.full_name`, trả 202 nếu repo không được quản lý, xử lý sự kiện issues opened, workflow_run, pull_request merged cho đúng dự án.
- `internal/pipeline`:
  - Thêm option `WithGitHubFor`, phương thức `RepoFor(dispatcher.Assignment) string` lấy `github_repo` từ dự án của job.
  - Tạo sub-task, issue con, job CODE/QA_REVIEW/REPLAN theo đúng `project_id` của task cha; lấy GitHub client theo `WithGitHubFor`.
  - `Reconcile` bù job cho mọi dự án `ACTIVE` khi `projectID` rỗng hoặc có `WithGitHubFor`; job bù gán đúng `project_id` của task.
- `internal/gateway`:
  - Thêm option `WithRepo(fn func(dispatcher.Assignment) string)` gán `repo` vào `ASSIGN_TASK.payload.repo`.

## File đã tạo/sửa
- `internal/gateway/hub.go`: thêm field `repoFn` và option `WithRepo`.
- `internal/gateway/client.go`: bổ sung `repo` vào payload của `ASSIGN_TASK` khi có `repoFn`.
- `internal/pipeline/pipeline.go`: thêm option `WithGitHubFor`, helper `githubFor`, phương thức `RepoFor`, và sử dụng GitHub client theo dự án khi tạo issue sub-task.
- `internal/pipeline/reconcile.go`: hỗ trợ duyệt qua các dự án `ACTIVE` khi ở chế độ đa dự án để bù SPEC và CODE jobs.
- `internal/tasks/service.go`: thêm `NewMultiService`, `WithRoleIn`, `WithOwner`, `WithMergerFor`, `CreateIn`, hỗ trợ GitHub client/Merger động theo `project_id`.
- `internal/tasks/handler.go`: kiểm tra phân quyền dự án và xử lý query/path parameters cho `tasks`, Kanban theo đúng đặc tả đa dự án.
- `internal/tasks/webhook.go`: thêm `WebhookHandlerMulti` định tuyến theo `repository.full_name` qua callback `projectOf`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-026-tasks-multi/verify.sh
ok  	github.com/namth/agenthub/internal/tasks	3.608s
ok  	github.com/namth/agenthub/internal/pipeline	4.144s
ok  	github.com/namth/agenthub/internal/gateway	3.349s
ok  	github.com/namth/agenthub/internal/e2e	2.770s
TASK-026 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
