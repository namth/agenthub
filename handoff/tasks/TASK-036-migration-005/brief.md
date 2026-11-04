# TASK-036 — Migration 005: WAITING_USER, làm rõ, allowlist, sandbox, token, đề xuất của lead

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-07 §5, FEAT-09 §5, FEAT-10 §5 · TC-74

## 1. Mục tiêu
Một migration `005_ops_remote.sql` tạo đủ schema cho FEAT-07 → FEAT-10, **không mất dữ liệu** trên DB production (đã có task, job, log).

## 2. Tài liệu bắt buộc đọc
- `docs/features/clarification-and-shared-skills.md` §5, `docs/features/ops-readiness.md` §5, `docs/features/remote-command.md` §5
- Test đặc tả: `internal/database/migrate005_test.go`; code: `internal/database/db.go` (`Migrate`), `internal/database/migrations/001_init.sql` (định nghĩa `tasks` gốc)

## 3. Phạm vi file
**Được tạo/sửa:** `internal/database/migrations/005_ops_remote.sql` (mới), `internal/database/db.go`.
**Không được sửa:** migration 001–004; file trong `tests.lock` và mọi `_test.go`; package khác; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- **Dựng lại `tasks`** để CHECK có thêm `'WAITING_USER'` (giữ nguyên mọi cột, kiểu, DEFAULT, khoá ngoại, 3 index `idx_tasks_status`, `idx_tasks_parent`, `idx_tasks_assigned_worker`, và cột nào migration 002–004 đã thêm nếu có): `CREATE TABLE tasks_new … ; INSERT INTO tasks_new SELECT … FROM tasks; DROP TABLE tasks; ALTER TABLE tasks_new RENAME TO tasks;` rồi tạo lại index.
- **Bẫy quan trọng:** DB mở với `foreign_keys(1)`; `DROP TABLE tasks` khi khoá ngoại bật sẽ **xoá dây chuyền** `jobs`, `task_logs`, `lead_journal`, task con. `PRAGMA foreign_keys` không đổi được trong transaction. Vì vậy sửa `Migrate`: migration có dòng đầu tiên đúng `-- agenthub:foreign_keys=off` thì chạy trên **một kết nối riêng** (`db.Conn(ctx)`): `PRAGMA foreign_keys=OFF` → `BEGIN` → nội dung file → `PRAGMA foreign_key_check` (có dòng nào → `ROLLBACK`, trả lỗi) → ghi `schema_migrations` → `COMMIT` → `PRAGMA foreign_keys=ON`, đóng kết nối. Migration khác giữ đường cũ.
- **Bảng/cột mới:** đúng DDL trong ba file đặc tả (`task_clarifications` với `UNIQUE(task_id, round)` và CHECK `status`/`answered_via`; `projects.allowed_commands`, `needs_ci_for_integration`, `is_sandbox`; `smoke_runs`; `personal_tokens`; `lead_proposals`), kèm index nêu trong đặc tả.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-036-migration-005/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không được làm mất hay đổi dữ liệu cũ; không đổi tên cột; mọi test cũ của repo phải xanh.
- Không dùng `PRAGMA legacy_alter_table` hay `writable_schema`.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc` (không dùng `test`, `cp`, `npm`).
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
