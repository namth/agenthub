# Report TASK-003 — Gateway WebSocket

> Ghi bởi trưởng phòng: agent viết xong code (auth.go, client.go, hub.go, register.go) nhưng phiên agy dừng trước khi tự viết report/nộp (bị chặn đọc file ngoài repo trong module cache).

## Đã làm
- Hub WebSocket `/ws/agent`: xác thực `X-Runner-Token`, bắt tay, ánh xạ bản tin → `dispatcher.API`, lỗi nghiệp vụ → `ERROR`, `Notify` không chặn (hàng đợi 256/kết nối).
- `RegisterHandler`: `POST /api/fleet/register`, token `crypto/rand`, chỉ lưu SHA-256, so khoá hằng thời gian.
- Thêm `github.com/gorilla/websocket v1.5.3`.

## Lệch so với brief
- Lỗi đỏ duy nhất (`TestNotifyRoutesToOwningWorker`) là do helper đọc trong test của trưởng phòng; test đã được sửa và khoá lại, code không đổi.

## Output verify (30 dòng cuối)
```
== 1. gofmt + vet
== 2. Phụ thuộc
== 3. Không lộ token
== 4. Test gateway (-race)
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
--- PASS: TestNotifyNeverBlocks (0.05s)
--- PASS: TestEnvelopeIsFlatJSON (0.00s)
--- PASS: TestHashTokenIsSHA256Hex (0.00s)
--- PASS: TestDBAuthenticator (0.06s)
--- PASS: TestRegisterWorker (0.05s)
ok  	github.com/namth/agenthub/internal/gateway	2.511s
== 5. Không phá test cũ
ok  	github.com/namth/agenthub/internal/database	0.430s
ok  	github.com/namth/agenthub/internal/server	0.339s
ok  	github.com/namth/agenthub/web	0.449s
TASK-003 ĐẠT
```
