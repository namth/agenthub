# TASK-046 — Token cá nhân, MCP `/mcp` và SCR-12 Hồ sơ

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-10: DEC-39, BR-62, BR-63 · TC-92, TC-93

## 1. Mục tiêu
Owner điều hành AgentHub từ Claude Code trên máy bất kỳ / Claude app qua MCP, xác thực bằng token cá nhân tạo trên trang Hồ sơ.

## 2. Tài liệu bắt buộc đọc
- `docs/features/remote-command.md` §2, §4 (SCR-12), §5 (`personal_tokens` — bảng đã có từ migration 005), §6 (token API, MCP tools), §7 BR-62, BR-63
- Test đặc tả: `internal/tokens/tokens_test.go`, `internal/mcp/mcp_test.go`, `web/src/pages/profile.test.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** package mới `internal/tokens/`, `internal/mcp/` (không phải `_test.go`); `cmd/server/main.go` và file mới `cmd/server/mcp_backend.go`; `web/src/pages/Profile.tsx` (mới), `web/src/nav.ts`, `web/src/App.tsx`, `web/src/layout/AppShell.tsx` (thêm liên kết Hồ sơ ở menu người dùng).
**Không được sửa:** file trong `tests.lock` và mọi file test; package Go khác; migration; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**`internal/tokens`**
```go
var ErrUnauthorized, ErrForbidden, ErrNotFound, ErrConflict, ErrLimit, ErrValidation error
type Token struct { ID, Name, Last4 string; ExpiresAt time.Time; LastUsedAt *time.Time; CreatedAt time.Time }
func NewStore(db *sql.DB, now func() time.Time) *Store
func (s *Store) Create(userID, name string, days int) (plain string, t Token, err error) // days ∈ {30,90,365}; name 1–40 ký tự; tối đa 10 token còn hiệu lực/người (ErrLimit); trùng tên còn hiệu lực → ErrConflict
func (s *Store) List(userID string) ([]Token, error)                                    // chỉ token chưa thu hồi, mới nhất trước
func (s *Store) Revoke(userID, id string) error                                         // token người khác → ErrForbidden; không có → ErrNotFound (HTTP 404)
func (s *Store) Authenticate(plain string) (userID string, err error)                   // sai/hết hạn/thu hồi → ErrUnauthorized; ghi last_used_at tối đa 1 lần/phút
func Handler(s *Store, authz func(*http.Request) (userID, role string, ok bool)) http.Handler // GET/POST/DELETE /api/me/tokens[/{id}]; role ≠ ADMIN → 403
```
Token: `"ahp_"` + 32 byte `crypto/rand` mã hoá base64url (không padding); lưu `SHA-256` hex, `last4` = 4 ký tự cuối. `POST` → `201 {id, name, token, last4, expires_at}` — token gốc chỉ xuất hiện ở đây.

**`internal/mcp`** — MCP Streamable HTTP tối thiểu, JSON-RPC 2.0, **không thêm thư viện**:
```go
type Backend interface {
    CreateTask(ctx context.Context, userID, project, title, description string) (map[string]any, error)
    ListTasks(ctx context.Context, userID, project, status string, limit int) ([]map[string]any, error)
    TaskStatus(ctx context.Context, userID, id string) (map[string]any, error)
    AnswerClarification(ctx context.Context, userID, id string, answers map[string]string) (map[string]any, error)
    ApproveTask(ctx context.Context, userID, id string) (map[string]any, error)
    MergeTask(ctx context.Context, userID, id string) (map[string]any, error)
    DecideProposal(ctx context.Context, userID, taskID, proposalID string, accept bool) (map[string]any, error)
    FleetStatus(ctx context.Context, userID string) (map[string]any, error)
}
func Handler(tokens *tokens.Store, isOwner func(userID string) bool, b Backend, now func() time.Time) http.Handler // POST /mcp
```
- Header `Authorization: Bearer <token>`: thiếu/sai → HTTP `401`; token hợp lệ nhưng không phải owner → `403`; quá 60 lời gọi/phút/token → `429`.
- Phương thức: `initialize` → `{protocolVersion: "2025-06-18", capabilities: {tools: {}}, serverInfo: {name: "agenthub", version}}`; `notifications/initialized` → HTTP `202` không body; `tools/list` → 8 tool: `create_task`, `list_tasks`, `task_status`, `answer_clarification`, `approve_task`, `merge_task`, `decide_proposal`, `fleet_status`, mỗi tool có `description` tiếng Việt và `inputSchema` JSON Schema (`required` đúng tham số bắt buộc); `tools/call` → gọi Backend, trả `{content: [{type: "text", text: <JSON kết quả>}]}`; lỗi Backend → `{content: [...lỗi...], isError: true}`; tool lạ / tham số thiếu → lỗi JSON-RPC `-32602`; phương thức lạ → `-32601`; JSON hỏng → `-32700`.
- `cmd/server/mcp_backend.go`: cài Backend trên service sẵn có (tasks, fleet). `AnswerClarification` và `DecideProposal` trả lỗi "chưa hỗ trợ" cho tới khi TASK-039/TASK-048 có service (để sẵn chỗ nối).

**SCR-12 Hồ sơ** (`/profile`, mọi người dùng; route và liên kết "Hồ sơ" trong menu người dùng): thông tin GitHub; nếu `role === 'ADMIN'`: danh sách token (tên, tạo lúc, dùng lần cuối, `…<last4>`), form Tạo token (tên + hết hạn 30/90/365), sau khi tạo hiện token **một lần** với nút Sao chép + đoạn `claude mcp add --transport http agenthub <origin>/mcp --header "Authorization: Bearer <token>"`, nút Thu hồi (confirm). Partner: không có phần token. 4 trạng thái theo `remote-command.md` §4.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-046-tokens-mcp/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không log hay trả token gốc ở bất kỳ đâu ngoài `POST` tạo; so sánh hash bằng `subtle.ConstantTimeCompare` hoặc tra theo hash.
- Token không được dùng làm cookie web hay token runner.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`, `node` — mỗi lần một lệnh; không `test`, `cp`, `npm`, `npx`, `which`; kiểm tra web bằng verify.sh.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
