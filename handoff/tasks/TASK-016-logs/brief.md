# TASK-016 — Lưu log job vào `task_logs`, SSE log theo máy

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Phần backend cho SCR-06 Live terminal (TASK-07 trong `docs/11-tasks.md`).

## 1. Mục tiêu
Log mà runner stream lên được lưu (giới hạn số dòng mỗi task) và phát trực tiếp cho Admin theo từng máy.

## 2. Tài liệu bắt buộc đọc
- `docs/06-database-schema.md` §2 (bảng `task_logs`), §3 (dọn log); `docs/02` §2 (chỉ Admin xem log)
- `internal/gateway/hub.go` (cách các `Option` hiện có làm), `internal/logs/logs_test.go`, `internal/gateway/logsink_test.go`

## 3. Phạm vi file
**Được tạo/sửa:** các file `.go` (không phải `_test.go`) trong `internal/logs/`; trong `internal/gateway/` chỉ thêm option `WithLogSink`.

**Không được sửa:** file trong `tests.lock`; package khác, `cmd/`, `web/`, `go.mod`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
// gateway
func WithLogSink(fn func(workerID, assignmentID, chunk string)) Option // gọi sau khi api.Log thành công, ngoài khoá Hub
// logs
type Line struct { ID int64 `json:"id"`; WorkerID string `json:"worker_id"`; AssignmentID string `json:"assignment_id"`; TaskID string `json:"task_id"`; Chunk string `json:"chunk"`; TS time.Time `json:"ts"` }
func NewStore(db *sql.DB, now func() time.Time, keepPerTask int) *Store // keepPerTask ≤ 0 → 5000
func (s *Store) Append(workerID, assignmentID, chunk string) error     // tra task qua assignments→jobs; không có → lỗi; chunk cắt ≤ 64 KB; xoá dòng cũ vượt keepPerTask của task đó
func (s *Store) Recent(workerID string, n int) ([]Line, error)          // n dòng mới nhất của máy, thứ tự ID tăng dần
func (s *Store) Subscribe(workerID string) (<-chan Line, func())         // chỉ log của máy đó; đầy kênh → bỏ, không chặn Append
func Handler(db *sql.DB, s *Store, authz func(*http.Request) (string, bool)) http.Handler
```
- `GET /api/admin/workers/{id}/logs/stream`: chỉ ADMIN (401/403 JSON chuẩn docs/07); máy không có → 404 `NOT_FOUND`. SSE: đăng ký nhận trước, rồi gửi `event: history` với `data` = mảng `Recent(id, 500)`, sau đó mỗi dòng mới `event: log` + `data` = JSON `Line`; comment `: ping` mỗi 20 giây; flush sau mỗi frame.
- Ghi `timestamp` từ `now()`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-016-logs/verify.sh` exit 0 (gồm `-race`)
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- `Append` không bao giờ chặn vì người nghe chậm.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
