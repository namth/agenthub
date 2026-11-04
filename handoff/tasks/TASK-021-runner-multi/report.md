# Báo cáo TASK-021

## Đã làm
- Bổ sung `var Version = "dev"` và `var ErrUpdated = errors.New("UPDATED")` trong package runner.
- Mở rộng `Config` với các trường: `GitFor func(repo string) (Git, error)`, `SelfPath string`, `NoUpdate bool`, `StatusFile string`.
- Hỗ trợ đa repo:
  - Hiện thực `NewRepoCache(home, cloneURL, exec)` kiểm tra định dạng repo (`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$` và không chứa `..`), tự clone khi chưa có `<home>/repos/<owner>/<name>/.git`, dùng worktrees ở `<home>/worktrees/<owner>-<name>`, an toàn đa luồng (mutex theo repo).
  - Khi nhận `ASSIGN_TASK`, nếu `payload.repo` khác rỗng và có `GitFor`, runner dùng `GitFor(repo)`. Nếu lỗi, runner gửi `LOG_CHUNK` `[runner] không chuẩn bị được repo <repo>: …` và không nộp. Nếu không có `repo` hoặc không có `GitFor`, dùng `git` mặc định.
- Phiên bản & tự cập nhật:
  - Bản tin `HANDSHAKE` gửi `data.version = Version`.
  - Khi nhận `HANDSHAKE_OK.server_version` khác `Version` và `!NoUpdate`: khi mọi agent IDLE (không job đang chạy), tải binary `/download/agenthub-<GOOS>-<GOARCH>` và `.sha256` từ host của `ServerURL`, kiểm tra SHA-256 (dòng đầu, token đầu), ghi file tạm cùng thư mục `SelfPath`, `chmod 0755`, rename đè `SelfPath` và `Run` trả về `ErrUpdated`.
  - Tải/kiểm tra lỗi giữ bản cũ, ghi log, thử lại sau 30 phút.
- Trạng thái runner (`StatusFile` & `--status`):
  - Ghi `StatusFile` dạng JSON `{connected, server, version, agents:[{id,status}], updated_at}` (RFC3339, quyền 0600, không chứa token) sau khi handshake thành công và mỗi nhịp tim. Mất kết nối ghi `connected: false`.
  - Lệnh con `--status` đọc file status, in `ONLINE <server> agents=…` và exit 0 nếu `connected` và `updated_at` trong vòng 2 phút; ngược lại in lý do và exit 1.
- `Main`:
  - Thêm cờ `--home` (mặc định `~/.agenthub`), `--status`, `--status-file` (mặc định `<home>/status.json`), `--no-update`; chuyển cờ `--repo` thành tuỳ chọn.
  - Mặc định dùng `NewRepoCache(home, ...)`, đọc token từ `<home>/token` nếu không có biến môi trường `AGENTHUB_RUNNER_TOKEN` hoặc cờ `--token-file`.
  - Khi `Run` trả về `ErrUpdated`, `Main` exit 0 để dịch vụ khởi động lại.

## File đã tạo/sửa
- `internal/runner/cache.go`: Hiện thực `NewRepoCache` với cơ chế clone cache theo repo và worktree per-repo.
- `internal/runner/update.go`: Hiện thực logic tự cập nhật (tải binary + sha256, kiểm tra checksum, thay thế file nguyên tử) và ghi `StatusFile`.
- `internal/runner/runner.go`: Bổ sung `Version`, `ErrUpdated`, mở rộng `Config`, tích hợp đa repo trong `runTask`, tích hợp cập nhật khi idle và gửi version trong handshake.
- `internal/runner/main.go`: Bổ sung các cờ mới, xử lý lệnh `--status`, fallback đọc token từ `<home>/token`, cấu hình mặc định cho `NewRepoCache` và exit 0 khi nhận `ErrUpdated`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-021-runner-multi/verify.sh
== 1. gofmt + vet
== 2. Không log token, không nhận token qua cờ
== 3. Test runner (-race)
--- PASS: TestCancelKillsProcessGroup (0.51s)
--- PASS: TestEndToEndWithRealServer (1.42s)
--- PASS: TestRealGitWorktreeLifecycle (0.46s)
--- PASS: TestExecExecutor (0.20s)
--- PASS: TestGitPerRepo (0.30s)
--- PASS: TestRepoCacheClonesOnce (0.23s)
--- PASS: TestHandshakeSendsVersion (0.23s)
--- PASS: TestSelfUpdateWhenIdle (3.02s)
--- PASS: TestSelfUpdateRejectsBadChecksum (0.81s)
--- PASS: TestNoUpdateFlag (0.61s)
--- PASS: TestStatusFile (0.04s)
--- PASS: TestSpecResultSubmitted (0.01s)
--- PASS: TestEachAssignmentGetsOwnResultFile (0.01s)
--- PASS: TestRequiredResultMissingOrInvalid (1.24s)
--- PASS: TestCodeJobOpensPR (0.01s)
--- PASS: TestCodeJobWithoutResultStillSubmits (0.00s)
--- PASS: TestOpenPRFailureStillSubmits (0.01s)
--- PASS: TestGhPROpener (0.00s)
--- PASS: TestConnectHandshakeAndRequestPerAgent (0.00s)
--- PASS: TestHeartbeat (0.09s)
--- PASS: TestReconnectsAfterDrop (0.03s)
--- PASS: TestCodeJobSuccessCommitsPushesSubmits (0.01s)
--- PASS: TestLogChunksBatchedAndComplete (0.01s)
--- PASS: TestPromptIncludesHandoffNoteAsData (0.00s)
--- PASS: TestExhaustedMidCodeJobHandsOff (0.50s)
--- PASS: TestExhaustedPushFailStillReports (0.00s)
--- PASS: TestExhaustedOnSpecJobNoGit (0.00s)
--- PASS: TestCLIFailureNotSubmitted (0.40s)
--- PASS: TestAbortCancelsCLI (0.38s)
--- PASS: TestLeaseRenewLoopAndStop (0.83s)
--- PASS: TestParseResetsAt (0.00s)
--- PASS: TestDefaultExhaustPatterns (0.00s)
--- PASS: TestBuildHandoffNote (0.01s)
ok  	github.com/namth/agenthub/internal/runner	12.403s
== 4. Không phá test cũ
ok  	github.com/namth/agenthub/internal/api	0.106s
ok  	github.com/namth/agenthub/internal/auth	0.206s
ok  	github.com/namth/agenthub/internal/database	0.325s
ok  	github.com/namth/agenthub/internal/dispatcher	0.591s
ok  	github.com/namth/agenthub/internal/e2e	0.095s
ok  	github.com/namth/agenthub/internal/gateway	1.153s
ok  	github.com/namth/agenthub/internal/logs	0.276s
ok  	github.com/namth/agenthub/internal/office	0.087s
ok  	github.com/namth/agenthub/internal/pipeline	0.202s
ok  	github.com/namth/agenthub/internal/runner	11.142s
ok  	github.com/namth/agenthub/internal/server	0.036s
ok  	github.com/namth/agenthub/internal/tasks	0.106s
ok  	github.com/namth/agenthub/internal/telegram	0.259s
ok  	github.com/namth/agenthub/web	0.008s
== 5. Binary thật: server + runner
TASK-021 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
