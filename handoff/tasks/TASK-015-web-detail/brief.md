# TASK-015 — Giao diện SCR-04 Chi tiết task, SCR-07 Cài đặt

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity

## 1. Mục tiêu
Xem chi tiết một task (mô tả, issue, sub-task, PR) và duyệt/từ chối ngay tại đó; trang Cài đặt có công tắc Văn phòng ảo, sidebar ẩn/hiện mục "Văn phòng" theo cài đặt.

## 2. Tài liệu bắt buộc đọc
- `docs/03-screens-and-ui.md` §1, §3; `docs/features/virtual-office.md` §4 (công tắc)
- API: `internal/tasks/tasks_test.go` (`GET /api/tasks/{id}`, approve, reject), `internal/office/office_test.go` (`/api/admin/settings`)
- `web/src/pages/detail.test.tsx` (đặc tả chi tiết nhất); `web/src/api/client.ts`, `web/src/components/StateView.tsx`, `web/src/App.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** `web/src/pages/TaskDetail.tsx`, `web/src/pages/Settings.tsx`, `web/src/App.tsx`, file mới trong `web/src/components/`, `web/src/hooks/`.

**Không được sửa:** file trong `tests.lock`; `web/src/nav.ts`, `web/package.json`; trang khác; mọi thứ ngoài `web/src`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- **`TaskDetail({ role })`:** đọc `:id` từ route; `GET /api/tasks/<id>`; tiêu đề là `heading`; hiện mô tả, `#<github_issue_id>`, trạng thái (nhãn tiếng Việt), danh sách sub-task `data-testid="subtasks"` (tiêu đề, trạng thái, link PR nếu có). 404 → "Không tìm thấy task"; lỗi khác → `ErrorBanner` + "Thử lại". `role === 'ADMIN'` và `status === 'PENDING_APPROVAL'` → nút "Duyệt" (`POST …/approve`) và "Từ chối" (`window.prompt` lý do; null/rỗng → không gọi; có → `POST …/reject {reason}`); xong tải lại.
- **`Settings({ onOfficeChange })`:** `GET /api/admin/settings`; checkbox có nhãn "Bật văn phòng ảo"; đổi → `PATCH /api/admin/settings {office_enabled}`; thành công → `onOfficeChange(giá trị mới)`; lỗi → `ErrorBanner`, trả checkbox về giá trị cũ, không gọi callback.
- **`App.tsx`:** user ADMIN → đọc `GET /api/admin/settings` để có `officeEnabled` (thay `const officeEnabled = false`); truyền `onOfficeChange` cho Settings để cập nhật ngay; truyền `role` cho TaskDetail.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-015-web-detail/verify.sh` exit 0 (typecheck, vitest, build)
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không hard-code dữ liệu; không nhánh riêng cho test.
- Lệnh được phép: `node`, `bash` (chạy `npx`), `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
