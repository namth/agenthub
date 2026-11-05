# 02. PHÂN HỆ HỆ THỐNG & MA TRẬN PHÂN QUYỀN (MODULES & RBAC)

---

## 1. CÁC PHÂN HỆ HỆ THỐNG (CORE MODULES)

Hệ thống được chia thành 5 phân hệ chức năng:

```
┌────────────────────────────────────────────────────────┐
│               1. PHÂN HỆ AUTH & USERS                  │
│    (GitHub OAuth, Session Tokens, RBAC Middleware)     │
└──────────────────────────┬─────────────────────────────┘
                           │
┌──────────────────────────▼─────────────────────────────┐
│             2. PHÂN HỆ QUẢN LÝ TASK & KANBAN           │
│    (Two-way GitHub Sync, Webhooks, Approval Gate)      │
└──────────────────────────┬─────────────────────────────┘
                           │
┌──────────────────────────▼─────────────────────────────┐
│          3. PHÂN HỆ ĐIỀU PHỐI (ORCHESTRATOR)           │
│ (Job Queue, Lead Lease Manager, Capability Match)      │
└──────────────────────────┬─────────────────────────────┘
                           │
┌──────────────────────────▼─────────────────────────────┐
│             4. PHÂN HỆ GATEWAY & RUNNER FLEET          │
│ (WebSocket, Heartbeat, Agent Status, Limit Detection)  │
└──────────────────────────┬─────────────────────────────┘
                           │
┌──────────────────────────▼─────────────────────────────┐
│          5. PHÂN HỆ THÔNG BÁO (NOTIFICATION HUB)       │
│      (Telegram Bot Alerts, Inline Approval Buttons)    │
└────────────────────────────────────────────────────────┘
```

---

## 2. MA TRẬN PHÂN QUYỀN CHI TIẾT (RBAC MATRIX)

Hệ thống phân biệt quyền hạn dựa trên vai trò ánh xạ từ GitHub Repository:
- **`ADMIN` (Owner/Maintainer trên GitHub Repo):** Toàn quyền kiểm soát.
- **`PARTNER` (Collaborator / Read-Access trên GitHub Repo):** Giới hạn ở việc yêu cầu & xem tiến độ.
- **`WORKER_AGENT` (Tài khoản máy con qua API Token):** Kéo việc, báo cáo trạng thái và stream log.

| Hành động / Tính năng | ADMIN (Bạn) | PARTNER (Đối tác) | WORKER_AGENT | Ghi chú |
| :--- | :---: | :---: | :---: | :--- |
| **Đăng nhập bằng GitHub OAuth** | ✅ | ✅ | ❌ | Worker dùng Runner Token riêng |
| **Tạo Yêu cầu tính năng (Feature Request)** | ✅ | ✅ | ❌ | Tự động tạo Issue trên GitHub |
| **Xem bảng Kanban tiến độ công việc** | ✅ | ✅ | ❌ | Partner chỉ thấy thẻ task tổng |
| **Phê duyệt yêu cầu (Approve & Plan)** | ✅ | ❌ | ❌ | Kích hoạt Claude Code lập kế hoạch |
| **Xem thông tin Cụm máy (Fleet Management)** | ✅ | ❌ | ❌ | Tránh lộ IP / Cấu hình máy cá nhân |
| **Xem Live Terminal Log của máy con** | ✅ | ❌ | ❌ | Bảo mật mã nguồn nhạy cảm |
| **Nhận task & Cập nhật tiến độ task** | ❌ | ❌ | ✅ | Giao tiếp qua WebSocket |
| **Stream stdout/stderr về Server** | ❌ | ❌ | ✅ | Buffer 100ms / stream chunk |
| **Chạy kiểm định QA tự động (Claude)** | ✅ | ❌ | ✅ (Claude) | Kích hoạt khi PR được mở |
| **Bấm nút Merge PR vào nhánh `main`** | ✅ | ❌ | ❌ | Quyền quyết định cuối cùng |
| **Cấu hình Telegram Bot & Webhooks** | ✅ | ❌ | ❌ | Qua `.env` trên server (DEC-30); SCR-07 chỉ hiện trạng thái |
| **Trao / thu hồi vai Trưởng phòng (lead)** | ✅ | ❌ | ❌ | FEAT-01, SCR-05 |
| **Sửa ưu tiên lead & hạn mềm của agent** | ✅ | ❌ | ❌ | FEAT-01 |
| **Báo hết limit (`AGENT_EXHAUSTED`)** | ❌ | ❌ | ✅ | Chỉ cho agent thuộc máy mình (BR-05) |
| **Gia hạn / nhả lease, gửi `LEAD_ACTION`** | ❌ | ❌ | ✅ (lead) | Đúng `epoch` hiện hành (BR-03) |
| **Xem Văn phòng ảo (SCR-08)** | ✅ | ✅ | ❌ | FEAT-02 bản 2, chỉ khi bật; partner xem đầy đủ (DEC-27) |
| **Bật/tắt Văn phòng ảo (SCR-07)** | ✅ (owner) | ❌ | ❌ | FEAT-02 |
| **Xem trạng thái đăng nhập agent** | ✅ mọi máy | ✅ máy của mình | ❌ | FEAT-06, SCR-05/SCR-11 |
| **Yêu cầu kiểm lại đăng nhập (`auth-recheck`)** | ✅ mọi máy | ✅ máy của mình | ❌ | FEAT-06 |
| **Báo trạng thái đăng nhập (`AGENT_AUTH`)** | ❌ | ❌ | ✅ | Chỉ agent thuộc máy mình (BR-05) |
| **Trả lời phỏng vấn làm rõ (`POST /api/tasks/{id}/clarify`)** | ✅ owner / ADMIN dự án | ✅ task do mình tạo | ❌ | FEAT-07, DEC-33 (partner khác trong dự án chỉ xem) |
| **Cho trưởng phòng tự quyết (`/clarify/auto`)** | ✅ | ❌ | ❌ | FEAT-07 |
| **Trả `NEED_INFO` / kết quả REPLAN có hành động** | ❌ | ❌ | ✅ | FEAT-07, FEAT-10 (BR-64) |
| **Sửa lệnh được phép, chạy thử dây chuyền (SCR-09)** | ✅ owner | ❌ | ❌ | FEAT-09 |
| **Nhận việc của dự án (máy của mình)** | mọi dự án | ✅ chỉ dự án mình là thành viên | ✅ | FEAT-09, DEC-37 |
| **Lệnh Telegram, token cá nhân, MCP `/mcp`** | ✅ owner | ❌ | ❌ | FEAT-10, DEC-39 |
| **Xác nhận đề xuất huỷ của trưởng phòng AI** | ✅ | ❌ | ❌ | FEAT-10 (ADMIN dự án cũng được) |
| **Xoá tài khoản agent khỏi hub** | ✅ mọi máy | ✅ máy của mình | ❌ | FEAT-11, DEC-43 |

---

## 3. LOGIC NGHIỆP VỤ BẢO VỆ PHÂN QUYỀN

1. **RBAC Middleware trên Go Server:** Mọi API bắt đầu bằng `/api/admin/*` đều kiểm tra JWT claim `role == "ADMIN"`. Nếu vi phạm lập tức trả về `403 Forbidden`.
2. **Ẩn dữ liệu nhạy cảm với Partner:**
   - Trường `worker_ip`, `machine_specs`, `internal_prompt_logs` bị loại bỏ (omit) khỏi JSON response khi user có role `PARTNER`.
3. **Runner Token Authentication:**
   - Mỗi máy con khi kết nối WebSocket đến `/ws/agent` phải truyền header `X-Runner-Token: <token>`.
   - Token được hash SHA-256 lưu trong bảng `workers` của SQLite.
4. **Ánh xạ vai trò từ GitHub (DEC-09):** `admin`, `maintain` → `ADMIN`; `write`, `triage`, `read` → `PARTNER`.
5. **Fencing:** bản tin của runner về một job phải mang `assignment_id` hiện hành; bản tin của lead phải mang `epoch` hiện hành (FEAT-01 BR-03, BR-04).

## 4. ĐA DỰ ÁN & MÁY CỦA PARTNER (FEAT-04, FEAT-05)
- Quản trị hệ thống = owner (`GITHUB_OWNER`); vai ADMIN/PARTNER tính **theo từng dự án** (`project_members`, DEC-17). Ma trận chi tiết: [features/multi-project.md](features/multi-project.md) §2, [features/join-machine.md](features/join-machine.md) §2.
- Mọi người dùng tự thêm máy của mình (mã tham gia); máy có chủ (`workers.owner_user_id`); máy partner mặc định không làm trưởng phòng (DEC-20).
