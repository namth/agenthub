# TASK-003 — Gateway WebSocket `/ws/agent`, xác thực runner, đăng ký máy

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Ứng với TASK-03 trong `docs/11-tasks.md` + phần WebSocket của FEAT-01. Dispatcher thật đang được làm song song ở máy khác — task này **chỉ** dùng interface `dispatcher.API` trong `internal/dispatcher/types.go`.

## 1. Mục tiêu
Cổng WebSocket mà Runner trên các máy con kết nối vào: xác thực bằng runner token, chuyển bản tin của runner thành lời gọi `dispatcher.API`, đẩy sự kiện dispatcher (`Notify`) về đúng máy; kèm endpoint đăng ký máy mới.

## 2. Tài liệu bắt buộc đọc
- `docs/07-api-contracts.md` mục 1.3, 2 và `docs/features/lead-rotation.md` mục 6.1–6.2 (định dạng bản tin)
- `docs/02-modules-and-features.md` mục 3 (Runner Token), `docs/10-env-and-security.md` mục 2.1
- `internal/dispatcher/types.go` — hợp đồng đã chốt (interface `API`, lỗi, hằng `Ev*`)
- `internal/gateway/gateway_test.go`, `internal/gateway/register_test.go` — đặc tả chi tiết nhất

## 3. Phạm vi file
**Được tạo/sửa:**
- Các file `.go` (không phải `_test.go`) trong `internal/gateway/`
- `go.mod`, `go.sum` — chỉ thêm `github.com/gorilla/websocket` (v1.5.3)

**Không được sửa:**
- Mọi file trong `tests.lock` (gồm hai file test và `internal/dispatcher/types.go`)
- `internal/dispatcher/`, `internal/database/`, `internal/server/`, `cmd/`, `web/`
- `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`

## 4. Đặc tả chi tiết
**API package `gateway`:**
- `var ErrUnauthorized = errors.New("UNAUTHORIZED")`
- `type Authenticator func(token string) (workerID string, err error)`
- `func HashToken(token string) string` — SHA-256, chuỗi hex thường.
- `func DBAuthenticator(db *sql.DB) Authenticator` — tra `workers.token_hash = HashToken(token)`; token rỗng/sai → `ErrUnauthorized`.
- `func RegisterHandler(db *sql.DB, registrationKey string) http.Handler` — `POST /api/fleet/register`, header `X-Registration-Key`. Body `{name, os, agents}`; `name` khớp `^[a-zA-Z0-9-]+$`, `os` khác rỗng, `agents` khác rỗng và ⊆ {`CLAUDE`,`ANTIGRAVITY`}. Thành công: tạo dòng `workers` (`id = name`), sinh token ngẫu nhiên `crypto/rand` 32 byte hex, chỉ lưu hash; trả `{"success":true,"data":{"worker_id","runner_token"},"error":null}`. Lỗi: 401 `UNAUTHORIZED`, 400 `VALIDATION_ERROR`, 409 `CONFLICT` (trùng tên), 405 khi sai method. Định dạng lỗi như `docs/07` §1 + §1.5. So khoá bằng `subtle.ConstantTimeCompare`.
- `func New(api dispatcher.API, auth Authenticator) *Hub`; `*Hub` cài `http.Handler` (nâng cấp WebSocket, mọi path), `Notify(dispatcher.Event)` (thoả `dispatcher.Notifier`), `Close()`.

**Kết nối:** token ở header `X-Runner-Token` lúc nâng cấp (sai/thiếu → HTTP 401, không nâng cấp). Bản tin đầu tiên phải là `HANDSHAKE` → `api.Connect(workerID, kinds)` → `HANDSHAKE_OK {agents:[{id,status}]}`; bản tin khác trước đó → `ERROR NOT_HANDSHAKEN`. `workerID` luôn lấy từ token, **không** tin dữ liệu runner gửi. Cùng máy kết nối lại → đóng kết nối cũ, không gọi `Disconnect`. Kết nối đóng (thật) → `api.Disconnect(workerID)`.

**Runner → Server:** `PING`→`Heartbeat`+`PONG{timestamp}`; `REQUEST_TASK`→`RequestJob`→`ASSIGN_TASK{assignment_id, agent_id, job_kind, attempt, payload{task_id, branch_name, handoff_note}}` hoặc `NO_TASK{agent_id}`; `LOG_CHUNK`→`Log`; `TASK_SUBMITTED`→`Submit`; `AGENT_EXHAUSTED`→`Exhausted` (`resets_at` RFC3339 hoặc null → nil); `LEASE_RENEW`→`Renew`→`LEASE_GRANTED{agent_id, epoch, expires_at}`; `LEAD_RELEASE`→`Release`; `LEAD_ACTION` chỉ hỗ trợ `NOTE` (`data.task_id`, `data.note` → `LeadNote`), action khác → `ERROR UNSUPPORTED`.

**Lỗi:** `{"event":"ERROR","code":…,"ref":{…}}`. Dùng `errors.Is` ánh xạ lỗi dispatcher sang code cùng tên (`STALE_ASSIGNMENT`, `STALE_EPOCH`, `AGENT_NOT_OWNED`, `NOT_LEAD`, `NOT_FOUND`); `ref` chứa `assignment_id`/`epoch`/`agent_id` của bản tin gây lỗi. JSON hỏng / event lạ → `BAD_REQUEST`. Lỗi khác → `INTERNAL`, không lộ nội dung lỗi. Lỗi không làm đóng kết nối.

**Server → Runner (`Notify`):** worker của agent = phần trước dấu `/` cuối trong `AgentID`. `EvLeaseGranted`→`LEASE_GRANTED` (expires_at = `Until`), `EvLeaseRevoked`→`LEASE_REVOKED{agent_id, epoch, reason}`, `EvAbortTask`→`ABORT_TASK{assignment_id, reason}` — chỉ gửi máy sở hữu; `EvJobAvailable`→`JOB_AVAILABLE` cho mọi máy; sự kiện khác bỏ qua. `Notify` **không bao giờ chặn**: mỗi kết nối có hàng đợi gửi có giới hạn (vd 256); đầy → đóng kết nối đó. Thời gian ra định dạng RFC3339 UTC.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-003-gateway-ws/verify.sh` exit 0 (gồm `-race`)
- [ ] Không test nào bị skip; `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mỗi kết nối: một goroutine đọc, một goroutine ghi (gorilla không cho ghi đồng thời).
- Không gọi `api.*` khi đang giữ mutex của Hub (tránh deadlock với `Notify`).
- Không lưu hay log token bản rõ.
- Không chạm `internal/server` để gắn route — việc nối dây làm ở task sau.
