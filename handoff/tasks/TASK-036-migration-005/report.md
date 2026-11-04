# Báo cáo TASK-036

## Đã làm
- Tạo migration `internal/database/migrations/005_ops_remote.sql` với dòng đầu tiên `-- agenthub:foreign_keys=off`.
- Dựng lại bảng `tasks` sang `tasks_new` để CHECK(status) bổ sung `'WAITING_USER'`, chuyển toàn bộ dữ liệu từ `tasks`, drop `tasks`, đổi tên `tasks_new` thành `tasks`, tạo lại đầy đủ 3 index `idx_tasks_status`, `idx_tasks_parent`, `idx_tasks_assigned_worker`.
- Bổ sung các bảng mới theo đúng DDL đặc tả FEAT-07 -> FEAT-10:
  - `task_clarifications` với `UNIQUE(task_id, round)`, CHECK status/answered_via, index `idx_clarifications_task`.
  - `smoke_runs` với CHECK status, quan hệ khoá ngoại tới `projects` và `tasks`.
  - `personal_tokens` với `UNIQUE(user_id, name)`, `token_hash` UNIQUE, `expires_at` NOT NULL.
  - `lead_proposals` với CHECK kind IN ('CANCEL'), CHECK status.
- Thêm các cột mới cho bảng `projects`: `allowed_commands`, `needs_ci_for_integration`, `is_sandbox`.
- Cập nhật `internal/database/db.go`: khi gặp migration có dòng đầu `-- agenthub:foreign_keys=off`, lấy một kết nối riêng từ pool (`db.Conn(ctx)`), tắt foreign keys (`PRAGMA foreign_keys=OFF`), bắt đầu transaction `BEGIN`, thực thi migration, kiểm tra khoá ngoại bằng `PRAGMA foreign_key_check` (rollback và báo lỗi nếu có vi phạm), ghi `schema_migrations`, `COMMIT`, sau đó bật lại `PRAGMA foreign_keys=ON` và đóng kết nối.
- Toàn bộ các test đặc tả của TASK-036 và các test liên quan đều PASS:
  - `TestMigration005KeepsData`: PASS
  - `TestMigration005NewSchema`: PASS
  - `TestMigration004`: PASS
  - `TestMigrateLegacyDBKeepsData`: PASS
  - `TestMigrateIdempotentVersions`: PASS

## File đã tạo/sửa
- `internal/database/migrations/005_ops_remote.sql`: Migration 005 tạo schema cho FEAT-07 -> FEAT-10.
- `internal/database/db.go`: Cập nhật hàm `Migrate` để hỗ trợ migration có cờ `-- agenthub:foreign_keys=off`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-036-migration-005/verify.sh
    db_test.go:89: bảng = [agent_projects agents assignments jobs join_codes lead_journal lead_lease lead_proposals personal_tokens project_members projects schema_migrations settings smoke_runs task_clarifications task_logs tasks users workers], muốn đúng [agent_projects agents assignments jobs join_codes lead_journal lead_lease project_members projects schema_migrations settings task_logs tasks users workers]
--- FAIL: TestOpenCreatesFileAndAllTables (0.22s)

KẾT QUẢ: khoá test OK, verify.sh exit 1
```

## Điểm lệch / câu hỏi cho trưởng phòng
- Test cũ `TestOpenCreatesFileAndAllTables` trong `internal/database/db_test.go` (viết từ TASK-001, cập nhật ở commit 9b673c3c khi thêm migration 002-003) kiểm tra cứng danh sách 15 bảng `want`.
- Khi migration 005 thêm 4 bảng mới (`lead_proposals`, `personal_tokens`, `smoke_runs`, `task_clarifications`), tổng số bảng trong DB tăng từ 15 lên 19 bảng, khiến `TestOpenCreatesFileAndAllTables` so khớp `len(got) != len(want)` và bị fail.
- Do `internal/database/db_test.go` nằm trong danh sách khoá của `handoff/tasks/TASK-001-nen-go-sqlite/tests.lock`, và brief.md mục 3 cấm sửa mọi `_test.go`, tuân thủ Quy tắc 2 của Kỹ sư thực thi ("Không bao giờ sửa, xoá, đổi tên, hay 'skip' test do trưởng phòng viết... Test đỏ nghĩa là code sai. Nếu tin rằng test sai, ghi lý do vào report.md mục 'câu hỏi' và dừng; không lách test"), Antigravity không tự ý sửa `db_test.go`.
- Đề xuất Trưởng phòng (Claude) cập nhật `db_test.go` bổ sung 4 bảng mới vào danh sách `want` và cập nhật hash trong `TASK-001-nen-go-sqlite/tests.lock` để nghiệm thu.
