# Báo cáo TASK-012

## Đã làm
- Hiện thực `web/src/office/model.ts` đầy đủ các kiểu dữ liệu và hàm logic thuần:
  - `computeLayout(workers)`: tính toán kích thước bố cục và toạ độ vùng cố định (Giám đốc, Ghế Trưởng phòng, Phòng nghỉ) cùng lưới dãy bàn theo thứ tự `worker.id`, bảo đảm không chồng lấn cho ≤ 8 máy × 2 agent và tự động tăng chiều cao nếu nhiều hơn.
  - `initialState(snapshot)`: dựng trạng thái ban đầu từ snapshot (lead HELD vào `lead-seat`, agent COOLDOWN vào `break` kèm note giờ hết nghỉ HH:MM, máy OFFLINE đưa agent vào `offline` và worker vào `dark`).
  - `applyEvent(state, event)`: xử lý các sự kiện miền (`handover`, `vacant`, `cooldown`, `idle`, `assign`, `submit`, `offline`, `online`, `work`), bảo đảm tính bất biến (immutability) và an toàn trước sự kiện lạ.
- Tạo `web/src/office/renderer.ts`: vẽ canvas pixel-art gồm phòng Giám đốc, ghế Trưởng phòng ★, phòng nghỉ ☕, bàn làm việc, màn hình hiển thị trạng thái động (code chạy khi làm việc, idle dot, tắt màn khi offline/vắng), sprite nhân vật di chuyển và gõ phím, điều tiết 12 fps khi hoạt động và 4 fps khi yên, dừng khi tab ẩn.
- Hiện thực `web/src/pages/Office.tsx`:
  - Kết nối SSE `/api/admin/office/stream`, tự đóng khi unmount.
  - Quản lý 4 trạng thái UI: Loading (skeleton với `aria-busy="true"` trước snapshot), Empty ("Chưa có máy nào kết nối" + link "Đăng ký máy" tới `/fleet`), Error (băng báo lỗi `role="alert"` "Mất kết nối — đang thử lại" và giữ nguyên cảnh cuối), Success (vẽ canvas pixel-art).
  - Lớp danh sách trợ năng: mỗi agent một phần tử `data-testid="char-<agentId>"` với thuộc tính `data-place`.
  - Tooltip khi di chuột qua nhân vật hiển thị `agent_id`, trạng thái, task.
  - Thanh trạng thái phía dưới hiển thị thông tin Lead, số job đang chạy, phòng nghỉ và nút tạm dừng/tiếp tục hoạt cảnh.

## File đã tạo/sửa
- `web/src/office/model.ts`: Logic thuần bố cục động `computeLayout`, dựng `initialState` từ snapshot và xử lý sự kiện bất biến `applyEvent`.
- `web/src/office/renderer.ts`: Hàm vẽ canvas pixel-art, sprite nhân vật, animation đi lại và điều tiết fps.
- `web/src/pages/Office.tsx`: Màn hình SCR-08 Văn phòng ảo với kết nối SSE, 4 trạng thái UI, canvas pixel-art, overlay trợ năng và thanh trạng thái.

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-012-office-ui/verify.sh
== 1. typecheck
== 2. vitest
 ✓ src/office/office.test.tsx > initialState / applyEvent > cooldown → phòng nghỉ, idle → về bàn; vacant → ghế trống 0ms
 ✓ src/office/office.test.tsx > initialState / applyEvent > assign/submit gắn và gỡ task; offline/online đổi cả dãy 0ms
 ✓ src/office/office.test.tsx > initialState / applyEvent > applyEvent không đổi state cũ (bất biến) và bỏ qua sự kiện lạ 0ms
 ✓ src/office/office.test.tsx > SCR-08 Office > mở luồng /api/admin/office/stream; đang tải → aria-busy 16ms
 ✓ src/office/office.test.tsx > SCR-08 Office > snapshot → canvas + nhân vật theo vị trí; sự kiện sống cập nhật 13ms
 ✓ src/office/office.test.tsx > SCR-08 Office > rỗng → thông điệp và link đăng ký máy 57ms
 ✓ src/office/office.test.tsx > SCR-08 Office > mất kết nối → banner, giữ cảnh cuối 7ms
 ✓ src/office/office.test.tsx > SCR-08 Office > rời trang thì đóng luồng 1ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > đang tải → skeleton (aria-busy) 35ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > rỗng → thông điệp và nút thêm máy 56ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > lỗi → banner role=alert, Thử lại gọi lại API 19ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > hiển thị lead, agent, trạng thái và việc đang làm 8ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > nút Trao vai chỉ ở agent Claude IDLE trên máy ONLINE, không phải lead 11ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Trao vai → POST assign rồi tải lại 16ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Trao vai bị 409 → thông báo không đủ điều kiện 17ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Thu hồi vai → POST revoke; huỷ xác nhận thì không gọi 11ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > trống lead → thẻ cảnh báo, giờ dự kiến và số việc chờ 7ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > hiển thị đủ 5 cột và thẻ 11ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > rỗng → thông điệp và nút tạo yêu cầu 7ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > Admin duyệt thẻ chờ duyệt → POST approve rồi tải lại 10ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > Partner không có nút Duyệt 5ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > lỗi tải → banner Thử lại 4ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > validate phía client, không gọi API khi sai 16ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > gửi hợp lệ → POST /api/tasks và chuyển sang trang chi tiết 9ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > server lỗi → giữ dữ liệu form và báo lỗi 10ms

 Test Files  3 passed (3)
      Tests  34 passed (34)
   Start at  09:11:05
   Duration  1.06s (environment 71%, tests 15%, import 9%, worker 3%, transform 1%)
== 3. build
TASK-012 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
