# Báo cáo TASK-008

## Đã làm
- [x] Tạo helper client API (`web/src/api/client.ts`) dùng `fetch`, hỗ trợ credentials: 'include', parse response chuẩn `{ success, data, error }` và xử lý mã lỗi ApiError.
- [x] Hiện thực SCR-05 Fleet (`web/src/pages/Fleet.tsx`) với đầy đủ 4 trạng thái giao diện:
  - Loading: skeleton có thuộc tính `aria-busy="true"`.
  - Empty: thông điệp "Chưa có máy nào kết nối" kèm nút "Thêm máy mới".
  - Error: ErrorBanner `role="alert"` kèm nút "Thử lại" gọi lại API.
  - Success: hiển thị thẻ Lead (`data-testid="lead-card"`) trạng thái HELD (agent_id, epoch, nút Thu hồi vai có window.confirm) hoặc VACANT (cảnh báo trống lead, số việc chờ, giờ dự kiến eta, không có nút thu hồi); hiển thị danh sách máy (`data-testid="worker-<id>"`) và từng agent (`data-testid="agent-<id>"`) với trạng thái ONLINE/OFFLINE/IDLE/BUSY/COOLDOWN, màu LED theo theme, mã tác vụ hiện tại, thời gian hết nghỉ; nút "Trao vai" chỉ hiện cho agent Claude IDLE trên máy ONLINE khác lead, xác nhận và gửi POST assign, xử lý lỗi 409 AGENT_NOT_ELIGIBLE.
- [x] Hiện thực SCR-02 Kanban (`web/src/pages/Kanban.tsx`):
  - Hiển thị 5 cột (`data-testid="column-<key>"`) và các thẻ tác vụ.
  - Trạng thái rỗng: thông điệp "Chưa có tác vụ nào trong hàng đợi" và link "Tạo yêu cầu đầu tiên" tới `/tasks/new`.
  - Nút "Duyệt" cho thẻ chờ duyệt (`backlog`) chỉ hiển thị khi `role === 'ADMIN'`, gửi POST approve và tải lại dữ liệu.
  - Xử lý trạng thái tải (Loading) và lỗi (ErrorBanner với nút "Thử lại").
- [x] Hiện thực SCR-03 Tạo yêu cầu (`web/src/pages/TaskCreate.tsx`):
  - Form với các trường "Tiêu đề" và "Mô tả chi tiết", nút "Gửi yêu cầu".
  - Validate phía client: tiêu đề 6–120 ký tự sau khi trim ("Tiêu đề tối thiểu 6 ký tự", "Tiêu đề tối đa 120 ký tự"), mô tả tối thiểu 20 ký tự ("Mô tả tối thiểu 20 ký tự"), chặn gọi API khi dữ liệu sai.
  - Gửi POST /api/tasks với tiêu đề đã trim; thành công điều hướng sang `/tasks/:id`; lỗi server hiển thị ErrorBanner và giữ nguyên dữ liệu đã nhập trong form.
- [x] Cập nhật `web/src/App.tsx` truyền `role={auth.user.role}` cho component Kanban.
- [x] Kiểm thử nghiệm thu đạt 20/20 test PASS, không có test nào bị skip, typecheck và build hoàn tất thành công.

## File đã tạo/sửa
- `web/src/api/client.ts`: Helper gọi API qua fetch, xử lý ApiResponse và ApiError.
- `web/src/pages/Fleet.tsx`: Màn hình Quản trị cụm máy (SCR-05).
- `web/src/pages/Kanban.tsx`: Màn hình Bảng Kanban điều phối (SCR-02).
- `web/src/pages/TaskCreate.tsx`: Màn hình Form tạo yêu cầu mới (SCR-03).
- `web/src/App.tsx`: Truyền role của user cho Kanban trong routing.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-008-web-screens/verify.sh
== 1. typecheck
== 2. vitest

 RUN  v5.1.0 /Users/namtran/agy-farm/wt/web-screens/web

 ✓ src/nav.test.ts > phân quyền điều hướng (docs/02, docs/03) > Partner không thấy Fleet, Terminal, Văn phòng, Cài đặt 1ms
 ✓ src/nav.test.ts > phân quyền điều hướng (docs/02, docs/03) > Admin thấy Văn phòng chỉ khi bật 1ms
 ✓ src/nav.test.ts > phân quyền điều hướng (docs/02, docs/03) > sidebar không chứa route có tham số 0ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > đang tải → skeleton (aria-busy) 35ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > rỗng → thông điệp và nút thêm máy 53ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > lỗi → banner role=alert, Thử lại gọi lại API 17ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > hiển thị lead, agent, trạng thái và việc đang làm 8ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > nút Trao vai chỉ ở agent Claude IDLE trên máy ONLINE, không phải lead 11ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Trao vai → POST assign rồi tải lại 16ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Trao vai bị 409 → thông báo không đủ điều kiện 17ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > Thu hồi vai → POST revoke; huỷ xác nhận thì không gọi 11ms
 ✓ src/pages/screens.test.tsx > SCR-05 Fleet > trống lead → thẻ cảnh báo, giờ dự kiến và số việc chờ 7ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > hiển thị đủ 5 cột và thẻ 11ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > rỗng → thông điệp và nút tạo yêu cầu 7ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > Admin duyệt thẻ chờ duyệt → POST approve rồi tải lại 11ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > Partner không có nút Duyệt 4ms
 ✓ src/pages/screens.test.tsx > SCR-02 Kanban > lỗi tải → banner Thử lại 4ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > validate phía client, không gọi API khi sai 17ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > gửi hợp lệ → POST /api/tasks và chuyển sang trang chi tiết 10ms
 ✓ src/pages/screens.test.tsx > SCR-03 Tạo yêu cầu > server lỗi → giữ dữ liệu form và báo lỗi 9ms

 Test Files  2 passed (2)
      Tests  20 passed (20)
   Start at  08:57:15
   Duration  977ms (environment 73%, tests 17%, import 7%, transform 1%, worker 1%)
== 3. build
TASK-008 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
