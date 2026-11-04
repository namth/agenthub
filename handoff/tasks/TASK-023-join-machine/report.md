# Báo cáo TASK-023

## Đã làm
- Hiện thực toàn bộ API và logic thêm máy (`internal/machines/`):
  - `POST /api/fleet/join-codes`: xác thực đăng nhập, kiểm tra tên máy hợp lệ (`^[a-zA-Z0-9-]{2,40}$`), kiểm tra xung đột tên máy đã có của người khác, sinh mã ngẫu nhiên 8 ký tự `XXXX-XXXX` từ bảng ký tự không gây nhầm lẫn, lưu SHA-256 vào bảng `join_codes`, hết hạn 15 phút, trả lệnh cài `command` và khối hướng dẫn Markdown `agent_prompt` đầy đủ ràng buộc.
  - `POST /api/fleet/join`: công khai, kiểm soát tần suất lỗi bằng sliding window (> 5 lần lỗi/phút/IP trả 429 `TOO_MANY_REQUESTS`), kiểm tra mã hợp lệ/chưa dùng/còn hạn và đánh dấu nguyên tử `UPDATE join_codes SET used_at = ... WHERE used_at IS NULL`, cấp `runner_token` 32 byte hex và lưu hash, tạo/ghi đè bản ghi trong `workers` và `agents` (`lead_eligible = 1` với owner, 0 với partner).
  - `GET /install.sh`: phục vụ script POSIX `sh` (đã kiểm tra cú pháp `bash -n`), nhúng bằng `//go:embed`, tự động thay thế `__BASE_URL__`, phát hiện OS/CPU, tải binary và kiểm tra checksum SHA-256 trước khi gọi join, lưu token quyền `0600` vào `$AGENTHUB_HOME/token` tuyệt đối không in token, cài dịch vụ khởi động `io.agenthub.runner` (macOS caffeinate) hoặc systemd user (Linux) trừ khi có cờ `--no-service`, in bảng kiểm công cụ hệ thống `git / gh / claude / agy / go / node`.
  - `GET /download/<file>`: chỉ phục vụ file khớp mẫu `^agenthub-(darwin|linux)-(arm64|amd64)(\.sha256)?$` từ `RunnersDir`, chống path traversal.
  - `GET /api/my/machines`: yêu cầu đăng nhập, trả danh sách máy của người dùng kèm danh sách agent, dự án được gán và job đang chạy (ACTIVE assignment).
  - `PATCH /api/my/machines/{id}`: chỉ chủ máy hoặc system owner, hỗ trợ cập nhật `paused`, gán danh sách `projects` cho tất cả agent của máy nếu người dùng là thành viên (`projects: null` xoá toàn bộ danh sách để nhận mọi dự án).
  - `DELETE /api/my/machines/{id}`: chỉ chủ máy hoặc system owner, chuyển các assignment ACTIVE sang `RECLAIMED` và job về `QUEUED`, reset trạng thái agent về `IDLE`, đổi `token_hash` thành chuỗi ngẫu nhiên không thể dùng lại, gọi callback `OnRemoved(id)`, trả HTTP 204 No Content.
- Bổ sung unit tests trong `internal/machines/machines_extra_test.go` và `export_test.go` kiểm tra định dạng mã, regex tải binary, cơ chế sliding window rate limiter, nâng tổng số test PASS lên 12 (đạt yêu cầu >= 10).
- Chạy tự kiểm thành công toàn bộ verify script bao gồm `gofmt`, `go vet`, kiểm tra không log mã/token, `go test -race` và kiểm thử toàn bộ dự án (`go build ./... && go test ./...`).

## File đã tạo/sửa
- `internal/machines/handler.go`: HTTP handler, router, các endpoint join-codes, join, download, my machines, rate limiter.
- `internal/machines/install.sh`: Script cài đặt máy POSIX sh, hỗ trợ cờ và biến môi trường override, kiểm tra toàn vẹn và dịch vụ nền.
- `internal/machines/export_test.go`: Export các hàm nội bộ phục vụ kiểm thử ngoài package.
- `internal/machines/machines_extra_test.go`: Unit tests bổ sung kiểm tra bộ sinh mã, regex download, rate limiter.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-023-join-machine/verify.sh
--- PASS: TestExtraJoinCodeGenerator (0.00s)
--- PASS: TestExtraDownloadRegex (0.00s)
--- PASS: TestExtraRateLimiterSlidingWindow (0.00s)
--- PASS: TestJoinCodeShapeAndPrompt (0.11s)
--- PASS: TestJoinOnceAndOwnership (0.11s)
--- PASS: TestJoinExpiredAndRateLimit (0.10s)
--- PASS: TestJoinNameTakenByOther (0.10s)
--- PASS: TestMyMachinesScope (0.12s)
--- PASS: TestPauseAndProjects (0.10s)
--- PASS: TestRemoveMachine (0.11s)
--- PASS: TestDownload (0.10s)
--- PASS: TestInstallScriptEndToEnd (1.15s)
ok  	github.com/namth/agenthub/internal/machines	3.355s
TASK-023 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
