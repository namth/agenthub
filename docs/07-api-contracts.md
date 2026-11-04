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
  - Trả về `lead` (state, agent_id, epoch, expires_at, eta) và danh sách máy, mỗi máy kèm `agents[]` (kind, status, cooldown_until, lead_priority, hạn mềm, current_job). Chi tiết: FEAT-01 mục 6.3.
* **`POST /api/admin/lead/assign`** · **`POST /api/admin/lead/revoke`** · **`PATCH /api/admin/agents/:id`**: Chỉ `ADMIN` (FEAT-01).

### 1.4 Nhóm Cài đặt & Văn phòng ảo (FEAT-02)
* **`GET/PATCH /api/admin/settings`**: Chỉ `ADMIN`; gồm `office_enabled`.
* **`GET /api/admin/office/stream`**: Chỉ `ADMIN`, SSE; `404` khi tắt văn phòng ảo.

### 1.5 Mã lỗi chung
| HTTP | `error.code` | Khi nào |
| :--- | :--- | :--- |
| 400 | `VALIDATION_ERROR` | Body sai định dạng/miền giá trị |
| 401 | `UNAUTHORIZED` | Thiếu/sai JWT, sai chữ ký webhook |
| 403 | `FORBIDDEN` | Sai vai trò |
| 404 | `NOT_FOUND` | Không có tài nguyên, hoặc tính năng đang tắt |
| 409 | `AGENT_NOT_ELIGIBLE` · `NO_LEAD` | Thao tác lead không hợp lệ ở trạng thái hiện tại |
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
    "data": { "os": "darwin", "agents": [ { "kind": "CLAUDE" }, { "kind": "ANTIGRAVITY" } ] }
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

## 3. API ĐA DỰ ÁN & MÁY (FEAT-04, FEAT-05)
Chi tiết: [features/multi-project.md](features/multi-project.md) §6 và [features/join-machine.md](features/join-machine.md) §6. Gồm `GET /api/projects`, `GET /api/admin/github/repos`, `POST /api/admin/projects`, `PATCH /api/admin/projects/{id}`, `GET /api/tasks?project_id=`, `POST /api/fleet/join-codes`, `POST /api/fleet/join`, `GET /install.sh`, `GET /download/agenthub-{os}-{arch}`, `GET/PATCH/DELETE /api/my/machines/{id}`, `GET /api/contributions`, `GET /api/contributions/timeline`; WS `ASSIGN_TASK.payload.repo`, `HANDSHAKE.data.version`, `HANDSHAKE_OK.server_version`.
