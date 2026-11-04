# TASK-032 — Dispatcher: trưởng phòng mọi loại agent (DEC-29)

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-06/FEAT-01: DEC-29, BR-42 (thay một phần BR-06, BR-07)

## 1. Mục tiêu
Vai trưởng phòng không còn khoá cho Claude: agent bất kỳ loại có `lead_eligible = 1` đều là ứng viên; job `lead_only` đi theo người giữ lease; QA vẫn độc lập.

## 2. Tài liệu bắt buộc đọc
- `docs/14-decisions.md` DEC-29; `docs/features/agent-auth.md` §7 BR-42; `docs/features/lead-rotation.md` BR-06, BR-07 (bản đã sửa)
- Test đặc tả: `internal/dispatcher/lead_any_test.go`, `internal/api/fleet_lead_any_test.go`, `internal/api/fleet_test.go` (đã sửa kỳ vọng `lead_priority` cho Antigravity)

## 3. Phạm vi file
**Được tạo/sửa:** file không phải `_test.go` trong `internal/dispatcher/` và `internal/api/`.

**Không được sửa:** file trong `tests.lock` và mọi `_test.go`; package khác; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- **Connect/ConnectAuth:** agent **mới** loại `ANTIGRAVITY` được tạo với `lead_eligible = 0`; agent đã có giữ nguyên giá trị (Admin có thể đã bật).
- **Bộ chọn lead** (`tryAssignLeaseLocked`) và **`AdminAssign`**: bỏ điều kiện `kind = 'CLAUDE'`. Điều kiện còn lại giữ nguyên: `lead_eligible = 1`, `status = IDLE`, `auth_status <> 'NEEDS_AUTH'`, máy `ONLINE`, `paused = 0`, `gh_auth <> 'NEEDS_AUTH'`, không bị cấm 10 phút. Thứ tự BR-06 giữ nguyên (`lead_priority` → hạn mềm → `idle_since`).
- **`EvLeadVacant.Until`** = `MIN(cooldown_until)` của agent `lead_eligible = 1` đang `COOLDOWN` (mọi loại), thay vì chỉ Claude.
- **`RequestJob` của lead:** chọn job `lead_only = 1` **bất kể `required_agent`** (vẫn ưu tiên trước job thường, mọi dự án).
- **QA độc lập:** job `QA_REVIEW` không giao cho agent có assignment `SUBMITTED` của job `CODE` cùng `task_id` (agent khác cùng loại vẫn nhận; nếu chỉ còn tác giả thì job chờ).
- **API `PATCH /api/admin/agents/{id}`:** `lead_priority` nhận cho **mọi** `kind` (bỏ lỗi "chỉ áp dụng cho agent CLAUDE"); `POST /api/admin/lead/assign` dùng `AdminAssign` như trên. `GET /api/fleet` `lead.eta` theo agent `lead_eligible` đang COOLDOWN (mọi loại); `lead.waiting_jobs` = job `lead_only` hoặc `required_agent = 'CLAUDE'` đang QUEUED (giữ ý nghĩa "việc trí tuệ đang chờ").

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-032-lead-any-kind/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi test dispatcher/api cũ phải xanh với `-race` (TC-11 → TC-22, BR-29, BR-33, TC-53, TASK-030).
- Không đổi schema (không migration mới).
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
