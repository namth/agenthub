# 07. HỢP ĐỒNG DỮ LIỆU API & WEBSOCKET (API CONTRACTS)

---

## 1. REST API ENDPOINTS

Tất cả các API REST trả về định dạng chuẩn:
```json
{
  "success": true,
  "data": { ... },
  "error": null
}
```

### 1.1 Nhóm Xác thực (Auth)
* **`GET /api/auth/github`**: Chuyển hướng người dùng đến trang đăng nhập GitHub OAuth.
* **`GET /api/auth/callback`**: Nhận authorization code từ GitHub, tạo JWT session cookie và chuyển hướng về Dashboard.
* **`GET /api/auth/me`**: Trả về thông tin user hiện tại (`id`, `username`, `avatar_url`, `role`).
* **`POST /api/auth/logout`**: Xóa JWT cookie.

### 1.2 Nhóm Quản lý Tác vụ (Tasks & Kanban)
* **`GET /api/tasks`**:
  - Query params: `status`, `project_id`.
  - Quyền: `ADMIN`, `PARTNER`.
  - Trả về danh sách Task theo từng cột Kanban.
* **`POST /api/tasks`**:
  - Body: `{ "title": "Tên tính năng", "description": "Mô tả chi tiết" }`.
  - Quyền: `ADMIN`, `PARTNER`.
  - Tạo mới task, tạo Issue trên GitHub với label `status:pending-approval`.
* **`POST /api/tasks/:id/approve`**:
  - Quyền: Chỉ `ADMIN`.
  - Phê duyệt task, chuyển sang `PLANNING`, kích hoạt điều phối cho Claude Code.
* **`POST /api/tasks/:id/reject`**:
  - Quyền: Chỉ `ADMIN`.
  - Body: `{ "reason": "Lý do từ chối" }`. Đóng GitHub Issue.

### 1.3 Nhóm Quản lý Cụm máy (Fleet Management)
* **`GET /api/fleet`**:
  - Quyền: Chỉ `ADMIN`.
  - Trả về `lead` (state, agent_id, epoch, expires_at, eta) và danh sách máy, mỗi máy kèm `owner` (`{id, login}` hoặc `null` = Hệ thống), `version`, `paused` và `agents[]` (kind, status, cooldown_until, lead_priority, hạn mềm, `lead_eligible`, `projects` — mảng id hoặc `null` = mọi dự án, current_job gồm `project_id`, `project`, `task_title`). Chi tiết: FEAT-01 mục 6.3, FEAT-05 §4.
* **`POST /api/admin/lead/assign`** · **`POST /api/admin/lead/revoke`** · **`PATCH /api/admin/agents/:id`**: Chỉ `ADMIN` (FEAT-01). `PATCH` nhận thêm `lead_eligible` (boolean) và `projects` (mảng id dự án thay thế toàn bộ; `null`/`[]` = mọi dự án; id không tồn tại → 400).

### 1.4 Nhóm Cài đặt & Văn phòng ảo (FEAT-02)
* **`GET/PATCH /api/admin/settings`**: Chỉ owner; gồm `office_enabled`, `telegram_enabled` (chỉ đọc, DEC-30).
* **`GET /api/office/stream`**: Owner và `PARTNER` (DEC-27), SSE; snapshot có `users[]`, `auth_status`, `gh_auth`; `404` khi tắt văn phòng ảo; `429` quá 3 kết nối/người hoặc 30 tổng. `GET /api/admin/office/stream` trả `308` tới đường mới. Chi tiết: [features/virtual-office.md](features/virtual-office.md) §6.
* **`GET /api/office/status`**: Mọi người dùng đã đăng nhập → `{ "enabled": bool }` (sidebar biết có hiện SCR-08).
* **`POST /api/my/machines/{id}/auth-recheck`**: Chủ máy hoặc owner; body `{ "agent_id"?: "…" }` → `202`; máy offline → `409 WORKER_OFFLINE` (FEAT-06).

### 1.5 Mã lỗi chung
| HTTP | `error.code` | Khi nào |
| :--- | :--- | :--- |
| 400 | `VALIDATION_ERROR` | Body sai định dạng/miền giá trị |
| 401 | `UNAUTHORIZED` | Thiếu/sai JWT, sai chữ ký webhook |
| 403 | `FORBIDDEN` | Sai vai trò |
| 404 | `NOT_FOUND` | Không có tài nguyên, hoặc tính năng đang tắt |
| 409 | `AGENT_NOT_ELIGIBLE` · `NO_LEAD` | Thao tác lead không hợp lệ ở trạng thái hiện tại (gồm agent `NEEDS_AUTH`, `lead_eligible = 0`) |
| 409 | `WORKER_OFFLINE` | Yêu cầu cần runner trả lời nhưng máy đang offline (FEAT-06) |
| 429 | `TOO_MANY_CONNECTIONS` | Quá giới hạn kết nối SSE |
* **`POST /api/fleet/register`**:
  - Body: `{ "name": "Macbook-M2", "os": "darwin", "agents": ["CLAUDE", "ANTIGRAVITY"] }`.
  - Trả về: `{ "worker_id": "...", "runner_token": "secret_abc_123" }`.

---

## 2. WEBSOCKET PROTOCOL (`/ws/agent`)

Giao tiếp hai chiều thời gian thực giữa Go Orchestrator và Runner Client trên từng máy con.

### 2.1 Bản tin từ Runner gửi lên Server (Client -> Server)
* **Bắt tay ban đầu (Handshake):**
  ```json
  {
    "event": "HANDSHAKE",
    "token": "secret_abc_123",
    "data": { "os": "darwin", "gh_auth": "OK",
              "agents": [ { "kind": "CLAUDE", "auth_status": "OK" }, { "kind": "ANTIGRAVITY", "auth_status": "UNKNOWN" } ] }
  }
  ```
* **Báo cáo nhịp tim (Heartbeat):**
  ```json
  { "event": "PING", "timestamp": 1798982345 }
  ```
* **Kéo việc khi rảnh (Pull Request):**
  ```json
  { "event": "REQUEST_TASK", "agent_id": "mac-mini/antigravity" }
  ```
* **Stream Log thời gian thực:**
  ```json
  {
    "event": "LOG_CHUNK",
    "assignment_id": "asg_123",
    "chunk": "\u001b[32m[INFO]\u001b[0m Compiling packages...\n"
  }
  ```
* **Báo cáo hoàn thành:**
  ```json
  {
    "event": "TASK_SUBMITTED",
    "assignment_id": "asg_123",
    "pr_url": "https://github.com/owner/repo/pull/15",
    "branch": "feat/task-123"
  }
  ```

### 2.2 Bản tin từ Server gửi xuống Runner (Server -> Client)
* **Giao việc (Assign Task):**
  ```json
  {
    "event": "ASSIGN_TASK",
    "assignment_id": "asg_123",
    "agent_id": "mac-mini/antigravity",
    "job_kind": "CODE",
    "attempt": 1,
    "payload": {
      "github_issue_id": 102,
      "title": "Viết schema và migration SQLite",
      "prompt": "Hãy tạo file migration cho bảng workers và tasks...",
      "branch_name": "feat/task-102",
      "target_files": ["internal/database/schema.go"],
      "handoff_note": null
    }
  }
  ```
* **Lệnh hủy hoặc timeout (Abort):**
  ```json
  { "event": "ABORT_TASK", "assignment_id": "asg_123", "reason": "Timeout hoặc bị Admin hủy" }
  ```

### 2.3 Bản tin chuyển giao & trưởng phòng (FEAT-01)
* Runner → Server: `AGENT_EXHAUSTED`, `LEASE_RENEW`, `LEAD_RELEASE`, `LEAD_ACTION`.
* Server → Runner: `HANDSHAKE_OK`, `LEASE_GRANTED`, `LEASE_REVOKED`, `ERROR` (`STALE_ASSIGNMENT`, `STALE_EPOCH`, `AGENT_NOT_OWNED`, `NOT_LEAD`).
* Payload đầy đủ: [features/lead-rotation.md](features/lead-rotation.md) mục 6.1–6.2.

### 2.4 Bản tin trạng thái đăng nhập (FEAT-06)
* Runner → Server: `AGENT_AUTH { scope: agent|gh, agent_id?, auth_status, detail?, assignment_id?, wip_commit?, handoff_note? }` — gửi khi giá trị đổi, khi trả lời `AUTH_RECHECK`, hoặc khi job đang chạy lỗi vì chưa đăng nhập (kèm `assignment_id`, fencing BR-04).
* Server → Runner: `AUTH_RECHECK { agent_id? }`; `LEASE_REVOKED.reason` thêm giá trị `AUTH`.
* Payload đầy đủ: [features/agent-auth.md](features/agent-auth.md) §6.

## 3. API ĐA DỰ ÁN & MÁY (FEAT-04, FEAT-05)
Chi tiết: [features/multi-project.md](features/multi-project.md) §6 và [features/join-machine.md](features/join-machine.md) §6. Gồm `GET /api/projects` (mỗi dự án có `open_tasks` = số task gốc chưa DONE/REJECTED), `GET /api/admin/github/repos`, `POST /api/admin/projects`, `PATCH /api/admin/projects/{id}`, `POST /api/admin/projects/{id}/webhook` (tạo lại webhook; GitHub lỗi → 502 `GITHUB_ERROR`), `GET /api/tasks?project_id=`, `POST /api/fleet/join-codes`, `POST /api/fleet/join`, `GET /install.sh`, `GET /download/agenthub-{os}-{arch}`, `GET/PATCH/DELETE /api/my/machines/{id}`, `GET /api/contributions`, `GET /api/contributions/timeline`; WS `ASSIGN_TASK.payload.repo`, `HANDSHAKE.data.version`, `HANDSHAKE_OK.server_version`.
