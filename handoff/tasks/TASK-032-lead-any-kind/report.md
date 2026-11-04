# Báo cáo TASK-032

## Đã làm
- [x] **Connect / ConnectAuth:** agent mới loại `ANTIGRAVITY` khởi tạo với `lead_eligible = 0`; agent đã có giữ nguyên giá trị khi kết nối lại (không ghi đè giá trị Admin đã bật).
- [x] **Bộ chọn lead (`tryAssignLeaseLocked`) & `AdminAssign`:** bỏ điều kiện `kind = 'CLAUDE'`, giữ nguyên các điều kiện `lead_eligible = 1`, `status = IDLE`, `auth_status <> 'NEEDS_AUTH'`, máy `ONLINE`, `paused = 0`, `gh_auth <> 'NEEDS_AUTH'`, không bị cấm 10 phút. Giữ nguyên thứ tự ưu tiên BR-06 (`lead_priority` → hạn mềm → `idle_since`).
- [x] **`EvLeadVacant.Until`:** tính theo `MIN(cooldown_until)` của mọi agent `lead_eligible = 1` đang `COOLDOWN` (mọi loại agent thay vì chỉ Claude).
- [x] **`RequestJob` của lead:** ưu tiên chọn job `lead_only = 1` trước job thường, bất kể `required_agent`.
- [x] **QA độc lập:** job `QA_REVIEW` không giao cho agent đã có assignment `SUBMITTED` của job `CODE` cùng `task_id`.
- [x] **API `PATCH /api/admin/agents/{id}`:** cho phép cập nhật `lead_priority` cho mọi loại agent (bỏ lỗi giới hạn riêng cho Claude).
- [x] **API `GET /api/fleet`:** cập nhật `lead.eta` theo `MIN(cooldown_until)` của các agent `lead_eligible = 1` đang `COOLDOWN`; cập nhật `lead.waiting_jobs` đếm các job đang `QUEUED` có `lead_only = 1` hoặc `required_agent = 'CLAUDE'`.
- [x] Chạy xác minh `python3 scripts/handoff.py verify TASK-032` đạt exit 0, tất cả test (bao gồm -race) đều pass.

## File đã tạo/sửa
- `internal/dispatcher/service.go`: Xử lý `lead_eligible` mặc định khi connect, mở rộng bộ chọn lead và AdminAssign cho mọi loại agent, cập nhật ETA khi vacant, giao job `lead_only` bất kể `required_agent`, bảo đảm QA độc lập không tự review.
- `internal/api/fleet.go`: Bỏ giới hạn `lead_priority` theo loại agent trong `PATCH /api/admin/agents/{id}`; cập nhật `ETA` và `waiting_jobs` trong `GET /api/fleet`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-032-lead-any-kind/verify.sh
ok  	github.com/namth/agenthub/internal/dispatcher	14.371s
ok  	github.com/namth/agenthub/internal/api	3.870s
ok  	github.com/namth/agenthub/internal/gateway	4.092s
ok  	github.com/namth/agenthub/internal/machines	5.485s
TASK-032 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
