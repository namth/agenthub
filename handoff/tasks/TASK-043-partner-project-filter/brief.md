# TASK-043 — Dispatcher: máy partner chỉ nhận việc dự án mình là thành viên

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-09: DEC-37, BR-58 · TC-86, TC-87

## 1. Mục tiêu
Lỗi bảo mật hiện tại: máy partner mới nối (không có dòng `agent_projects`) nhận việc **mọi** dự án, kể cả repo partner không được xem. Sửa để máy partner chỉ nhận việc dự án mà chủ máy là thành viên, và thu hồi việc khi chủ máy mất tư cách thành viên.

## 2. Tài liệu bắt buộc đọc
- `docs/features/ops-readiness.md` §7 BR-58; `docs/14-decisions.md` DEC-37, DEC-15, DEC-20
- Test đặc tả: `internal/dispatcher/partner_filter_test.go`; code: `internal/dispatcher/service.go` (`RequestJob`, `Tick`)

## 3. Phạm vi file
**Được tạo/sửa:** file không phải `_test.go` trong `internal/dispatcher/`.
**Không được sửa:** file trong `tests.lock` và mọi `_test.go`; package khác; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- **"Máy partner"** = `workers.owner_user_id IS NOT NULL` và `users.role <> 'ADMIN'` của chủ máy (ở chế độ đa dự án chỉ owner hệ thống có role ADMIN — DEC-17). Máy không có chủ hoặc chủ ADMIN: giữ quy tắc cũ.
- **`RequestJob`** (cả nhánh job thường lẫn job `lead_only`): với máy partner, chỉ chọn job có `project_id IN (SELECT project_id FROM project_members WHERE user_id = <chủ máy>)`, rồi mới áp `agent_projects` như cũ (không có dòng = mọi dự án **mình tham gia**).
- **`Tick`**: assignment `ACTIVE` trên máy partner mà chủ máy không còn là thành viên dự án của job → `RECLAIMED` (giống thu hồi khi offline nhưng **không tăng `attempt`**), agent về `IDLE`, job `QUEUED`, phát `EvJobReclaimed` + `EvAbortTask` (để runner dừng CLI) + `EvJobAvailable`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-043-partner-project-filter/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi test dispatcher cũ phải xanh với `-race` (lead, fairness, max_parallel, allowlist BR-29, paused, auth).
- Gọi `notify` ngoài khoá như các chỗ khác.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
