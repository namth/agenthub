# Báo cáo TASK-007

## Đã làm
- Hiện thực hoá client GitHub REST v3 (`NewGitHubClient`) đáp ứng interface `GitHub` với đầy đủ `CreateIssue`, `SetLabels`, `CloseIssue`.
- Hiện thực API `POST /api/tasks` (ADMIN, PARTNER): kiểm tra độ dài `title` sau khi trim (6–120 rune) và `description` (≥20 rune); tạo issue GitHub với nhãn `status:pending-approval`, `type:feature` trước; lỗi GitHub trả 502 `GITHUB_ERROR` không lưu DB; thành công lưu task `PENDING_APPROVAL` và trả về 201.
- Hiện thực API `POST /api/tasks/{id}/approve` (chỉ ADMIN): chỉ chuyển từ `PENDING_APPROVAL` (khác trả 409 `INVALID_TRANSITION`, không thấy trả 404 `NOT_FOUND`), cập nhật `PLANNING`, đổi nhãn GitHub sang `status:planning`, kích hoạt job `SPEC` cho agent `CLAUDE` vào hàng đợi dispatcher.
- Hiện thực API `POST /api/tasks/{id}/reject` (chỉ ADMIN): kiểm tra `reason` sau trim không rỗng (rỗng trả 400 `VALIDATION_ERROR`), chuyển trạng thái `REJECTED`, đóng issue GitHub kèm bình luận lý do.
- Hiện thực API `GET /api/tasks` (ADMIN, PARTNER): trả về bảng Kanban đúng 5 cột theo DEC-11 (`backlog`, `planning`, `in_progress`, `review`, `done`), bỏ qua các task `REJECTED`; thẻ gồm `id, title, status, github_issue_id, parent_id, created_at`; ADMIN xem thêm `branch_name, pr_url, assigned_worker_id`; PARTNER chỉ thấy task gốc (`parent_id IS NULL`) và ẩn hoàn toàn 3 trường nội bộ (không tồn tại key trong JSON); hỗ trợ lọc `?status=`.
- Hiện thực API `GET /api/tasks/{id}` (ADMIN, PARTNER): trả về thông tin task kèm mảng `subtasks[]`; chặn PARTNER xem sub-task trực tiếp (trả về 403 `FORBIDDEN`).
- Cập nhật `updated_at` mỗi lần chuyển trạng thái.
- Hiện thực Webhook `POST /api/webhooks/github`: đọc body tối đa 1 MB; xác minh `X-Hub-Signature-256` bằng `hmac.Equal` (sai hoặc thiếu tiền tố trả 401 `UNAUTHORIZED`); bắt sự kiện `issues.opened` và đồng bộ task `PENDING_APPROVAL` với `created_by` = user hệ thống `system-github` (tự động tạo nếu chưa có, role `PARTNER`); bảo đảm tính idempotent khi GitHub gửi lại; bỏ qua các sự kiện khác với mã 202; tuyệt đối không log token GitHub hay webhook secret.

## File đã tạo/sửa
- `internal/tasks/github.go`: Khai báo interface `GitHub` và triển khai REST v3 client `NewGitHubClient` cùng các phương thức `CreateIssue`, `SetLabels`, `CloseIssue`.
- `internal/tasks/handler.go`: Khai báo kiểu `Authz`, router `Handler` và các hàm xử lý tạo task, duyệt, từ chối, danh sách Kanban và chi tiết task cùng các tiện ích tạo UUID và ghi JSON response.
- `internal/tasks/webhook.go`: Triển khai `WebhookHandler` xác thực HMAC-SHA256 dùng `hmac.Equal` và xử lý đồng bộ issue từ GitHub.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-007-tasks-github/verify.sh
== 1. gofmt + vet
== 2. So chữ ký hằng thời gian, không log bí mật
== 3. Test tasks (-race)
--- PASS: TestCreateTaskCreatesIssue (0.04s)
--- PASS: TestCreateValidation (0.04s)
--- PASS: TestCreateGitHubFailureNoRow (0.04s)
--- PASS: TestApproveMovesToPlanningAndQueuesSpec (0.04s)
--- PASS: TestRejectRequiresReasonAndClosesIssue (0.04s)
--- PASS: TestKanbanColumnsDEC11 (0.05s)
--- PASS: TestPartnerSeesOnlyRootTasksWithoutInternals (0.04s)
--- PASS: TestTaskDetail (0.04s)
--- PASS: TestWebhookSignature (0.04s)
--- PASS: TestWebhookIssueOpenedSyncsOnce (0.04s)
ok  	github.com/namth/agenthub/internal/tasks	1.863s
== 4. Không phá test cũ
ok  	github.com/namth/agenthub/internal/database	0.493s
ok  	github.com/namth/agenthub/internal/dispatcher	1.017s
ok  	github.com/namth/agenthub/internal/gateway	1.686s
ok  	github.com/namth/agenthub/internal/runner	5.398s
ok  	github.com/namth/agenthub/internal/server	0.923s
ok  	github.com/namth/agenthub/web	1.385s
TASK-007 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
