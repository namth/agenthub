# TASK-050 — Runner: nhiều tài khoản agent trên một máy

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-11: DEC-41 → DEC-43, BR-66, BR-67, BR-69 · TC-101, TC-102
> Làm song song với TASK-049 (server). Giao thức chốt ở `docs/features/agent-accounts.md` §6 — không đổi.

## 1. Mục tiêu
Runner chạy được nhiều tài khoản cùng loại (mỗi tài khoản một thư mục cấu hình), chỉ báo lên server tài khoản đã đăng nhập OK, có lệnh `agenthub account …` để thêm/xoá/đăng nhập lại, và tự gỡ tài khoản khi server gửi `AGENT_REMOVE`.

## 2. Tài liệu bắt buộc đọc
- `docs/features/agent-accounts.md` §1, §3, §5, §6, §7
- Test đặc tả: `internal/runner/accounts_test.go`, `internal/machines/install_accounts_test.go`

## 3. Phạm vi file
**Được tạo/sửa:** file không phải test trong `internal/runner/`; `internal/machines/install.sh`; `cmd/server/main.go` (chỉ thêm nhánh `os.Args[1] == "account"`).
**Không được sửa:** mọi file test; package khác; `web/`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**Kiểu mới (`internal/runner`):**
```go
type AgentConfig struct { Kind, Name string; Command []string; ConfigDir string; EverOK *bool } // Name "" = kind viết thường
type EnvExecutor interface { RunEnv(ctx context.Context, dir string, argv, env []string, out io.Writer) (int, error) }
type AccountEntry struct { Name, Kind string; Command []string; ConfigDir string `json:"config_dir"`; EverOK bool `json:"ever_ok"` } // json: name, kind, command
type AccountFile struct { Agents []AccountEntry `json:"agents"`; Removed []string `json:"removed"` }
func LoadAccounts(path string) (AccountFile, error) // không có file → rỗng, nil
func SaveAccounts(path string, f AccountFile) error  // ghi tạm rồi rename, quyền 0600
func (f AccountFile) NextName(kind string) string    // chưa có tài khoản kind đó → kind viết thường; có rồi → "<kind>-2", "-3"… bỏ qua tên trong Agents và Removed
func (f AccountFile) AgentConfigs() []AgentConfig    // EverOK luôn khác nil (lấy từ file)
// Config thêm: AccountsPath string — "" = không ghi file
```
- Id agent phía runner = `<worker>/<Name>`; khớp `HANDSHAKE_OK`/`ASSIGN_TASK` theo **name** (bỏ cách khớp hậu tố `/kind`).
- **Env theo tài khoản:** `ConfigDir` khác rỗng → CLAUDE thêm `CLAUDE_CONFIG_DIR=<dir>`, ANTIGRAVITY thêm `HOME=<dir>` (đường dẫn `~` mở rộng). Áp dụng cho **job, lệnh kiểm đăng nhập và probe** của agent đó. Executor cài `EnvExecutor` → gọi `RunEnv(…, os.Environ() + env)`; không cài → gọi `Run` với argv có tiền tố `env K=V …`. `ExecExecutor` cài `RunEnv`.
- **HANDSHAKE:** mỗi agent gửi `kind`, `name`, `auth_status`, `auth_detail`; `ever_ok` chỉ gửi khi `EverOK != nil`. Agent `EverOK` = false **không** được gửi và không xin việc. Probe/kiểm đăng nhập vẫn chạy cho agent false; khi ra OK → đặt true, ghi `AccountsPath` (nếu có), đóng kết nối để HELLO lại. Agent `EverOK == nil` (cờ `--agent` cũ) gửi bình thường, không có trường `ever_ok`.
- **`AGENT_REMOVE {agent_id}`:** dừng xin việc cho agent đó, bỏ khỏi danh sách đang chạy; có `AccountsPath` → bỏ khỏi `Agents`, thêm name vào `Removed`, `SaveAccounts`. Không xoá thư mục `config_dir`.
- **Khởi động (`Main`):** có `$AGENTHUB_HOME/agents.json` (mặc định `~/.agenthub`) → dùng nó (bỏ qua `--agent`), `AccountsPath` = file đó. Không có file → dùng `--agent` như cũ (EverOK nil).

**CLI `agenthub account <lệnh>`** (`cmd/server/main.go` gọi `runner.AccountMain(os.Args[2:])`; `--help` in đủ 4 lệnh):
- `add <claude|antigravity>`: name = `NextName`; tài khoản đầu tiên của kind dùng `config_dir` rỗng, các tài khoản sau `~/.agenthub/accounts/<name>`; chạy CLI tương tác với env tương ứng (`claude` / `agy`), stdin/stdout nối terminal, in hướng dẫn "đăng nhập xong thì thoát CLI"; xong chạy probe: OK → `ever_ok: true`; ghi file; in "Khởi động lại runner: …" (macOS `launchctl kickstart -k gui/$(id -u)/io.agenthub.runner`, Linux `systemctl --user restart agenthub-runner`).
- `login <name>`: chạy lại CLI tương tác + probe cho tài khoản đó, cập nhật `ever_ok`.
- `list`: bảng name, kind, config_dir, ever_ok.
- `remove <name>`: bỏ khỏi file, thêm vào `removed` (server vẫn giữ agent tới khi owner xoá trên web — in nhắc).

**`install.sh`:** với mỗi CLI có cài (`claude`, `agy`) chạy probe ngắn (`claude -p "Trả lời đúng một chữ: OK"`, `agy -p "…"`, timeout 60s). Chỉ CLI probe OK (exit 0) mới vào `agents` của payload join và vào `$AGENTHUB_HOME/agents.json` (`name` = kind viết thường, `ever_ok: true`, `command` như cờ `--agent` hiện tại), quyền 0600. CLI có cài mà chưa đăng nhập → in "<cli> chưa đăng nhập — sau khi đăng nhập chạy: agenthub account add <claude|antigravity>". Dịch vụ launchd/systemd không truyền `--agent` khi đã có `agents.json`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-050-agent-accounts-runner/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không in token; không đọc/ghi gì trong thư mục cấu hình CLI ngoài việc đặt env.
- Không dùng `--dangerously-skip-permissions` hay `bypassPermissions` ở bất kỳ đâu (kể cả probe).
- Test cũ của runner (`runner_test.go`, `auth_test.go`, `probe_test.go`, `allowlist_test.go`…) dùng `AgentConfig{Kind, Command}` không có Name → phải vẫn xanh (id `mac/claude`, `mac/antigravity`).
- `install.sh` phải chạy được bằng `sh` (POSIX, không bash-ism); kiểm `sh -n`.
- Mọi test `-race` xanh.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`, `sh` — mỗi lần một lệnh; không ghép `&&` `||` `|`; không `which`, `test`, `cp`, `npm`, `npx`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
