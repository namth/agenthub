# 11. LỘ TRÌNH THỰC HIỆN & DANH MỤC CÔNG VIỆC (IMPLEMENTATION TASKS)

---

## 1. TỔNG QUAN CÁC CỘT MỐC (MILESTONES)

| Milestone | Tên giai đoạn | Thời gian dự kiến | Mục tiêu đầu ra |
| :--- | :--- | :--- | :--- |
| **M1** | Foundation & Database Core | 1 ngày | Khởi tạo Go project, SQLite migrations, Single Binary embedding skeleton. |
| **M2** | WebSocket Hub & Runner Daemon | 1–2 ngày | Máy chủ WebSocket, kịch bản Python Runner, cơ chế tạo/dọn dẹp Git Worktree. |
| **M3** | GitHub Two-way Sync & Webhook | 1 ngày | Đọc/tạo Issue qua API, nhận webhook cập nhật trạng thái tự động. |
| **M4** | Frontend Dashboard & Kanban | 2 ngày | Giao diện Kanban, màn hình quản lý cụm máy và live terminal xterm.js. |
| **M5** | Telegram Bot & 2-Layer QA Gate | 1 ngày | Bot thông báo kèm nút duyệt nhanh, tích hợp luồng Claude Code review PR. |

---

## 2. DANH MỤC TICKET CHI TIẾT KÈM TIÊU CHUẨN NGHIỆM THU (DoD)

### Milestone 1: Nền tảng Máy chủ & Cơ sở dữ liệu
- [ ] **TASK-01: Khởi tạo Go Module & SQLite Schema**
  - **Mô tả:** Thiết lập `go.mod`, package `internal/database` kết nối SQLite bằng `modernc.org/sqlite`, chạy file `schema.sql` tự động khi khởi động.
  - **DoD:** Chạy `go run cmd/server/main.go` tạo ra file `./data/agenthub.db` với đầy đủ các bảng nền (`users`, `projects`, `workers`, `tasks`, `task_logs`); các bảng FEAT-01/02 thuộc TASK-10, TASK-15.
- [ ] **TASK-02: Cơ chế Single Binary (`go:embed`) Skeleton**
  - **Mô tả:** Tạo thư mục `web/dist` giả lập và cấu hình `//go:embed` trong Go để serve HTML/CSS tĩnh qua router `chi`.
  - **DoD:** Mở `http://localhost:8080` trả về trang HTML đơn giản mà không cần file rời bên ngoài binary.

### Milestone 2: Cổng WebSocket & Runner Daemon
- [ ] **TASK-03: Xây dựng WebSocket Hub & Protocol**
  - **Mô tả:** Viết package `internal/gateway`, xử lý Handshake, Ping/Pong 15s, và đăng ký danh sách agent của từng máy (`agents[]` trong HANDSHAKE).
  - **DoD:** Viết unit test giả lập 5 clients kết nối đồng thời, gửi ping và nhận pong trong < 10ms.
- [ ] **TASK-04: Xây dựng Script `runner.py` trên máy con**
  - **Mô tả:** Script Python kết nối vào WebSocket server, nhận task, chạy lệnh CLI qua `subprocess`, bắt stdout và stream ngược về server.
  - **DoD:** Khi server bắn lệnh `ASSIGN_TASK`, máy con tự động tạo worktree trong `.worktrees/`, chạy lệnh test và xóa worktree khi xong.

- [ ] **TASK-10: Schema Agent, Job, Lease (FEAT-01)**
  - **Mô tả:** Bảng `agents`, `jobs`, `assignments`, `lead_lease`, `lead_journal`; sửa `workers`, `task_logs` theo [features/lead-rotation.md](features/lead-rotation.md) mục 5.
  - **DoD:** `go test ./internal/database/...` tạo đủ bảng; CHECK chặn giá trị trạng thái sai.
- [ ] **TASK-11: Job Queue, Lead Lease Manager & Fencing (FEAT-01)**
  - **Mô tả:** `internal/dispatcher`: hàng đợi job, cấp/gia hạn/thu hồi lease, chọn lead (BR-06), trống lead, thu hồi 120 giây, attempt/REPLAN/UNBLOCK.
  - **DoD:** `go test ./internal/dispatcher/... -run 'Lease|Fencing|Reclaim|Vacant'` qua TC-11→TC-22.
- [ ] **TASK-12: Runner phát hiện hết limit & bàn giao (FEAT-01)**
  - **Mô tả:** `exhaust_patterns`, commit WIP + push, `handoff_note`, `LEASE_RENEW`/`LEAD_RELEASE`, hạn mềm.
  - **DoD:** Test runner với CLI giả qua TC-25; TC-15 chạy end-to-end với runner thật + CLI giả.

### Milestone 3: Tích hợp GitHub & Đồng bộ Trạng thái
- [ ] **TASK-05: GitHub OAuth & Webhook Handler**
  - **Mô tả:** Viết flow OAuth lấy token, map vai trò `ADMIN`/`PARTNER`. Endpoint nhận Webhook xử lý sự kiện `issues` và `pull_request`.
  - **DoD:** Tạo một Issue trên GitHub ➡️ Xuất hiện bản ghi tương ứng trong bảng `tasks` của SQLite trong vòng 1 giây.

### Milestone 4: Giao diện Web Điều phối
- [ ] **TASK-06: Bảng Kanban & Form Tạo Tính năng**
  - **Mô tả:** Dựng giao diện Kanban 5 cột bằng React + Tailwind CSS + `shadcn/ui`. Tích hợp Web Form tạo yêu cầu cho đối tác.
  - **DoD:** Kéo thả thẻ hoặc bấm nút "Approve" cập nhật trạng thái API tức thì.
- [ ] **TASK-07: Màn hình Quản lý Fleet & Live Terminal Console**
  - **Mô tả:** Màn hình danh sách máy con với đèn LED trạng thái. Nhúng `xterm.js` để render các chuỗi màu ANSI khi Worker stream log.
  - **DoD:** Log từ lệnh terminal trên máy con hiển thị mượt mà trên trình duyệt không bị giật.

- [ ] **TASK-13: SCR-05 Fleet có Agent & Lead (FEAT-01)**
  - **Mô tả:** Thẻ Lead, cột agent, đếm ngược cooldown, Trao/Thu hồi vai, drawer sửa agent, 4 trạng thái UI.
  - **DoD:** Playwright qua TC-26, TC-27.
- [ ] **TASK-15: Office Event Bus & SSE (FEAT-02)**
  - **Mô tả:** `internal/office`: bộ đệm vòng 400, chuyển sự kiện miền → OfficeEvent, `GET /api/admin/office/stream`, bảng `settings` + API.
  - **DoD:** `go test ./internal/office/...` qua TC-28→TC-32.
- [ ] **TASK-16: SCR-08 Văn phòng ảo (FEAT-02)**
  - **Mô tả:** Chuyển canvas `office/public/index.html` (edusystem, nhánh `claude/office-simulation-gui-ca46c2`) sang `web/src/pages/Office.tsx`, bố cục động, phòng nghỉ, ghế Trưởng phòng, hoạt cảnh chuyển giao.
  - **DoD:** Playwright qua TC-33→TC-35; đủ 4 trạng thái UI.
- [ ] **TASK-17: Công tắc Văn phòng ảo ở SCR-07 (FEAT-02)**
  - **DoD:** Bật/tắt → mục sidebar và route `/office` đổi theo (TC-31).

### Milestone 5: Telegram Bot & Luồng Nghiệm thu Tự động
- [ ] **TASK-08: Tích hợp Telegram Bot Notifier & Inline Buttons**
  - **Mô tả:** Gửi tin nhắn thông báo khi có Issue mới hoặc PR mới. Xử lý Callback Query khi Admin bấm nút duyệt trên điện thoại.
  - **DoD:** Bấm nút "Phê duyệt" trên Telegram ➡️ Trạng thái Task trên Web chuyển sang `PLANNING`.
- [ ] **TASK-09: Tích hợp 2-Layer QA Gate (CI + Claude Code)**
  - **Mô tả:** Tự động lắng nghe webhook CI pass ➡️ giao PR cho máy Claude Code kéo về review và tự comment kết quả lên PR.
  - **DoD:** PR được Claude Code tự động gắn nhãn `qa-passed` sau khi vượt qua bài kiểm tra.
- [ ] **TASK-14: Telegram cho chuyển giao (FEAT-01)**
  - **Mô tả:** Tin đổi lead, vào/thoát trống lead, task vượt `MAX_ATTEMPTS`.
  - **DoD:** Test với Telegram API giả: đúng nội dung, không gửi trùng (TC-17).

---

## 3. BACKLOG TÍNH NĂNG (chưa grill — xem `docs/14-decisions.md` mục BACKLOG)
- **FEAT-03:** Dây chuyền vai trò BA / Designer / Dev / Test.
- **FEAT-04:** Đa dự án song song (GitHub App, RBAC theo dự án, dispatcher chia lượt).
