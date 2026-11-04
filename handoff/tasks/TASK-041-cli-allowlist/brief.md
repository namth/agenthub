# TASK-041 — Allowlist lệnh theo dự án, runner cấp quyền CLI (không bỏ qua quyền)

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-09: DEC-34, DEC-38, BR-56, BR-59 · TC-83, TC-84, TC-88

## 1. Mục tiêu
CLI chạy không tương tác hiện không có quyền ghi file/chạy `git`, `gh`, test → job thất bại. Cấp đúng tập lệnh cho phép theo dự án qua cờ phiên của CLI, **không bao giờ** dùng chế độ bỏ qua quyền.

## 2. Tài liệu bắt buộc đọc
- `docs/features/ops-readiness.md` §4 (SCR-09 Lệnh được phép, validate), §6, §7 BR-56, BR-59
- Test đặc tả: `internal/runner/allowlist_test.go`, `internal/pipeline/allowlist_test.go`, `internal/gateway/extras_test.go`, `internal/projects/allowlist_test.go`

## 3. Phạm vi file
**Được tạo/sửa:** file không phải `_test.go` trong `internal/runner/`, `internal/pipeline/`, `internal/gateway/`, `internal/projects/`; `cmd/server/main.go` (nối dây).
**Không được sửa:** file trong `tests.lock` và mọi `_test.go`; package khác; migration; `web/`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- **projects:** `PATCH /api/admin/projects/{id}` nhận thêm `allowed_commands` (mảng chuỗi hoặc `null` = xoá), `needs_ci_for_integration` (bool), `is_sandbox` (bool). Validate theo `ops-readiness.md` §4: ≤ 40 mục, mỗi mục `^[a-zA-Z0-9_./ -]{1,60}$` sau khi trim, không chứa `sudo`, không là `*`, không chứa `rm -rf /`; sai → `400 VALIDATION_ERROR`. Lưu JSON vào `projects.allowed_commands`. `GET /api/projects` (owner) trả thêm ba trường (`allowed_commands` là mảng, rỗng `[]` khi NULL).
- **pipeline:** `func DefaultAllowedCommands() []string` = `git`, `gh pr create`, `gh pr view`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`. `func (p *Pipeline) PayloadFor(a dispatcher.Assignment) map[string]any` → `{"allowed_commands": <mặc định ∪ dự án của job, bỏ trùng, giữ thứ tự mặc định trước>, "needs_ci": <bool>}`.
- **gateway:** `func WithPayloadExtras(fn func(dispatcher.Assignment) map[string]any) Option` — gộp vào `ASSIGN_TASK.payload`, **không** ghi đè khoá đã có (`task_id`, `prompt`, `repo`…). `main.go` nối `WithPayloadExtras(pl.PayloadFor)`.
- **runner:** khi `payload.allowed_commands` có mặt (kể cả mảng rỗng), sau khi thay `{prompt}`:
  - agent `CLAUDE`: nối thêm `--permission-mode acceptEdits --allowedTools Read Edit Write Glob Grep Bash(<tiền tố>:*)…` (mỗi tiền tố một phần tử argv, vd `Bash(go test:*)`).
  - agent `ANTIGRAVITY`: nối thêm `--settings <JSON>` với `{"permissions":{"allow":["command(<từ đầu tiên của tiền tố>)",…]}}` (bỏ trùng, vd `go test` và `gofmt` → `command(go)`, `command(gofmt)`).
  - `needs_ci = true` → thêm vào cuối prompt (trong khối DỮ LIỆU) dòng: "Dự án dùng CI cho test tích hợp: không chạy test cần secret, dựa vào CI gate."
  - Không có `allowed_commands` trong payload → argv giữ nguyên như hiện nay (server cũ).
  - Tuyệt đối không thêm `--dangerously-skip-permissions`, `bypassPermissions`, hay `"allow": ["*"]`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-041-cli-allowlist/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi test cũ của các package trên phải xanh với `-race`.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc` (không `test`, `cp`, `npm`).
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
