# TASK-001 — Nền Go + SQLite schema đầy đủ + single binary

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Gộp TASK-01, TASK-02 và phần bảng của TASK-10 trong `docs/11-tasks.md`.

## 1. Mục tiêu
Dựng khung server Go đầu tiên của AgentHub: một binary duy nhất, tự tạo file SQLite với **toàn bộ** schema (bảng nền + bảng FEAT-01/02), phục vụ frontend tĩnh nhúng bằng `go:embed` và một endpoint `/api/health`. Mọi task sau xây trên khung này.

## 2. Tài liệu bắt buộc đọc
- `CONTEXT.md` mục 2–3
- `docs/06-database-schema.md` mục 2 (DDL bảng nền) và 2.1
- `docs/features/lead-rotation.md` mục 5 (DDL `agents`, `jobs`, `assignments`, `lead_lease`, `lead_journal`; diff `workers`, `task_logs`)
- `docs/features/virtual-office.md` mục 5 (bảng `settings`)
- `docs/07-api-contracts.md` mục 1 (định dạng response chuẩn) và 1.5 (mã lỗi)
- `docs/09-conventions.md` mục 2.2 (quy chuẩn Go)

## 3. Phạm vi file
**Được tạo/sửa:**
- `go.mod`, `go.sum` — module `github.com/namth/agenthub`, Go 1.27
- `cmd/server/main.go`
- `internal/database/db.go`, `internal/database/schema.sql`
- `internal/server/server.go` (có thể tách thêm file trong `internal/server/` nếu cần, không phải file `_test.go`)
- `web/embed.go`, `web/dist/index.html` (trang HTML tạm, có chữ `AgentHub`)
- `Makefile` (target `build`, `test`)

**Không được sửa:**
- Mọi file trong `tests.lock` (test do trưởng phòng viết)
- `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`

## 4. Đặc tả chi tiết
**Phụ thuộc:** `modernc.org/sqlite` (driver `"sqlite"`, không CGO), `github.com/go-chi/chi/v5`. Không thêm thư viện khác.

**`internal/database`** (package `database`):
- `schema.sql` = gộp DDL của 3 tài liệu ở mục 2, đúng tên bảng/cột/CHECK/index như tài liệu, mọi câu `IF NOT EXISTS`/`INSERT OR IGNORE` để chạy lại không lỗi. Nhúng bằng `//go:embed schema.sql`.
- `func Open(path string) (*sql.DB, error)`: tạo thư mục cha nếu chưa có; mở DB; bật `PRAGMA foreign_keys = ON`, `journal_mode = WAL`, `busy_timeout = 5000` **cho mọi kết nối** (dùng tham số DSN `_pragma=...` của modernc, hoặc `db.SetMaxOpenConns(1)` — chọn một, ghi lý do trong report); gọi `Migrate`.
- `func Migrate(db *sql.DB) error`: chạy `schema.sql`, gọi lại nhiều lần không lỗi.

**`web`** (package `web`, file `web/embed.go`): `//go:embed all:dist`; `func Static() fs.FS` trả cây con `dist` (đọc `index.html` trực tiếp, không có tiền tố `dist/`). Lưu ý: `go:embed` không dùng được `..`, nên KHÔNG đặt chỉ thị embed trong `cmd/server` như ví dụ cũ ở `docs/05`.

**`internal/server`** (package `server`): `func New(db *sql.DB, static fs.FS) http.Handler` dùng chi:
- `GET /api/health` → `200`, `{"success":true,"data":{"status":"ok"},"error":null}`, `Content-Type: application/json`.
- Mọi đường `/api/*` khác → `404` JSON `{"success":false,"data":null,"error":{"code":"NOT_FOUND","message":"..."}}`.
- Đường khác: có file tĩnh tương ứng → trả file; không có → trả `index.html` (SPA fallback), `200`.

**`cmd/server/main.go`:** đọc `PORT` (mặc định `8080`), `DATABASE_PATH` (mặc định `./data/agenthub.db`); `database.Open`; nghe `:PORT` với `server.New(db, web.Static())`; log một dòng khi sẵn sàng; tắt êm khi nhận SIGINT/SIGTERM.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-001-nen-go-sqlite/verify.sh` exit 0 (gofmt, go vet, toàn bộ test, build binary, chạy binary ở thư mục khác và gọi HTTP thật)
- [ ] Không test nào bị `t.Skip`
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không viết thêm/sửa file `_test.go` đã khoá; muốn thêm test riêng thì đặt tên file mới, nhưng không bắt buộc.
- Không dùng `mattn/go-sqlite3` (CGO) — phá mục tiêu biên dịch chéo (docs/05 §2).
- Schema phải khớp tài liệu từng cột; thiếu/đổi tên cột là test đỏ. Thấy tài liệu mâu thuẫn → ghi vào mục câu hỏi của `report.md`, không tự sửa docs.
- Không triển khai logic dispatcher/lease/websocket — đó là task sau.
- `foreign_keys` phải bật trên **mọi** kết nối trong pool, không chỉ kết nối đầu.
