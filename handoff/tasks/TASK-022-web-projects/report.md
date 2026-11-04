# Báo cáo TASK-022

## Đã làm
- Hiện thực hoá quản lý trạng thái dự án hiện tại với `project.ts`: hỗ trợ lưu `localStorage`, dispatch sự kiện `agenthub:project` và hook `useSelectedProject`.
- Tạo component `ProjectSwitcher` đặt trong sidebar `AppShell`, hỗ trợ lựa chọn dự án cho Owner (có "Tất cả dự án") và Partner (chọn dự án đầu tiên theo mặc định), tự động đồng bộ khi lựa chọn thay đổi.
- Cập nhật trang `Kanban` (SCR-02) tải task theo `project_id` khi chọn dự án cụ thể hoặc toàn bộ khi chọn "Tất cả", hiển thị repo name trên thẻ khi xem tất cả, cho phép duyệt khi người dùng là Admin hoặc có vai Admin trong dự án, và hiển thị thông báo chỉ xem khi dự án đã lưu trữ.
- Cập nhật trang `TaskCreate` (SCR-03) chỉ tải danh sách dự án khi đã chọn dự án trong hệ thống; tự động đính kèm `project_id` khi chọn dự án cụ thể hoặc hiển thị dropdown chọn dự án khi chọn "Tất cả dự án" (validate bắt buộc chọn dự án trước khi gửi).
- Viết lại trang `Projects` (SCR-09) với đầy đủ 4 trạng thái UI: skeleton khi đang tải, banner lỗi kèm Thử lại hoặc cảnh báo quyền PAT khi lỗi GitHub API, hiển thị bảng dự án (repo, nhánh mặc định, trạng thái webhook kèm nút Tạo lại webhook, số task mở, chỉnh sửa trọng số 1–5 và song song tối đa 1–50, lưu trữ/khôi phục dự án) và khối danh sách repo chưa thêm kèm nút Thêm.
- Đảm bảo toàn bộ 82 test web PASS, TypeScript typecheck sạch (`tsc -b`), build Vite thành công, và `verify.sh` exit 0.

## File đã tạo/sửa
- `web/src/project.ts`: Khai báo constant, helper lưu/đọc `localStorage` dự án đã chọn, hook `useSelectedProject`, type `Project`.
- `web/src/components/ProjectSwitcher.tsx`: Ô chọn dự án trong sidebar cho Owner và Partner.
- `web/src/layout/AppShell.tsx`: Nhúng `ProjectSwitcher` vào sidebar `<aside>`.
- `web/src/pages/Kanban.tsx`: Điều phối Kanban theo dự án, hiển thị repo trên thẻ, kiểm tra quyền duyệt theo vai dự án và cảnh báo chỉ xem khi lưu trữ.
- `web/src/pages/TaskCreate.tsx`: Tạo yêu cầu theo dự án hoặc chọn dự án từ form khi xem tất cả dự án.
- `web/src/pages/Projects.tsx`: Trang SCR-09 quản lý danh sách dự án và thêm repo từ GitHub.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-022-web-projects/verify.sh
 ✓ src/nav.test.ts > phân quyền điều hướng (docs/02, docs/03) > Partner không thấy Fleet, Terminal, Văn phòng, Cài đặt, Dự án 1ms
 ✓ src/nav.test.ts > phân quyền điều hướng (docs/02, docs/03) > Admin thấy Văn phòng chỉ khi bật 1ms
 ✓ src/nav.test.ts > phân quyền điều hướng (docs/02, docs/03) > sidebar không chứa route có tham số 0ms

 Test Files  8 passed (8)
      Tests  82 passed (82)
   Start at  14:31:11
   Duration  2.41s (environment 58%, tests 24%, import 13%, transform 4%)
TASK-022 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
