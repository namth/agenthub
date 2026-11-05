# TASK-006 — REST API Fleet & quản trị trưởng phòng

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Phần API của TASK-13 (FEAT-01). Giao diện SCR-05 làm ở task sau.

## 1. Mục tiêu
API để Admin xem cụm máy/agent/lead và trao, thu hồi vai trưởng phòng, sửa ưu tiên & hạn mềm của agent.

## 2. Tài liệu bắt buộc đọc
- `docs/features/lead-rotation.md` §2, §4 (validate), §6.3; `docs/07-api-contracts.md` §1, §1.5
- `internal/dispatcher/types.go` (interface `API`, lỗi), `internal/database/schema.sql`
- `internal/api/fleet_test.go` — đặc tả chi tiết nhất

## 3. Phạm vi file
**Được tạo/sửa:** các file `.go` (không phải `_test.go`) trong `internal/api/`.

**Không được sửa:** file trong `tests.lock`; mọi package khác (`internal/dispatcher`, `internal/auth`, …), `cmd/`, `web/`, `go.mod`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
// Authz trả vai trò người gọi; ok=false nghĩa là chưa đăng nhập.
type Authz func(r *http.Request) (role string, ok bool)
func FleetHandler(db *sql.DB, d dispatcher.API, authz Authz) http.Handler
```
- Mọi đường chỉ cho `ADMIN`: chưa đăng nhập → 401 `UNAUTHORIZED`; vai khác → 403 `FORBIDDEN`. Kiểm quyền **trước** khi đọc body hay gọi dispatcher.
- Response chuẩn `docs/07` §1: `{"success","data","error":{"code","message"}|null}`. Thời gian ra RFC3339 UTC (DB lưu `"2006-01-02 15:04:05"` UTC).
- `GET /api/fleet` → `data = {lead, workers}`:
  - `lead`: `state`, `agent_id` (null khi VACANT), `epoch`, `expires_at`, `vacant_since`, `eta` (= MIN `cooldown_until` của agent CLAUDE đang COOLDOWN, null nếu không có), `waiting_jobs` (số job `required_agent='CLAUDE'` đang `QUEUED`).
  - `workers`: sắp theo `id`, luôn là mảng (rỗng → `[]`); mỗi máy `id, name, os, conn_status, offline_since, agents[]`; mỗi agent `id, kind, status, cooldown_until, lead_priority, soft_max_jobs, soft_window_hours, jobs_in_window` (số assignment bắt đầu trong `soft_window_hours` gần nhất, theo thời gian thật hiện tại), `current_job` (`{job_id, task_id, kind, attempt}` của assignment ACTIVE, hoặc null).
- `POST /api/admin/lead/assign {agent_id}` → `d.AdminAssign`; thiếu `agent_id` → 400 `VALIDATION_ERROR`. `POST /api/admin/lead/revoke {reason?}` → `d.AdminRevoke`. Thành công trả `data = {state, agent_id, epoch, expires_at}`. Lỗi: `ErrNotFound`→404 `NOT_FOUND`, `ErrAgentNotEligible`→409 `AGENT_NOT_ELIGIBLE`, `ErrNoLead`→409 `NO_LEAD` (dùng `errors.Is`); lỗi khác → 500 `INTERNAL` không lộ chi tiết.
- `PATCH /api/admin/agents/{id}` — `id` được URL-encode (`mac%2Fclaude`); chi định tuyến theo `RawPath` nên phải `url.PathUnescape`. Body chỉ nhận `lead_priority` (1–99 hoặc null, **chỉ agent CLAUDE**), `soft_max_jobs` (1–500 hoặc null), `soft_window_hours` (1–24, không null). Trường vắng mặt giữ nguyên, `null` xoá; khoá lạ hoặc sai kiểu → 400 `VALIDATION_ERROR`. Không có agent → 404. Trả `data` = agent sau khi sửa (cùng dạng trong `GET /api/fleet`).
- Dùng `chi` (đã có trong `go.mod`).

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-006-fleet-api/verify.sh` exit 0 (gồm `-race`)
- [ ] Không test nào bị skip; `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không sửa trạng thái lease/agent bằng SQL trực tiếp — trao/thu hồi luôn qua `dispatcher.API`. `PATCH` được UPDATE cột cấu hình của `agents`.
- Gọi lệnh bằng tên ngắn (`go`, `bash`); không đọc file ngoài repo.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
