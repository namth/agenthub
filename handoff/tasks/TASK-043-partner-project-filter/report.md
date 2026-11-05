# Báo cáo TASK-043

## Đã làm
- Cập nhật logic nhận diện "máy partner": `workers.owner_user_id IS NOT NULL` và `users.role <> 'ADMIN'` của chủ máy (DEC-17). Máy không có chủ hoặc chủ ADMIN giữ nguyên quy tắc cũ.
- Cập nhật `RequestJob` cho cả nhánh job thường lẫn job `lead_only`: nếu là máy partner, chỉ chọn job có `project_id IN (SELECT project_id FROM project_members WHERE user_id = <chủ máy>)`, sau đó áp `agent_projects` như cũ cho nhánh job thường (không có dòng = mọi dự án mình tham gia).
- Cập nhật `Tick`: quét các assignment `ACTIVE` trên máy partner mà chủ máy không còn là thành viên dự án của job -> chuyển assignment thành `RECLAIMED`, chuyển agent về `IDLE`, chuyển job về `QUEUED` giữ nguyên `attempt` (không tăng), phát các sự kiện `EvJobReclaimed` + `EvAbortTask` + `EvJobAvailable`.
- Tự kiểm bằng `python3 scripts/handoff.py verify TASK-043`, kiểm tra `gofmt`, `go vet`, toàn bộ test trong `tests.lock` cùng các test cũ với cờ `-race`, và `go build ./...` đạt exit 0.

## File đã tạo/sửa
- `internal/dispatcher/service.go`: Bổ sung kiểm tra máy partner và lọc `project_members` trong `RequestJob`, bổ sung thu hồi assignment do mất quyền thành viên dự án trong `Tick`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-043-partner-project-filter/verify.sh
ok  	github.com/namth/agenthub/internal/dispatcher	25.258s
ok  	github.com/namth/agenthub/internal/api	4.799s
ok  	github.com/namth/agenthub/internal/gateway	3.557s
TASK-043 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
