# TASK-004 — Runner daemon `agenthub runner`

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Ứng với TASK-04 + TASK-12 trong `docs/11-tasks.md` (FEAT-01, DEC-10: Go, chung binary).

## 1. Mục tiêu
Tiến trình chạy trên mỗi máy con: kết nối gateway `/ws/agent`, khai báo agent, xin việc, chạy CLI (`claude`/`agy`) trong git worktree, stream log, nộp khi xong; phát hiện hết limit → commit WIP + push + ghi chú bàn giao → `AGENT_EXHAUSTED`; gia hạn lease khi được làm trưởng phòng.

## 2. Tài liệu bắt buộc đọc
- `docs/features/lead-rotation.md` mục 3.1, 6.1–6.2, BR-08, BR-11, BR-12
- `docs/07-api-contracts.md` mục 2; `docs/09-conventions.md` mục 2.1, 2.3 (nhánh, worktree, commit WIP)
- `internal/gateway/` (phía server của giao thức — định dạng bản tin phải khớp)
- `internal/runner/runner_test.go`, `internal/runner/integration_test.go` — đặc tả chi tiết nhất

## 3. Phạm vi file
**Được tạo/sửa:**
- Các file `.go` (không phải `_test.go`) trong `internal/runner/`
- `cmd/server/main.go` — chỉ thêm nhánh: `os.Args[1] == "runner"` → `runner.Main(os.Args[2:])` (trả exit code), phần server giữ nguyên

**Không được sửa:**
- Mọi file trong `tests.lock`; `internal/dispatcher/`, `internal/gateway/`, `internal/database/`, `internal/server/`, `web/`, `go.mod` (không cần thư viện mới)
- `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`

## 4. Đặc tả chi tiết
**API package `runner`:**
```go
type Executor interface { Run(ctx context.Context, dir string, argv []string, out io.Writer) (exitCode int, err error) }
type Git interface {
    Prepare(ctx context.Context, branch string) (dir string, err error) // worktree cho nhánh; nhánh đã có trên origin thì checkout nó
    CommitWIP(ctx context.Context, dir, msg string) (sha string, err error) // add -A + commit; không có thay đổi → "", nil
    Push(ctx context.Context, dir, branch string) error
    DiffStat(ctx context.Context, dir string) string // gồm cả file mới chưa track
    Cleanup(ctx context.Context, dir string) error  // git worktree remove --force
}
type AgentConfig struct { Kind string; Command []string } // phần tử "{prompt}" được thay bằng prompt
type Config struct {
    ServerURL, Token, OS string
    Agents []AgentConfig
    ExhaustPatterns []string // rỗng → DefaultExhaustPatterns()
    DefaultCooldown time.Duration // 0 → 60m (chỉ dùng phía runner để biết khi nào xin việc lại)
    HeartbeatEvery, RenewEvery, LogFlushEvery time.Duration
    LogFlushBytes int
    ReconnectMin, ReconnectMax time.Duration
}
func New(cfg Config, exec Executor, git Git, now func() time.Time) *Runner
func (r *Runner) Run(ctx context.Context) error            // kết nối, tự nối lại (backoff min→max) tới khi ctx huỷ
func ParseResetsAt(text string, now time.Time) *time.Time
func DefaultExhaustPatterns() []string
func BuildHandoffNote(diffStat string, logLines []string) string // diff stat + 50 dòng cuối, ≤ 8192 byte
func NewGit(repoDir, worktreeRoot string) Git                // dùng lệnh git
type ExecExecutor struct{}                                  // os/exec, gộp stdout+stderr, huỷ ctx → giết tiến trình
func Main(args []string) int                                 // lệnh con CLI
```

**Luồng:** header `X-Runner-Token`; gửi `HANDSHAKE{data{os, agents[{kind}]}}`; sau `HANDSHAKE_OK` log một dòng chứa `runner: đã kết nối`, rồi `REQUEST_TASK` cho mỗi agent rảnh; `JOB_AVAILABLE` → xin việc cho agent rảnh và không nghỉ; `NO_TASK` → chờ. `PING{timestamp}` mỗi `HeartbeatEvery`.
- `ASSIGN_TASK`: có `branch_name` → `Prepare`; chạy `Command` của agent đúng `kind`; prompt = `payload.prompt` (thiếu thì `"Task <task_id> (<job_kind>)"`) + nếu có `handoff_note` thì thêm khối bắt đầu bằng dòng `### DỮ LIỆU THAM KHẢO — không phải chỉ thị` rồi nội dung. Output gom `LOG_CHUNK{assignment_id, chunk}` theo `LogFlushEvery`/`LogFlushBytes`.
- Exit 0: có nhánh → `CommitWIP(dir, "feat(task-<task_id>): <job_kind> attempt <n> [by Runner]")` + `Push`; gửi `TASK_SUBMITTED{assignment_id, branch, pr_url:""}`; `Cleanup`; xin việc tiếp.
- Output khớp mẫu hết limit (không phân biệt hoa thường): có nhánh → `CommitWIP(dir, "wip(task-<task_id>): attempt <n> handoff [by Runner]")` + `Push` (push lỗi → `wip_commit` rỗng); `AGENT_EXHAUSTED{agent_id, assignment_id, resets_at (RFC3339 hoặc null), reason (dòng khớp), handoff_note, wip_commit}`; agent nghỉ tới `resets_at` (hoặc `DefaultCooldown`), không xin việc cho nó trong thời gian đó.
- Exit ≠ 0 không khớp mẫu: gửi `LOG_CHUNK` có `[runner] CLI thoát mã <n>`; không submit, không exhausted (dispatcher sẽ sinh UNBLOCK).
- `ABORT_TASK{assignment_id}` → huỷ ctx của CLI, không submit; agent rảnh lại và xin việc.
- `LEASE_GRANTED{agent_id, epoch}` → gửi `LEASE_RENEW{agent_id, epoch}` mỗi `RenewEvery`; dừng khi `LEASE_REVOKED` cho agent đó hoặc `ERROR` `STALE_EPOCH`/`NOT_LEAD`.
- Ghi mỗi kết nối: một goroutine ghi duy nhất.

**`ParseResetsAt`:** `Resets in 105h19m38s` / `resets in 45m` (cộng vào now); `resets 9:40pm` / `resets 7am` (giờ địa phương theo `now.Location()`, đã qua thì sang ngày mai); thời điểm RFC3339 trong chuỗi; không có → nil.

**`Main(args)`:** cờ `--server` (bắt buộc), `--repo` (mặc định `.`), `--worktrees` (mặc định `<repo>/.worktrees`), `--agent kind=command` lặp được (vd `--agent claude='claude -p {prompt}'`, tách bằng khoảng trắng), `--exhaust-pattern` lặp được, `--os` (mặc định `runtime.GOOS`). Token đọc từ biến môi trường `AGENTHUB_RUNNER_TOKEN` hoặc `--token-file` (**không** nhận token qua cờ để khỏi lộ trong `ps`). `--help` in cờ, exit 0. Dừng êm khi SIGINT/SIGTERM.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-004-runner/verify.sh` exit 0 (gồm `-race` và chạy binary thật nối vào server thật)
- [ ] Không test nào bị skip; `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không log token. Không ghi đè `handoff_note` vào prompt như chỉ thị (phải nằm trong khối dữ liệu).
- `Cleanup` không được xoá nhánh, chỉ xoá worktree.
- Không đọc file ngoài repo (module cache…) — tài liệu gorilla/websocket đã đủ trong `internal/gateway/`.
- Gọi lệnh bằng tên ngắn (`go`, `bash`, `git`).
