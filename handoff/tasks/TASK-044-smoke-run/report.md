# Báo cáo TASK-044: Dự án sandbox: Chạy thử dây chuyền + giao diện SCR-09 (allowlist, sandbox)

## Đã làm
- Hiện thực package `internal/smoke` với các hàm và phương thức theo đặc tả: `Start`, `Track`, `List`, `Get`, `Handler`.
- Xử lý nâng priority = 50 cho các job QUEUED của task và sub-task; theo dõi 3 bước SPEC, CODE, QA; timeout 30 phút; cleanup đóng issue và PR mẫu.
- Cài đặt `TaskOps` trên `tasks.Service` + GitHub client trong `cmd/server/main.go`, gắn router `smoke.Handler` và ticker 10 giây cho `Track`.
- Bổ sung giao diện SCR-09 trong `web/src/pages/Projects.tsx` với khung `project-ops-<id>`, textarea lệnh được phép, các công tắc CI/sandbox, nút Lưu cấu hình và nút Chạy thử dây chuyền kèm bảng `smoke-runs-<id>` thăm dò mỗi 3 giây.
- Vượt qua toàn bộ unit test Go (`TestStartRules`, `TestTrackToPassed`, `TestTrackFailedAndTimeout`, `TestHTTP`) và Vitest frontend (`projects_ops.test.tsx`, `projects.test.tsx`).

## File đã tạo/sửa
- `internal/smoke/smoke.go`: Hiện thực core logic và HTTP handler cho smoke testing.
- `cmd/server/main.go`: Cài đặt `TaskOps`, gắn route `smoke.Handler`, ticker `Track`.
- `web/src/pages/Projects.tsx`: Thêm `ProjectOps` với form SCR-09 và bảng theo dõi smoke runs.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-044-smoke-run/verify.sh
ok  	github.com/namth/agenthub/internal/smoke	2.066s
 Test Files  15 passed (15)
      Tests  148 passed (148)
TASK-044 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
