# Báo cáo TASK-010

## Đã làm
- Hiện thực Event Bus cho văn phòng ảo trong `internal/office`: bộ đệm vòng 400 sự kiện, cấp phát ID tuần tự tăng dần từ 1, lấy mốc thời gian `now()`, làm sạch `Text`.
- Bộ chuyển đổi sự kiện từ `dispatcher.Event` sang `office.Event`: ánh xạ đầy đủ theo bảng quy tắc (handover, assign, submit, cooldown, idle, vacant, offline, online, error, bỏ qua các sự kiện không cần vẽ), ghi nhớ lead hiện tại và lead bị thu hồi.
- Che chuỗi nhạy cảm và giới hạn độ dài `Text` (BR-20, TC-29): che `ghp_`, `gho_`, `sk-`, và chuỗi chữ-số liên tục >= 32 ký tự bằng `***`, cắt tối đa 80 ký tự rune.
- Đăng ký và phân phối sự kiện qua kênh có đệm, người nghe chậm bị bỏ tin và không bao giờ chặn `Emit`/`Notify`.
- Xử lý API cài đặt (`GET /api/admin/settings`, `PATCH /api/admin/settings`): chỉ cho phép ADMIN (401/403 chuẩn), validate body nghiêm ngặt chỉ chấp nhận `office_enabled` kiểu boolean (400 `VALIDATION_ERROR`), lưu cấu hình vào bảng `settings` trong SQLite. Đóng toàn bộ stream SSE đang mở khi tắt văn phòng.
- Xử lý endpoint SSE (`GET /api/admin/office/stream`): chỉ ADMIN, trả về 404 khi văn phòng tắt, giới hạn kết nối đồng thời `MaxStreams` (trả về 429 `TOO_MANY_CONNECTIONS`). Đăng ký nhận trước rồi mới gửi; hỗ trợ nối lại qua `Last-Event-ID` (gửi bù các sự kiện thiếu nếu còn trong bộ đệm, gửi `event: snapshot` kèm `recent` nếu đã rơi khỏi bộ đệm hoặc không có header). Gửi heartbeat comment `: ping` định kỳ.
- Tự kiểm bằng `python3 scripts/handoff.py verify TASK-010` đạt exit code 0 với 8/8 test PASS (bao gồm kiểm tra `-race`), không phá vỡ test của các package cũ.

## File đã tạo/sửa
- `internal/office/event.go`: Định nghĩa struct `Event` và hàm `cleanText` che secret, giới hạn 80 rune.
- `internal/office/bus.go`: Cài đặt `Bus`, bộ đệm vòng, các hàm `NewBus`, `Notify`, `Emit`, `Since`, `Subscribe`.
- `internal/office/handler.go`: Cài đặt `Handler`, cấu hình `Options`, các endpoint `/api/admin/settings` và `/api/admin/office/stream` (SSE).

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-010-office-sse/verify.sh
== 1. gofmt + vet
== 2. Test office (-race)
--- PASS: TestMapping (0.00s)
--- PASS: TestRedactAndTruncate (0.00s)
--- PASS: TestRingBufferAndIDs (0.00s)
--- PASS: TestRBACAndDisabled (0.04s)
--- PASS: TestStreamSnapshotThenLive (0.03s)
--- PASS: TestLastEventIDResumes (0.03s)
--- PASS: TestMaxStreams (0.04s)
--- PASS: TestDisableClosesStreams (0.03s)
ok  	github.com/namth/agenthub/internal/office	1.202s
== 3. Không phá test cũ
go: downloading github.com/golang-jwt/jwt/v5 v5.3.1
ok  	github.com/namth/agenthub/internal/database	0.291s
ok  	github.com/namth/agenthub/internal/dispatcher	0.529s
ok  	github.com/namth/agenthub/internal/gateway	1.085s
ok  	github.com/namth/agenthub/internal/runner	4.193s
ok  	github.com/namth/agenthub/internal/server	0.053s
ok  	github.com/namth/agenthub/internal/auth	0.129s
ok  	github.com/namth/agenthub/web	0.007s
TASK-010 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
