# TASK-010 — Văn phòng ảo backend: event bus, SSE, bật/tắt

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Ứng với TASK-15 (FEAT-02). Giao diện SCR-08 làm ở task sau.

## 1. Mục tiêu
Chuyển sự kiện dispatcher thành sự kiện "văn phòng", giữ 400 sự kiện gần nhất trong RAM và phát cho trình duyệt Admin qua SSE; công tắc bật/tắt trong `settings`.

## 2. Tài liệu bắt buộc đọc
- `docs/features/virtual-office.md` §2–7 (bảng chuyển sự kiện, BR-19 → BR-23); `docs/07-api-contracts.md` §1, §1.5
- `internal/dispatcher/types.go` (`Ev*`, kể cả `EvJobAssigned`, `EvJobSubmitted`); `internal/database/schema.sql` (bảng `settings`)
- `internal/office/office_test.go` — đặc tả chi tiết nhất

## 3. Phạm vi file
**Được tạo/sửa:** các file `.go` (không phải `_test.go`) trong `internal/office/`.

**Không được sửa:** file trong `tests.lock`; package khác, `cmd/`, `web/`, `go.mod`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
type Event struct {
    ID   int64     `json:"id"`
    TS   time.Time `json:"ts"`
    Who  string    `json:"who"`
    Type string    `json:"type"`
    To   string    `json:"to,omitempty"`
    Task string    `json:"task,omitempty"`
    Text string    `json:"text,omitempty"`
}
func NewBus(size int, now func() time.Time, loc *time.Location) *Bus
func (b *Bus) Notify(e dispatcher.Event)           // thoả dispatcher.Notifier, không chặn
func (b *Bus) Emit(e Event)                         // nguồn khác (duyệt task…); gán ID tăng dần từ 1, TS = now(), làm sạch Text
func (b *Bus) Since(id int64) ([]Event, bool)       // sự kiện có ID > id; ok=false nếu sự kiện id+1 đã rơi khỏi bộ đệm
func (b *Bus) Subscribe() (<-chan Event, func())    // kênh có đệm; người nghe chậm bị bỏ tin, không chặn Emit
type Options struct { PingEvery time.Duration; MaxStreams int } // 0 → 20s, 5
func Handler(db *sql.DB, b *Bus, snapshot func() (any, error), authz func(*http.Request) (string, bool), opt Options) http.Handler
```
- **Chuyển sự kiện** (bus nhớ lead hiện tại và agent bị thu hồi gần nhất): `LEASE_GRANTED` → `{who: <lead cũ nếu vừa bị thu hồi, không thì "system">, type: handover, to: <lead mới>, text: "epoch N"}`; `LEASE_REVOKED` → chỉ ghi nhớ; `JOB_ASSIGNED` → `{who: <lead hiện tại hoặc "system" khi trống lead>, type: assign, to: agent, task}`; `JOB_SUBMITTED` → `{who: agent, type: submit}`; `AGENT_COOLDOWN` → `{who: agent, type: cooldown, text: "đến HH:MM"}` theo `loc`; `AGENT_IDLE` → idle; `LEAD_VACANT` → `{who: system, type: vacant, text: "dự kiến HH:MM" | "chưa rõ"}` và xoá lead hiện tại; `LEAD_RESTORED` bỏ qua (LEASE_GRANTED đã vẽ); `WORKER_OFFLINE/ONLINE` → `{who: worker, type: offline|online}`; `JOB_FAILED` → `{who: system, type: error, task}`; còn lại bỏ qua.
- **BR-20:** che `ghp_…`, `gho_…`, `sk-…`, mọi chuỗi chữ-số liên tục ≥ 32 ký tự bằng `***`; cắt `Text` ≤ 80 ký tự (rune).
- **HTTP** (mọi đường chỉ ADMIN: 401/403 JSON chuẩn docs/07): `GET /api/admin/settings` → `data {office_enabled: bool}`; `PATCH /api/admin/settings` body đúng một khoá `office_enabled` kiểu bool → lưu `settings` (`"true"`/`"false"`), khác → 400 `VALIDATION_ERROR`. Tắt → đóng mọi stream đang mở.
- `GET /api/admin/office/stream`: tắt → 404 `NOT_FOUND`; quá `MaxStreams` → 429 `TOO_MANY_CONNECTIONS`. `Content-Type: text/event-stream`, flush sau mỗi frame. **Đăng ký nhận trước** rồi mới gửi: có `Last-Event-ID` và `Since` ok → gửi bù các sự kiện thiếu; ngược lại gửi `event: snapshot` với `data` = object của `snapshot()` cộng khoá `recent` (mọi sự kiện trong bộ đệm). Sự kiện sống: `id: <ID>` / `event: office` / `data: <JSON Event>`. Comment `: ping` mỗi `PingEvery`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-010-office-sse/verify.sh` exit 0 (gồm `-race`)
- [ ] Không test nào bị skip; `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- `Notify`/`Emit` không bao giờ chặn; không gọi `snapshot()` khi giữ khoá của Bus.
- `ls`/`cat`/`grep` không được phép qua run_command — dùng công cụ đọc/liệt kê file có sẵn. Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
