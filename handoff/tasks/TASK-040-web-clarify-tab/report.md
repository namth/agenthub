# Báo cáo TASK-040

## Đã làm
- Tạo component `ClarifyPanel` (`web/src/components/ClarifyPanel.tsx`) hiển thị `section[data-testid="clarify"]`:
  - Banner trạng thái `role="status"` chứa "Trưởng phòng cần bạn trả lời" khi task ở trạng thái `WAITING_USER`.
  - Hiển thị từng câu hỏi trong `fieldset` có `legend` và lý do, các lựa chọn radio (phương án trùng `recommended` được gắn thêm " (Đề xuất)"), radio "Khác" kèm ô nhập `aria-label="Khác: <câu hỏi>"`.
  - Tự động hiển thị ô nhập `aria-label="Khác: <câu hỏi>"` khi câu hỏi không có options.
  - Ô textarea `aria-label="Ghi chú thêm"` tối đa 1000 ký tự.
  - Nút "Gửi câu trả lời" (chỉ hiện khi `can_answer`): validate đủ câu trả lời (thiếu báo `role="alert"` "Hãy trả lời đủ các câu hỏi", không gọi API), hiển thị nút "Đang gửi…" (disabled), gửi POST `/api/tasks/{id}/clarify` với đúng body, hiển thị thông báo thành công `role="status"` "Đã gửi — trưởng phòng đang hoàn thiện kế hoạch" và tải lại task/clarifications; xử lý 409 báo "đã được trả lời" và tải lại; lỗi khác hiển thị alert và giữ nguyên nội dung đã nhập.
  - Nút "Cho trưởng phòng tự quyết" (chỉ hiện khi `can_auto` và có vòng WAITING): xác nhận bằng `window.confirm` và gọi `POST /api/tasks/{id}/clarify/auto`.
  - Hiển thị chỉ đọc (disable input, không hiện nút gửi) khi `can_answer = false`.
  - Hiển thị lịch sử các vòng `ANSWERED` / `AUTO_DECIDED` trong `data-testid="clarify-history"`.
  - Đảm bảo an toàn không dùng `dangerouslySetInnerHTML`. Thiết kế nút bấm đạt chuẩn chiều cao tối thiểu `min-h-11` trên di động.
- Tích hợp `ClarifyPanel` vào `web/src/pages/TaskDetail.tsx`: gọi `GET /api/tasks/{id}/clarifications` khi tải task (trừ khi task ở `PENDING_APPROVAL` chưa duyệt), cập nhật `STATUS_LABELS` và `STATUS_COLORS` cho `WAITING_USER`.
- Cập nhật `web/src/pages/Kanban.tsx`: mở rộng interface `Task` với trường `clarify_pending`, hiển thị nhãn cam `Cần làm rõ (<clarify_pending> câu)` cho thẻ có trạng thái `WAITING_USER`.

## File đã tạo/sửa
- `web/src/components/ClarifyPanel.tsx`: Component hiển thị phần làm rõ câu hỏi, submit trả lời, tự quyết và lịch sử làm rõ.
- `web/src/pages/TaskDetail.tsx`: Tích hợp ClarifyPanel, tải dữ liệu clarifications và bổ sung nhãn cho trạng thái WAITING_USER.
- `web/src/pages/Kanban.tsx`: Thêm trường `clarify_pending` vào Task interface và hiển thị nhãn cam "Cần làm rõ (n câu)".

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-040-web-clarify-tab/verify.sh
 Test Files  13 passed (13)
      Tests  139 passed (139)
TASK-040 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
