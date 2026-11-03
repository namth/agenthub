# 04. LUỒNG DI CHUYỂN NGƯỜI DÙNG & TÁC NHÂN (USER FLOWS)

---

## 1. LUỒNG 1: ĐỐI TÁC TẠO YÊU CẦU & BẠN PHÊ DUYỆT (FEATURE INTAKE & APPROVAL)

```mermaid
sequenceDiagram
    autonumber
    actor Partner as Đối tác
    participant Web as AgentHub Web UI
    participant Server as Go Orchestrator Core
    participant DB as SQLite DB
    participant GH as GitHub API
    actor You as Bạn (Product Owner)
    participant Tele as Telegram Bot

    Partner->>Web: Truy cập /tasks/new & điền form tính năng
    Web->>Server: POST /api/tasks (Payload: title, description)
    Server->>GH: Tạo Issue (Labels: `status:pending-approval`, `type:feature`)
    GH-->>Server: Trả về GitHub Issue #101
    Server->>DB: Lưu Task (status='PENDING_APPROVAL', github_issue_id=101)
    Server->>Tele: Bắn tin nhắn: "🔔 Có yêu cầu mới #101 từ Đối tác" [Nút: Duyệt ngay / Từ chối]
    
    alt Bạn duyệt qua Telegram
        You->>Tele: Bấm nút "Duyệt ngay"
        Tele->>Server: Webhook callback (action='approve', issue_id=101)
    else Bạn duyệt qua Web Dashboard
        You->>Web: Bấm nút "Approve & Plan" trên SCR-04
        Web->>Server: POST /api/tasks/101/approve
    end

    Server->>DB: Cập nhật status='PLANNING'
    Server->>GH: Cập nhật Label `status:planning`
    Server->>Web: Bắn sự kiện WebSocket cập nhật Kanban sang cột "Planning"
```

---

## 2. LUỒNG 2: CLAUDE CODE LẬP KẾ HOẠCH & BẺ NHỎ TÁC VỤ (PLANNING FLOW)

```mermaid
sequenceDiagram
    autonumber
    participant Server as Go Orchestrator Core
    participant RunnerC as Runner (Máy Claude Code)
    participant Claude as Claude Code CLI
    participant GH as GitHub API
    participant DB as SQLite DB

    Server->>RunnerC: Gửi WS Event: `DISPATCH_TASK` (Task #101, Role: PLANNER)
    RunnerC->>RunnerC: Tạo worktree tạm: `.worktrees/plan-101`
    RunnerC->>Claude: Kích hoạt: `claude -p "Phân tích yêu cầu #101 và tạo các Sub-tasks kỹ thuật..."`
    loop Stream logs
        Claude-->>RunnerC: In logs tiến độ
        RunnerC-->>Server: WS Event: `LOG_CHUNK` (Live streaming)
    end
    Claude->>RunnerC: Trả về danh sách 4 Sub-tasks kèm DoD & File targets
    RunnerC->>GH: Tạo 4 Sub-issues trên GitHub (#102, #103, #104, #105) gắn nhãn `parent:#101`
    RunnerC->>Server: WS Event: `TASK_COMPLETED` (Kèm danh sách sub-issue IDs)
    Server->>DB: Lưu các Sub-tasks vào DB (status='READY')
    RunnerC->>RunnerC: Xóa worktree tạm `.worktrees/plan-101`
```

---

## 3. LUỒNG 3: DÀN MÁY ANTIGRAVITY THI CÔNG PHÂN TÁN (EXECUTION FLOW)

```mermaid
sequenceDiagram
    autonumber
    participant Server as Go Orchestrator Core
    participant RunnerA as Runner (Máy Antigravity)
    participant AGY as Antigravity CLI
    participant Git as Local Git Worktree
    participant GH as GitHub Remote

    RunnerA->>Server: WS Event: `REQUEST_TASK` (Máy đang rảnh - IDLE)
    Server->>Server: Kiểm tra Queue -> Giao Sub-task #102
    Server-->>RunnerA: WS Event: `ASSIGN_TASK` (Sub-task #102, prompt, branch='feat/task-102')
    
    RunnerA->>Git: `git worktree add .worktrees/task-102 -b feat/task-102`
    RunnerA->>AGY: Khởi chạy Antigravity thực hiện nhiệm vụ trong thư mục worktree
    loop Live Terminal
        AGY-->>RunnerA: Logs tiến trình code/test
        RunnerA-->>Server: WS Event: `LOG_CHUNK`
    end
    AGY->>Git: `git add -A && git commit -m "feat: complete sub-task #102 [by Antigravity]"`
    RunnerA->>GH: `git push origin feat/task-102`
    RunnerA->>GH: Tạo Pull Request (PR) liên kết Issue #102
    RunnerA->>Git: `git worktree remove .worktrees/task-102`
    RunnerA->>Server: WS Event: `TASK_SUBMITTED` (pr_url, status='WAITING_QA')
```

---

## 4. LUỒNG 4: KIỂM ĐỊNH CHẤT LƯỢNG 2 LỚP & XUẤT BẢN (QA & MERGE)

1. **Lớp 1 (GitHub Actions CI):** Tự động kích hoạt khi PR được mở. Chạy build, lint, unit tests. Nếu fail -> Gắn label `ci:failed` -> Server tự giao lại task cho Antigravity sửa lỗi.
2. **Lớp 2 (Claude Code AI Review):** Nếu CI passed -> Server giao PR cho máy Claude Code kéo về review logic, bảo mật.
   - Nếu đạt: Claude comment review và gắn nhãn `qa-passed`.
   - Nếu chưa đạt: Claude comment chi tiết các điểm cần refactor.
3. **Xuất bản (Release):** Server bắn tin Telegram cho Bạn: *"PR #15 cho Task #101 đã QA Passed"*. Bạn bấm nút **Merge** trên Dashboard -> Code được gộp vào `main` và tính năng được đóng gói bàn giao.

---

## 5. LUỒNG 5: CHUYỂN GIAO KHI HẾT LIMIT & LUÂN CHUYỂN TRƯỞNG PHÒNG

Chi tiết sơ đồ ở [features/lead-rotation.md](features/lead-rotation.md) mục 3. Tóm tắt:
1. **Agent hết limit giữa job:** Runner commit WIP + push nhánh, sinh `handoff_note`, gửi `AGENT_EXHAUSTED` → agent `COOLDOWN`; job về hàng đợi (`attempt + 1`); agent `IDLE` khác nhận, checkout cùng nhánh, làm tiếp.
2. **Lead hết limit / mất kết nối / hết hạn lease:** Server tăng `epoch`, trao lease cho agent Claude `IDLE` kế tiếp (BR-06), ghi `lead_journal`, báo Telegram. Lead cũ gửi lệnh với `epoch` cũ bị từ chối.
3. **Không còn Claude:** chế độ trống lead — job code vẫn chạy, việc trí tuệ chờ; Claude đầu tiên hết cooldown tự nhận lease.
4. **Admin can thiệp:** trao/thu hồi vai trên SCR-05.
