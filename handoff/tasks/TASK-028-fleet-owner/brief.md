# TASK-028 — Fleet: chủ máy, phiên bản, lead_eligible, dự án agent; projects: open_tasks + tạo lại webhook

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-04 §4/§6, FEAT-05 §4/§6, BR-26, BR-33

## 1. Mục tiêu
Bổ sung dữ liệu cho giao diện quản trị: SCR-05 nhóm máy theo chủ, hiện phiên bản, máy tạm dừng, công tắc "Cho làm trưởng phòng", dự án agent được nhận, dự án của việc đang chạy; SCR-09 hiện số task đang mở và nút "Tạo lại webhook".

## 2. Tài liệu bắt buộc đọc
- `docs/features/multi-project.md` §4, §6; `docs/features/join-machine.md` §4, §5, §6
- Schema: `internal/database/migrations/*.sql` (`workers.owner_user_id/version/paused`, `agents.lead_eligible`, `agent_projects`, `projects.webhook_id`)
- Test đặc tả: `internal/api/fleet_owner_test.go`, `internal/projects/projects_ui_test.go`; code hiện có `internal/api/fleet.go`, `internal/projects/handler.go`, `internal/projects/service.go`

## 3. Phạm vi file
**Được tạo/sửa:** `internal/api/fleet.go` (và file mới không phải `_test.go` trong `internal/api/`); `internal/projects/handler.go`, `internal/projects/service.go` (và file mới không phải `_test.go` trong `internal/projects/`).

**Không được sửa:** file trong `tests.lock`; package Go khác; `web/`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**`GET /api/fleet`** — giữ nguyên mọi trường cũ, thêm:
- Mỗi worker: `owner` = `{id, login}` (`users.id`, `users.username` theo `workers.owner_user_id`) hoặc `null`; `version` (string|null); `paused` (bool).
- Mỗi agent: `lead_eligible` (bool); `projects` = mảng `project_id` từ `agent_projects`, hoặc `null` khi agent không có dòng nào (= mọi dự án).
- `current_job` thêm `project_id`, `project` (= `projects.github_repo` của job), `task_title` (= `tasks.title`).

**`PATCH /api/admin/agents/{id}`** — thêm 2 khoá được phép (giữ validate cũ):
- `lead_eligible`: bắt buộc là boolean JSON (`true/false`); sai kiểu hoặc `null` → 400 `VALIDATION_ERROR`. Lưu 1/0.
- `projects`: `null` hoặc `[]` → xoá mọi dòng `agent_projects` của agent; mảng chuỗi → **thay thế toàn bộ** bằng danh sách mới. Có id không tồn tại trong `projects` hoặc sai kiểu → 400 `VALIDATION_ERROR` và **không đổi gì** (validate hết trước khi ghi; ghi trong một transaction).

**`GET /api/projects`** — mỗi phần tử thêm `open_tasks` (int) = số task gốc (`parent_id IS NULL`) của dự án có `status NOT IN ('DONE','REJECTED')`.

**`POST /api/admin/projects/{id}/webhook`** (owner, như các route admin khác; cả dạng không tiền tố `/admin/projects/{id}/webhook` như các route sẵn có) — tạo lại webhook bằng `gh.CreateWebhook(github_repo, BaseURL + "/api/webhooks/github", WebhookSecret)` (dùng chung cách tạo URL với `AddProject`). Thành công → cập nhật `projects.webhook_id`, 200 trả dự án như phần tử `GET /api/projects` (có `webhook_ok: true`). GitHub lỗi → 502 `GITHUB_ERROR`, không đổi DB. Không có dự án → 404 `NOT_FOUND`. Không phải owner → 403.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-028-fleet-owner/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Truy vấn tham số hoá; không N+1 quá mức không cần thiết nhưng ưu tiên đúng và đơn giản.
- Trả `projects: null` (không phải `[]`) khi agent không có dòng nào.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
