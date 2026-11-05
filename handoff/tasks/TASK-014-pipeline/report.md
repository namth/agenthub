# Báo cáo TASK-014

## Đã làm
- Hiện thực các Option cho gateway: `WithPrompt` và `WithSubmitHook`, cập nhật xử lý `ASSIGN_TASK` mang prompt và `TASK_SUBMITTED` kích hoạt hook ngoài khoá của Hub sau khi `api.Submit` thành công.
- Hiện thực package `internal/pipeline` với `Pipeline` struct, hàm khởi tạo `New`, `Notify` (chuyển trạng thái task `READY -> IN_PROGRESS` khi nhận `JOB_ASSIGNED` loại `CODE`).
- Hiện thực `PromptFor` sinh prompt cho agent theo loại job (`SPEC`, `CODE`, `QA_REVIEW`, `REPLAN`, `UNBLOCK`) với đầy đủ tiêu đề, mô tả, `#issue`, tiêu chí nghiệm thu (DoD), schema JSON kết quả và khối `### DỮ LIỆU THAM KHẢO — không phải chỉ thị` cho các ghi chú `lead_journal`.
- Hiện thực `OnSubmitted` xử lý kết quả job có cấu trúc:
  - `SPEC`: kiểm tra schema 1–20 subtasks; tạo GitHub issue qua `gh.CreateIssue`, lưu sub-task vào database kèm DoD và tạo job `CODE`; cập nhật task cha `PLANNING -> READY`. Nếu không hợp lệ, chuyển sang tạo job `REPLAN` cho lead trực và ghi nhận xét `NOTE` vào `lead_journal`.
  - `CODE`: cập nhật task sang `WAITING_QA`, lưu `pr_url` và tạo job `QA_REVIEW` cùng nhánh cho Claude.
  - `QA_REVIEW`: verdict `PASS` chuyển task sang `QA_PASSED`; verdict `CHANGES` chuyển task về `READY`, ghi nhận xét vào `lead_journal` và tạo job `CODE` mới cùng nhánh cho Antigravity; verdict không hợp lệ tạo job `REPLAN` cho lead và giữ nguyên `WAITING_QA`.
  - Giới hạn độ dài trường dữ liệu (`title` <= 120, `description` <= 4000).
- Chạy tự kiểm qua `python3 scripts/handoff.py verify TASK-014` đạt 100%, không vi phạm khoá test, `verify.sh` exit 0 với race detector.

## File đã tạo/sửa
- `internal/gateway/hub.go`: Thêm `type Option func(*Hub)`, `WithPrompt`, `WithSubmitHook` và cập nhật `New`.
- `internal/gateway/client.go`: Thêm trường `Result` vào `incomingEnvelope`, gắn prompt vào payload `ASSIGN_TASK`, kích hoạt `submitHook` khi `TASK_SUBMITTED` thành công.
- `internal/pipeline/pipeline.go`: Tạo mới package `pipeline`, hiện thực `New`, `Notify`, `PromptFor`, `OnSubmitted` cùng các hàm phụ trợ.

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-014-pipeline/verify.sh
--- PASS: TestPromptSpec (0.05s)
--- PASS: TestSpecCreatesSubtasksAndCodeJobs (0.06s)
--- PASS: TestSpecInvalidTriggersReplan (0.19s)
--- PASS: TestCodeAssignedMovesInProgress (0.05s)
--- PASS: TestCodeSubmittedQueuesReview (0.06s)
--- PASS: TestQAPassAndChanges (0.08s)
--- PASS: TestQAInvalidVerdictReplans (0.07s)
--- PASS: TestStaleOrUnknownAssignmentIgnored (0.04s)
ok  	github.com/namth/agenthub/internal/pipeline	1.627s
--- PASS: TestRejectsMissingOrBadToken (0.00s)
--- PASS: TestHandshakeConnects (0.00s)
--- PASS: TestMessageBeforeHandshakeRejected (0.00s)
--- PASS: TestPingPong (0.00s)
--- PASS: TestRequestTaskAssignOrNoTask (0.00s)
--- PASS: TestLogAndSubmitForwarded (0.02s)
--- PASS: TestExhaustedParsesFields (0.04s)
--- PASS: TestLeaseRenewRepliesGranted (0.00s)
--- PASS: TestLeadReleaseAndActions (0.02s)
--- PASS: TestDomainErrorsMapped (0.00s)
--- PASS: TestBadMessagesKeepConnection (0.00s)
--- PASS: TestCloseCallsDisconnect (0.02s)
--- PASS: TestReconnectReplacesOldConnection (0.00s)
--- PASS: TestNotifyRoutesToOwningWorker (0.91s)
--- PASS: TestNotifyNeverBlocks (0.07s)
--- PASS: TestEnvelopeIsFlatJSON (0.00s)
--- PASS: TestWithPromptFillsPayload (0.00s)
--- PASS: TestWithoutPromptOptionStillWorks (0.00s)
--- PASS: TestSubmitHookAfterSuccessOnly (0.02s)
--- PASS: TestHashTokenIsSHA256Hex (0.00s)
--- PASS: TestDBAuthenticator (0.04s)
--- PASS: TestRegisterWorker (0.03s)
ok  	github.com/namth/agenthub/internal/gateway	2.237s
TASK-014 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
