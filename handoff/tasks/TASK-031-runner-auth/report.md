# Báo cáo TASK-031

## Đã làm
- Hiện thực API công khai mới trong package `runner`:
  - Các hằng số `AuthOK`, `AuthNeeds`, `AuthUnknown`, `AuthKindGH`.
  - Hàm `DefaultAuthChecks()` khai báo lệnh kiểm cho CLAUDE (`claude auth status`) và GH (`gh auth status`), không có ANTIGRAVITY.
  - Hàm `DefaultAuthPatterns()` bao gồm đầy đủ 5 mẫu: "not logged in", "please run /login", "authentication required", "invalid api key", "gh auth login".
  - Hàm `ParseAuthResult(kind string, exit int, output string, err error) (status, detail string)` xử lý đúng logic JSON cho CLAUDE, exit & "not logged" cho GH, patterns cho các kind khác, che secret và giới hạn độ dài detail ≤ 80 rune, rỗng khi OK.
- Mở rộng `Config` và `Runner`:
  - Thêm các trường `AuthChecks`, `AuthPatterns`, `AuthCheckEvery`, `AuthFreshFor`, `AuthCheckTimeout` với giá trị mặc định theo đặc tả.
  - Cập nhật `Main` đặt `AuthChecks: DefaultAuthChecks()`.
- Kiểm tra đăng nhập trước khi gửi HANDSHAKE:
  - Chạy các lệnh kiểm qua `Executor` với timeout `AuthCheckTimeout`.
  - Gửi `gh_auth`, `agents[].auth_status`, `auth_detail` (bỏ khi rỗng). Kind không có lệnh kiểm là `UNKNOWN`.
- Chặn gửi `REQUEST_TASK` khi agent `NEEDS_AUTH` hoặc máy có `gh_auth = NEEDS_AUTH` (cả sau HANDSHAKE_OK lẫn JOB_AVAILABLE).
- Kiểm định kỳ `AuthCheckEvery`:
  - Chỉ kiểm GH và các agent không BUSY có lệnh kiểm.
  - Chỉ gửi `AGENT_AUTH` khi giá trị đổi. Chuyển từ `NEEDS_AUTH` sang khác thì gửi `REQUEST_TASK` cho agent rảnh.
- Kiểm tra fresh trước `REQUEST_TASK`:
  - Nếu lần kiểm cũ hơn `AuthFreshFor` thì kiểm lại trước (agent không có lệnh kiểm thì bỏ qua).
- Xử lý bản tin `AUTH_RECHECK`:
  - Khi có `agent_id`: kiểm ngay agent đó, nếu không có lệnh kiểm (agy) thì trả `UNKNOWN` và xoá `NEEDS_AUTH` cục bộ; luôn gửi `AGENT_AUTH` kể cả không đổi; sau đó xin việc nếu rảnh.
  - Khi rỗng: kiểm cả máy (GH + mọi agent), luôn gửi `AGENT_AUTH` cho từng đối tượng; sau đó xin việc cho các agent rảnh không `NEEDS_AUTH`.
- Nhận diện lỗi job do mất đăng nhập:
  - Khi CLI exit ≠ 0 và không khớp `ExhaustPatterns` nhưng output khớp `AuthPatterns`: commit WIP + push nhánh nếu có; gửi `AGENT_AUTH` kèm `assignment_id`, `detail`, `handoff_note`, `wip_commit`; chuyển agent thành `NEEDS_AUTH` cục bộ, không gửi `TASK_SUBMITTED` hay `AGENT_EXHAUSTED`.
- Bổ sung `auth_status` và `gh_auth` vào `status.json` (`StatusFile`), cập nhật `--status` in định dạng `id:status:auth_status` (vd `iMac-8GB/claude:IDLE:NEEDS_AUTH`).

## File đã tạo/sửa
- `internal/runner/auth.go`: Tạo mới — các hằng số auth, `DefaultAuthChecks`, `DefaultAuthPatterns`, `ParseAuthResult`, các hàm hỗ trợ probe và xử lý `AUTH_RECHECK`, định kỳ auth, kiểm tra fresh trước `REQUEST_TASK`.
- `internal/runner/runner.go`: Sửa — bổ sung cấu hình và trạng thái auth vào `Config`, `agentState`, `Runner`, `New`; tích hợp probe trước HANDSHAKE; thêm kiểm tra guard trong `handleHandshakeOK`, `handleJobAvailable`, `runTask` (nhận diện lỗi auth pattern, commit WIP + push, gửi `AGENT_AUTH`); gọi `maybeSendRequestTask`.
- `internal/runner/update.go`: Sửa — ghi `gh_auth` và `auth_status` của từng agent vào `status.json`.
- `internal/runner/main.go`: Sửa — gán `AuthChecks: DefaultAuthChecks()` trong cấu hình `Main`; cập nhật đọc và hiển thị `auth_status` trong cờ `--status`.

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-031-runner-auth/verify.sh
ok  	github.com/namth/agenthub/internal/runner	16.378s
TASK-031 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

Toàn bộ 10 test case trong `internal/runner/auth_test.go` cùng các test suite khác trong package `runner` đều PASS với `-race`:
- `TestParseAuthResult`
- `TestDefaultAuthChecksAndPatterns`
- `TestHandshakeIncludesAuthAndBlocksRequests`
- `TestAuthCheckTimeoutIsUnknown`
- `TestGhNeedsAuthBlocksAllRequests`
- `TestRecheckClearsNeedsAuth`
- `TestRecheckWithoutCommandReturnsUnknown`
- `TestAuthPatternOnJobFailure`
- `TestExhaustBeatsAuthPattern`
- `TestPeriodicCheckSendsOnlyChanges`

## Điểm lệch / câu hỏi cho trưởng phòng
Không
