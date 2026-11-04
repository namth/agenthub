# Báo cáo TASK-015

## Đã làm
- Hiện thực hoá màn hình `TaskDetail` (`SCR-04`):
  - Đọc tham số `:id` từ URL qua `useParams`.
  - Tải dữ liệu tác vụ từ `GET /api/tasks/:id`.
  - Hỗ trợ đầy đủ 4 trạng thái giao diện: Đang tải (`Loading` với `aria-busy="true"`), 404 không tìm thấy ("Không tìm thấy task"), Lỗi khác (`ErrorBanner` với nút "Thử lại"), Thành công (tiêu đề heading, mô tả, `#<github_issue_id>`, nhãn trạng thái tiếng Việt, danh sách sub-task `data-testid="subtasks"` kèm trạng thái và link PR).
  - Phân quyền duyệt/từ chối tác vụ: Admin thấy nút "Duyệt" (`POST /api/tasks/:id/approve`) và "Từ chối" (`window.prompt` lý do, gửi `POST /api/tasks/:id/reject` với `{ reason }`) khi task ở trạng thái `PENDING_APPROVAL`. Sau khi duyệt/từ chối thành công, tự động tải lại chi tiết task. Partner hoặc task đã duyệt không hiển thị các nút này.
- Hiện thực hoá màn hình `Settings` (`SCR-07`):
  - Tải cấu hình từ `GET /api/admin/settings`.
  - Hiển thị checkbox với nhãn "Bật văn phòng ảo".
  - Khi thay đổi checkbox, gửi `PATCH /api/admin/settings` với `{ office_enabled }`. Khi thành công gọi callback `onOfficeChange(giá trị mới)`. Khi lỗi hiển thị `ErrorBanner`, giữ/trả checkbox về giá trị cũ và không gọi callback.
- Cập nhật `App.tsx`:
  - Người dùng vai trò `ADMIN` tự động tải `GET /api/admin/settings` để khởi tạo trạng thái `officeEnabled`.
  - Truyền `role` cho `TaskDetail` (`SCR-04`).
  - Truyền `onOfficeChange` cho `Settings` (`SCR-07`) để cập nhật `officeEnabled` toàn ứng dụng ngay khi thay đổi cài đặt (sidebar ẩn/hiện mục "Văn phòng").

## File đã tạo/sửa
- `web/src/pages/TaskDetail.tsx`: Giao diện chi tiết task SCR-04 với 4 trạng thái và chức năng duyệt/từ chối theo vai trò.
- `web/src/pages/Settings.tsx`: Giao diện cài đặt SCR-07 với công tắc bật/tắt văn phòng ảo.
- `web/src/App.tsx`: Đọc cấu hình văn phòng ảo cho admin, kết nối props cho `TaskDetail` và `Settings`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-015-web-detail/verify.sh
== 1. typecheck
== 2. vitest
 ✓ src/office/office.test.tsx > SCR-08 Office > rời trang thì đóng luồng 4ms
 ✓ src/pages/detail.test.tsx > SCR-04 Chi tiết task > đang tải → aria-busy; xong → tiêu đề, mô tả, issue, sub-task và PR 127ms
 ✓ src/pages/detail.test.tsx > SCR-04 Chi tiết task > task không tồn tại → thông báo 36ms
 ✓ src/pages/detail.test.tsx > SCR-04 Chi tiết task > lỗi khác → banner Thử lại 34ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > rỗng → thông điệp và nút thêm máy 82ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > lỗi → banner role=alert, Thử lại gọi lại API 65ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > hiển thị lead, agent, trạng thái và việc đang làm 9ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > nút Trao vai chỉ ở agent Claude IDLE trên máy ONLINE, không phải lead 11ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Trao vai → POST assign rồi tải lại 14ms
 ✓ src/pages/detail.test.tsx > SCR-04 Chi tiết task > Admin duyệt task chờ duyệt → POST approve rồi tải lại 10ms
 ✓ src/pages/detail.test.tsx > SCR-04 Chi tiết task > Admin từ chối → hỏi lý do, POST reject kèm reason; huỷ thì không gọi 10ms
 ✓ src/pages/detail.test.tsx > SCR-04 Chi tiết task > Partner không có nút Duyệt/Từ chối; task đã duyệt cũng không 14ms
 ✓ src/pages/detail.test.tsx > SCR-07 Cài đặt > đọc và bật văn phòng ảo → PATCH, báo onOfficeChange 12ms
 ✓ src/pages/detail.test.tsx > SCR-07 Cài đặt > PATCH lỗi → trả lại trạng thái cũ và báo lỗi 9ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Trao vai bị 409 → thông báo không đủ điều kiện 16ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Thu hồi vai → POST revoke; huỷ xác nhận thì không gọi 12ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > trống lead → thẻ cảnh báo, giờ dự kiến và số việc chờ 6ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > hiển thị đủ 5 cột và thẻ 11ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > rỗng → thông điệp và nút tạo yêu cầu 7ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > Admin duyệt thẻ chờ duyệt → POST approve rồi tải lại 11ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > Partner không có nút Duyệt 4ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > lỗi tải → banner Thử lại 4ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > validate phía client, không gọi API khi sai 17ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > gửi hợp lệ → POST /api/tasks và chuyển sang trang chi tiết 10ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > server lỗi → giữ dữ liệu form và báo lỗi 9ms

 Test Files  4 passed (4)
      Tests  42 passed (42)
   Start at  09:30:32
   Duration  1.55s (environment 69%, tests 15%, import 10%, transform 6%)
== 3. build
TASK-015 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
