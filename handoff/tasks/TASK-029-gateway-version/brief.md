# TASK-029 — Gateway: trao đổi phiên bản trong handshake, CloseWorker

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-05: DEC-19, BR-32, BR-35

## 1. Mục tiêu
Runner biết phiên bản server để tự cập nhật (DEC-19); server lưu phiên bản runner để SCR-05/SCR-11 hiển thị; gỡ máy thì đóng kết nối WebSocket ngay (BR-32).

## 2. Tài liệu bắt buộc đọc
- `docs/features/join-machine.md` §6, §7 (BR-32, BR-35); `docs/07` mục WebSocket
- Test đặc tả: `internal/gateway/version_test.go`; code: `internal/gateway/hub.go`, `internal/gateway/client.go`

## 3. Phạm vi file
**Được tạo/sửa:** file không phải `_test.go` trong `internal/gateway/`.

**Không được sửa:** file trong `tests.lock`; package khác; `web/`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- `func WithServerVersion(v string) Option` — `HANDSHAKE_OK` thêm `"server_version": v` khi `v != ""` (không có option → giữ nguyên như cũ).
- `func WithVersionSink(fn func(workerID, version string)) Option` — `HANDSHAKE.data.version` (string) khác rỗng → gọi `fn(workerID, version)` sau khi `Connect` thành công và **trước khi** gửi `HANDSHAKE_OK`. Rỗng → không gọi.
- `func (h *Hub) CloseWorker(workerID string) bool` — có kết nối của worker đó → đóng nó (đi qua đường `removeClient` bình thường để dispatcher nhận `Disconnect`), trả `true`; không có → `false`. Không ảnh hưởng worker khác; an toàn khi gọi đồng thời (giữ `h.mu` đúng cách, không gọi `close()` trong khi giữ khoá nếu `close` có thể chờ).

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-029-gateway-version/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không đổi hành vi cũ; mọi test gateway cũ phải xanh (chạy với `-race`).
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
