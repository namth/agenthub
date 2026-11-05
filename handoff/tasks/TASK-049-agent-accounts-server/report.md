# Báo cáo TASK-049

## Đã làm
- **Migration 006** (`internal/database/migrations/006_agent_accounts.sql`):
  - Dòng đầu có marker `-- agenthub:foreign_keys=off`.
  - Dựng lại bảng `agents_new`, copy toàn bộ các cột cũ của `agents`.
  - Bổ sung `name TEXT`, `ever_ok INTEGER NOT NULL DEFAULT 1`, `removed_at DATETIME`.
  - Bỏ ràng buộc `UNIQUE(worker_id, kind)`, bổ sung `UNIQUE(worker_id, name)`.
  - Tạo trigger `trg_agents_default_name` trên `AFTER INSERT ON agents WHEN NEW.name IS NULL` tự động điền `name = substr(NEW.id, instr(NEW.id, '/') + 1)`.
  - Chuyển dữ liệu cũ: `name` = phần sau `/`; `ever_ok = 0` cho agent `auth_status <> 'OK'` và không có assignment nào; còn lại 1; `removed_at = NULL`.
  - Drop bảng `agents` cũ và đổi tên `agents_new` thành `agents`, tạo lại index `idx_agents_status`.
- **Dispatcher** (`internal/dispatcher/types.go`, `internal/dispatcher/service.go`):
  - Định nghĩa lỗi `ErrConflict = errors.New("CONFLICT")` và hằng số sự kiện `EvAgentRemoved = "AGENT_REMOVED"`.
  - Mở rộng `AgentHello` thêm `Name string`, `EverOK *bool`; `Agent` thêm `Name string`.
  - Cài đặt `RemoveAgent(agentID string) error`: kiểm tra agent tồn tại/chưa xoá (`ErrNotFound`), kiểm tra assignment ACTIVE (`ErrConflict`), nhả lead lease nếu đang giữ (`revokeLeadLocked`), đặt `removed_at = nowStr, status = 'IDLE'`, phát `EvAgentRemoved`.
  - Cài đặt `Tombstoned(workerID string, ids []string) []string`: trả về danh sách ID đã bị `removed_at`.
  - Cập nhật `ConnectAuth`: validate `name` theo `^[a-z0-9][a-z0-9-]{0,31}$`, bỏ qua agent nếu `removed_at` khác NULL (không hồi sinh), tính `ever_ok` (không bao giờ hạ 1 -> 0, hàng mới phụ thuộc `EverOK` hoặc `AuthStatus == OK`), upsert theo `ON CONFLICT(id)`, chỉ trả về agent có `ever_ok = 1 AND removed_at IS NULL`.
  - Lọc bỏ agent có `ever_ok = 0` hoặc `removed_at IS NOT NULL` trong `RequestJob`, `AdminAssign`, bầu lead lease, `Tick`.
- **Gateway** (`internal/gateway/hub.go`, `internal/gateway/client.go`):
  - Xử lý `EvAgentRemoved` trong `Hub.Notify`: gửi bản tin JSON `{"event":"AGENT_REMOVE","agent_id":...}` tới máy tương ứng.
  - Đọc `name` và `ever_ok` trong `HELLO` `data.agents[]`; sau `ConnectAuth`, nếu API hỗ trợ interface `Tombstoned`, tự động gửi `AGENT_REMOVE` cho từng ID tombstone runner còn báo.
- **API** (`internal/api/fleet.go`):
  - Thêm trường `Name string json:"name"` vào `AgentItem`.
  - Lọc agent trong `handleGetFleet` theo `ever_ok = 1 AND removed_at IS NULL`.
  - Bổ sung route `DELETE /api/admin/agents/{id}` (yêu cầu quyền ADMIN): unescape id, gọi `RemoveAgent`, ánh xạ `ErrNotFound` -> 404, `ErrConflict` -> 409, thành công trả về 200 `{"removed": id}`.
- **Machines** (`internal/machines/handler.go`):
  - Thêm `RemoveAgent func(agentID string) error` vào `machines.Config`.
  - Trong `POST /api/fleet/join`: khi chèn hàng agent mới cho máy, đặt `ever_ok = 0`, `name = agLower`.
  - Trong `GET /api/my/machines`: thêm `Name string json:"name"` vào `agentDTO`, lọc agent theo `ever_ok = 1 AND removed_at IS NULL`.
  - Bổ sung route `DELETE /api/my/machines/{id}/agents/{name}`: kiểm tra máy thuộc người gọi hoặc owner (nếu không -> 403 `FORBIDDEN`), kiểm tra agent tồn tại và chưa xoá (nếu không -> 404 `NOT_FOUND`, không gọi `RemoveAgent`), gọi `cfg.RemoveAgent`, ánh xạ `ErrConflict` -> 409 `CONFLICT`, `ErrNotFound` -> 404 `NOT_FOUND`, thành công trả về 200 `{"removed": id}`.
- **Office** (`internal/office/snapshot.go`):
  - Thêm trường `Name string json:"name"` vào `AgentSnapshot`.
  - Lọc agent trong `BuildSnapshot` theo `ever_ok = 1 AND removed_at IS NULL`, scan và gán `Name`.
- **Server Main** (`cmd/server/main.go`):
  - Nối `machines.Config.RemoveAgent = disp.RemoveAgent`.

## File đã tạo/sửa
- `internal/database/migrations/006_agent_accounts.sql`: Migration 006 dựng lại bảng agents, bỏ UNIQUE(worker_id, kind), thêm name, ever_ok, removed_at, UNIQUE(worker_id, name).
- `internal/dispatcher/types.go`: Thêm ErrConflict, EvAgentRemoved, mở rộng AgentHello và Agent.
- `internal/dispatcher/service.go`: Cài đặt RemoveAgent, Tombstoned, cập nhật ConnectAuth và bộ lọc ever_ok/tombstone.
- `internal/gateway/hub.go`: Xử lý sự kiện EvAgentRemoved gửi AGENT_REMOVE tới runner.
- `internal/gateway/client.go`: Nhận name/ever_ok từ handshake, gửi AGENT_REMOVE cho các tombstoned agent.
- `internal/api/fleet.go`: Bổ sung name vào AgentItem, lọc ever_ok/removed_at, thêm route DELETE /api/admin/agents/{id}.
- `internal/machines/handler.go`: Bổ sung Config.RemoveAgent, join gán ever_ok=0, lọc GET /api/my/machines, thêm route DELETE /api/my/machines/{id}/agents/{name}.
- `internal/office/snapshot.go`: Bổ sung name vào AgentSnapshot, lọc agent ever_ok=1 và removed_at IS NULL trong snapshot.
- `cmd/server/main.go`: Nối machines.Config.RemoveAgent với disp.RemoveAgent.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-049-agent-accounts-server/verify.sh
ok  	github.com/namth/agenthub/internal/database	9.782s
ok  	github.com/namth/agenthub/internal/dispatcher	22.746s
ok  	github.com/namth/agenthub/internal/api	6.015s
ok  	github.com/namth/agenthub/internal/machines	6.725s
ok  	github.com/namth/agenthub/internal/gateway	3.643s
ok  	github.com/namth/agenthub/internal/office	5.689s
TASK-049 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

Toàn bộ 15/15 ca kiểm thử nghiệm thu của TASK-049 đều PASS:
- `TestMigration006AgentAccounts`, `TestMigration005KeepsData`
- `TestHelloTwoClaudeAccounts`, `TestHelloDefaultNameKeepsLegacyID`, `TestHelloRejectsBadName`
- `TestNeverOKHiddenAndNotDispatched`, `TestRemoveAgent`, `TestRemoveLeadReleasesLease`
- `TestFleetHidesNeverOKAndRemoved`, `TestDeleteAgent`, `TestFleetShape`
- `TestJoinNewMachineAgentsNotEverOK`, `TestOwnerOfMachineDeletesAgent`
- `TestAgentRemovedRoutesToWorker`, `TestSnapshotOnlyRealAgents`

Toàn bộ các package server khác đều PASS (exit 0). `gofmt`, `go vet ./internal/... ./cmd/...` và `go build ./...` đều thành công 100%.

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
