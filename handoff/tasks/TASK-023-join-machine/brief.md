# TASK-023 — Thêm máy: mã tham gia, `/api/fleet/join`, `install.sh`, tải binary, API Máy của tôi

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-05: DEC-18, DEC-20, BR-30 → BR-32, BR-34

## 1. Mục tiêu
Mọi người dùng tự thêm máy của mình bằng một lệnh (hoặc dán khối hướng dẫn cho agent local); máy có chủ; chủ máy tạm dừng/gỡ/chọn dự án cho máy mình.

## 2. Tài liệu bắt buộc đọc
- `docs/features/join-machine.md` (toàn bộ, nhất là §3 luồng, §4 khối hướng dẫn cho agent, §6 API, §7 BR); `docs/14-decisions.md` DEC-18, DEC-20
- `internal/database/migrations/003_join_machine.sql`; `internal/gateway/register.go` (cách sinh/lưu token hiện có: `HashToken`)
- `internal/machines/machines_test.go` (đặc tả chi tiết nhất)

## 3. Phạm vi file
**Được tạo:** các file trong `internal/machines/` (mới; được dùng `embed` cho `install.sh`).

**Không được sửa:** file trong `tests.lock`; mọi package khác; `cmd/`, `web/`, `go.mod`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
type Config struct {
    BaseURL    func() string                   // URL công khai, vd https://agent.oa.io.vn
    RunnersDir string                          // chứa agenthub-<os>-<arch> và .sha256
    Now        func() time.Time
    IsOwner    func(userID string) bool
    IsMember   func(userID, projectID string) bool
    OnRemoved  func(workerID string)           // main nối: đóng kết nối gateway + thu hồi việc
}
func Handler(db *sql.DB, cfg Config, authz func(*http.Request) (userID string, ok bool)) http.Handler
```
- `POST /api/fleet/join-codes {name}` (đăng nhập): `name` khớp `^[a-zA-Z0-9-]{2,40}$`; tên đã là máy của người khác → 409 `CONFLICT`. Mã 8 ký tự `XXXX-XXXX` từ bảng `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (`crypto/rand`), lưu `gateway.HashToken(code)`, hết hạn `Now()+15m`. Trả **201** `{code, expires_at (RFC3339), command, agent_prompt}`; `command` = `curl -fsSL <BaseURL>/install.sh | sh -s -- --join <code> --name <name>`; `agent_prompt` theo mẫu `docs/features/join-machine.md` §4 (chứa mã, BaseURL, lệnh cài, `gh auth login`, `claude` → `/login`, `agy`, `agenthub runner --status`, cấm in token, cấm `--dangerously-skip-permissions`).
- `POST /api/fleet/join {code, name, os, arch, agents}` (không cần đăng nhập): mã sai/hết hạn/đã dùng → 401 `UNAUTHORIZED`; sai > 5 lần/phút/IP (`X-Forwarded-For` đầu tiên, không có thì `RemoteAddr`) → 429 `TOO_MANY_REQUESTS`. Đánh dấu dùng bằng `UPDATE … WHERE used_at IS NULL`. Tạo/ghi đè máy `id = name` thuộc người tạo mã: token ngẫu nhiên 32 byte hex, lưu hash; tạo dòng `agents` cho từng kind (`<name>/<kind thường>`), `lead_eligible` = 1 nếu chủ là owner, ngược lại 0. Trả `{worker_id, runner_token}`.
- `GET /install.sh`: `text/x-shellscript`, `#!/bin/sh` POSIX (`sh -n` hợp lệ), chèn sẵn BaseURL. Cờ: `--join`, `--name`, `--no-service`. Môi trường ghi đè cho test: `AGENTHUB_HOME` (mặc định `$HOME/.agenthub`), `AGENTHUB_UNAME_S`, `AGENTHUB_UNAME_M`. Các bước: nhận diện OS (`Darwin|Linux` → `darwin|linux`) và CPU (`arm64|aarch64` → `arm64`, `x86_64` → `amd64`); tải `/download/agenthub-<os>-<arch>` và `.sha256`; kiểm SHA-256 (`shasum -a 256` hoặc `sha256sum`) — sai → in lỗi, thoát ≠ 0, **không** gọi join; phát hiện agent (`command -v claude`, `command -v agy`); `POST /api/fleet/join` (dùng `curl`, JSON); ghi token vào `$AGENTHUB_HOME/token` với `umask 077`; **không in token**; trừ khi `--no-service`: cài LaunchAgent `io.agenthub.runner` (macOS, `caffeinate -ims` bọc runner) hoặc `systemd --user` (Linux) chạy `agenthub runner --server <wss URL>/ws/agent --agent …` cho agent phát hiện được; cuối cùng in bảng kiểm `git / gh / claude / agy / go / node` (có/không, đã đăng nhập hay chưa với gh) kèm lệnh gợi ý cài.
- `GET /download/<file>`: chỉ phục vụ file đúng mẫu `agenthub-(darwin|linux)-(arm64|amd64)(.sha256)?` trong `RunnersDir`; còn lại 404.
- `GET /api/my/machines` (đăng nhập): máy có `owner_user_id = userID` → `[{id, name, os, conn_status, version, paused, agents:[{id, kind, status, projects:[…], current_job}]}]`.
- `PATCH /api/my/machines/{id} {paused?, projects?}`: chủ máy (owner cũng được); người khác → 403. `projects` (mảng id hoặc null): mọi id phải `IsMember` → không thì 403; ghi `agent_projects` cho **mọi agent** của máy (null = xoá hết = mọi dự án).
- `DELETE /api/my/machines/{id}`: chủ máy hoặc owner → assignment ACTIVE của máy thành `RECLAIMED` + job về `QUEUED`, đổi `token_hash` sang giá trị ngẫu nhiên không dùng được, gọi `OnRemoved(id)`, trả 204.
- JSON chuẩn docs/07; 401 chưa đăng nhập.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-023-join-machine/verify.sh` exit 0 (gồm `-race`)
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không log mã tham gia hay token; script không in token.
- `install.sh` không cài phần mềm nào khác (chỉ in gợi ý).
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
