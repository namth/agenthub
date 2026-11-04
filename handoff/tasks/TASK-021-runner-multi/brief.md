# TASK-021 — Runner đa repo, phiên bản + tự cập nhật, lệnh `--status`

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · DEC-15, DEC-19, BR-28, BR-35, FEAT-05 §4

## 1. Mục tiêu
Một runner mỗi máy làm được việc của mọi dự án (tự clone repo), tự cập nhật khi server lên phiên bản mới, và có lệnh `--status` để người dùng/agent local kiểm máy đã nối chưa.

## 2. Tài liệu bắt buộc đọc
- `docs/14-decisions.md` DEC-15, DEC-19; `docs/features/multi-project.md` §6–7; `docs/features/join-machine.md` §4, §6–7
- `internal/runner/` hiện có; `internal/runner/multi_test.go` (đặc tả chi tiết nhất)

## 3. Phạm vi file
**Được sửa:** các file `.go` (không phải `_test.go`) trong `internal/runner/`; `cmd/server/main.go` chỉ khi cần truyền `-ldflags -X` (không bắt buộc).

**Không được sửa:** file trong `tests.lock` (mọi test runner cũ); package khác; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- `var Version = "dev"` (đặt bằng `-ldflags "-X github.com/namth/agenthub/internal/runner.Version=…"`); `var ErrUpdated = errors.New("UPDATED")`.
- `Config` thêm: `GitFor func(repo string) (Git, error)` · `SelfPath string` (file binary để thay khi cập nhật; rỗng → `os.Executable()`) · `NoUpdate bool` · `StatusFile string`.
- **Đa repo:** `ASSIGN_TASK.payload.repo` khác rỗng và có `GitFor` → dùng `GitFor(repo)`; lỗi → `LOG_CHUNK` `[runner] không chuẩn bị được repo <repo>: …`, không nộp. Không có `repo` hoặc không có `GitFor` → dùng `git` truyền vào `New` (giữ tương thích).
- `func NewRepoCache(home string, cloneURL func(repo string) string, exec Executor) func(repo string) (Git, error)`: `repo` phải khớp `^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$` và không chứa `..`; chưa có `<home>/repos/<owner>/<name>/.git` → `git clone <cloneURL(repo)>` vào đó; trả `NewGit(<repo dir>, <home>/worktrees/<owner>-<name>)`; an toàn khi gọi đồng thời (mutex theo repo).
- **Phiên bản:** `HANDSHAKE.data.version = Version`. `HANDSHAKE_OK.server_version` khác `Version` và `!NoUpdate` → khi **mọi agent IDLE** (không job đang chạy): tải `http(s)://<host>/download/agenthub-<GOOS>-<GOARCH>` và `….sha256` (suy từ `ServerURL`: `ws→http`, `wss→https`, bỏ path), kiểm SHA-256 (dòng đầu, token đầu), ghi file tạm cùng thư mục `SelfPath`, `chmod 0755`, `rename` đè `SelfPath`, rồi `Run` trả `ErrUpdated`. Tải/kiểm lỗi → giữ bản cũ, log, thử lại sau 30 phút (không thoát).
- **Status:** mỗi lần bắt tay xong và mỗi nhịp tim ghi `StatusFile` (nếu đặt) JSON `{connected, server, version, agents:[{id,status}], updated_at}` (RFC3339), quyền 0600, **không** chứa token; mất kết nối → `connected:false`.
- **`Main`:** cờ mới `--home` (mặc định `~/.agenthub`), `--status`, `--status-file` (mặc định `<home>/status.json`), `--no-update`; `--repo` thành tuỳ chọn. Mặc định dùng `NewRepoCache(home, func(r) "https://github.com/"+r+".git", ExecExecutor{})`, token mặc định đọc `<home>/token` nếu không có `AGENTHUB_RUNNER_TOKEN`/`--token-file`. `--status`: đọc file, in `ONLINE <server> agents=…` và exit 0 nếu `connected` và `updated_at` trong 2 phút; ngược lại in lý do, exit 1. `Run` trả `ErrUpdated` → `Main` exit 0 (dịch vụ khởi động lại).

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-021-runner-multi/verify.sh` exit 0 (gồm `-race`)
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không ghi đè binary khi sai checksum; không cập nhật khi đang có job.
- Không in token ra stdout/log/status.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
