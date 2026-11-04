# Báo cáo TASK-046

## Đã làm
- Hiện thực hoá package `internal/tokens`:
  - `Store` với các phương thức `Create`, `List`, `Revoke`, `Authenticate`.
  - Sinh token prefix `"ahp_"` + 32 byte `crypto/rand` mã hoá base64url không padding, lưu hash SHA-256 hex và last4 trong bảng `personal_tokens`.
  - Kiểm tra hợp lệ: `days` ∈ {30, 90, 365}, tên 1–40 ký tự, tối đa 10 token hiệu lực/người (`ErrLimit`), tên trùng còn hiệu lực (`ErrConflict`).
  - Throttling `last_used_at` tối đa 1 lần/phút khi gọi `Authenticate`.
  - HTTP handler `tokens.Handler`: phục vụ `GET /api/me/tokens`, `POST /api/me/tokens` (trả token 1 lần ở mã 201), `DELETE /api/me/tokens/{id}` (kiểm quyền ADMIN).
- Hiện thực hoá package `internal/mcp`:
  - MCP Streamable HTTP tối thiểu theo JSON-RPC 2.0, không dùng thư viện ngoài.
  - Xác thực Bearer token qua `tokens.Store`, giới hạn chỉ owner/ADMIN (DEC-39), trả 401 khi thiếu/sai token, 403 khi partner, 429 khi quá 60 lời gọi/phút/token.
  - Hỗ trợ các phương thức JSON-RPC: `initialize`, `notifications/initialized` (202), `tools/list` (8 tools với mô tả tiếng Việt và inputSchema), `tools/call`.
  - Lọc và che secret BR-20 trước khi trả kết quả tool call.
  - Xử lý các mã lỗi JSON-RPC chuẩn: `-32700` (Parse error), `-32601` (Method not found), `-32602` (Invalid params).
- Hiện thực hoá `cmd/server/mcp_backend.go` cài đặt `mcp.Backend` trên `tasks.Service`, `sql.DB` và `dispatcher.API`. Để sẵn chỗ nối trả lỗi "chưa hỗ trợ" cho `AnswerClarification` và `DecideProposal`.
- Gắn route `/mcp` và `/api/me/tokens` trong `cmd/server/main.go`.
- Giao diện SCR-12 Hồ sơ:
  - Trang `web/src/pages/Profile.tsx`: hiển thị thông tin tài khoản, danh sách token cá nhân, form tạo token (tên + hạn 30/90/365 ngày), hộp hiển thị token mới một lần kèm nút sao chép và đoạn lệnh cấu hình Claude Code MCP, nút thu hồi kèm xác nhận. 4 trạng thái Empty, Loading, Error, Success. Đối tác không có phần quản lý token.
  - Cập nhật route trong `web/src/App.tsx`, định nghĩa điều hướng trong `web/src/nav.ts` và liên kết Hồ sơ trong `web/src/layout/AppShell.tsx`.

## File đã tạo/sửa
- `internal/tokens/tokens.go`: Quản lý lưu trữ, sinh token, xác thực và HTTP handler cho token cá nhân.
- `internal/mcp/mcp.go`: Giao thức MCP Streamable HTTP JSON-RPC 2.0 và 8 tools điều hành.
- `cmd/server/mcp_backend.go`: Cài đặt `mcp.Backend` kết nối với service backend của AgentHub.
- `cmd/server/main.go`: Khởi tạo store, backend và mount `/mcp`, `/api/me/tokens`.
- `web/src/pages/Profile.tsx`: Trang SCR-12 Hồ sơ cá nhân và quản trị token MCP.
- `web/src/nav.ts`: Thêm `SCR-12` vào danh mục điều hướng `NAV`.
- `web/src/App.tsx`: Đăng ký route `/profile` hiển thị `Profile`.
- `web/src/layout/AppShell.tsx`: Thêm liên kết Hồ sơ trong menu tài khoản người dùng (cả desktop và mobile).

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-046-tokens-mcp/verify.sh
ok  	github.com/namth/agenthub/internal/tokens	1.857s
ok  	github.com/namth/agenthub/internal/mcp	1.924s
 Test Files  14 passed (14)
      Tests  145 passed (145)
TASK-046 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
