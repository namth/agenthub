# TASK-037 — Skill `agenthub-spec` nhúng runner; job không nhánh chạy trên snapshot repo

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-08: DEC-32, BR-50, BR-51 · TC-81

## 1. Mục tiêu
1. Mọi máy, mọi dự án, mọi loại agent làm SPEC/REPLAN/UNBLOCK đều đọc cùng một chuẩn lập kế hoạch: skill `agenthub-spec` (đã viết sẵn ở `internal/skills/agenthub-spec/SKILL.md`) nhúng trong binary.
2. Sửa lỗ hổng: job không có nhánh (SPEC, REPLAN, UNBLOCK, QA không nhánh) hiện chạy CLI ở thư mục hiện hành của runner → agent **không thấy codebase**. Phải chạy trên một bản snapshot của nhánh mặc định của repo dự án.

## 2. Tài liệu bắt buộc đọc
- `docs/features/clarification-and-shared-skills.md` §7 (BR-50, BR-51), `docs/14-decisions.md` DEC-32
- Test đặc tả: `internal/runner/skills_test.go` (gồm `TestEmbeddedSkill`); code: `internal/runner/runner.go` (`runTask`), `internal/runner/git.go`, `internal/runner/cache.go`

## 3. Phạm vi file
**Được tạo/sửa:** `internal/skills/skills.go` (mới, `go:embed`), file không phải `_test.go` trong `internal/runner/`.
**Không được sửa:** `internal/skills/agenthub-spec/SKILL.md` (nội dung do trưởng phòng viết); file trong `tests.lock` và mọi `_test.go`; package khác; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- **`internal/skills`:** `//go:embed agenthub-spec/SKILL.md` → `var AgentHubSpec string`; `func Files() map[string]string` trả `{"agenthub-spec/SKILL.md": AgentHubSpec}`.
- **Snapshot cho job không nhánh** — interface tuỳ chọn (không thêm method vào `Git` để fake cũ không vỡ):
  ```go
  type Snapshotter interface { Snapshot(ctx context.Context, parentDir string) (dir string, err error) }
  ```
  `realGit` cài: `git fetch origin` rồi `git worktree add --detach <parentDir>/repo origin/HEAD` (fallback nhánh mặc định), trả đường dẫn; dọn bằng `Cleanup` như worktree thường. Trong `runTask`: `branchName == ""` và git client cài `Snapshotter` → `dir = Snapshot(ctx, tmpDir)`; lỗi → log và chạy như cũ (`dir = ""`). Không cài → như cũ.
- **Chép skill** cho job `SPEC`, `REPLAN`, `UNBLOCK` (không cho `CODE`, `QA_REVIEW`): `base = dir` nếu có, ngược lại `tmpDir`. Nếu `<dir>/.agents/skills/agenthub-spec/SKILL.md` tồn tại → dùng file đó. Ngược lại ghi mọi `skills.Files()` vào `<base>/.agenthub/skills/` và tạo `<base>/.agenthub/.gitignore` nội dung `*` (để `git status` sạch). Thêm vào cuối prompt (trước dòng `AGENTHUB_RESULT_FILE`): `AGENTHUB_SKILL=<đường dẫn tuyệt đối SKILL.md>` và dòng "Đọc và tuân thủ file skill trên trước khi làm."
- Instruction cho SPEC: thay câu cũ bằng "Ghi kết quả JSON theo skill: {\"status\":\"NEED_INFO\",\"questions\":[…]} hoặc {\"status\":\"DONE\",\"subtasks\":[…]} vào file trên."

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-037-runner-skills/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi test runner cũ phải xanh với `-race` (fakeGit cũ không cài `Snapshotter`).
- Không đưa đường dẫn skill vào job CODE/QA; không commit `.agenthub/`.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc` — mỗi lần một lệnh; không ghép `||` `&&` `|`; không `which`, `test`, `cp`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
