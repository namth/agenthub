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
- [x] **TASK-01: Khởi tạo Go Module & SQLite Schema**
  - **Mô tả:** Thiết lập `go.mod`, package `internal/database` kết nối SQLite bằng `modernc.org/sqlite`, chạy file `schema.sql` tự động khi khởi động.
  - **DoD:** Chạy `go run cmd/server/main.go` tạo ra file `./data/agenthub.db` với đầy đủ các bảng nền (`users`, `projects`, `workers`, `tasks`, `task_logs`); các bảng FEAT-01/02 thuộc TASK-10, TASK-15.
- [x] **TASK-02: Cơ chế Single Binary (`go:embed`) Skeleton**
  - **Mô tả:** Tạo thư mục `web/dist` giả lập và cấu hình `//go:embed` trong Go để serve HTML/CSS tĩnh qua router `chi`.
  - **DoD:** Mở `http://localhost:8080` trả về trang HTML đơn giản mà không cần file rời bên ngoài binary.

### Milestone 2: Cổng WebSocket & Runner Daemon
- [x] **TASK-03: Xây dựng WebSocket Hub & Protocol**
  - **Mô tả:** Viết package `internal/gateway`, xử lý Handshake, Ping/Pong 15s, và đăng ký danh sách agent của từng máy (`agents[]` trong HANDSHAKE).
  - **DoD:** Viết unit test giả lập 5 clients kết nối đồng thời, gửi ping và nhận pong trong < 10ms.
- [x] **TASK-04: Runner daemon `agenthub runner` trên máy con (Go, DEC-10)**
  - **Mô tả:** Lệnh con Go kết nối vào WebSocket server, nhận task, chạy lệnh CLI qua `subprocess`, bắt stdout và stream ngược về server.
  - **DoD:** Khi server bắn lệnh `ASSIGN_TASK`, máy con tự động tạo worktree trong `.worktrees/`, chạy lệnh test và xóa worktree khi xong.

- [x] **TASK-10: Schema Agent, Job, Lease (FEAT-01)**
  - **Mô tả:** Bảng `agents`, `jobs`, `assignments`, `lead_lease`, `lead_journal`; sửa `workers`, `task_logs` theo [features/lead-rotation.md](features/lead-rotation.md) mục 5.
  - **DoD:** `go test ./internal/database/...` tạo đủ bảng; CHECK chặn giá trị trạng thái sai.
- [x] **TASK-11: Job Queue, Lead Lease Manager & Fencing (FEAT-01)**
  - **Mô tả:** `internal/dispatcher`: hàng đợi job, cấp/gia hạn/thu hồi lease, chọn lead (BR-06), trống lead, thu hồi 120 giây, attempt/REPLAN/UNBLOCK.
  - **DoD:** `go test ./internal/dispatcher/... -run 'Lease|Fencing|Reclaim|Vacant'` qua TC-11→TC-22.
- [x] **TASK-12: Runner phát hiện hết limit & bàn giao (FEAT-01)**
  - **Mô tả:** `exhaust_patterns`, commit WIP + push, `handoff_note`, `LEASE_RENEW`/`LEAD_RELEASE`, hạn mềm.
  - **DoD:** Test runner với CLI giả qua TC-25; TC-15 chạy end-to-end với runner thật + CLI giả.

### Milestone 3: Tích hợp GitHub & Đồng bộ Trạng thái
- [x] **TASK-05: GitHub OAuth & Webhook Handler**
  - **Mô tả:** Viết flow OAuth lấy token, map vai trò `ADMIN`/`PARTNER`. Endpoint nhận Webhook xử lý sự kiện `issues` và `pull_request`.
  - **DoD:** Tạo một Issue trên GitHub ➡️ Xuất hiện bản ghi tương ứng trong bảng `tasks` của SQLite trong vòng 1 giây.

### Milestone 4: Giao diện Web Điều phối
- [x] **TASK-06: Bảng Kanban & Form Tạo Tính năng**
  - **Mô tả:** Dựng giao diện Kanban 5 cột bằng React + Tailwind CSS + `shadcn/ui`. Tích hợp Web Form tạo yêu cầu cho đối tác.
  - **DoD:** Kéo thả thẻ hoặc bấm nút "Approve" cập nhật trạng thái API tức thì.
- [x] **TASK-07: Màn hình Quản lý Fleet & Live Terminal Console**
  - **Mô tả:** Màn hình danh sách máy con với đèn LED trạng thái. Nhúng `xterm.js` để render các chuỗi màu ANSI khi Worker stream log.
  - **DoD:** Log từ lệnh terminal trên máy con hiển thị mượt mà trên trình duyệt không bị giật.

- [x] **TASK-13: SCR-05 Fleet có Agent & Lead (FEAT-01)**
  - **Mô tả:** Thẻ Lead, cột agent, đếm ngược cooldown, Trao/Thu hồi vai, drawer sửa agent, 4 trạng thái UI.
  - **DoD:** Playwright qua TC-26, TC-27.
- [x] **TASK-15: Office Event Bus & SSE (FEAT-02)**
  - **Mô tả:** `internal/office`: bộ đệm vòng 400, chuyển sự kiện miền → OfficeEvent, `GET /api/admin/office/stream`, bảng `settings` + API.
  - **DoD:** `go test ./internal/office/...` qua TC-28→TC-32.
- [x] **TASK-16: SCR-08 Văn phòng ảo (FEAT-02)**
  - **Mô tả:** Chuyển canvas `office/public/index.html` (edusystem, nhánh `claude/office-simulation-gui-ca46c2`) sang `web/src/pages/Office.tsx`, bố cục động, phòng nghỉ, ghế Trưởng phòng, hoạt cảnh chuyển giao.
  - **DoD:** Playwright qua TC-33→TC-35; đủ 4 trạng thái UI.
- [x] **TASK-17: Công tắc Văn phòng ảo ở SCR-07 (FEAT-02)**
  - **DoD:** Bật/tắt → mục sidebar và route `/office` đổi theo (TC-31).

### Milestone 5: Telegram Bot & Luồng Nghiệm thu Tự động
- [x] **TASK-08: Tích hợp Telegram Bot Notifier & Inline Buttons**
  - **Mô tả:** Gửi tin nhắn thông báo khi có Issue mới hoặc PR mới. Xử lý Callback Query khi Admin bấm nút duyệt trên điện thoại.
  - **DoD:** Bấm nút "Phê duyệt" trên Telegram ➡️ Trạng thái Task trên Web chuyển sang `PLANNING`.
- [x] **TASK-09: Tích hợp 2-Layer QA Gate (CI + Claude Code)**
  - **Mô tả:** Tự động lắng nghe webhook CI pass ➡️ giao PR cho máy Claude Code kéo về review và tự comment kết quả lên PR.
  - **DoD:** PR được Claude Code tự động gắn nhãn `qa-passed` sau khi vượt qua bài kiểm tra.
- [x] **TASK-14: Telegram cho chuyển giao (FEAT-01)**
  - **Mô tả:** Tin đổi lead, vào/thoát trống lead, task vượt `MAX_ATTEMPTS`.
  - **DoD:** Test với Telegram API giả: đúng nội dung, không gửi trùng (TC-17).

### Đợt FEAT-06 + FEAT-02 bản 2 (đặc tả và xong 2026-11-04)
- [x] **TASK-030: Server — migration 004, `AGENT_AUTH`/`AUTH_RECHECK`, guard dispatcher, Telegram (FEAT-06)**
  - **DoD:** `go test ./internal/...` qua TC-58, TC-60 → TC-63, TC-66. Chi tiết: [features/agent-auth.md](features/agent-auth.md) §9.
- [x] **TASK-031: Runner — kiểm đăng nhập, `auth_patterns`, trả lời `AUTH_RECHECK` (FEAT-06)**
  - **DoD:** `go test ./internal/runner/...` qua TC-57, TC-59.
- [x] **TASK-032: Dispatcher — trưởng phòng mọi loại agent (DEC-29)**
  - **DoD:** `go test ./internal/dispatcher/...` qua TC-64, TC-65; TC-11 → TC-22 vẫn xanh.
- [x] **TASK-034: Web — huy hiệu đăng nhập SCR-05/SCR-11, công tắc lead mọi agent, dòng Telegram SCR-07 (FEAT-06)**
  - **DoD:** TC-67; `cd web && npm run typecheck && npm run build`.
- [x] **TASK-033: Office server bản 2 — `users`/presence/auth trong snapshot, `last_seen_at`, quyền partner, giới hạn SSE (FEAT-02)**
  - **DoD:** `go test ./internal/office/... ./internal/tasks/...` qua TC-28 → TC-32, TC-68 → TC-70. Chi tiết: [features/virtual-office.md](features/virtual-office.md) §9.
- [x] **TASK-035: Office UI bản 2 — bố cục hai cụm + khu chỉ huy, nội thất, logo INOVA (FEAT-02)**
  - **DoD:** `cd web && npx vitest run src/office` qua TC-33 → TC-35, TC-71 → TC-73; typecheck + build; ảnh chụp 0/3/9 máy.

**Thứ tự:** TASK-030 → (TASK-031 ∥ TASK-032 ∥ TASK-033) → (TASK-034 ∥ TASK-035). Phạm vi file không chồng nhau: 031 `internal/runner`, 032 `internal/dispatcher` (phần chọn lead/giao job lead), 033 `internal/office` + hook duyệt trong `internal/tasks`, 034 `web/src/pages/Fleet.tsx|MyMachines.tsx|Settings.tsx`, 035 `web/src/office/*` + `web/src/pages/Office.tsx` + `web/src/nav.ts`. TASK-030 chạm `internal/dispatcher` (guard BR-37) nên phải xong trước TASK-032.

---

## 3. BACKLOG TÍNH NĂNG (chưa grill — xem `docs/14-decisions.md` mục BACKLOG)
- **FEAT-03:** Dây chuyền vai trò BA / Designer / Dev / Test.
- **FEAT-04:** Đa dự án song song (GitHub App, RBAC theo dự án, dispatcher chia lượt).

---

## 4. ĐỐI CHIẾU VỚI TASK ĐÃ GIAO (handoff/tasks)
| Handoff | Ứng với kế hoạch | Trạng thái |
| :--- | :--- | :--- |
| TASK-001 | TASK-01, TASK-02, bảng TASK-10 | accepted |
| TASK-002 | TASK-11 | accepted |
| TASK-003 | TASK-03 + WS FEAT-01 | accepted |
| TASK-004 | TASK-04, TASK-12 | accepted |
| TASK-005 | TASK-05 (auth) | accepted |
| TASK-006 | TASK-13 (API) | accepted |
| TASK-007 | TASK-05 (webhook), TASK-06 (API) | accepted |
| TASK-008 | TASK-06, TASK-13 (UI) | accepted |
| TASK-009 | TASK-08, TASK-14 | accepted |
| TASK-010 | TASK-15 | accepted |
| TASK-011 | tách tasks.Service (Telegram duyệt nhanh) | accepted |
| TASK-012 | TASK-16 | accepted |
| TASK-013 | DEC-12 (runner) | accepted |
| TASK-014 | TASK-09 phần Claude review, DEC-12 (pipeline) | accepted |
| TASK-015 | SCR-04, SCR-07, TASK-17 | accepted |
| TASK-016 | TASK-07 (backend log) | accepted |
| TASK-017 | TASK-07 (SCR-06) | accepted |
| TASK-018 | TASK-09 (Chốt chặn CI, Merge → DONE) | accepted |
| TASK-030 → TASK-035 | FEAT-06 + FEAT-02 bản 2 (030/031/032/033 máy chính, 034/035 macbook) | accepted |
| TASK-036 → TASK-047 | FEAT-07 → FEAT-10 (036/037/038/040/041/043/048 máy chính; 039/042/044/045/046/047 macbook) | accepted |
| TASK-048 | REPLAN/UNBLOCK có hành động | in_progress |

---

### Milestone 6: Phòng IT phân tán vận hành thật (FEAT-07 → FEAT-10, đặc tả 2026-11-04)
DoD chi tiết ở §9 của [clarification-and-shared-skills.md](features/clarification-and-shared-skills.md), [ops-readiness.md](features/ops-readiness.md), [remote-command.md](features/remote-command.md).

**Ưu tiên 1 — chạy thật & an toàn (FEAT-09):**
- [x] **TASK-041: Allowlist lệnh theo dự án, runner dựng cờ claude/agy (không bỏ qua quyền)** — TC-83, TC-84, TC-88
- [x] **TASK-042: Runner kiểm đăng nhập bằng lời gọi thật + mẫu lỗi OAuth** — TC-85
- [x] **TASK-043: Máy partner chỉ nhận việc dự án mình là thành viên** — TC-86, TC-87
- [x] **TASK-044: Dự án sandbox + Chạy thử dây chuyền** — TC-89

**Ưu tiên 2 — làm rõ yêu cầu & kỹ năng (FEAT-07/08):**
- [x] **TASK-036: Migration `005_ops_remote.sql` (dựng lại `tasks` + `WAITING_USER`, `task_clarifications`, cột FEAT-09, `personal_tokens`, `lead_proposals`, `smoke_runs`)** — TC-74 · *làm đầu tiên, các task khác phụ thuộc*
- [x] **TASK-037: Skill `agenthub-spec` nhúng binary, runner chép vào worktree, prompt SPEC** — TC-81
- [x] **TASK-038: Pipeline `NEED_INFO`/`WAITING_USER`, nhắc 24h/72h, 3 vòng, GitHub + Telegram owner** — TC-75, TC-76, TC-80
- [x] **TASK-039: API làm rõ + `/answer` trên GitHub + callback Telegram** — TC-77 → TC-79
- [x] **TASK-040: Web SCR-04 tab Làm rõ, nhãn Kanban** — TC-82

**Ưu tiên 3 — điều hành mọi nơi (FEAT-10):**
- [x] **TASK-045: Lệnh Telegram cho owner (`/new`, `/tasks`, `/status`, Merge, đề xuất huỷ)** — TC-90, TC-91
- [x] **TASK-046: Token cá nhân + MCP `/mcp` + SCR-12 Hồ sơ** — TC-92, TC-93
- [x] **TASK-047: Web responsive 375px** — TC-96
- [ ] **TASK-048: REPLAN/UNBLOCK có hành động + `lead_proposals`** — TC-94, TC-95

**Thứ tự & phạm vi file:** TASK-036 → (041 `internal/projects`+`internal/runner` exec ∥ 043 `internal/dispatcher` ∥ 037 `internal/skills`+`internal/runner` worktree) — 041 và 037/042 cùng `internal/runner` nên chạy nối tiếp nhau → 042 → 044 → 038 → 039 ∥ 040 → 045 ∥ 046 ∥ 047 → 048.

**Trưởng phòng tự làm (ngoài handoff):** nối dây `cmd/server/main.go`; sự kiện `JOB_ASSIGNED/SUBMITTED`; giết cả nhóm tiến trình khi huỷ CLI; `Reconcile` bù job còn thiếu (30 giây/lần); nút Merge trên SCR-04; sửa route `/office`; test trọn dây chuyền `internal/e2e`.

**Đợt FEAT-04/05 (2026-11-04):** TASK-019 migration + schema (xong) → TASK-020 dự án ∥ TASK-027 dispatcher ∥ TASK-021 runner đa repo ∥ TASK-023 thêm máy → TASK-026 tasks/pipeline theo dự án → TASK-022 UI dự án ∥ TASK-024 UI máy ∥ TASK-025 Đóng góp ∥ TASK-028 API bổ sung cho UI (fleet chủ máy/phiên bản/`lead_eligible`/dự án agent, `open_tasks`, tạo lại webhook) ∥ TASK-029 gateway phiên bản + `CloseWorker` → nối `cmd/server` + `deploy/build.sh`. DoD ở [features/multi-project.md](features/multi-project.md) §9, [features/join-machine.md](features/join-machine.md) §9.

**Còn lại (nhỏ):** nhãn chữ trên canvas văn phòng ảo bị cắt, dãy bàn chưa ghi tên máy; Telegram chưa có nút Merge (chỉ web); chưa có dữ liệu thật trên GitHub/Telegram để chạy thử ngoài môi trường test.
