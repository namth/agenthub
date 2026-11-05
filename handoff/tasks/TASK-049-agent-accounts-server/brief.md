# TASK-049 — Server: tài khoản agent linh động

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-11: DEC-41 → DEC-43, BR-66 → BR-70 · TC-97 → TC-100
> Làm song song với TASK-050 (runner). Giao thức chốt ở `docs/features/agent-accounts.md` §6 — không đổi.

## 1. Mục tiêu
Một máy có nhiều agent cùng loại (id `<máy>/<name>`); agent chưa từng đăng nhập OK bị ẩn và không nhận việc; owner/chủ máy xoá được agent, máy không tự đăng ký lại.

## 2. Tài liệu bắt buộc đọc
- `docs/features/agent-accounts.md` (toàn bộ, ngắn)
- Test đặc tả: `internal/database/migrate006_test.go`, `internal/dispatcher/accounts_test.go`, `internal/api/fleet_accounts_test.go`, `internal/machines/accounts_test.go`, `internal/gateway/accounts_test.go`, `internal/office/accounts_test.go`
- Mẫu migration dựng lại bảng với FK tắt: `internal/database/migrations/005_ops_remote.sql` + cách `Migrate` xử lý marker `-- agenthub:foreign_keys=off`

## 3. Phạm vi file
**Được tạo/sửa:** `internal/database/migrations/006_agent_accounts.sql`; file không phải test trong `internal/dispatcher/`, `internal/api/`, `internal/machines/` (trừ `install.sh`), `internal/gateway/`, `internal/office/`; `cmd/server/main.go`.
**Không được sửa:** mọi file test; `internal/runner/`, `internal/machines/install.sh` (TASK-050); `web/`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**Migration 006** (marker `-- agenthub:foreign_keys=off`, dựng lại `agents`, copy mọi cột cũ):
- thêm `name TEXT` (cho phép NULL), `ever_ok INTEGER NOT NULL DEFAULT 1`, `removed_at DATETIME`; bỏ `UNIQUE(worker_id, kind)`, thêm `UNIQUE(worker_id, name)`.
- trigger `AFTER INSERT ON agents WHEN NEW.name IS NULL` → `UPDATE agents SET name = substr(NEW.id, instr(NEW.id, '/') + 1) WHERE id = NEW.id`.
- dữ liệu cũ: `name` = phần sau `/`; `ever_ok = 0` cho agent `auth_status <> 'OK'` **và** không có assignment nào; còn lại 1.

**dispatcher** (`types.go`, `service.go`):
```go
type AgentHello struct { Kind, Name, AuthStatus, AuthDetail string; EverOK *bool } // Name "" = kind viết thường
const EvAgentRemoved = "AGENT_REMOVED" // WorkerID, AgentID
var ErrConflict = errors.New("CONFLICT")
func (s *Service) RemoveAgent(agentID string) error            // ErrNotFound (không có/đã xoá), ErrConflict (assignment ACTIVE)
func (s *Service) Tombstoned(workerID string, ids []string) []string // id trong ids đã removed_at
```
- `ConnectAuth`: id = `workerID + "/" + name`; name phải khớp `^[a-z0-9][a-z0-9-]{0,31}$` (sai → bỏ qua phần tử, không lỗi cả HELLO). Upsert theo `id` (không còn `ON CONFLICT(worker_id, kind)`). `removed_at` khác NULL → bỏ qua (không hồi sinh, không xoá tombstone). `ever_ok`: `EverOK` true **hoặc** `AuthStatus == OK` → 1; `EverOK` false và hàng mới → 0; không bao giờ hạ 1 → 0. Trả về chỉ agent `ever_ok = 1 AND removed_at IS NULL`.
- `Connect(workerID, kinds)` (đường cũ, không có thông tin đăng nhập): giữ hành vi, hàng mới `ever_ok = 1`.
- `RequestJob` / chọn lead: bỏ agent `ever_ok = 0` hoặc `removed_at` khác NULL (BR-68).
- `RemoveAgent`: có assignment ACTIVE → `ErrConflict`; agent đang giữ lead → nhả lease như `AdminRevoke` (lead mới bầu lại bình thường); đặt `removed_at`, `status='IDLE'`; phát `EvAgentRemoved`.

**gateway**: `Notify(EvAgentRemoved)` → gửi `{"event":"AGENT_REMOVE","agent_id":…}` tới máy `WorkerID`. Handshake: đọc thêm `name`, `ever_ok` (bool, có thể thiếu) trong `data.agents[]`; sau `ConnectAuth`, nếu API cài interface tuỳ chọn `interface{ Tombstoned(string, []string) []string }` → gửi `AGENT_REMOVE` cho từng id trả về.

**api** (`fleet.go`): `GET /api/fleet` chỉ agent `ever_ok = 1 AND removed_at IS NULL`, thêm trường `name`. `DELETE /api/admin/agents/{id}` (id URL-encoded như PATCH) → ADMIN; `RemoveAgent` → `200 {"removed": id}`, `ErrNotFound` → 404 `NOT_FOUND`, `ErrConflict` → 409 `CONFLICT`; khác ADMIN → 403.

**machines** (`handler.go`): `Config.RemoveAgent func(agentID string) error`. `DELETE /api/my/machines/{id}/agents/{name}`: máy không thuộc người gọi (và không phải owner) → 403; agent không có/đã xoá → 404 (không gọi `RemoveAgent`); lỗi `errors.Is(err, dispatcher.ErrConflict)` → 409 `CONFLICT`; OK → `200 {"removed": id}`. `GET /api/my/machines`: agent lọc như fleet, thêm `name`. Join máy (`POST /api/fleet/join`): hàng agent mới `ever_ok = 0`.

**office** (`snapshot.go`): chỉ agent `ever_ok = 1 AND removed_at IS NULL`, thêm `name` vào `AgentSnapshot`.

**main**: `machines.Config.RemoveAgent = dispatcher.RemoveAgent`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-049-agent-accounts-server/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Test cũ chèn `INSERT INTO agents (id, worker_id, kind)` không có `name` — phải chạy được nhờ trigger + `ever_ok DEFAULT 1`. Không sửa test cũ.
- Không dùng `ON CONFLICT(worker_id, kind)` nữa (ràng buộc đã bỏ → lỗi SQL).
- Không làm hồi sinh agent đã xoá qua bất kỳ đường nào (HELLO, Connect, SetAuth).
- Mọi test với `-race` phải xanh.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc` — mỗi lần một lệnh; không ghép `&&` `||` `|`; không `which`, `test`, `cp`, `npm`, `npx`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
