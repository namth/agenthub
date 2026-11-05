# Báo cáo TASK-002

## Đã làm
- Cài đặt `dispatcher.Service` triển khai đầy đủ hợp đồng `dispatcher.API` (`Connect`, `Heartbeat`, `Disconnect`, `Tick`, `EnqueueJob`, `RequestJob`, `Log`, `Submit`, `Exhausted`, `Lead`, `Renew`, `Release`, `LeadNote`, `AdminAssign`, `AdminRevoke`).
- Quản lý vòng đời agent và worker: kết nối, heartbeat, tự động đánh dấu OFFLINE sau 45s, thu hồi assignment sau 120s OFFLINE.
- Cơ chế quản lý Lead Lease với fencing (`epoch`), luân chuyển lead theo BR-06 (ưu tiên lead_priority -> hạn mềm còn lại -> thời gian rảnh), bàn giao và loại trừ 10 phút khi Admin thu hồi.
- Trạng thái VACANT và RESTORED phát đúng 1 lần khi vào và thoát chế độ trống lead; các job CODE vẫn tiếp tục chạy trong khi job trí tuệ xếp hàng.
- Fencing bằng `assignment_id`: phát hiện và từ chối các thao tác cũ/stale (`ErrStaleAssignment`, `ErrStaleEpoch`, `ErrAgentNotOwned`).
- Hạn mềm (`soft_max_jobs`, `soft_window_hours`) chuyển agent sang COOLDOWN (`SOFT_LIMIT`) và tự động chuyển giao vai lead.
- Job attempt và failover: vượt quá `MaxAttempts` (3 lần) thì job FAILED, task chuyển PLANNING, tự động tạo job REPLAN (`lead_only = 1`, `priority = 10`) và phát `EvJobFailed`.
- Log idle monitoring: tự động tạo job UNBLOCK (`lead_only = 1`) khi assignment ACTIVE không có log trong 15 phút.
- Tuân thủ toàn bộ quy tắc về thời gian (lấy từ `Clock`, định dạng UTC `2006-01-02 15:04:05`, không dùng `time.Now()`).

## File đã tạo/sửa
- `internal/dispatcher/service.go`: Triển khai toàn bộ struct `Service` và các phương thức của `dispatcher.API`, hàm helper xử lý đồng bộ giao dịch và phát sự kiện sau commit.

## Kết quả tự kiểm
```
== 1. gofmt + vet
== 2. Không dùng time.Now() trực tiếp
== 3. Test dispatcher (-race)
--- PASS: TestConnectRegistersAgentsAndOnline (0.31s)
--- PASS: TestFirstClaudeTakesLeaseAntigravityNever (0.06s)
--- PASS: TestOfflineAfter45sNoReclaimBefore120s (0.10s)
--- PASS: TestReclaimAfter120sOffline (0.14s)
--- PASS: TestDisconnectMarksOfflineAndRevokesLead (0.07s)
--- PASS: TestRenewExtendsAndStaleEpochRejected (0.08s)
--- PASS: TestLeaseExpiresAndGoesToBestCandidate (0.21s)
--- PASS: TestCandidateOrdering (0.30s)
--- PASS: TestReleaseAndAdminActions (0.09s)
--- PASS: TestAdminRevokeBansAgentTenMinutes (0.50s)
--- PASS: TestConcurrentLeaseChangesKeepEpochConsistent (0.14s)
--- PASS: TestLeadNote (0.05s)
--- PASS: TestJobRoutingByRequiredAgent (0.11s)
--- PASS: TestPriorityThenFIFO (0.13s)
--- PASS: TestExhaustedMidJobHandsOffToAnotherAgent (0.11s)
--- PASS: TestExhaustedWithoutResetUsesDefaultCooldown (2.25s)
--- PASS: TestStaleAssignmentRejected (0.11s)
--- PASS: TestExhaustedForeignAgentRejected (0.09s)
--- PASS: TestMaxAttemptsFailsAndReplans (0.09s)
--- PASS: TestLeadOnlyJobsGoOnlyToLeadAndFirst (0.08s)
--- PASS: TestVacancyKeepsCodeFlowingAndRestores (1.54s)
--- PASS: TestSoftLimitCooldownAndLeadHandover (0.08s)
--- PASS: TestLogIdleCreatesUnblockOnce (1.14s)
ok  	github.com/namth/agenthub/internal/dispatcher	9.994s
== 4. Không phá test cũ
ok  	github.com/namth/agenthub/internal/database	0.571s
ok  	github.com/namth/agenthub/internal/server	0.480s
ok  	github.com/namth/agenthub/web	0.686s
TASK-002 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không

## Vòng sửa 1
- **Mục 1 (Nhả khoá trước khi gọi Notifier):** Sửa cấu trúc `defer` trong tất cả phương thức có phát sự kiện sang closure duy nhất `defer func() { s.mu.Unlock(); s.notify(events) }()`. Đảm bảo `s.mu.Unlock()` luôn được gọi trước khi `s.notify(events)` phát sự kiện, tránh deadlock khi Notifier gọi ngược vào API (đạt `TestNotifyCalledWithoutHoldingLock`).
- **Mục 2 (Phát EvJobAvailable khi job QUEUED):** Bổ sung phát sự kiện `EvJobAvailable` kèm `JobID` tại:
  - Khi job được trả về `QUEUED` sau `Exhausted` (attempt < MaxAttempts).
  - Khi tạo job `REPLAN` (`QUEUED`) sau khi vượt quá `MaxAttempts`.
  - Khi job được trả về `QUEUED` sau khi thu hồi `RECLAIMED` trong `Tick`.
  - Khi tạo job `REPLAN` (`QUEUED`) do vượt quá `MaxAttempts` trong `Tick`.
  - Khi tạo job `UNBLOCK` (`QUEUED`) do log idle trong `Tick`.
  (đạt `TestRequeuedJobsAnnounceAvailability`).
- **Kết quả tự kiểm sau sửa:**
```
== 1. gofmt + vet
== 2. Không dùng time.Now() trực tiếp
== 3. Test dispatcher (-race)
--- PASS: TestNotifyCalledWithoutHoldingLock (0.08s)
--- PASS: TestRequeuedJobsAnnounceAvailability (0.20s)
--- PASS: TestConnectRegistersAgentsAndOnline (0.06s)
--- PASS: TestFirstClaudeTakesLeaseAntigravityNever (0.09s)
--- PASS: TestOfflineAfter45sNoReclaimBefore120s (0.10s)
--- PASS: TestReclaimAfter120sOffline (0.17s)
--- PASS: TestDisconnectMarksOfflineAndRevokesLead (0.07s)
--- PASS: TestRenewExtendsAndStaleEpochRejected (0.08s)
--- PASS: TestLeaseExpiresAndGoesToBestCandidate (0.09s)
--- PASS: TestCandidateOrdering (0.19s)
--- PASS: TestReleaseAndAdminActions (0.06s)
--- PASS: TestAdminRevokeBansAgentTenMinutes (0.38s)
--- PASS: TestConcurrentLeaseChangesKeepEpochConsistent (0.10s)
--- PASS: TestLeadNote (0.08s)
--- PASS: TestJobRoutingByRequiredAgent (0.08s)
--- PASS: TestPriorityThenFIFO (0.06s)
--- PASS: TestExhaustedMidJobHandsOffToAnotherAgent (0.07s)
--- PASS: TestExhaustedWithoutResetUsesDefaultCooldown (1.58s)
--- PASS: TestStaleAssignmentRejected (0.08s)
--- PASS: TestExhaustedForeignAgentRejected (0.11s)
--- PASS: TestMaxAttemptsFailsAndReplans (0.14s)
--- PASS: TestLeadOnlyJobsGoOnlyToLeadAndFirst (0.13s)
--- PASS: TestVacancyKeepsCodeFlowingAndRestores (1.07s)
--- PASS: TestSoftLimitCooldownAndLeadHandover (0.08s)
--- PASS: TestLogIdleCreatesUnblockOnce (1.28s)
ok  	github.com/namth/agenthub/internal/dispatcher	7.965s
== 4. Không phá test cũ
ok  	github.com/namth/agenthub/internal/database	0.786s
ok  	github.com/namth/agenthub/internal/server	0.906s
ok  	github.com/namth/agenthub/web	0.580s
TASK-002 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```
