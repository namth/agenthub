# Báo cáo TASK-017

## Đã làm
- Hiện thực hàm `ansiToSegments` tại `web/src/components/ansi.ts`: phân tích mã SGR 0, 1, 30–37/90–97, 39; loại bỏ mọi chuỗi ESC khác và ký tự `\x1b` đơn lẻ; trả về danh sách các segments với định dạng text, color, bold.
- Xây dựng giao diện `SCR-06 Live terminal` tại `web/src/pages/LiveTerminal.tsx`:
  - Lấy `:id` từ route param và hiển thị tên máy con trên tiêu đề.
  - Kết nối Server-Sent Events qua `new EventSource('/api/admin/workers/<id>/logs/stream')`.
  - Xử lý sự kiện `history` (thay toàn bộ logs, giữ tối đa 5.000 dòng mới nhất) và sự kiện `log` (nối thêm vào logs).
  - Trạng thái loading trước khi nhận `history` có thuộc tính `aria-busy="true"`.
  - Hiển thị "Chưa có log" khi lịch sử rỗng và chưa có log nào.
  - Vùng hiển thị log `data-testid="terminal-output"` nền tối, font mono; mỗi dòng có `data-line`; các đoạn có màu hiển thị qua thẻ `span` có `data-color="<tên>"`.
  - Bộ lọc `<select>` nhãn "Lọc theo task" hỗ trợ xem tất cả hoặc theo từng `task_id` đã xuất hiện.
  - Tính năng tự động cuộn xuống cuối khi có log mới, có tùy chọn checkbox "Tạm dừng cuộn".
  - Xử lý mất kết nối qua `ErrorBanner` hiển thị thông báo "Mất kết nối — đang thử lại" (vẫn giữ log trên màn hình), tự động ẩn banner khi `onopen` và đóng kết nối SSE (`close()`) khi rời màn hình.
  - Tuyệt đối không dùng `dangerouslySetInnerHTML`.
- Cập nhật `web/src/pages/Fleet.tsx`: bổ sung link "Xem log" cho mỗi máy trỏ tới `/fleet/<id>/terminal`.

## File đã tạo/sửa
- `web/src/components/ansi.ts`: Tạo mới bộ phân tích màu ANSI sang mảng segments.
- `web/src/pages/LiveTerminal.tsx`: Hiện thực màn hình SCR-06 Live terminal kết nối SSE log stream.
- `web/src/pages/Fleet.tsx`: Thêm nút/link "Xem log" dẫn tới `/fleet/<id>/terminal`.

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-017-terminal/verify.sh
== 1. typecheck
== 2. vitest
 ✓ src/pages/detail.test.tsx > SCR-07 Cài đặt > PATCH lỗi → trả lại trạng thái cũ và báo lỗi 9ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > rỗng → thông điệp và nút thêm máy 65ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > lỗi → banner role=alert, Thử lại gọi lại API 41ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > hiển thị lead, agent, trạng thái và việc đang làm 11ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > nút Trao vai chỉ ở agent Claude IDLE trên máy ONLINE, không phải lead 13ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Trao vai → POST assign rồi tải lại 15ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Trao vai bị 409 → thông báo không đủ điều kiện 18ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Thu hồi vai → POST revoke; huỷ xác nhận thì không gọi 16ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > trống lead → thẻ cảnh báo, giờ dự kiến và số việc chờ 7ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > hiển thị đủ 5 cột và thẻ 10ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > rỗng → thông điệp và nút tạo yêu cầu 7ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > Admin duyệt thẻ chờ duyệt → POST approve rồi tải lại 12ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > Partner không có nút Duyệt 5ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > lỗi tải → banner Thử lại 5ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > validate phía client, không gọi API khi sai 19ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > gửi hợp lệ → POST /api/tasks và chuyển sang trang chi tiết 11ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > server lỗi → giữ dữ liệu form và báo lỗi 15ms
 ✓ src/pages/terminal.test.tsx > ansiToSegments > tách màu SGR cơ bản và reset 2ms
 ✓ src/pages/terminal.test.tsx > ansiToSegments > bỏ mã điều khiển lạ, không lộ ký tự ESC 0ms
 ✓ src/pages/terminal.test.tsx > SCR-06 Live terminal > mở luồng log của đúng máy; đang tải → aria-busy 27ms
 ✓ src/pages/terminal.test.tsx > SCR-06 Live terminal > history rỗng → "Chưa có log" 7ms
 ✓ src/pages/terminal.test.tsx > SCR-06 Live terminal > hiện lịch sử rồi nối log sống, giữ màu ANSI 14ms
 ✓ src/pages/terminal.test.tsx > SCR-06 Live terminal > giới hạn số dòng hiển thị (giữ dòng mới nhất) 395ms
 ✓ src/pages/terminal.test.tsx > SCR-06 Live terminal > lọc theo task 6ms
 ✓ src/pages/terminal.test.tsx > SCR-06 Live terminal > mất kết nối → banner; nối lại thì ẩn; rời trang đóng luồng 42ms

 Test Files  5 passed (5)
      Tests  50 passed (50)
   Start at  09:42:23
   Duration  1.64s (environment 67%, tests 19%, import 11%, transform 3%, worker 1%)
== 3. Không dangerouslySetInnerHTML trong terminal
== 4. build
TASK-017 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
