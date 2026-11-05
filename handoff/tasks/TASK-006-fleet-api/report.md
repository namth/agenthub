# Báo cáo TASK-006

## Đã làm
- Hiện thực `FleetHandler(db *sql.DB, d dispatcher.API, authz Authz) http.Handler` dùng `chi/v5` router.
- Phân quyền RBAC chuẩn: chỉ cho phép `ADMIN`; chưa đăng nhập trả về 401 `UNAUTHORIZED`, vai trò khác trả về 403 `FORBIDDEN`; kiểm quyền trước khi đọc body hay gọi dispatcher.
- `GET /api/fleet`: trả về cấu trúc `data = {lead, workers}` với `lead` (`state`, `agent_id`, `epoch`, `expires_at`, `vacant_since`, `eta`, `waiting_jobs`) và `workers` (sắp xếp theo id, danh sách `agents` với `cooldown_until`, `lead_priority`, `soft_max_jobs`, `soft_window_hours`, `jobs_in_window`, `current_job`).
- `POST /api/admin/lead/assign`: validate body yêu cầu `agent_id`, gọi `d.AdminAssign`, xử lý chuẩn các lỗi `ErrNotFound` (404), `ErrAgentNotEligible` (409), `ErrNoLead` (409), `INTERNAL` (500).
- `POST /api/admin/lead/revoke`: gọi `d.AdminRevoke`, trả về thông tin lease mới hoặc trạng thái VACANT, xử lý lỗi `ErrNoLead` (409).
- `PATCH /api/admin/agents/{id}`: giải mã URL-encoded id với `url.PathUnescape`, kiểm tra agent tồn tại, validate chặt chẽ các trường (`lead_priority` 1–99 hoặc null chỉ cho CLAUDE, `soft_max_jobs` 1–500 hoặc null, `soft_window_hours` 1–24 không null, từ chối khoá lạ hoặc sai kiểu với 400 `VALIDATION_ERROR`), cập nhật DB và trả về thông tin agent sau khi sửa.
- Tuân thủ nghiêm ngặt quy tắc không ghi vào bảng `lead_lease` trực tiếp bằng SQL; mọi thay đổi vai trò lead đều ủy thác qua `dispatcher.API`.

## File đã tạo/sửa
- `internal/api/fleet.go`: Hiện thực toàn bộ API Fleet và quản trị trưởng phòng theo brief và hợp đồng `docs/07-api-contracts.md`.

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-006-fleet-api/verify.sh
== 1. gofmt + vet
== 2. Lease chỉ đổi qua dispatcher
== 3. Test api (-race)
--- PASS: TestRBAC (0.04s)
--- PASS: TestFleetEmpty (0.04s)
--- PASS: TestFleetShape (0.06s)
--- PASS: TestFleetVacantShowsETAAndWaiting (0.06s)
--- PASS: TestAssignAndRevoke (0.05s)
--- PASS: TestAssignToCooldownAgent (0.04s)
--- PASS: TestPatchAgent (0.05s)
ok  	github.com/namth/agenthub/internal/api	1.355s
== 4. Không phá test cũ
ok  	github.com/namth/agenthub/internal/database	0.267s
ok  	github.com/namth/agenthub/internal/dispatcher	0.490s
ok  	github.com/namth/agenthub/internal/gateway	1.074s
ok  	github.com/namth/agenthub/internal/runner	4.219s
ok  	github.com/namth/agenthub/internal/server	0.051s
ok  	github.com/namth/agenthub/web	0.007s
TASK-006 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
