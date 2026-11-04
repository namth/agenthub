# Báo cáo TASK-030

## Đã làm
- Tạo migration `004_agent_auth_office.sql` bổ sung các cột `agents.auth_status`, `agents.auth_checked_at`, `agents.auth_detail`, `workers.gh_auth`, `users.last_seen_at`, chỉ mục `idx_agents_auth`, và cập nhật `lead_eligible = 0` cho các agent không phải CLAUDE theo DEC-29.
- Thêm `AuthAPI`, `AgentHello`, hằng `AuthOK`, `AuthNeeds`, `AuthUnknown`, `RevokeAuth`, `EvAgentAuth`, `EvWorkerAuth`, `ErrInvalidAuth`, mở rộng `Agent` và `Event` trong package `dispatcher`.
- Cài đặt `ConnectAuth`, `SetAuth`, `SetGhAuth`, `AuthRequired` trong `dispatcher.Service`, khử secret và cắt ngắn `detail` ≤ 80 ký tự (`sanitizeDetail`), áp dụng guard BR-37 trong `RequestJob`, bộ chọn lead và `AdminAssign`.
- Cập nhật `gateway`: mở rộng `HANDSHAKE` nhận `gh_auth` và `agents[].auth_status/auth_detail`, định tuyến bản tin `AGENT_AUTH` tới `SetGhAuth`, `AuthRequired` hoặc `SetAuth`, thêm `Hub.SendAuthRecheck`.
- Cập nhật `telegram.Notifier`: gửi thông báo khi vào `NEEDS_AUTH` kèm gợi ý lệnh và khi thoát về `OK` (`EvAgentAuth`, `EvWorkerAuth`).
- Cập nhật `machines`: thêm `Config.Recheck`, cài đặt endpoint `POST /api/my/machines/{id}/auth-recheck`, bổ sung các trường auth vào `GET /api/my/machines`, và đảm bảo `ANTIGRAVITY` luôn có `lead_eligible = 0` khi join qua `POST /api/fleet/join`.
- Cập nhật `api.FleetHandler`: trả về các trường `gh_auth`, `auth_status`, `auth_detail`, `auth_checked_at` tại `GET /api/fleet`.
- Nối dây `machines.Config.Recheck = hub.SendAuthRecheck` trong `cmd/server/main.go`.

## File đã tạo/sửa
- `internal/database/migrations/004_agent_auth_office.sql`: Migration 004 theo đặc tả §5.
- `internal/dispatcher/types.go`: Định nghĩa kiểu và interface `AuthAPI`, hằng số, sự kiện, lỗi `ErrInvalidAuth`.
- `internal/dispatcher/service.go`: Cài đặt `AuthAPI`, guard BR-37, che secret detail, quản lý thu hồi/trao lease và requeue assignment không phạt attempt.
- `internal/gateway/hub.go`: Bổ sung phương thức `SendAuthRecheck`.
- `internal/gateway/client.go`: Xử lý auth trong `HANDSHAKE` và bản tin `AGENT_AUTH`.
- `internal/telegram/notifier.go`: Bổ sung thông báo Telegram cho sự kiện `AGENT_AUTH` và `WORKER_AUTH`.
- `internal/machines/handler.go`: Bổ sung trường auth vào `GET /api/my/machines`, endpoint `POST /api/my/machines/{id}/auth-recheck`, khoá `lead_eligible = 0` cho Antigravity khi join.
- `internal/api/fleet.go`: Bổ sung trường auth cho `GET /api/fleet`.
- `cmd/server/main.go`: Nối `machines.Config.Recheck` với `hub.SendAuthRecheck`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-030-agent-auth-server/verify.sh
ok  	github.com/namth/agenthub/internal/database	6.193s
ok  	github.com/namth/agenthub/internal/dispatcher	12.363s
ok  	github.com/namth/agenthub/internal/gateway	4.192s
ok  	github.com/namth/agenthub/internal/telegram	2.170s
ok  	github.com/namth/agenthub/internal/machines	5.684s
ok  	github.com/namth/agenthub/internal/api	4.728s
TASK-030 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
