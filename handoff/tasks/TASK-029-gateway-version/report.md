# Báo cáo TASK-029

## Đã làm
- Thêm option `WithServerVersion(v string)`: khi `v != ""`, phản hồi `HANDSHAKE_OK` trả kèm `"server_version": v`. Khi không cấu hình hoặc rỗng thì giữ nguyên như cũ.
- Thêm option `WithVersionSink(fn func(workerID, version string))`: khi `HANDSHAKE.data.version` khác rỗng, gọi hàm `fn` sau khi `Connect` thành công và trước khi gửi `HANDSHAKE_OK`. Khi rỗng thì không gọi.
- Cập nhật `handshakeData` nhận trường `version` từ `HANDSHAKE.data`.
- Hiện thực phương thức `(h *Hub) CloseWorker(workerID string) bool`: tìm kết nối của worker tương ứng và đóng kết nối (không giữ mutex `h.mu` khi đóng), để chu trình `removeClient` thông thường thông báo `Disconnect` cho dispatcher, trả về `true` nếu tìm thấy và đóng kết nối, `false` nếu không có hoặc đã đóng; an toàn khi gọi đồng thời và không ảnh hưởng worker khác.

## File đã tạo/sửa
- `internal/gateway/hub.go`: Thêm trường cấu hình `serverVersion`, `versionSink` trong struct `Hub`, thêm các hàm option `WithServerVersion`, `WithVersionSink`, và phương thức `CloseWorker`.
- `internal/gateway/client.go`: Cập nhật `handshakeData` có trường `Version`, gọi `versionSink` và gắn `server_version` vào `HANDSHAKE_OK` trong `handleHandshake`; cập nhật `close()` trả về `bool` nhận diện đóng lần đầu bằng `sync.Once`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-029-gateway-version/verify.sh
ok  	github.com/namth/agenthub/internal/gateway	2.574s
TASK-029 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
