# Báo cáo TASK-001

## Đã làm
- Khởi tạo Go module `github.com/namth/agenthub` với Go 1.27, sử dụng driver SQLite thuần Go `modernc.org/sqlite` (không CGO) và `github.com/go-chi/chi/v5`.
- Xây dựng `internal/database/schema.sql` gộp đầy đủ DDL theo đặc tả từ `docs/06-database-schema.md`, `docs/features/lead-rotation.md` và `docs/features/virtual-office.md`. Đầy đủ 11 bảng, các chỉ mục, ràng buộc CHECK, khóa ngoại cascade, và seed dữ liệu khởi tạo (`lead_lease`, `settings`).
- Hiện thực `internal/database/db.go`: hàm `Open` tự tạo thư mục cha nếu chưa có, cấu hình tham số DSN `_pragma` cho `foreign_keys(1)`, `journal_mode(WAL)`, `busy_timeout(5000)`, và gọi `Migrate`.
- Hiện thực `web/embed.go` và `web/dist/index.html`: nhúng tài nguyên frontend bằng `go:embed all:dist`, hàm `Static()` trả cây con `dist` phục vụ `index.html` và static assets.
- Hiện thực `internal/server/server.go`: Chi router cung cấp endpoint `/api/health` trả envelope JSON chuẩn, trả lỗi JSON 404 cho các route `/api/*` không tồn tại, phục vụ file tĩnh và SPA fallback trả về `index.html` cho các đường dẫn khác.
- Hiện thực `cmd/server/main.go`: đọc cấu hình từ biến môi trường `PORT` (mặc định 8080) và `DATABASE_PATH` (mặc định `./data/agenthub.db`), khởi tạo DB, lắng nghe HTTP và tắt êm khi nhận tín hiệu SIGINT hoặc SIGTERM.
- Tạo `Makefile` với hai target `build` và `test`.
- Đã chạy kiểm thử tự động, toàn bộ 32+ unit test PASS, không có test nào bị skip, build binary không CGO và chạy thử nghiệm HTTP / SIGTERM thành công.

## File đã tạo/sửa
- `go.mod`, `go.sum`: Cấu hình module Go 1.27 và khai báo các dependencies.
- `cmd/server/main.go`: Điểm khởi chạy server binary đơn lẻ, quản lý vòng đời và graceful shutdown.
- `internal/database/db.go`: Quản lý kết nối SQLite (`Open`) và chạy migration (`Migrate`).
- `internal/database/schema.sql`: Toàn bộ DDL SQLite và dữ liệu ban đầu.
- `internal/server/server.go`: HTTP handler với router Chi, xử lý API health, API 404, file tĩnh và SPA fallback.
- `web/dist/index.html`: Trang HTML tạm chứa chữ AgentHub.
- `web/embed.go`: Go embed cho thư mục `dist` và hàm `Static()`.
- `Makefile`: Target `build` và `test`.

## Kết quả tự kiểm
```text
== 1. gofmt
== 2. Không dùng CGO sqlite
== 3. go vet
== 4. go test
--- PASS: TestOpenCreatesFileAndAllTables (0.01s)
--- PASS: TestMigrateIsIdempotentAndSeeds (0.01s)
--- PASS: TestSchemaColumns (0.01s)
--- PASS: TestIndexesExist (0.01s)
--- PASS: TestForeignKeysOnEveryConnection (0.01s)
--- PASS: TestValidRowsAccepted (0.01s)
--- PASS: TestConstraintsRejectInvalid (0.15s)
--- PASS: TestDeleteWorkerCascadesAgents (0.01s)
ok  	github.com/namth/agenthub/internal/database	0.211s
--- PASS: TestHealth (0.01s)
--- PASS: TestUnknownAPIReturnsJSON404 (0.01s)
--- PASS: TestStaticFileServed (0.01s)
--- PASS: TestSPAFallback (0.01s)
ok  	github.com/namth/agenthub/internal/server	0.047s
--- PASS: TestStaticHasIndexAtRoot (0.00s)
ok  	github.com/namth/agenthub/web	0.005s
== 5. Build single binary (CGO tắt) và chạy ở thư mục trống
health: {"success":true,"data":{"status":"ok"},"error":null}
== 6. Tắt êm khi SIGTERM
TASK-001 ĐẠT
```

## Điểm lệch / câu hỏi cho trưởng phòng
- Về cấu hình pragma SQLite: Chọn dùng tham số DSN của `modernc.org/sqlite` (`_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)`) thay vì giới hạn `db.SetMaxOpenConns(1)`, nhằm đảm bảo mọi kết nối được mở ra bởi connection pool (kể cả khi test ép mở lại kết nối liên tục với `db.SetMaxIdleConns(0)`) đều được kích hoạt foreign keys và WAL an toàn.
- Ngoài ra: Không có điểm lệch hay câu hỏi nào khác.
