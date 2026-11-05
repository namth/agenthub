# Báo cáo TASK-047

## Đã làm
- **AppShell**: Bổ sung thanh trên cùng `header[data-testid="mobile-header"]` (ẩn trên màn hình `md:hidden`) với tên AgentHub và nút mở menu trượt `aria-label="Mở menu"` kèm `aria-expanded`. Ẩn sidebar `aside[data-testid="sidebar"]` dưới màn hình `md` (`hidden md:flex`). Ngăn kéo `role="dialog"` `aria-label="Menu"` chứa đầy đủ các liên kết điều hướng, bộ chuyển đổi dự án `ProjectSwitcher` và thông tin người dùng; đóng khi chọn một mục chuyển trang, khi bấm nền mờ (`data-testid="menu-backdrop"`), hoặc khi bấm phím `Escape`. Khung `main` có lớp `min-w-0 p-4 md:p-6`.
- **Kanban (SCR-02)**: Khung danh sách cột `data-testid="kanban-board"` hỗ trợ cuộn ngang (`overflow-x-auto` và `snap-x`), mỗi cột có độ rộng ≥ 16rem (`w-64 md:w-auto shrink-0 snap-start`).
- **Nút hành động chính**:
  - `TaskDetail.tsx` (SCR-04): Thêm lớp `min-h-11` cho các nút bấm ngón tay Duyệt, Từ chối, Merge vào main.
  - `MyMachines.tsx` (SCR-11): Thêm lớp `min-h-11` cho nút "Thêm máy mới", "Tạm dừng" / "Tiếp tục", và "Gỡ".
- **Văn phòng ảo (SCR-08)**: Khung canvas bọc trong thẻ có lớp `overflow-x-auto max-w-full`, thanh trạng thái `office-status` đã có `flex-wrap` để tự xuống dòng không tràn màn hình.
- **Bảng dữ liệu**: Kiểm tra các bảng trong Projects và Contributions đều đã nằm trong phần tử cha có lớp `overflow-x-auto`.

## File đã tạo/sửa
- `web/src/layout/AppShell.tsx`: Thêm thanh header di động, ngăn kéo dialog menu và ẩn sidebar trên màn hình nhỏ.
- `web/src/pages/Kanban.tsx`: Thêm `data-testid="kanban-board"`, hỗ trợ cuộn ngang với `overflow-x-auto snap-x` và các cột `shrink-0`.
- `web/src/pages/TaskDetail.tsx`: Thêm `min-h-11` cho các nút Duyệt, Từ chối, Merge vào main.
- `web/src/pages/MyMachines.tsx`: Thêm `min-h-11` cho nút Thêm máy mới, Tạm dừng/Tiếp tục, Gỡ máy.
- `web/src/pages/Office.tsx`: Bọc khung canvas văn phòng ảo bằng `overflow-x-auto max-w-full`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-047-web-responsive/verify.sh
 Test Files  12 passed (12)
      Tests  131 passed (131)
TASK-047 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
