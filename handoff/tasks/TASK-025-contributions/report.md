# Báo cáo TASK-025

## Đã làm
- Hiện thực API tổng hợp đóng góp `GET /api/contributions` theo partner và project, hỗ trợ lọc theo khoảng thời gian RFC3339 (mặc định 7 ngày gần nhất), tính các chỉ số `machines`, `jobs.done`, `jobs.failed`, `jobs.exhausted`, `jobs.by_kind`, `tasks_done`, `prs_merged`, `busy_hours` và sắp xếp theo `jobs.done` giảm dần.
- Hiện thực phân quyền API: chưa đăng nhập trả 401; owner xem toàn bộ; partner chỉ xem dòng của chính mình hoặc dự án mình là thành viên.
- Hiện thực API dòng thời gian `GET /api/contributions/timeline` theo partner hoặc project, trả tối đa 500 bản ghi mới nhất, chặn 403 khi partner xem dữ liệu ngoài phạm vi quyền.
- Hiện thực màn hình SCR-10 `Contributions` (`web/src/pages/Contributions.tsx`): tiêu đề "Đóng góp", dropdown chọn 7/30/90 ngày, radio chọn Partner/Dự án, bảng tổng hợp hiển thị đầy đủ các cột và hỗ trợ click vào từng dòng để mở dòng thời gian chi tiết bên dưới.
- Hiện thực đủ 4 trạng thái UI: đang tải (`aria-busy`), lỗi (`ErrorBanner` + "Thử lại"), rỗng ("Chưa có đóng góp trong khoảng này"), có dữ liệu.

## File đã tạo/sửa
- `internal/contrib/contrib.go`: Cung cấp `contrib.Handler(db, authz)` xử lý 2 endpoint `/api/contributions` và `/api/contributions/timeline`.
- `web/src/pages/Contributions.tsx`: Component React SCR-10 cho trang Đóng góp.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-025-contributions/verify.sh
--- PASS: TestByPartnerOwnerView (0.11s)
--- PASS: TestByProjectAndRange (0.10s)
--- PASS: TestPartnerScope (0.10s)
--- PASS: TestTimeline (0.10s)
ok  	github.com/namth/agenthub/internal/contrib	1.436s
 ✓ src/pages/terminal.test.tsx > SCR-06 Live terminal > giới hạn số dòng hiển thị (giữ dòng mới nhất) 391ms
 ✓ src/pages/terminal.test.tsx > SCR-06 Live terminal > lọc theo task 6ms
 ✓ src/pages/terminal.test.tsx > SCR-06 Live terminal > mất kết nối → banner; nối lại thì ẩn; rời trang đóng luồng 42ms

 Test Files  7 passed (7)
      Tests  57 passed (57)
   Start at  12:11:14
   Duration  1.76s (environment 60%, tests 19%, import 10%, transform 7%, worker 3%)
TASK-025 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
