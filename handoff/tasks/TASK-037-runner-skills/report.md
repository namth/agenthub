# Báo cáo TASK-037

## Đã làm
- Nhúng skill `agenthub-spec/SKILL.md` qua `//go:embed` trong package `internal/skills`, cung cấp biến `AgentHubSpec` và hàm `Files() map[string]string`.
- Định nghĩa interface `Snapshotter` với method `Snapshot(ctx context.Context, parentDir string) (dir string, err error)`.
- Cài đặt `Snapshot` trên `realGit`: `git fetch origin` rồi `git worktree add --detach <parentDir>/repo origin/HEAD` (fallback `origin/main`, `origin/master`, `HEAD`), dọn dẹp bằng `Cleanup` như worktree thường.
- Trong `runTask`: hỗ trợ job không nhánh chạy trên snapshot repo (`branchName == ""` và git client cài `Snapshotter`), log lỗi và giữ `dir = ""` nếu snapshot lỗi hoặc không cài `Snapshotter`.
- Chép skill cho các job `SPEC`, `REPLAN`, `UNBLOCK` (không cho `CODE`, `QA_REVIEW`): ưu tiên file `.agents/skills/agenthub-spec/SKILL.md` của dự án trong `dir` nếu có; nếu không thì ghi toàn bộ `skills.Files()` vào `<base>/.agenthub/skills/` và tạo `<base>/.agenthub/.gitignore` với nội dung `*`.
- Cập nhật prompt: thêm dòng `AGENTHUB_SKILL=<đường dẫn tuyệt đối SKILL.md>` và dòng "Đọc và tuân thủ file skill trên trước khi làm." trước dòng `AGENTHUB_RESULT_FILE`.
- Cập nhật instruction cho `SPEC`: "Ghi kết quả JSON theo skill: {\"status\":\"NEED_INFO\",\"questions\":[…]} hoặc {\"status\":\"DONE\",\"subtasks\":[…]} vào file trên."
- Đảm bảo dọn dẹp snapshot worktree bằng `gitClient.Cleanup` ở mọi đường thoát trong `runTask`.
- Tự kiểm qua `python3 scripts/handoff.py verify TASK-037` đạt 100% (exit 0).

## File đã tạo/sửa
- `internal/skills/skills.go`: Tạo mới — nhúng `agenthub-spec/SKILL.md` bằng `//go:embed` và xuất hàm `Files()`.
- `internal/runner/git.go`: Sửa — định nghĩa `Snapshotter` interface và cài đặt method `Snapshot` trên `realGit`.
- `internal/runner/runner.go`: Sửa — hỗ trợ snapshot cho job không nhánh, inject skill vào prompt và file system, cập nhật instruction cho SPEC, dọn dẹp worktree snapshot khi hoàn thành hoặc lỗi.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-037-runner-skills/verify.sh
ok  	github.com/namth/agenthub/internal/runner	18.950s
TASK-037 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
