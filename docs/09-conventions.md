# 09. QUY CHUẨN MÃ NGUỒN & CẤU TRÚC THƯ MỤC (CONVENTIONS)

---

## 1. CẤU TRÚC THƯ MỤC TOÀN DỰ ÁN (PROJECT STRUCTURE)

```text
agenthub/
├── cmd/
│   └── server/
│       └── main.go               # Điểm khởi chạy của Single Binary Go Server
├── internal/
│   ├── auth/                     # GitHub OAuth, JWT, RBAC Middleware
│   │   ├── oauth.go
│   │   └── jwt.go
│   ├── database/                 # SQLite connection, DDL migrations
│   │   ├── db.go
│   │   └── schema.sql
│   ├── dispatcher/               # Task Dispatcher, Queue, Capability matching
│   │   └── dispatcher.go
│   ├── gateway/                  # WebSocket Hub, Client connection pool
│   │   ├── hub.go
│   │   └── client.go
│   ├── github/                   # GitHub REST client, Webhook handler
│   │   ├── client.go
│   │   └── webhook.go
│   ├── telegram/                 # Telegram Bot notifier & callback handler
│   │   └── bot.go
│   └── models/                   # Struct models & DTOs
│       ├── task.go
│       └── worker.go
├── web/                          # Frontend SPA
│   ├── index.html
│   ├── package.json
│   ├── vite.config.ts
│   ├── src/
│   │   ├── components/           # UI Components (KanbanCard, FleetRow, Terminal)
│   │   ├── pages/                # Screens (SCR-01 .. SCR-07)
│   │   ├── hooks/                # useWebSocket, useAuth
│   │   └── App.tsx
│   └── dist/                     # Thư mục build được Go nhúng (go:embed)
│   └── runner/                   # Runner daemon máy con (lệnh `agenthub runner`, DEC-10)
├── docs/                         # Bộ 14 tài liệu kỹ thuật + docs/features/
├── CONTEXT.md                    # Root Context
├── go.mod
├── go.sum
└── Makefile                      # Lệnh build frontend & single binary
```

---

## 2. QUY CHUẨN ĐẶT TÊN (NAMING CONVENTIONS)

### 2.1 Git Branches & Worktrees
* **Nhánh chính thức:** `main` (bảo vệ tuyệt đối, không commit trực tiếp).
* **Nhánh tính năng của Worker:** `feat/task-<github_issue_id>` (ví dụ: `feat/task-102`).
* **Nhánh sửa lỗi kiểm thử:** `fix/task-<github_issue_id>-review`.
* **Đường dẫn Git Worktree trên máy con:** `.worktrees/task-<github_issue_id>`.
* **Làm tiếp sau chuyển giao:** máy kế tiếp dùng **cùng nhánh** `feat/task-<id>` (`git fetch && git worktree add .worktrees/task-<id> feat/task-<id>`), không tạo nhánh mới.

### 2.2 Go Code Conventions
* Sử dụng chuẩn Go chuẩn mực: `gofmt` và `golangci-lint`.
* Tên package viết thường, ngắn gọn, một từ duy nhất: `auth`, `gateway`, `dispatcher`.
* Error handling luôn tường minh: `if err != nil { return fmt.Errorf("dispatcher: %w", err) }`.

### 2.3 Commit Message Convention
Mọi commit từ cả người và AI đều phải tuân thủ chuẩn Conventional Commits kèm tag định danh AI:
* `feat(auth): integrate github oauth flow [by Antigravity]`
* `plan(task-101): breakdown architecture specifications [by Claude]`
* `qa(pr-15): verify unit tests and edge cases [by Claude]`
* `wip(task-102): attempt 1 handoff [by Runner]` — commit tự động khi agent hết limit giữa chừng (FEAT-01 BR-11); không squash tay, PR cuối squash-merge.
