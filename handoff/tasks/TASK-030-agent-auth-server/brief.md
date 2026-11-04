# TASK-030 — Server: trạng thái đăng nhập agent

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-06: DEC-22 → DEC-24, BR-37 → BR-41

## 1. Mục tiêu
Server biết agent nào chưa đăng nhập CLI (`NEEDS_AUTH`), không giao việc/không trao lead cho agent đó, báo Telegram, và cho chủ máy yêu cầu runner kiểm lại.

## 2. Tài liệu bắt buộc đọc
- `docs/features/agent-auth.md` §3, §5, §6, §7 (BR-37 → BR-41)
- Test đặc tả: `internal/dispatcher/auth_test.go`, `internal/gateway/auth_test.go`, `internal/telegram/auth_test.go`, `internal/machines/auth_test.go`, `internal/api/fleet_auth_test.go`, `internal/database/migrate004_test.go`

## 3. Phạm vi file
**Được tạo/sửa:**
- `internal/database/migrations/004_agent_auth_office.sql` (tạo mới, đúng nội dung `docs/features/agent-auth.md` §5)
- file không phải `_test.go` trong: `internal/dispatcher/`, `internal/gateway/`, `internal/telegram/`, `internal/machines/`, `internal/api/`
- `cmd/server/main.go` (chỉ nối dây: `machines.Config.Recheck` ← `hub.SendAuthRecheck`)

**Không được sửa:** file trong `tests.lock` và mọi `_test.go` có sẵn; `internal/runner/`, `internal/office/`, `web/`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**dispatcher (`types.go` + `service.go`)** — giữ nguyên interface `API`; thêm interface tuỳ chọn để không phá các fake hiện có:
```go
const (AuthOK = "OK"; AuthNeeds = "NEEDS_AUTH"; AuthUnknown = "UNKNOWN"; RevokeAuth = "AUTH")
const (EvAgentAuth = "AGENT_AUTH"; EvWorkerAuth = "WORKER_AUTH")
type AgentHello struct { Kind, AuthStatus, AuthDetail string } // AuthStatus "" = giữ giá trị cũ
type AuthAPI interface {
    ConnectAuth(workerID, ghAuth string, agents []AgentHello) ([]Agent, error) // ghAuth "" = giữ cũ
    SetAuth(workerID, agentID, status, detail string) error
    SetGhAuth(workerID, status, detail string) error
    AuthRequired(workerID, agentID, assignmentID, detail, handoffNote, wipCommit string) error
}
var ErrInvalidAuth = errors.New("VALIDATION_ERROR")
```
- `Agent` thêm trường `AuthStatus string`; `Event` thêm `AuthStatus`, `PrevAuthStatus`, `Detail string`.
- `Connect(w, kinds)` = `ConnectAuth(w, "", hello với AuthStatus "")`. `ConnectAuth` ghi `auth_status/auth_detail/auth_checked_at` **trong cùng transaction, trước** khi thử trao lease → agent `NEEDS_AUTH` không bao giờ nhận lease lúc bắt tay.
- `SetAuth`: agent không thuộc máy → `ErrAgentNotOwned`; status ngoài 3 giá trị → `ErrInvalidAuth`; luôn cập nhật `auth_checked_at`; **chỉ khi giá trị đổi** mới phát `EvAgentAuth {WorkerID, AgentID, AuthStatus, PrevAuthStatus, Detail}`. Agent đang giữ lease chuyển `NEEDS_AUTH` → thu hồi lease `RevokeAuth`, `epoch+1`, chọn người kế nhiệm (BR-40). Agent về `OK/UNKNOWN` khi lease `VACANT` → thử trao lease.
- `SetGhAuth`: tương tự cho `workers.gh_auth`, phát `EvWorkerAuth {WorkerID, AuthStatus, PrevAuthStatus, Detail}`.
- `AuthRequired`: fencing như `Exhausted` (BR-04); `SetAuth(NEEDS_AUTH)`; assignment → `RECLAIMED` (lưu `handoff_note`, `wip_commit`); job về `QUEUED` với **`jobs.attempt` giữ nguyên** (lần giao kế tiếp có `Assignment.Attempt` bằng lần trước; không tính vào `MaxAttempts`, không sinh `REPLAN`/`EvJobFailed`); phát `EvJobAvailable`.
- BR-37: `RequestJob` trả `nil, nil` khi agent `NEEDS_AUTH` hoặc máy `gh_auth = NEEDS_AUTH`. Bộ chọn lead và `AdminAssign` loại agent `NEEDS_AUTH` / máy `gh_auth = NEEDS_AUTH` (`ErrAgentNotEligible`).
- `detail` cắt ≤ 80 ký tự, che chuỗi giống secret (`ghp_…`, `sk-…`, ≥ 32 ký tự liền không khoảng trắng → `***`).

**gateway** — nếu `api` cài `dispatcher.AuthAPI` thì: HANDSHAKE đọc `data.gh_auth`, `data.agents[].auth_status/auth_detail` và gọi `ConnectAuth`; bản tin `AGENT_AUTH {scope, agent_id, auth_status, detail, assignment_id?, handoff_note?, wip_commit?}`: `scope=gh` → `SetGhAuth`; có `assignment_id` → `AuthRequired`; còn lại `SetAuth`. Lỗi → `ERROR` (`ErrInvalidAuth` → code `VALIDATION_ERROR`). Không cài `AuthAPI` → `AGENT_AUTH` bị bỏ qua (không lỗi). Thêm `func (h *Hub) SendAuthRecheck(workerID, agentID string) bool` — gửi `{"event":"AUTH_RECHECK","agent_id":…}` (bỏ `agent_id` khi rỗng); máy không kết nối → `false`.

**telegram** — `Notifier.Notify`: `EvAgentAuth`/`EvWorkerAuth` gửi tin khi `AuthStatus == NEEDS_AUTH` (tin chứa "🔑", tên agent/máy và lệnh gợi ý: `claude` → `/login`, `gh auth login`, hoặc `agy`) và khi `PrevAuthStatus == NEEDS_AUTH && AuthStatus == OK` (chứa "✅"). Các chuyển khác không gửi.

**machines** — `Config.Recheck func(workerID, agentID string) bool`; `POST /api/my/machines/{id}/auth-recheck` body `{agent_id?}`: chủ máy hoặc owner → `Recheck` true → `202`; false → `409 WORKER_OFFLINE`; người khác → `403`; không có máy → `404`; `agent_id` không thuộc máy → `400 VALIDATION_ERROR`. `GET /api/my/machines`: máy thêm `gh_auth`, agent thêm `auth_status`, `auth_detail`, `auth_checked_at`. `/api/fleet/join`: agent `ANTIGRAVITY` luôn `lead_eligible = 0`.

**api (`fleet.go`)** — `GET /api/fleet`: máy thêm `gh_auth`, agent thêm `auth_status`, `auth_detail`, `auth_checked_at`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-030-agent-auth-server/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không đổi hành vi cũ: mọi test cũ trong các package trên phải xanh (`-race`). Không thêm method vào interface `dispatcher.API`.
- Notify gọi **ngoài** khoá (giữ quy ước `s.notify(events)` sau khi unlock).
- Không ghi DB bên trong gateway; mọi trạng thái đi qua dispatcher.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
