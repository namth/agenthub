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
| **Cấu hình Telegram Bot & Webhooks** | ✅ | ❌ | ❌ | Thiết lập hệ thống |
| **Trao / thu hồi vai Trưởng phòng (lead)** | ✅ | ❌ | ❌ | FEAT-01, SCR-05 |
| **Sửa ưu tiên lead & hạn mềm của agent** | ✅ | ❌ | ❌ | FEAT-01 |
| **Báo hết limit (`AGENT_EXHAUSTED`)** | ❌ | ❌ | ✅ | Chỉ cho agent thuộc máy mình (BR-05) |
| **Gia hạn / nhả lease, gửi `LEAD_ACTION`** | ❌ | ❌ | ✅ (lead) | Đúng `epoch` hiện hành (BR-03) |
| **Xem Văn phòng ảo (SCR-08)** | ✅ | ❌ | ❌ | FEAT-02, chỉ khi bật |

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
