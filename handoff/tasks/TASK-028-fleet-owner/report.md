# Báo cáo TASK-028

## Đã làm
- Bổ sung vào `GET /api/fleet`:
  - Mỗi worker: trường `owner` (`{id, login}` theo `workers.owner_user_id` và bảng `users`) hoặc `null`; `version` (`string|null`); `paused` (`bool`).
  - Mỗi agent: trường `lead_eligible` (`bool`); `projects` (mảng `project_id` từ `agent_projects`, hoặc `null` khi agent không có dòng nào).
  - `current_job`: bổ sung `project_id`, `project` (`projects.github_repo` của job), `task_title` (`tasks.title`).
- Bổ sung vào `PATCH /api/admin/agents/{id}`:
  - Thêm hỗ trợ `lead_eligible`: bắt buộc kiểu boolean JSON (`true/false`), sai kiểu hoặc `null` trả về 400 `VALIDATION_ERROR`, lưu 1/0 vào database.
  - Thêm hỗ trợ `projects`: `null` hoặc `[]` xoá hết dòng trong `agent_projects` của agent; mảng chuỗi thay thế toàn bộ trong transaction; nếu có `project_id` không tồn tại trong bảng `projects` hoặc sai kiểu thì trả về 400 `VALIDATION_ERROR` và không thay đổi dữ liệu.
- Bổ sung vào `GET /api/projects`: mỗi phần tử có thêm `open_tasks` (int) đếm số task gốc (`parent_id IS NULL`) của dự án có `status NOT IN ('DONE', 'REJECTED')`.
- Thêm route `POST /api/admin/projects/{id}/webhook` và `POST /admin/projects/{id}/webhook`:
  - Yêu cầu quyền owner (các role khác trả về 403 `FORBIDDEN`).
  - Kiểm tra dự án tồn tại (không tồn tại trả về 404 `NOT_FOUND`).
  - Gọi `gh.CreateWebhook` tạo lại webhook (dùng chung URL với `AddProject`); nếu lỗi trả về 502 `GITHUB_ERROR` và không đổi DB.
  - Nếu thành công: cập nhật `projects.webhook_id`, trả về HTTP 200 với dữ liệu dự án như phần tử của `GET /api/projects` (với `webhook_ok: true`).

## File đã tạo/sửa
- `internal/api/fleet.go`: Bổ sung các trường mới cho WorkerItem, AgentItem, CurrentJobItem; cập nhật truy vấn trong `handleGetFleet` và `getAgentDetail`; cập nhật validate và logic xử lý transaction trong `handlePatchAgent`.
- `internal/projects/handler.go`: Bổ sung `OpenTasks` vào `projectListItem` và truy vấn trong `handleListProjects`; đăng ký route và hiện thực `handleRecreateWebhook`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-028-fleet-owner/verify.sh
ok  	github.com/namth/agenthub/internal/api	3.364s
ok  	github.com/namth/agenthub/internal/projects	3.192s
TASK-028 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
