# Báo cáo TASK-011

## Đã làm
- Tách `tasks.Service` với các phương thức `Create`, `Approve`, `Reject`, kiểu dữ liệu `Task`, `Created`, options `Option`, `WithOnCreated`, và các lỗi chuẩn `ErrNotFound`, `ErrInvalidTransition`, `ErrValidation`, `ErrGitHub`.
- Hiện thực hoá `Service.Create`: xác thực độ dài title và description, gọi GitHub tạo issue trước, lưu DB, truy vấn `username` từ bảng `users` cho trường `Requester`, và gọi đồng bộ callback `OnCreated` đúng 1 lần khi thành công.
- Hiện thực hoá `Service.Approve` (thoả interface `telegram.Approver`): chỉ chuyển trạng thái từ `PENDING_APPROVAL` sang `PLANNING`, cập nhật nhãn GitHub, enqueue job SPEC cho Claude. Không nuốt lỗi `EnqueueJob`: nếu enqueue lỗi, hàm trả lỗi về caller (task vẫn ở trạng thái PLANNING trong DB và GitHub labels đã cập nhật để có thể thử lại).
- Hiện thực hoá `Service.Reject` (thoả interface `telegram.Approver`): kiểm tra lý do không rỗng, chỉ chuyển trạng thái từ `PENDING_APPROVAL` sang `REJECTED`, đóng GitHub issue kèm lý do từ chối.
- Tách `HandlerWithService(s *Service, authz Authz) http.Handler`, đồng thời giữ nguyên chữ ký hàm `Handler(db, gh, d, projectID, authz)` gọi qua `HandlerWithService(NewService(...), authz)`.
- Ánh xạ lỗi Service sang mã HTTP đúng quy ước: `ErrValidation` → 400, `ErrNotFound` → 404, `ErrInvalidTransition` → 409, `ErrGitHub` → 502, `EnqueueJob`/lỗi DB khác → 500.

## File đã tạo/sửa
- `internal/tasks/service.go`: Khai báo lỗi, kiểu dữ liệu, hàm `NewService`, các phương thức `Create`, `Approve`, `Reject` và hook `WithOnCreated`.
- `internal/tasks/handler.go`: Định nghĩa `HandlerWithService`, điều hướng các route qua `Service`, ánh xạ lỗi bằng `writeServiceError`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-011-tasks-service/verify.sh
--- PASS: TestServiceApproveRejectShareRules (0.06s)
--- PASS: TestServiceCreateHookAndErrors (0.04s)
--- PASS: TestHandlerUsesServiceHook (0.04s)
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
ok  	github.com/namth/agenthub/internal/tasks	1.855s
TASK-011 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
- Cách xử lý lỗi `EnqueueJob`: Trong `Service.Approve`, khi `s.disp.EnqueueJob` gặp lỗi, lỗi được trả về caller (`return fmt.Errorf("enqueue spec job: %w", err)`), HTTP handler phản hồi mã 500. Lúc này task đã chuyển sang trạng thái `PLANNING` trong DB và nhãn GitHub đã cập nhật, giúp hệ thống không bị xung đột trạng thái nếu retry enqueue job sau đó.
