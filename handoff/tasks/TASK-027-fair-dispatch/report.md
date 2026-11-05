# Báo cáo TASK-027

## Đã làm
- Cập nhật hàm `RequestJob` để hỗ trợ chọn job cho mô hình đa dự án (DEC-16, BR-25, BR-29):
  - Agent là lead: ưu tiên nhận job `lead_only = 1` trước trên mọi dự án, không xét `agent_projects`.
  - Các job còn lại (`lead_only = 0`): chỉ chọn các job `QUEUED` đúng `required_agent`, thuộc dự án `status = 'ACTIVE'`, dự án chưa đạt giới hạn `max_parallel` (đếm số job `RUNNING` của dự án), và dự án nằm trong danh sách `agent_projects` của agent (nếu agent không có dòng nào thì nhận mọi dự án).
  - Thứ tự ưu tiên chọn job: `priority` tăng dần -> tỉ lệ `RUNNING / weight` của dự án tăng dần -> `created_at` tăng dần -> `id` tăng dần.
  - Toàn bộ truy vấn chọn job và gán job thực hiện trong cùng một transaction.
- Hỗ trợ máy tạm dừng (`workers.paused = 1`, BR-32):
  - `RequestJob` trả về `nil, nil` khi máy đang ở trạng thái tạm dừng.
  - Loại trừ máy tạm dừng khỏi danh sách ứng viên lead trong `tryAssignLeaseLocked` và `AdminAssign`.
  - Trong `Tick()`, nếu lead hiện tại thuộc máy bị tạm dừng thì thu hồi lease ngay lập tức với lý do `RELEASED` và thử trao quyền cho ứng viên hợp lệ khác.
- Hỗ trợ điều kiện trưởng phòng (`agents.lead_eligible`, BR-06, BR-33):
  - Loại trừ agent có `lead_eligible = 0` khỏi danh sách ứng viên lead tự động (`tryAssignLeaseLocked`).
  - Trả về lỗi `ErrAgentNotEligible` khi Admin gọi `AdminAssign` cho agent có `lead_eligible = 0`.
- Chạy kiểm thử tự động với `verify.sh` đạt 100% (37/37 tests PASS gồm `-race`, không phá vỡ test cũ).

## File đã tạo/sửa
- `internal/dispatcher/service.go`: Hiện thực hoá logic chia lượt đa dự án, xử lý máy tạm dừng (`paused = 1`), và kiểm tra điều kiện ứng viên lead (`lead_eligible = 1`).

## Kết quả tự kiểm
```
== 1. gofmt + vet
== 2. Không dùng time.Now() trực tiếp
== 3. Test dispatcher (-race)
--- PASS: TestNotifyCalledWithoutHoldingLock (0.10s)
--- PASS: TestRequeuedJobsAnnounceAvailability (0.20s)
--- PASS: TestFairShareByWeight (0.16s)
--- PASS: TestPriorityStillBeatsFairness (0.10s)
--- PASS: TestMaxParallel (0.12s)
--- PASS: TestArchivedProjectSkipped (0.10s)
--- PASS: TestAgentProjectAllowlist (0.10s)
--- PASS: TestLeadOnlyIgnoresAllowlist (0.10s)
--- PASS: TestPausedWorker (0.10s)
--- PASS: TestLeadEligible (0.10s)
--- PASS: TestAssignAndSubmitEvents (0.10s)
--- PASS: TestConnectRegistersAgentsAndOnline (0.10s)
--- PASS: TestFirstClaudeTakesLeaseAntigravityNever (0.09s)
--- PASS: TestOfflineAfter45sNoReclaimBefore120s (0.12s)
--- PASS: TestReclaimAfter120sOffline (0.17s)
--- PASS: TestDisconnectMarksOfflineAndRevokesLead (0.10s)
--- PASS: TestRenewExtendsAndStaleEpochRejected (0.11s)
--- PASS: TestLeaseExpiresAndGoesToBestCandidate (0.14s)
--- PASS: TestCandidateOrdering (0.32s)
--- PASS: TestReleaseAndAdminActions (0.10s)
--- PASS: TestAdminRevokeBansAgentTenMinutes (0.34s)
--- PASS: TestConcurrentLeaseChangesKeepEpochConsistent (0.13s)
--- PASS: TestLeadNote (0.09s)
--- PASS: TestJobRoutingByRequiredAgent (0.11s)
--- PASS: TestPriorityThenFIFO (0.11s)
--- PASS: TestExhaustedMidJobHandsOffToAnotherAgent (0.11s)
--- PASS: TestExhaustedWithoutResetUsesDefaultCooldown (1.43s)
--- PASS: TestStaleAssignmentRejected (0.11s)
--- PASS: TestExhaustedForeignAgentRejected (0.09s)
--- PASS: TestMaxAttemptsFailsAndReplans (0.13s)
--- PASS: TestLeadOnlyJobsGoOnlyToLeadAndFirst (0.10s)
--- PASS: TestVacancyKeepsCodeFlowingAndRestores (0.94s)
--- PASS: TestSoftLimitCooldownAndLeadHandover (0.12s)
--- PASS: TestLogIdleCreatesUnblockOnce (1.00s)
ok  	github.com/namth/agenthub/internal/dispatcher	8.368s
== 4. Không phá test cũ
TASK-027 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
