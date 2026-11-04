# TASK-033 — Office server bản 2: presence, snapshot users/auth, quyền partner, giới hạn SSE

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-02 bản 2: DEC-26, DEC-27, DEC-30; BR-23 (sửa), BR-44, BR-45

## 1. Mục tiêu
Văn phòng ảo biết ai đang online (khu chỉ huy), snapshot có danh sách người dùng + trạng thái đăng nhập agent, partner xem được, giới hạn SSE theo người.

## 2. Tài liệu bắt buộc đọc
- `docs/features/virtual-office.md` §2, §3 (bảng sự kiện: dòng `auth`, `presence`, `approve`), §5, §6, §7 (BR-23, BR-44, BR-45)
- Test đặc tả: `internal/office/office_v2_test.go`, `internal/office/office_test.go` (đã sửa TC-30), `internal/tasks/approved_test.go`

## 3. Phạm vi file
**Được tạo/sửa:** file không phải `_test.go` trong `internal/office/`; `internal/tasks/service.go`, `internal/tasks/handler.go` (chỉ thêm hook duyệt); `cmd/server/main.go` (nối dây).

**Không được sửa:** file trong `tests.lock` và mọi `_test.go`; package khác; `web/`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**`office.Options`** thêm: `MaxPerUser int` (mặc định 3; chỉ áp dụng khi có `User`), `User func(*http.Request) (userID string, ok bool)`, `Presence *Presence`, `TelegramEnabled bool`. `MaxStreams` mặc định đổi thành **30**.

**Route:** `GET /api/office/stream` — mọi người dùng đã đăng nhập (`authz` ok, không xét vai) → như stream cũ; `GET /api/admin/office/stream` → `308` tới `/api/office/stream` (giữ query). Vượt `MaxPerUser` của cùng `userID` hoặc vượt `MaxStreams` tổng → `429 TOO_MANY_CONNECTIONS`. Có `Presence` → gọi `StreamOpened(userID)` khi mở, `StreamClosed(userID)` khi đóng. `GET /api/admin/settings` thêm `"telegram_enabled": opt.TelegramEnabled` (vẫn chỉ ADMIN).

**`Bus.Notify`** thêm: `dispatcher.EvAgentAuth` → `Event{Who: AgentID, Type: "auth", Text: AuthStatus}` **chỉ khi** `AuthStatus` hoặc `PrevAuthStatus` là `NEEDS_AUTH`; `dispatcher.EvWorkerAuth` → tương tự với `Who: WorkerID`.

**Presence** (file mới):
```go
func NewPresence(db *sql.DB, now func() time.Time, bus *Bus, isOwner func(userID string) bool) *Presence
func (p *Presence) Touch(userID string)                   // ghi users.last_seen_at tối đa 1 lần / 60s / người (so trong RAM)
func (p *Presence) StreamOpened(userID string); func (p *Presence) StreamClosed(userID string)
func (p *Presence) Online(userID string) bool             // last_seen ≥ now−5 phút hoặc đang mở ≥ 1 stream
func (p *Presence) Tick()                                  // so trạng thái online với lần trước, phát Event{Who:"user:"+username, Type:"presence", Text:"online"|"offline"} khi đổi
func (p *Presence) Users() ([]UserView, error)             // thứ tự BR-44: owner → partner online (theo username) → partner vắng (theo username)
func (p *Presence) Middleware(userOf func(*http.Request) (string, bool)) func(http.Handler) http.Handler // gọi Touch rồi chuyển tiếp
type UserView struct { Login string `json:"login"`; AvatarURL string `json:"avatar_url"`; IsOwner bool `json:"is_owner"`; Online bool `json:"online"` }
```
**Snapshot:** `func BuildSnapshot(db *sql.DB, lead dispatcher.Lease, p *Presence) (map[string]any, error)` — `lead {state, agent_id (null khi VACANT), epoch}`; `workers[]` sắp theo id: `{id, conn_status, gh_auth, owner (login hoặc null), agents[]: {id, kind, status, auth_status, cooldown_until (RFC3339|null), job ({task, kind} của assignment ACTIVE | null)}}`; `users` = `p.Users()` (mảng rỗng nếu `p == nil`). `cmd/server/main.go`: thay `fleetSnapshot` bằng `office.BuildSnapshot`, tạo `Presence` (owner = `isOwnerID`), bọc router API bằng `Presence.Middleware`, chạy `Tick` mỗi 60s, truyền `User`/`Presence`/`TelegramEnabled` vào `office.Options`, đăng ký route `/api/office/*`.

**Duyệt task:** `tasks.WithOnApproved(fn func(taskID, userID string)) Option` — handler `POST /api/tasks/{id}/approve` gọi `fn(taskID, userID)` sau khi `Approve` thành công (không gọi khi lỗi). `main.go`: `fn` phát `office.Event{Who: "user:"+username, Type: "approve", Task: taskID}`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-033-office-server-v2/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không gọi DB/snapshot khi giữ khoá của `Bus`; `Tick`/`Touch` an toàn khi gọi đồng thời (`-race`).
- `User == nil` → giữ hành vi cũ (chỉ giới hạn tổng) để test cũ xanh.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
