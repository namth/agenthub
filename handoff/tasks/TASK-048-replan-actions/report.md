# Báo cáo TASK-048

## Đã làm
- Hiện thực xử lý kết quả REPLAN / UNBLOCK trong `internal/pipeline`:
  - Hành động `retry`: ghi nhật ký `lead_journal` (loại `DECISION`), task chuyển sang `READY` và tạo job `CODE` mới cùng nhánh (với REPLAN); chỉ ghi nhật ký (với UNBLOCK).
  - Hành động `replan`: với task gốc thì chuyển các sub-task chưa `DONE` sang `REJECTED`, đóng issue GitHub tương ứng kèm lý do, tạo các sub-task mới và job `CODE` tương ứng; với sub-task thì chuyển sub-task đó sang `REJECTED` và tạo các sub-task mới cùng cha.
  - Hành động `ask_user`: chuyển task gốc sang `WAITING_USER`, tạo vòng phỏng vấn câu hỏi làm rõ mới và gọi hook `onNeedInfo`.
  - Hành động `cancel`: tạo đề xuất `lead_proposals` với trạng thái `PENDING`, loại `CANCEL`, gắn `epoch` hiện tại, giữ nguyên task và gọi hook `WithOnProposal`.
  - Kết quả không hợp lệ: gọi hook `WithOnReplanInvalid`, kiểm tra giới hạn không lặp quá 2 REPLAN trong vòng 24 giờ.
  - Mở rộng `PromptFor` cho REPLAN/UNBLOCK với schema 4 hành động và các dữ liệu tham khảo: số lần thử, nhận xét QA gần nhất, 20 dòng log cuối từ `task_logs`.
- Thêm quản lý đề xuất huỷ trong `internal/tasks`:
  - Thêm struct `Proposal` và hàm `DecideProposal(ctx, userID, role, taskID, proposalID, accept)`.
  - Kiểm tra quyền: `ADMIN` hoặc ADMIN dự án; kiểm tra chuyển trạng thái từ `PENDING`.
  - Khi chấp nhận: cập nhật đề xuất sang `ACCEPTED`, task và các sub-task chưa `DONE` sang `REJECTED`, huỷ các job `QUEUED`, đóng issue GitHub.
  - Khi từ chối: cập nhật đề xuất sang `REJECTED`, ghi `lead_journal` "Admin từ chối huỷ", giữ nguyên trạng thái task.
  - Cung cấp các endpoint REST: `POST /api/tasks/{id}/proposals/{pid}/accept`, `POST /api/tasks/{id}/proposals/{pid}/reject`, và bổ sung mảng `proposals` (chỉ `PENDING`) trong `GET /api/tasks/{id}`.
- Tích hợp thông báo và điều khiển qua Telegram trong `internal/telegram`:
  - Thêm phương thức `NotifyProposal` gửi tin nhắn kèm các nút inline "Đồng ý huỷ" (`proposal_accept:<task>:<pid>`) và "Từ chối" (`proposal_reject:<task>:<pid>`).
  - Thêm interface `ProposalDecider` và xử lý callback `proposal_accept` / `proposal_reject` trong webhook handler.
  - Thêm phương thức `NotifyReplanInvalid`.
- Tích hợp máy chủ trong `cmd/server/`:
  - Cài đặt `ProposalDecider` cho `TelegramCommander`.
  - Cài đặt tool `DecideProposal` cho `serverMCPBackend`.
  - Nối các hook `WithOnProposal` và `WithOnReplanInvalid` trong `main.go`.
- Cập nhật giao diện web (SCR-04) trong `web/src/pages/TaskDetail.tsx`:
  - Hiển thị băng thông báo `lead-proposal` với nội dung "Trưởng phòng đề xuất huỷ: <lý do>".
  - Hiển thị nút "Đồng ý huỷ" (kèm xác nhận qua dialog confirm) và nút "Từ chối" cho vai trò `ADMIN`.
  - Vai trò `PARTNER` chỉ xem băng thông báo, không có các nút hành động.
- Xác minh toàn bộ tiêu chí nghiệm thu qua script `verify.sh`: tất cả test Go và Vitest đều vượt qua.

## File đã tạo/sửa
- `internal/pipeline/pipeline.go`: Hiện thực 4 hành động REPLAN/UNBLOCK, xử lý schema không hợp lệ, bổ sung prompt schema và hook `WithOnProposal`, `WithOnReplanInvalid`.
- `internal/tasks/service.go`: Thêm struct `Proposal` và hàm `DecideProposal`.
- `internal/tasks/handler.go`: Bổ sung `proposals` vào `GET /api/tasks/{id}` và định tuyến cho các endpoint accept/reject proposal.
- `internal/telegram/notifier.go`: Bổ sung `NotifyProposal` và `NotifyReplanInvalid`.
- `internal/telegram/webhook.go`: Bổ sung interface `ProposalDecider` và xử lý callback query proposal.
- `cmd/server/telegram_commander.go`: Hiện thực `DecideProposal` cho Telegram commander.
- `cmd/server/mcp_backend.go`: Hiện thực `DecideProposal` cho MCP backend.
- `cmd/server/main.go`: Nối hook `WithOnProposal` và `WithOnReplanInvalid`.
- `web/src/pages/TaskDetail.tsx`: Hiển thị băng đề xuất huỷ SCR-04 và xử lý hành động accept/reject.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-048-replan-actions/verify.sh
ok  	github.com/namth/agenthub/internal/pipeline	12.056s
ok  	github.com/namth/agenthub/internal/tasks	9.268s
ok  	github.com/namth/agenthub/internal/telegram	1.998s
 Test Files  16 passed (16)
      Tests  152 passed (152)
TASK-048 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
