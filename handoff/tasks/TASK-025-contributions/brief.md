# TASK-025 — Trang Đóng góp: API tổng hợp + SCR-10

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-05: DEC-21, BR-36, TC-56

## 1. Mục tiêu
Xem ai (chủ máy) và dự án nào đã đóng góp bao nhiêu công việc trong một khoảng thời gian, và xem chi tiết từng việc.

## 2. Tài liệu bắt buộc đọc
- `docs/14-decisions.md` DEC-21; `docs/features/join-machine.md` §4 (SCR-10), §6 (API), §7 BR-36
- Schema: `internal/database/migrations/*.sql` (assignments, jobs, tasks, workers.owner_user_id, project_members)
- `internal/contrib/contrib_test.go`, `web/src/pages/contributions.test.tsx` (đặc tả chi tiết nhất); `web/src/api/client.ts`, `web/src/components/StateView.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** `internal/contrib/` (mới, file không phải `_test.go`); `web/src/pages/Contributions.tsx` (mới), component mới trong `web/src/components/`.

**Không được sửa:** file trong `tests.lock`; package Go khác; `web/src/nav.ts`, `web/src/App.tsx`, `web/package.json`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**API** — `func Handler(db *sql.DB, authz func(*http.Request) (userID string, isOwner bool, ok bool)) http.Handler`; JSON chuẩn docs/07; chưa đăng nhập 401.
- `GET /api/contributions?by=partner|project&from=<RFC3339>&to=<RFC3339>` (thiếu from/to → 7 ngày gần nhất; `by` khác → 400). Assignment tính vào khoảng nếu `started_at` ∈ [from, to). Nhóm **partner** = `workers.owner_user_id` của agent làm assignment (`key` = user id, `label` = username; NULL → `key "system"`, `label "Hệ thống"`). Nhóm **project** = `jobs.project_id` (`label` = `github_repo`). Mỗi dòng: `machines` (số máy khác nhau có assignment), `jobs.done` (SUBMITTED), `jobs.failed` (FAILED/RECLAIMED/ABORTED), `jobs.exhausted` (EXHAUSTED), `jobs.by_kind` (đếm SUBMITTED theo `jobs.kind`), `tasks_done` (task `DONE` có `updated_at` trong khoảng và — với partner — có ít nhất một assignment SUBMITTED của nhóm đó), `prs_merged` (như tasks_done nhưng task có `pr_url`), `busy_hours` (tổng `ended_at − started_at` của assignment đã kết thúc, giờ, làm tròn 2 chữ số). Chỉ trả nhóm có ít nhất một assignment trong khoảng; sắp `jobs.done` giảm dần.
- Phạm vi: owner thấy tất cả; người khác chỉ dòng `key = userID` (partner) hoặc dự án trong `project_members` của họ (project).
- `GET /api/contributions/timeline?partner=<user id|system>|project=<id>&from&to` → mảng `[{at (RFC3339 của started_at), worker, agent, job_kind, task_id, task_title, project (github_repo), result (assignment.status), attempt}]`, mới nhất trước, tối đa 500. Người khác owner xem partner khác mình → 403; dự án không là thành viên → 403.

**SCR-10** — `export default function Contributions()`: tiêu đề "Đóng góp"; `<select>` nhãn "Khoảng thời gian" (7 / 30 / 90 ngày, mặc định 7); radio "Partner" / "Dự án" (mặc định Partner); bảng (`<table>`) cột: tên, Máy, Job xong, Lỗi/limit, Task DONE, PR merge, Giờ máy. Bấm một dòng → tải và hiện dòng thời gian bên dưới (thời điểm theo giờ địa phương, máy/agent, loại job, tên task, dự án, kết quả, lần thử). 4 trạng thái: tải (`aria-busy`), lỗi (`ErrorBanner` + Thử lại), rỗng ("Chưa có đóng góp trong khoảng này"), có dữ liệu.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-025-contributions/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Truy vấn tham số hoá; so sánh thời gian theo định dạng DB `"2006-01-02 15:04:05"` UTC.
- Lệnh được phép: `go`, `gofmt`, `bash`, `node`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc` (chạy `npx` qua `bash`).
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
