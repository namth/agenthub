# Báo cáo TASK-042

## Đã làm
- Bổ sung `DefaultAuthPatterns()` các mẫu: `failed to authenticate`, `oauth`, `session expired`, `token expired`, `could not be refreshed` (áp dụng cho probe và lỗi job BR-39).
- Thêm `DefaultAuthProbes()` cho CLAUDE và ANTIGRAVITY với prompt cực ngắn `-p "Trả lời đúng một từ: OK"`, không có cờ bỏ qua quyền.
- Hiện thực `ParseProbeResult(exit int, output string, err error) (status, detail string)` chuẩn hóa detail ≤ 80 ký tự, che secret.
- Mở rộng `Config` thêm `AuthProbes map[string][]string` (nil = không probe), `ProbeEvery` (mặc định 6h), `ProbeTimeout` (mặc định 60s); `Main` đặt `AuthProbes: DefaultAuthProbes()`.
- Lúc bắt tay: sau lệnh kiểm `AuthChecks`, chạy probe cho các kind có probe cấu hình; kết quả `OK` hoặc `NEEDS_AUTH` ghi đè lệnh kiểm, `UNKNOWN` giữ nguyên kết quả lệnh kiểm.
- Chạy định kỳ `ProbeEvery`: khi agent không BUSY, chạy probe, chỉ gửi `AGENT_AUTH` khi trạng thái thay đổi; chuyển từ `NEEDS_AUTH` sang trạng thái khác thì gọi `maybeSendRequestTask`.
- Xử lý `AUTH_RECHECK`: chạy cả lệnh kiểm lẫn probe, agent có probe (như agy) trả về kết quả thật thay vì `UNKNOWN` cố định.
- Không giữ lock `r.mu` khi chạy probe và không chạy probe trước mỗi `REQUEST_TASK`.
- Chạy `verify.sh` đạt 10/10 test case nghiệm thu với race detector `-race`.

## File đã tạo/sửa
- `internal/runner/auth.go`: Thêm mẫu OAuth, `DefaultAuthProbes`, `ParseProbeResult`, cập nhật `handleAuthRecheck`, `performPeriodicAuthCheck`, và thêm `performPeriodicAuthProbe`.
- `internal/runner/runner.go`: Thêm các trường config probe và khởi tạo mặc định trong `New`, chạy probe lúc bắt tay và khởi tạo ticker định kỳ `ProbeEvery`.
- `internal/runner/main.go`: Khởi tạo `AuthProbes: DefaultAuthProbes()`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-042-runner-auth-probe/verify.sh
ok  	github.com/namth/agenthub/internal/runner	18.173s
go: downloading github.com/go-chi/chi/v5 v5.5.0
go: downloading github.com/golang-jwt/jwt/v5 v5.3.1
TASK-042 ĐẠT
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
