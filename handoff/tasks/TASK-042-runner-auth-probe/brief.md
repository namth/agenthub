# TASK-042 — Runner: kiểm đăng nhập bằng lời gọi thật + mẫu lỗi OAuth

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-09: DEC-35, BR-57 · TC-85

## 1. Mục tiêu
`claude auth status` có thể báo `loggedIn: true` trong khi gọi thật lỗi "Failed to authenticate: OAuth session expired and could not be refreshed" (đã gặp trên iMac-8GB). Runner phải phát hiện đúng bằng một lời gọi thật cực ngắn, và nhận ra lỗi OAuth trong job.

## 2. Tài liệu bắt buộc đọc
- `docs/features/ops-readiness.md` §7 BR-57; `docs/features/agent-auth.md` §3, §7 (BR-38, BR-39)
- Test đặc tả: `internal/runner/probe_test.go` (dùng lại helper `startAuth`, `loggedIn`, `hsAgents`, `authFor` của `auth_test.go`); code: `internal/runner/auth.go`, `internal/runner/runner.go`, `internal/runner/main.go`

## 3. Phạm vi file
**Được tạo/sửa:** file không phải `_test.go` trong `internal/runner/`.
**Không được sửa:** file trong `tests.lock` và mọi `_test.go`; package khác; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
func DefaultAuthProbes() map[string][]string // {"CLAUDE": {"claude","-p","Trả lời đúng một từ: OK"}, "ANTIGRAVITY": {"agy","-p","Trả lời đúng một từ: OK"}}
func ParseProbeResult(exit int, output string, err error) (status, detail string)
```
- `Config` thêm `AuthProbes map[string][]string` (**nil = không probe**, giữ hành vi cũ), `ProbeEvery` (mặc định 6h), `ProbeTimeout` (mặc định 60s). `Main` đặt `AuthProbes: DefaultAuthProbes()`.
- `ParseProbeResult`: `err != nil` (kể cả timeout) → `UNKNOWN`; output khớp `DefaultAuthPatterns()` (không phân biệt hoa thường) → `NEEDS_AUTH`, `detail` = dòng khớp (≤ 80 ký tự); exit 0 → `OK`; còn lại `UNKNOWN`.
- `DefaultAuthPatterns()` thêm: `failed to authenticate`, `oauth`, `session expired`, `token expired`, `could not be refreshed` (giữ các mẫu cũ). Các mẫu này cũng áp dụng cho lỗi job (BR-39).
- **Lúc bắt tay:** sau lệnh kiểm `AuthChecks`, với mỗi kind có probe: chạy probe (qua `Executor`, timeout `ProbeTimeout`). Kết quả probe `OK` hoặc `NEEDS_AUTH` **ghi đè** kết quả lệnh kiểm; probe `UNKNOWN` giữ kết quả lệnh kiểm.
- **Định kỳ** `ProbeEvery`, chỉ khi agent không BUSY: chạy lại probe, gửi `AGENT_AUTH` **chỉ khi đổi**; từ `NEEDS_AUTH` sang khác → xin việc lại như BR-38.
- `AUTH_RECHECK` chạy cả lệnh kiểm lẫn probe (agent có probe thì không còn trả `UNKNOWN` cố định).
- Không chạy probe trong khi giữ `r.mu`; không chạy probe trước mỗi `REQUEST_TASK` (tốn token).

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-042-runner-auth-probe/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- `AuthProbes == nil` thì không chạy gì thêm: mọi test runner cũ (dùng `fakeExec` chung) phải xanh với `-race`.
- Không dùng chế độ bỏ qua quyền trong lệnh probe.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc` (không `test`, `cp`, `npm`).
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
