# Review TASK-002 — vòng 1: CẦN SỬA

Verify đạt 26/26, phạm vi đúng. Hai lỗ hổng do **đặc tả** của trưởng phòng chưa nêu rõ; đã bổ sung test khoá `internal/dispatcher/dispatcher_events_test.go` (ngưỡng verify nâng lên 28).

1. `internal/dispatcher/service.go` — mọi hàm có `defer s.mu.Unlock()` rồi `defer func(){ s.notify(events) }()`: defer chạy ngược nên `Notify` bị gọi khi còn giữ `s.mu` → Notifier gọi ngược vào API sẽ deadlock. **Kỳ vọng:** gom sự kiện, nhả khoá rồi mới `notify` (vd hàm nội bộ trả `events`, hàm public `Lock → gọi nội bộ → Unlock → notify`). Test: `TestNotifyCalledWithoutHoldingLock`.
2. `EvJobAvailable` chỉ phát trong `EnqueueJob`. **Kỳ vọng:** phát mỗi khi một job trở về/được tạo ở trạng thái `QUEUED` — sau `Exhausted`, sau thu hồi `RECLAIMED` trong `Tick`, khi tạo job `REPLAN`/`UNBLOCK` — kèm `JobID`. Test: `TestRequeuedJobsAnnounceAvailability`.

Không sửa `dispatcher_test.go`, `dispatcher_events_test.go`, `types.go`.
