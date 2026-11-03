# AGENTHUB - ROOT CONTEXT

> **Tóm tắt siêu ngắn gọn (Root Context) để AI Agent luôn đọc đầu tiên khi bắt đầu làm việc trong codebase.**

---

## 1. MỤC TIÊU DỰ ÁN
AgentHub là nền tảng điều phối đa AI Agent phân tán (Distributed Multi-Agent Software Studio). Hệ thống kết nối Bạn (Product Owner), Đối tác (Partner), Claude Code (Tech Lead & QA) và Dàn máy Antigravity (Kỹ sư thi công) thành một chu trình phát triển phần mềm tự động khép kín.

## 2. KIẾN TRÚC & TECH STACK CHÍNH
- **Backend Core:** Golang (Router Chi / Fiber + Gorilla WebSocket), duy trì kết nối bền bỉ với cụm Worker.
- **Database:** SQLite (embedded, zero-config, driver thuần Go `modernc.org/sqlite`).
- **Frontend Dashboard:** Vite SPA (React + Tailwind CSS + shadcn/ui).
- **Đóng gói & Phân phối:** Đóng gói toàn bộ Backend + Database driver + Frontend nhúng thành **1 file nhị phân duy nhất (Single Binary)** qua `//go:embed`. Tiêu thụ ~20MB RAM.
- **Dữ liệu nguồn (System of Record):** GitHub Issues, Pull Requests, Branches & Webhooks.
- **Bảo mật & Phân quyền:** GitHub OAuth (Admin: Repository Owner; Partner: Collaborator).
- **Cách ly mã nguồn Worker:** Mỗi task chạy trên một **Git Worktree** độc lập (`.worktrees/task-<id>`).
- **Kiểm định chất lượng (QA):** 2 Lớp (GitHub Actions CI chạy trước -> Claude Code AI Review chuyên sâu).
- **Kênh thông báo tức thì:** Telegram Bot (thông báo duyệt yêu cầu & nghiệm thu PR).
- **Trưởng phòng trực & chuyển giao (FEAT-01):** Server Go là bộ điều phối duy nhất; việc trí tuệ là job; một agent Claude giữ `lead_lease` (epoch, TTL 90s). Hết limit → `COOLDOWN`, job giao lại cho agent rảnh làm tiếp từ nhánh; fencing bằng `assignment_id`/`epoch`.
- **Văn phòng ảo (FEAT-02):** SCR-08 tuỳ chọn, pixel-art chỉ quan sát, vẽ từ sự kiện server qua SSE.

## 3. CẤU TRÚC THƯ MỤC CỐT LÕI
```text
agenthub/
├── cmd/server/main.go            # Entrypoint của Single Binary Go Server
├── internal/
│   ├── auth/                     # GitHub OAuth & JWT Session
│   ├── database/                 # SQLite connection & migrations
│   ├── dispatcher/               # Task Queue & phân bổ việc cho Worker
│   ├── gateway/                  # WebSocket Hub (quản lý kết nối các máy con)
│   ├── github/                   # GitHub API Client & Webhook Handlers
│   └── telegram/                 # Telegram Bot alerts & inline approval
├── web/                          # Frontend source code (Vite + React + Tailwind)
│   ├── dist/                     # Build output được nhúng vào Go qua go:embed
│   └── src/                      # Kanban, Fleet Monitor, Live Terminal
│   (internal/runner/)            # Runner daemon máy con, lệnh `agenthub runner` (DEC-10)
├── docs/                         # Bộ 14 tài liệu + docs/features/
└── CONTEXT.md                    # File này
```

## 4. BỘ TÀI LIỆU CHI TIẾT (/docs)
1. [01-overview.md](docs/01-overview.md): Tổng quan bài toán, mục tiêu, đối tượng sử dụng.
2. [02-modules-and-features.md](docs/02-modules-and-features.md): Phân hệ & Ma trận phân quyền RBAC.
3. [03-screens-and-ui.md](docs/03-screens-and-ui.md): Bảng kiểm kê màn hình (SCR-XX), 4 trạng thái UI.
4. [04-user-flows.md](docs/04-user-flows.md): Luồng di chuyển của Đối tác, Bạn, Claude và Antigravity.
5. [05-architecture.md](docs/05-architecture.md): Sơ đồ kiến trúc Go + SQLite + WebSocket + Worktrees.
6. [06-database-schema.md](docs/06-database-schema.md): Sơ đồ quan hệ thực thể (ERD) SQLite.
7. [07-api-contracts.md](docs/07-api-contracts.md): Hợp đồng dữ liệu REST Endpoints & WebSocket Events.
8. [08-integrations.md](docs/08-integrations.md): Tích hợp GitHub OAuth/Webhook & Telegram Bot.
9. [09-conventions.md](docs/09-conventions.md): Quy chuẩn code Go, React, Git branch & Worktree naming.
10. [10-env-and-security.md](docs/10-env-and-security.md): Biến môi trường `.env.example` & chính sách bảo mật.
11. [11-tasks.md](docs/11-tasks.md): Lộ trình chia nhỏ Milestone & Tasks kèm DoD.
12. [12-business-rules.md](docs/12-business-rules.md): Ràng buộc logic, máy trạng thái Task & Heartbeat.
13. [13-test-cases-and-qa.md](docs/13-test-cases-and-qa.md): Kế hoạch kiểm thử, Ma trận Test Cases & QA Gate.
14. [14-decisions.md](docs/14-decisions.md): Nhật ký quyết định `DEC-NN` + backlog tính năng.

## 5. TÍNH NĂNG ĐÃ ĐẶC TẢ (/docs/features)
| Mã | Tính năng | File |
| :--- | :--- | :--- |
| FEAT-01 | Luân chuyển Trưởng phòng & chuyển giao khi hết limit | [features/lead-rotation.md](docs/features/lead-rotation.md) |
| FEAT-02 | Văn phòng ảo (sơ đồ phối hợp trực quan) | [features/virtual-office.md](docs/features/virtual-office.md) |
| FEAT-03 | Dây chuyền vai trò BA/Designer/Dev/Test | BACKLOG |
| FEAT-04 | Đa dự án song song | BACKLOG |
