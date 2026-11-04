# TASK-027 — Dispatcher đa dự án: chia lượt, giới hạn, danh sách dự án, máy tạm dừng, lead_eligible

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-04/05: DEC-15, DEC-16, DEC-20, BR-25, BR-29, BR-32, BR-33

## 1. Mục tiêu
Nhiều dự án dùng chung dàn máy mà không dự án nào chiếm hết; máy tạm dừng không nhận việc; agent không đủ điều kiện không làm trưởng phòng.

## 2. Tài liệu bắt buộc đọc
- `docs/14-decisions.md` DEC-15, DEC-16, DEC-20; `docs/features/multi-project.md` §5, §7; `docs/features/join-machine.md` §5, §7
- `internal/database/migrations/002_multi_project.sql`, `003_join_machine.sql`; `internal/dispatcher/service.go`
- `internal/dispatcher/dispatcher_multi_test.go` (đặc tả chi tiết nhất)

## 3. Phạm vi file
**Được sửa:** các file `.go` (không phải `_test.go`) trong `internal/dispatcher/`, trừ `types.go`.

**Không được sửa:** file trong `tests.lock`; package khác; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- **Chọn job** cho agent rảnh (`RequestJob`):
  1. Agent là lead → job `lead_only` trước (mọi dự án, **không** xét `agent_projects`).
  2. Còn lại: job `QUEUED` đúng `required_agent`, `lead_only = 0`, thuộc dự án `status = 'ACTIVE'`, dự án chưa đạt `max_parallel` (đếm job `RUNNING` của dự án; NULL = không giới hạn), và nằm trong `agent_projects` của agent (agent không có dòng nào = mọi dự án).
  3. Sắp: `priority` tăng dần → tỉ lệ `RUNNING / weight` của dự án tăng dần → `created_at` → `id`.
- **Máy tạm dừng** (`workers.paused = 1`): `RequestJob` trả nil; không là ứng viên lead; nếu đang giữ lease thì `Tick` thu hồi (lý do `RELEASED`) rồi chọn người khác.
- **`agents.lead_eligible = 0`**: không là ứng viên lead (BR-06); `AdminAssign` → `ErrAgentNotEligible`.
- Mọi điều kiện trên làm trong cùng truy vấn/transaction đang dùng để nhận job (giữ an toàn khi nhiều goroutine).

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-027-fair-dispatch/verify.sh` exit 0 (gồm `-race`)
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không sửa test cũ; mọi test dispatcher cũ phải xanh.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
