# Báo cáo TASK-024

## Đã làm
- Cập nhật `web/src/api/client.ts`: `request` trả `undefined` khi `res.status === 204` (không parse JSON), bổ sung hàm `api.del<T>(url)`.
- Tạo mới component `web/src/components/AddMachineDialog.tsx`:
  - Validate tên máy client-side (`^[a-zA-Z0-9-]{2,40}$`), gọi `POST /api/fleet/join-codes`.
  - Hiển thị lệnh cài một dòng và khối hướng dẫn cho agent AI với các nút sao chép qua `navigator.clipboard.writeText`.
  - Đồng hồ đếm ngược thời gian hết hạn mã (còn mm:ss); khi hết hạn hiển thị nút tạo mã mới.
  - Thăm dò tự động `GET /api/my/machines` mỗi 3 giây; khi máy xuất hiện với trạng thái `ONLINE` thì báo "Đã kết nối ✓" và dừng thăm dò.
- Viết lại SCR-11 `web/src/pages/MyMachines.tsx`:
  - Đủ 4 trạng thái UI: tải (Loading skeleton `aria-busy`), lỗi (ErrorBanner + nút Thử lại), rỗng ("Chưa có máy nào" + Thêm máy mới), và có dữ liệu.
  - Hiển thị danh sách máy cá nhân: tên, trạng thái kết nối, phiên bản, nhãn Tạm dừng, danh sách agent cùng việc đang chạy.
  - Các thao tác: Tạm dừng / Tiếp tục (`PATCH /api/my/machines/{id}`), Gỡ máy có xác nhận confirm (`DELETE /api/my/machines/{id}` 204), chọn dự án nhận việc (`PATCH` `projects`).
  - Nút "Thêm máy mới" mở hộp thoại `AddMachineDialog` và tự động làm mới danh sách khi đóng.
- Cập nhật SCR-05 Fleet `web/src/pages/Fleet.tsx`:
  - Nhóm worker theo chủ máy: nhóm "Hệ thống" (`data-testid="owner-group-system"`) đặt đầu tiên cho `owner === null`, các nhóm partner (`data-testid="owner-group-<id>"`) theo `owner.login`.
  - Hiển thị phiên bản worker và nhãn "Tạm dừng" khi worker bị tạm dừng.
  - Hiển thị tên dự án và tiêu đề task đang chạy trong thông tin agent.
  - Bổ sung công tắc "Cho làm trưởng phòng" (`lead_eligible`) chỉ dành riêng cho agent `CLAUDE` (`PATCH /api/admin/agents/{id}`).
  - Bổ sung ô chọn dự án được nhận (`select` aria-label="Dự án được nhận") cho từng agent (`PATCH /api/admin/agents/{id}`).
  - Nút "Thêm máy mới" mở hộp thoại `AddMachineDialog` (đảm bảo đúng duy nhất 1 nút trên trang cả khi rỗng lẫn khi có dữ liệu).
  - Giữ nguyên các chức năng và test cũ của SCR-05 (lead-card, Trao vai, Thu hồi vai).

## File đã tạo/sửa
- `web/src/api/client.ts`: Xử lý HTTP 204 trả về undefined và thêm phương thức `api.del`.
- `web/src/components/AddMachineDialog.tsx`: Hộp thoại tạo mã tham gia và hướng dẫn kết nối máy mới.
- `web/src/pages/MyMachines.tsx`: Trang SCR-11 quản lý máy cá nhân của người dùng.
- `web/src/pages/Fleet.tsx`: Cập nhật SCR-05 quản trị cụm máy nhóm theo chủ máy, bật tắt quyền lead và chọn dự án cho agent.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-024-web-machines/verify.sh
 ✓ src/nav.test.ts > phân quyền điều hướng (docs/02, docs/03) > Partner không thấy Fleet, Terminal, Văn phòng, Cài đặt, Dự án 1ms
 ✓ src/nav.test.ts > phân quyền điều hướng (docs/02, docs/03) > Admin thấy Văn phòng chỉ khi bật 1ms
 ✓ src/nav.test.ts > phân quyền điều hướng (docs/02, docs/03) > sidebar không chứa route có tham số 0ms

 Test Files  8 passed (8)
      Tests  77 passed (77)
   Start at  14:35:13
   Duration  2.18s (environment 60%, tests 25%, import 11%, worker 2%, transform 2%)
TASK-024 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
