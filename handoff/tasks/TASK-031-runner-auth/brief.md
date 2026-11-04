# TASK-031 — Runner: kiểm đăng nhập CLI, auth_patterns, AUTH_RECHECK

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-06: DEC-23, BR-38, BR-39

## 1. Mục tiêu
Runner tự biết agent nào chưa đăng nhập (`claude auth status`, `gh auth status`), báo server qua HANDSHAKE/`AGENT_AUTH`, không xin việc khi `NEEDS_AUTH`, và nhận ra job lỗi vì mất đăng nhập để trả job về hàng đợi.

## 2. Tài liệu bắt buộc đọc
- `docs/features/agent-auth.md` §3 (bảng "Kiểm đăng nhập theo CLI"), §6 (WebSocket), §7 BR-38, BR-39
- Test đặc tả: `internal/runner/auth_test.go`; code: `internal/runner/runner.go`, `internal/runner/utils.go`, `internal/runner/main.go`

## 3. Phạm vi file
**Được tạo/sửa:** file không phải `_test.go` trong `internal/runner/`.

**Không được sửa:** file trong `tests.lock` và mọi `_test.go` có sẵn; package khác; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**API công khai mới (package `runner`):**
```go
const (AuthOK = "OK"; AuthNeeds = "NEEDS_AUTH"; AuthUnknown = "UNKNOWN"; AuthKindGH = "GH")
func DefaultAuthChecks() map[string][]string   // {"CLAUDE": {"claude","auth","status"}, "GH": {"gh","auth","status"}} — KHÔNG có ANTIGRAVITY
func DefaultAuthPatterns() []string            // gồm: "not logged in", "please run /login", "authentication required", "invalid api key", "gh auth login"
func ParseAuthResult(kind string, exit int, output string, err error) (status, detail string)
```
`Config` thêm: `AuthChecks map[string][]string` (khoá = kind agent hoặc `"GH"`; **nil = không kiểm gì**, mọi giá trị `UNKNOWN`), `AuthPatterns []string` (rỗng → mặc định), `AuthCheckEvery` (mặc định 10m), `AuthFreshFor` (mặc định 60s), `AuthCheckTimeout` (mặc định 15s). `Main` đặt `AuthChecks: DefaultAuthChecks()`.

**`ParseAuthResult`:** `err != nil` → `UNKNOWN`. `CLAUDE`: tìm JSON trong output, `"loggedIn": true` → `OK`, `false` → `NEEDS_AUTH`, không đọc được → `UNKNOWN` (không phụ thuộc exit code). `GH`: exit 0 → `OK`; output chứa "not logged" (không phân biệt hoa thường) → `NEEDS_AUTH`; còn lại `UNKNOWN`. Kind khác: exit 0 → `OK`; khớp `DefaultAuthPatterns` → `NEEDS_AUTH`; còn lại `UNKNOWN`. `detail` ≤ 80 ký tự (vd `"claude: loggedIn=false"`, dòng lỗi đầu tiên), rỗng khi `OK`.

**Hành vi:**
1. Trước khi gửi `HANDSHAKE`, chạy các lệnh kiểm (mỗi lệnh qua `Executor`, timeout `AuthCheckTimeout`, hết giờ → `UNKNOWN`). HANDSHAKE gửi `data.gh_auth` và `data.agents[].auth_status`, `auth_detail` (bỏ `auth_detail` khi rỗng). Kind không có lệnh kiểm → `UNKNOWN`.
2. Agent `NEEDS_AUTH` hoặc máy `gh_auth = NEEDS_AUTH` → **không gửi `REQUEST_TASK`** (cả sau `HANDSHAKE_OK` lẫn `JOB_AVAILABLE`).
3. Định kỳ `AuthCheckEvery`: kiểm lại GH và các agent **không BUSY** có lệnh kiểm; **chỉ gửi `AGENT_AUTH` khi giá trị đổi** (`{"event":"AGENT_AUTH","scope":"agent","agent_id","auth_status","detail"}` hoặc `scope:"gh"` không có `agent_id`). Từ `NEEDS_AUTH` sang khác → gửi `REQUEST_TASK` cho agent đó nếu rảnh.
4. Trước mỗi `REQUEST_TASK`, nếu lần kiểm của agent cũ hơn `AuthFreshFor` → kiểm lại trước (agent không có lệnh kiểm thì bỏ qua bước này).
5. `AUTH_RECHECK {agent_id?}`: kiểm ngay agent đó (rỗng = GH + mọi agent), **luôn** trả `AGENT_AUTH` cho từng thứ đã kiểm kể cả không đổi. Agent không có lệnh kiểm (agy) → trả `UNKNOWN` và xoá trạng thái `NEEDS_AUTH` cục bộ. Sau đó xin việc cho agent rảnh không còn `NEEDS_AUTH`.
6. Job CLI exit ≠ 0, **không** khớp `ExhaustPatterns` (hết limit vẫn ưu tiên) nhưng output khớp `AuthPatterns` (không phân biệt hoa thường): nếu job có nhánh → `CommitWIP` + `Push` như khi hết limit; gửi `AGENT_AUTH {scope:"agent", agent_id, auth_status:"NEEDS_AUTH", assignment_id, detail, handoff_note, wip_commit}` (`wip_commit` rỗng nếu push lỗi; `handoff_note` = `BuildHandoffNote`); agent thành `NEEDS_AUTH` cục bộ, không `TASK_SUBMITTED`, không `AGENT_EXHAUSTED`.
7. `status.json` (`StatusFile`) ghi thêm `auth_status` cho từng agent và `gh_auth`; `--status` in kèm (vd `iMac-8GB/claude:IDLE:NEEDS_AUTH`).

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-031-runner-auth/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- `AuthChecks == nil` thì không chạy lệnh kiểm nào (các test runner cũ dùng `fakeExec` chung; chạy thêm lệnh sẽ ăn mất bước giả của chúng) — mọi test cũ phải xanh với `-race`.
- Không chạy lệnh kiểm trong khi giữ `r.mu`; không gửi token/secret trong `detail`.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
