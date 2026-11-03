---
name: thuc-hien-task
description: Nhận và thực hiện một task do trưởng phòng (Claude) giao trong handoff/tasks — đọc brief, làm trong phạm vi, tự kiểm bằng verify.sh, viết report.md và nộp. Dùng khi được giao TASK-NNN, khi được yêu cầu làm/sửa một task trong handoff/tasks, hoặc khi nhận yêu cầu sửa sau nghiệm thu.
---

# Thực hiện task được giao

Nếu chưa biết task nào: chạy `python3 scripts/handoff.py list` và chọn task ở trạng thái `assigned` hoặc `changes_requested` (ưu tiên `changes_requested`).

1. Chạy `python3 scripts/handoff.py start <ID>`.
2. Đọc kỹ `handoff/tasks/<ID>-*/brief.md`. Nếu có `review.md` thì đây là vòng sửa — đọc nó trước và xử lý từng mục.
3. Đọc mọi tài liệu ở brief mục 2, và đọc `verify.sh` cùng các file test trong `tests.lock` để hiểu chính xác điều gì sẽ được kiểm.
4. Lập kế hoạch ngắn (file nào, thay đổi gì) bám theo brief mục 3. Không chờ duyệt — trưởng phòng đã duyệt qua brief.
5. Thực hiện thay đổi.
6. Tự kiểm: `python3 scripts/handoff.py verify <ID>` (kiểm khoá test và chạy `verify.sh`). Đỏ thì sửa code, chạy lại đến khi xanh. Không sửa test.
7. Viết `handoff/tasks/<ID>-*/report.md` theo mẫu `handoff/templates/report.md` (vòng sửa: thêm mục "Vòng sửa N" ở cuối, giữ nội dung cũ).
8. Chạy `python3 scripts/handoff.py submit <ID>`. Nếu lệnh báo lỗi, xử lý rồi chạy lại.
9. Kết thúc bằng một dòng: "<ID> đã nộp — chờ trưởng phòng nghiệm thu."

Nếu một thao tác bị hook từ chối vì chạm file test bị khoá: đó là chủ ý của trưởng phòng — không tìm cách khác để sửa file đó; ghi lý do vào mục "câu hỏi" của `report.md`.
