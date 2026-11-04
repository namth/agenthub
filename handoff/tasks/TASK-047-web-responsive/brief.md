# TASK-047 — Web dùng được trên điện thoại (375px)

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-10: DEC-40, BR-65 · TC-96

## 1. Mục tiêu
Owner và partner thao tác đủ trên điện thoại (375×667): mở menu, chọn dự án, xem Kanban, duyệt, merge, xem máy của mình, tạm dừng máy, xem văn phòng — không tràn ngang trang.

## 2. Tài liệu bắt buộc đọc
- `docs/features/remote-command.md` §4 (đoạn "Web trên điện thoại"), §7 BR-65
- Test đặc tả: `web/src/layout/responsive.test.tsx`; code: `web/src/layout/AppShell.tsx`, `web/src/pages/Kanban.tsx`, `TaskDetail.tsx`, `MyMachines.tsx`, `Fleet.tsx`, `Projects.tsx`, `Contributions.tsx`, `Office.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** `web/src/layout/AppShell.tsx`, các trang trong `web/src/pages/` (không phải test), `web/src/components/` (không phải test), `web/src/index.css`.
**Không được sửa:** file trong `tests.lock` và mọi file test; `web/src/office/model.ts`, `web/src/office/renderer.ts`; Go; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết (Tailwind, breakpoint `md` = 768px)
- **AppShell:** dưới `md` sidebar ẩn (`aside[data-testid="sidebar"]` có lớp `hidden md:flex`); thanh trên cùng `header[data-testid="mobile-header"]` (lớp `md:hidden`) có tên AgentHub và nút `aria-label="Mở menu"` (`aria-expanded`). Bấm → ngăn kéo `role="dialog"` `aria-label="Menu"` chứa đúng các mục sidebar + ô chọn dự án + tên người dùng; đóng khi bấm một mục, bấm nền mờ (`data-testid="menu-backdrop"`), hoặc phím Escape. `main` có lớp `min-w-0 p-4 md:p-6`.
- **Kanban:** khung cột `data-testid="kanban-board"` có lớp `overflow-x-auto` và `snap-x`; mỗi cột `shrink-0` (rộng ≥ 16rem trên điện thoại).
- **Bảng** (Fleet, Projects, Contributions, MyMachines nếu dùng bảng): mọi `<table>` nằm trong phần tử cha có lớp `overflow-x-auto`.
- **Nút hành động chính** (Duyệt, Từ chối, Merge ở SCR-04; Tạm dừng/Tiếp tục/Gỡ ở SCR-11; Thêm máy mới): lớp `min-h-11` (44px).
- **Văn phòng (SCR-08):** khung canvas cuộn ngang `overflow-x-auto`, thanh trạng thái `office-status` xuống dòng (`flex-wrap`) thay vì tràn.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-047-web-responsive/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi test web cũ phải xanh. Không đổi chữ, `aria-label`, `data-testid` đang có (test cũ dựa vào chúng).
- Không thêm thư viện.
- Lệnh được phép: `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`, `git`, `bash`, `node`, `python3` (không `test`, `cp`, `npm`, `npx` trực tiếp — chạy kiểm tra bằng `bash handoff/tasks/TASK-047-web-responsive/verify.sh`; `node_modules` đã cài sẵn).
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
