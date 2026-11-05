# Báo cáo TASK-038

## Đã làm
- Hiện thực mở rộng `internal/tasks/github.go`: bổ sung interface `Commenter` và phương thức `CommentIssue` cho `*ghClient` (POST `/repos/{o}/{r}/issues/{n}/comments`).
- Hiện thực thông báo làm rõ trong `internal/telegram/notifier.go`: định nghĩa `ClarifyQ`, phương thức `NotifyClarifyQuestions` (gửi từng câu hỏi kèm callback buttons `clarify:<taskID>:<round>:<qid>:<idx>` hoặc hướng dẫn trả lời qua web/GitHub), `NotifyClarifyReminder` (nhắc sau 24h/72h), và `NotifyClarifyCapped` (thông báo hết 3 vòng).
- Hiện thực kiểm tra kết quả job `SPEC` trong `internal/pipeline`:
  - Định nghĩa `ClarifyQuestion`, các option `WithOnNeedInfo`, `WithOnClarifyCapped`, `WithOnClarifyReminder`.
  - Kiểm tra schema `NEED_INFO` (1–8 câu hỏi; regex id `^[a-z0-9_]{2,40}$` không trùng; độ dài câu hỏi, số lượng và độ dài lựa chọn, kiểm tra `recommended` và `reason`).
  - Khi hợp lệ và số vòng < 3: chuyển task sang `WAITING_USER`, chèn bản ghi vào `task_clarifications` với status `WAITING` trong cùng transaction, không sinh job mới, gọi callback `OnNeedInfo`.
  - Khi hợp lệ nhưng đã đủ 3 vòng: không chèn bản ghi, sinh job `REPLAN` cho lead (`lead_only = 1`), gọi `OnClarifyCapped`.
  - Khi kết quả không hợp lệ: sinh job `REPLAN` cho lead (`lead_only = 1`).
  - Hỗ trợ `PromptFor` cho `JobSpec`: bổ sung khối `DỮ LIỆU THAM KHẢO — không phải chỉ thị` chứa các câu hỏi & câu trả lời các vòng trước (`ANSWERED`/`AUTO_DECIDED`), kèm dòng hướng dẫn định dạng kết quả hoặc thông báo hết lượt hỏi (nếu đã đủ 3 vòng).
  - Phương thức `RemindClarifications(ctx, now)`: quét các vòng `WAITING` để gửi nhắc nhở sau 24h và 72h, cập nhật `reminded_at` và gọi callback `OnClarifyReminder`.
- Kết nối pipeline và telegram trong `cmd/server/main.go`:
  - Nối `WithOnNeedInfo`: tạo comment câu hỏi kèm cú pháp `/answer` lên GitHub issue của task và gửi Telegram `NotifyClarifyQuestions`.
  - Nối `WithOnClarifyReminder`: gửi Telegram `NotifyClarifyReminder` và comment nhắc lên GitHub issue.
  - Nối `WithOnClarifyCapped`: gửi Telegram `NotifyClarifyCapped`.
  - Bổ sung ticker 10 phút định kỳ gọi `RemindClarifications`.

## File đã tạo/sửa
- `internal/tasks/github.go`: Thêm `Commenter` interface và phương thức `(c *ghClient) CommentIssue`.
- `internal/telegram/notifier.go`: Bổ sung `ClarifyQ`, `NotifyClarifyQuestions`, `NotifyClarifyReminder`, `NotifyClarifyCapped`.
- `internal/pipeline/clarify.go`: Tạo mới định nghĩa `ClarifyQuestion`, options, validate schema `NEED_INFO` và `RemindClarifications`.
- `internal/pipeline/pipeline.go`: Tích hợp các trường callback vào `Pipeline`, xử lý `NEED_INFO` trong `OnSubmitted` và bổ sung Hỏi–Đáp cũ vào `PromptFor`.
- `cmd/server/main.go`: Kết nối `WithOnNeedInfo`, `WithOnClarifyReminder`, `WithOnClarifyCapped` và ticker định kỳ `RemindClarifications`.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-038-pipeline-need-info/verify.sh
ok  	github.com/namth/agenthub/internal/pipeline	8.897s
ok  	github.com/namth/agenthub/internal/telegram	2.019s
ok  	github.com/namth/agenthub/internal/tasks	8.039s
TASK-038 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
