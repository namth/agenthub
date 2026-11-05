# Báo cáo TASK-041

## Đã làm
- **projects:** Mở rộng `PATCH /api/admin/projects/{id}` nhận và validate `allowed_commands` (≤ 40 mục, regex `^[a-zA-Z0-9_./ -]{1,60}$` sau trim, không `sudo`, không `*`, không `rm -rf /`), `needs_ci_for_integration`, `is_sandbox`. Lưu JSON vào cột `allowed_commands` của `projects`. Cập nhật `GET /api/projects` trả thêm 3 trường này (`allowed_commands` rỗng `[]` khi NULL).
- **pipeline:** Định nghĩa `DefaultAllowedCommands()` trả về 10 lệnh mặc định. Hiện thực hoá `PayloadFor(a dispatcher.Assignment)` gộp danh sách lệnh mặc định và lệnh riêng của dự án (giữ thứ tự, mặc định trước, bỏ trùng), cùng trường `needs_ci`.
- **gateway:** Bổ sung `WithPayloadExtras(fn func(dispatcher.Assignment) map[string]any)` và gộp vào payload của bản tin `ASSIGN_TASK` mà không ghi đè các trường có sẵn.
- **cmd/server/main.go:** Nối dây `gateway.WithPayloadExtras(pl.PayloadFor)`.
- **runner:** Khi `payload.allowed_commands` có mặt:
  - Agent `CLAUDE`: nối thêm `--permission-mode acceptEdits --allowedTools Read Edit Write Glob Grep Bash(<prefix>:*)…`.
  - Agent `ANTIGRAVITY`: nối thêm `--settings <JSON>` với `permissions.allow` chứa các `command(<từ đầu tiên của prefix>)` bỏ trùng.
  - Khi `needs_ci = true`: chèn ghi chú "Dự án dùng CI cho test tích hợp: không chạy test cần secret, dựa vào CI gate." vào cuối prompt trong khối DỮ LIỆU.
  - Tuyệt đối không dùng chế độ bỏ qua quyền. Khi không có `allowed_commands`, giữ nguyên argv cũ.

## File đã tạo/sửa
- `internal/projects/handler.go`: Bổ sung trường trong `projectListItem`, logic scan DB, và xử lý validate/patch cho `allowed_commands`, `needs_ci_for_integration`, `is_sandbox`.
- `internal/pipeline/pipeline.go`: Thêm `DefaultAllowedCommands()` và `PayloadFor()`.
- `internal/gateway/hub.go`: Thêm option `WithPayloadExtras` và trường lưu trữ callback trên `Hub`.
- `internal/gateway/client.go`: Gộp extras từ `payloadExtrasFn` vào `ASSIGN_TASK.payload` mà không ghi đè các key có sẵn.
- `internal/runner/runner.go`: Parse `allowed_commands` và `needs_ci`, cấu hình cờ phiên cho `CLAUDE` và `ANTIGRAVITY`, thêm ghi chú CI vào prompt.
- `cmd/server/main.go`: Nối `gateway.WithPayloadExtras(pl.PayloadFor)`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-041-cli-allowlist/verify.sh
ok  	github.com/namth/agenthub/internal/runner	22.054s
ok  	github.com/namth/agenthub/internal/pipeline	10.614s
ok  	github.com/namth/agenthub/internal/gateway	3.798s
ok  	github.com/namth/agenthub/internal/projects	6.264s
TASK-041 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
