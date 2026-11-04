# Báo cáo TASK-016

## Đã làm
- Hiện thực hoá `Option` `WithLogSink` trong package `gateway` (`hub.go`, `client.go`), kích hoạt hook sau khi `api.Log` thành công mà không giữ khoá Hub.
- Định nghĩa kiểu `Line` với các trường và thẻ JSON theo đúng đặc tả.
- Hiện thực hoá `Store` trong package `logs` (`store.go`):
  - `NewStore(db, now, keepPerTask)`: fallback `keepPerTask <= 0` về 5000, `now == nil` về `time.Now`.
  - `Append(workerID, assignmentID, chunk)`: tra cứu `task_id` qua bảng `assignments` và `jobs`; báo lỗi nếu không tồn tại; cắt `chunk` tối đa 64 KB; dọn log cũ vượt quá `keepPerTask` của task; phát dòng log đến các subscriber tương ứng qua kênh non-blocking.
  - `Recent(workerID, n)`: truy vấn `n` dòng log mới nhất của máy, sắp xếp theo ID tăng dần.
  - `Subscribe(workerID)`: trả về kênh nhận log `<-chan Line` và hàm huỷ `func()`, không chặn `Append` khi kênh đầy, chỉ gửi log của máy được đăng ký.
- Hiện thực hoá `Handler` trong package `logs` (`handler.go`):
  - Endpoint `GET /api/admin/workers/{id}/logs/stream` yêu cầu quyền ADMIN (trả về 401/403 định dạng JSON chuẩn).
  - Trả về 404 `NOT_FOUND` nếu worker không tồn tại trong database.
  - SSE stream: đăng ký subscriber trước, gửi frame ban đầu `event: history` với mảng `Recent(id, 500)`, sau đó gửi mỗi dòng mới `event: log` với JSON của `Line`.
  - Gửi comment `: ping\n\n` mỗi 20 giây và flush sau mỗi frame.
- Tự kiểm toàn bộ test và kiểm tra race condition bằng `python3 scripts/handoff.py verify TASK-016`.

## File đã tạo/sửa
- `internal/gateway/hub.go`: Thêm trường `logSink` và hàm cấu hình `WithLogSink(fn)`.
- `internal/gateway/client.go`: Gọi `logSink` khi nhận `LOG_CHUNK` sau khi `api.Log` thành công.
- `internal/logs/store.go`: Định nghĩa struct `Line`, `Store` cùng các phương thức `NewStore`, `Append`, `Recent`, `Subscribe`.
- `internal/logs/handler.go`: Định nghĩa `Handler` phục vụ endpoint SSE stream log cho Admin.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-016-logs/verify.sh
--- PASS: TestAppendStoresWithTask (0.05s)
--- PASS: TestRetentionPerTask (0.09s)
--- PASS: TestRecentAndSubscribe (0.15s)
--- PASS: TestStreamHTTP (0.17s)
ok  	github.com/namth/agenthub/internal/logs	1.478s
--- PASS: TestRejectsMissingOrBadToken (0.00s)
--- PASS: TestHandshakeConnects (0.00s)
--- PASS: TestMessageBeforeHandshakeRejected (0.00s)
--- PASS: TestPingPong (0.00s)
--- PASS: TestRequestTaskAssignOrNoTask (0.00s)
--- PASS: TestLogAndSubmitForwarded (0.02s)
--- PASS: TestExhaustedParsesFields (0.04s)
--- PASS: TestLeaseRenewRepliesGranted (0.00s)
--- PASS: TestLeadReleaseAndActions (0.02s)
--- PASS: TestDomainErrorsMapped (0.00s)
--- PASS: TestBadMessagesKeepConnection (0.00s)
--- PASS: TestCloseCallsDisconnect (0.02s)
--- PASS: TestReconnectReplacesOldConnection (0.00s)
--- PASS: TestNotifyRoutesToOwningWorker (0.91s)
--- PASS: TestNotifyNeverBlocks (0.07s)
--- PASS: TestEnvelopeIsFlatJSON (0.00s)
--- PASS: TestWithPromptFillsPayload (0.00s)
--- PASS: TestWithoutPromptOptionStillWorks (0.00s)
--- PASS: TestSubmitHookAfterSuccessOnly (0.02s)
--- PASS: TestWithLogSink (0.02s)
--- PASS: TestHashTokenIsSHA256Hex (0.00s)
--- PASS: TestDBAuthenticator (0.05s)
--- PASS: TestRegisterWorker (0.03s)
ok  	github.com/namth/agenthub/internal/gateway	2.245s
TASK-016 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
