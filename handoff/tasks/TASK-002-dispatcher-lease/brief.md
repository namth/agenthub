# TASK-002 — Dispatcher: hàng đợi job, lease trưởng phòng, fencing, cooldown

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Ứng với TASK-11 trong `docs/11-tasks.md` (FEAT-01).

## 1. Mục tiêu
Cài đặt `dispatcher.Service` — lõi điều phối của AgentHub: máy/agent, hàng đợi job, lease trưởng phòng có `epoch`, fencing bằng `assignment_id`, hết limit → COOLDOWN, hạn mềm, thu hồi khi mất kết nối, REPLAN/UNBLOCK. Không có WebSocket ở task này (gateway làm song song ở máy khác, chỉ dùng interface `dispatcher.API`).

## 2. Tài liệu bắt buộc đọc
- `docs/features/lead-rotation.md` mục 1, 5, 7 (BR-01 → BR-18, máy trạng thái Agent và Lease)
- `internal/dispatcher/types.go` — **hợp đồng đã chốt**: interface `API`, kiểu, lỗi, hằng sự kiện. Đọc kỹ chú thích từng hằng `Ev*`.
- `internal/database/schema.sql` (bảng `workers`, `agents`, `jobs`, `assignments`, `lead_lease`, `lead_journal`)
- `internal/dispatcher/dispatcher_test.go` — test nghiệm thu, là đặc tả hành vi chi tiết nhất.

## 3. Phạm vi file
**Được tạo/sửa:**
- `internal/dispatcher/service.go` và các file `.go` khác (không phải `_test.go`) trong `internal/dispatcher/`, **trừ** `types.go`

**Không được sửa:**
- Mọi file trong `tests.lock` (gồm `dispatcher_test.go` và `types.go`)
- `internal/database/`, `internal/server/`, `cmd/`, `web/`, `go.mod` (không cần thư viện mới)
- `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`

## 4. Đặc tả chi tiết
- `func New(db *sql.DB, clk Clock, cfg Config, n Notifier) *Service`; `*Service` cài đặt đủ `API`. `n` có thể nil.
- **Thời gian:** mọi thời điểm lấy từ `clk.Now()`, ghi DB dạng chuỗi UTC `"2006-01-02 15:04:05"` (cùng định dạng `CURRENT_TIMESTAMP`). **Không** dựa vào `DEFAULT CURRENT_TIMESTAMP` cho `started_at`, `created_at`, `idle_since`, `last_heartbeat`… — luôn ghi rõ giá trị từ Clock.
- **ID agent:** `"<worker_id>/" + strings.ToLower(kind)`. Worker của một agent đọc từ cột `agents.worker_id`.
- **Thứ tự kiểm lỗi:** không tìm thấy → `ErrNotFound`; agent/assignment không thuộc `workerID` → `ErrAgentNotOwned`; rồi mới đến `ErrStaleAssignment` / `ErrStaleEpoch`. Trả lỗi bằng đúng biến trong `types.go` (có thể bọc `%w`).
- **Giao dịch:** mọi thay đổi `lead_lease` và việc nhận job chạy trong **một** transaction SQLite (dùng `BEGIN IMMEDIATE` hoặc mutex trong `Service` để tránh SQLITE_BUSY khi nhiều goroutine). Đổi lead luôn `epoch+1`, ghi đúng 1 dòng `lead_journal(kind='HANDOVER')`, phát `LEASE_REVOKED` (nếu có lead cũ) rồi `LEASE_GRANTED`.
- **Khi nào thử trao lease:** sau `Connect`, sau mọi thay đổi khiến lease trống, và mỗi `Tick` khi đang `VACANT`. Không giành lease của lead đang hợp lệ (không preempt theo priority).
- **Lead vào COOLDOWN** (hết limit hoặc hạn mềm) → thu hồi lease ngay (`EXHAUSTED` hoặc `RELEASED`) và chọn người kế nhiệm. Máy của lead `OFFLINE` (Disconnect hoặc quá 45s) → thu hồi `OFFLINE`.
- `LEAD_VACANT` / `LEAD_RESTORED`: phát **một lần** khi vào/thoát VACANT. `Until` của `LEAD_VACANT` = `MIN(cooldown_until)` của agent CLAUDE đang COOLDOWN, nil nếu không có.
- **Tick** làm theo thứ tự: máy quá `OfflineAfter` → OFFLINE (+thu hồi lease); máy OFFLINE ≥ `ReclaimAfter` → assignment `RECLAIMED`, job `QUEUED`; lease hết hạn → thu hồi `EXPIRED`; agent hết cooldown → IDLE; assignment ACTIVE không log ≥ `LogIdleWarn` → tạo **một** job `UNBLOCK` (lead_only) cho task đó nếu chưa có UNBLOCK đang QUEUED/RUNNING; VACANT → thử trao.
- **Attempt:** `Assignment.Attempt` = số lần giao của job (1, 2, …). Job bị EXHAUSTED/RECLAIMED mà lần tới > `MaxAttempts` → job `FAILED`, `tasks.status='PLANNING'`, tạo job `REPLAN` (CLAUDE, lead_only, priority 10), phát `JOB_FAILED`.
- **Nộp:** `Submit` → assignment `SUBMITTED`, job `DONE`, agent `IDLE` (hoặc COOLDOWN nếu chạm hạn mềm, BR-09). `RequestJob` trả `HandoffNote` của assignment gần nhất có ghi chú của job đó.
- Ban 10 phút (`AdminRevokeBan`) chỉ áp cho agent vừa bị `AdminRevoke`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-002-dispatcher-lease/verify.sh` exit 0
- [ ] Không test nào bị skip; chạy được với `-race`
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không sửa `types.go` dù thấy thiếu — ghi vào mục câu hỏi của `report.md`.
- Không dùng `time.Now()` trực tiếp ở bất kỳ đâu trong package (verify grep kiểm).
- Không đọc/ghi thời gian ở định dạng khác nhau trong cùng cột (so sánh chuỗi sẽ sai).
- Không thêm nhánh code riêng cho test; không hard-code ID trong test.
- `Notify` được gọi **sau** khi transaction commit, không gọi khi đang giữ transaction.
