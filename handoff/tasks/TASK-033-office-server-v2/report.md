# Báo cáo TASK-033

## Đã làm
- Mở rộng `office.Options`: thêm `MaxPerUser` (mặc định 3; chỉ áp dụng khi có `User`), `User func(*http.Request) (userID string, ok bool)`, `Presence *Presence`, `TelegramEnabled bool`. Đổi mặc định `MaxStreams` thành 30.
- Cập nhật các route của `internal/office`:
  - `GET /api/office/stream`: cho phép mọi người dùng đã đăng nhập (`authz` thành công, không phân biệt vai trò), áp dụng giới hạn kết nối theo người dùng `MaxPerUser` và tổng `MaxStreams`, gọi `StreamOpened` khi kết nối và `StreamClosed` khi đóng kết nối.
  - `GET /api/admin/office/stream`: chuyển hướng `308 Permanent Redirect` sang `/api/office/stream` và giữ nguyên query string.
  - `GET /api/office/status`: trả về `{"enabled": bool}` cho mọi người dùng đã đăng nhập (401 nếu chưa đăng nhập).
  - `GET /api/admin/settings`: bổ sung `"telegram_enabled": opt.TelegramEnabled` (chỉ ADMIN).
- Mở rộng `Bus.Notify`: xử lý `dispatcher.EvAgentAuth` và `dispatcher.EvWorkerAuth` phát ra `Event{Type: "auth"}` khi trạng thái `AuthStatus` hoặc `PrevAuthStatus` là `NEEDS_AUTH`.
- Hiện thực `internal/office/presence.go`:
  - `NewPresence`, `Touch`, `StreamOpened`, `StreamClosed`, `Online`, `Tick`, `Users`, `Middleware`, và `UserView`.
  - `Touch` throttle tối đa 1 lần / 60s / người dùng trong RAM trước khi cập nhật DB `users.last_seen_at`.
  - `Online` trả về true khi `last_seen_at >= now - 5 phút` hoặc đang mở `>= 1` kết nối SSE.
  - `Users` trả về danh sách sắp xếp theo BR-44: owner đầu tiên -> partner online (theo username) -> partner vắng (theo username).
  - `Tick` phát hiện thay đổi online/offline và phát `Event{Who: "user:" + username, Type: "presence", Text: "online"|"offline"}`.
  - `Middleware` gọi `Touch` đối với mỗi request đã xác thực người dùng.
- Hiện thực `internal/office/snapshot.go`:
  - Hàm `BuildSnapshot(db, lead, p)` tạo snapshot với `lead` (`agent_id` null khi `VACANT`), `workers` sắp theo ID (kèm `conn_status`, `gh_auth`, `owner` login/username, `agents` gồm `auth_status`, `cooldown_until` RFC3339, và `job` của assignment `ACTIVE`), và `users` từ `p.Users()` (mảng rỗng nếu `p == nil`).
- Cập nhật `internal/tasks`:
  - Thêm `tasks.WithOnApproved(fn func(taskID, userID string)) Option`.
  - Trong handler `POST /api/tasks/{id}/approve`, gọi `fn(taskID, userID)` sau khi `Approve` thành công (không gọi khi lỗi/từ chối).
- Cập nhật `cmd/server/main.go`:
  - Khởi tạo `office.NewPresence` (owner kiểm tra qua `isOwnerID`).
  - Bọc HTTP server handler bằng `Presence.Middleware`.
  - Chạy background ticker gọi `presence.Tick()` mỗi 60 giây.
  - Truyền `User`, `Presence`, `TelegramEnabled` vào `office.Options`.
  - Thay thế `fleetSnapshot` bằng `office.BuildSnapshot`.
  - Đăng ký mount `/api/office/*`.
  - Truyền `tasks.WithOnApproved` phát sự kiện `office.Event{Who: "user:" + username, Type: "approve", Task: taskID}`.

## File đã tạo/sửa
- `internal/office/presence.go`: Hiện thực Presence theo dõi trạng thái online, middleware throttle `last_seen_at`, sắp xếp khu chỉ huy BR-44, và phát sự kiện `presence`.
- `internal/office/snapshot.go`: Hiện thực `BuildSnapshot` tổng hợp trạng thái `lead`, `workers`, `agents`, `job` active, và `users`.
- `internal/office/bus.go`: Xử lý sự kiện `EvAgentAuth` và `EvWorkerAuth` chuyển thành sự kiện văn phòng kiểu `auth`.
- `internal/office/handler.go`: Cập nhật `Options`, route `/api/office/stream`, redirect 308, `/api/office/status`, cấu hình `telegram_enabled`, và giới hạn số luồng SSE theo người dùng/tổng.
- `internal/tasks/service.go`: Thêm tuỳ chọn `WithOnApproved` và trường callback `onApproved`.
- `internal/tasks/handler.go`: Kích hoạt callback `onApproved` sau khi `s.Approve` thành công.
- `cmd/server/main.go`: Tích hợp `Presence`, `Presence.Middleware`, `Tick` định kỳ, `BuildSnapshot`, route `/api/office/*`, và hook `WithOnApproved`.

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-033-office-server-v2/verify.sh
ok  	github.com/namth/agenthub/internal/office	4.633s
ok  	github.com/namth/agenthub/internal/tasks	5.919s
TASK-033 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
