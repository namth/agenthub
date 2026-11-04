# Báo cáo TASK-013

## Đã làm
- [x] Thêm trường `OpenPR func(ctx context.Context, dir, branch, title, body string) (prURL string, err error)` vào `Config`.
- [x] Tạo `GhPROpener(exec Executor, base string)` chạy `gh pr create --head <branch> --base <base> --title <title> --body <body>` trong `dir`, trả URL là dòng cuối bắt đầu bằng `https://`.
- [x] Cập nhật `Main` trong `internal/runner/main.go` hỗ trợ cờ `--base` (mặc định `main`) và `--no-pr`, mặc định cấu hình `GhPROpener(ExecExecutor{}, base)`.
- [x] Quản lý thư mục tạm riêng cho từng assignment bằng `os.MkdirTemp`, file kết quả `result.json`, thêm biến `AGENTHUB_RESULT_FILE=<đường dẫn tuyệt đối>` cùng hướng dẫn schema DEC-12 vào cuối prompt. Xoá thư mục tạm sau khi hoàn tất.
- [x] Kiểm tra và đọc `result.json` khi exit 0:
  - Với `SPEC`, `QA_REVIEW`: bắt buộc có file, định dạng object JSON, kích thước <= 64KB. Nếu vi phạm, gửi `LOG_CHUNK` chứa `[runner]` và `result.json`, không submit.
  - Với `CODE`: tuỳ chọn (thiếu -> `result` null; có nhưng sai -> gửi `LOG_CHUNK` chứa `[runner]` và `result.json`, không submit).
- [x] Khi job `CODE` thành công: commit + push nhánh làm việc, gọi `OpenPR` mở PR; nếu mở PR lỗi thì gửi `LOG_CHUNK` chứa `PR` và vẫn submit với `pr_url: ""`.
- [x] Gửi bản tin `TASK_SUBMITTED` kèm `assignment_id`, `branch`, `pr_url`, `result`.
- [x] Chạy `verify.sh` đạt exit 0 (pass race condition, pass tất cả unit/integration test cũ và mới, binary thật server + runner).

## File đã tạo/sửa
- `internal/runner/pr.go`: Cài đặt `GhPROpener` sử dụng `gh pr create` để mở PR.
- `internal/runner/runner.go`: Thêm trường `OpenPR` vào `Config`, tạo thư mục tạm và file kết quả `result.json`, gắn `AGENTHUB_RESULT_FILE` vào prompt, kiểm tra và đọc kết quả có cấu trúc khi exit 0, mở PR và gửi `TASK_SUBMITTED`.
- `internal/runner/main.go`: Thêm cờ `--base`, `--no-pr` và khởi tạo mặc định `OpenPR` qua `GhPROpener`.

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-013-runner-result/verify.sh
== 1. gofmt + vet
== 2. Không log token, không nhận token qua cờ
== 3. Test runner (-race)
--- PASS: TestEndToEndWithRealServer (1.33s)
--- PASS: TestRealGitWorktreeLifecycle (0.61s)
--- PASS: TestExecExecutor (0.21s)
--- PASS: TestSpecResultSubmitted (0.01s)
--- PASS: TestEachAssignmentGetsOwnResultFile (0.01s)
--- PASS: TestRequiredResultMissingOrInvalid (1.24s)
--- PASS: TestCodeJobOpensPR (0.00s)
--- PASS: TestCodeJobWithoutResultStillSubmits (0.00s)
--- PASS: TestOpenPRFailureStillSubmits (0.00s)
--- PASS: TestGhPROpener (0.00s)
--- PASS: TestConnectHandshakeAndRequestPerAgent (0.00s)
--- PASS: TestHeartbeat (0.10s)
--- PASS: TestReconnectsAfterDrop (0.02s)
--- PASS: TestCodeJobSuccessCommitsPushesSubmits (0.00s)
--- PASS: TestLogChunksBatchedAndComplete (0.00s)
--- PASS: TestPromptIncludesHandoffNoteAsData (0.00s)
--- PASS: TestExhaustedMidCodeJobHandsOff (0.50s)
--- PASS: TestExhaustedPushFailStillReports (0.00s)
--- PASS: TestExhaustedOnSpecJobNoGit (0.00s)
--- PASS: TestCLIFailureNotSubmitted (0.40s)
--- PASS: TestAbortCancelsCLI (0.38s)
--- PASS: TestLeaseRenewLoopAndStop (0.82s)
--- PASS: TestParseResetsAt (0.00s)
--- PASS: TestDefaultExhaustPatterns (0.00s)
--- PASS: TestBuildHandoffNote (0.00s)
ok  	github.com/namth/agenthub/internal/runner	7.066s
== 4. Không phá test cũ
ok  	github.com/namth/agenthub/internal/api	0.359s
ok  	github.com/namth/agenthub/internal/auth	0.624s
ok  	github.com/namth/agenthub/internal/database	0.936s
ok  	github.com/namth/agenthub/internal/dispatcher	1.438s
ok  	github.com/namth/agenthub/internal/gateway	2.303s
ok  	github.com/namth/agenthub/internal/office	1.539s
ok  	github.com/namth/agenthub/internal/runner	7.378s
ok  	github.com/namth/agenthub/internal/server	2.040s
ok  	github.com/namth/agenthub/internal/tasks	1.800s
ok  	github.com/namth/agenthub/internal/telegram	1.948s
ok  	github.com/namth/agenthub/web	1.770s
== 5. Binary thật: server + runner
TASK-013 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
