# TASK-048 — REPLAN/UNBLOCK có hành động + đề xuất huỷ chờ Admin

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-10: DEC-40, BR-64 · TC-94, TC-95
> Dựa trên TASK-038 (pipeline `NEED_INFO`) đã gộp.

## 1. Mục tiêu
Hiện kết quả REPLAN/UNBLOCK của trưởng phòng AI bị bỏ qua → task kẹt hoặc SPEC làm lại từ đầu. Cho trưởng phòng hành động có kiểm soát: thử lại kèm hướng dẫn, lập lại kế hoạch, hỏi người dùng, hoặc **đề xuất** huỷ (Admin xác nhận).

## 2. Tài liệu bắt buộc đọc
- `docs/features/remote-command.md` §3, §4 (băng đề xuất huỷ SCR-04), §5 (`lead_proposals` — bảng đã có), §6 (schema `result.json` REPLAN/UNBLOCK, API proposals), §7 BR-64
- Test đặc tả: `internal/pipeline/replan_test.go`, `internal/tasks/proposals_test.go`, `internal/telegram/proposals_test.go`, `web/src/pages/proposal.test.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** file không phải test trong `internal/pipeline/`, `internal/tasks/`, `internal/telegram/`; `cmd/server/main.go`, `cmd/server/mcp_backend.go`, `cmd/server/telegram_commander.go`; `web/src/pages/TaskDetail.tsx`.
**Không được sửa:** file trong `tests.lock` và mọi file test; package khác; migration; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**pipeline — `OnSubmitted` job `REPLAN`/`UNBLOCK`** (`result.action`):
- `retry` (`note` bắt buộc, ≤ 2000): ghi `lead_journal` (task, kind `DECISION`, entry = note). REPLAN: task → `READY`, job `CODE` mới cùng `branch_name` của task (attempt bắt đầu lại). UNBLOCK: chỉ ghi journal.
- `replan` (`subtasks` 1–20 như SPEC): task mục tiêu là **task gốc** (`parent_id IS NULL`) → sub-task chưa `DONE` → `REJECTED` (đóng issue con, lý do "thay bởi kế hoạch mới"), tạo sub-task mới + job CODE như SPEC, task gốc `READY`. Task mục tiêu là **sub-task** → sub-task đó `REJECTED` (đóng issue), tạo sub-task mới cùng cha.
- `ask_user` (`questions` như NEED_INFO): đi đúng đường NEED_INFO của TASK-038 (WAITING_USER, vòng mới, hook) cho task gốc của task mục tiêu.
- `cancel` (`reason` bắt buộc): chèn `lead_proposals` (`PENDING`, kind `CANCEL`, `proposed_by` = agent, `epoch` = lease hiện hành); **task giữ nguyên**; hook `WithOnProposal(func(taskID, proposalID, reason string))`.
- Sai schema / thiếu `action`: không sinh REPLAN mới nếu task đã có ≥ 2 job REPLAN tạo trong 24 giờ qua (tránh vòng lặp); hook `WithOnReplanInvalid(func(taskID string))`.
- `PromptFor` REPLAN/UNBLOCK: thêm schema bốn hành động và (trong khối DỮ LIỆU) số lần thử, nhận xét QA gần nhất từ `lead_journal`, 20 dòng `task_logs` cuối.

**tasks — đề xuất**
```go
type Proposal struct { ID, Kind, Reason, ProposedBy, Status string; CreatedAt time.Time }
func (s *Service) DecideProposal(ctx context.Context, userID, role, taskID, proposalID string, accept bool) error
```
Quyền: `role == "ADMIN"` hoặc ADMIN dự án (`UserRoleIn`); khác → `ErrForbidden`. Proposal không `PENDING` → `ErrInvalidTransition`. Accept → proposal `ACCEPTED` (`decided_by`, `decided_at`), task (và sub-task chưa DONE) → `REJECTED`, đóng issue, huỷ job `QUEUED` của chúng (`CANCELLED`). Reject → proposal `REJECTED`, ghi journal "Admin từ chối huỷ". REST: `POST /api/tasks/{id}/proposals/{pid}/accept|reject` → `200`; `GET /api/tasks/{id}` thêm `proposals` (chỉ `PENDING`).

**telegram** — `NotifyProposal(taskID, title, proposalID, reason string)` gửi chat admin kèm nút "Đồng ý huỷ" (`proposal_accept:<task>:<pid>`) và "Từ chối" (`proposal_reject:<task>:<pid>`). Callback gọi interface tuỳ chọn `ProposalDecider { DecideProposal(ctx, taskID, proposalID string, accept bool) error }` nếu `Commander` cài.

**web (SCR-04)** — có proposal `PENDING`: băng `data-testid="lead-proposal"` "Trưởng phòng đề xuất huỷ: <lý do>"; ADMIN thấy nút **Đồng ý huỷ** (confirm) / **Từ chối** → POST tương ứng → tải lại; PARTNER chỉ xem.

**main** — nối hook `OnProposal` → `NotifyProposal`; `OnReplanInvalid` → Telegram; MCP `decide_proposal` → `DecideProposal`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-048-replan-actions/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- `cancel` không bao giờ tự huỷ task. Fencing: chỉ xử lý kết quả của assignment hợp lệ (đã có BR-04).
- Mọi test cũ phải xanh với `-race`.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`, `node` — mỗi lần một lệnh; không ghép `||` `&&` `|`; không `which`, `test`, `cp`, `npm`, `npx` (web chạy qua verify.sh).
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
