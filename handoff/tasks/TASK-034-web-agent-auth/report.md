# Báo cáo TASK-034

## Đã làm
- Tạo component `AuthBadge`: hiển thị nhãn "Cần đăng nhập" (đỏ) kèm dòng lệnh gợi ý theo kind (`CLAUDE` → `/login`, `ANTIGRAVITY` → `agy`) và tooltip `auth_detail` + gợi ý; nhãn xám "Chưa kiểm được" khi `'UNKNOWN'` hoặc thiếu; ẩn khi `'OK'`.
- Bổ sung băng GitHub CLI (`role="status"`) khi máy có `gh_auth = 'NEEDS_AUTH'` kèm gợi ý `gh auth login` và nút "Đã đăng nhập, kiểm lại" (gửi body `{}`) trên cả SCR-05 (Fleet) và SCR-11 (MyMachines).
- Thêm nút "Đã đăng nhập, kiểm lại" cho agent `NEEDS_AUTH` (gửi body `{ agent_id }`), xử lý đủ 4 trạng thái: đang gửi khoá nút và hiện "Đang kiểm…", 202 báo "Đã gửi yêu cầu kiểm lại" và hẹn giờ tải lại sau 3s, 409 role="alert" báo "Máy đang offline", các lỗi khác hiển thị role="alert" thông điệp lỗi.
- SCR-05 (Fleet): hỗ trợ công tắc "Cho làm trưởng phòng" cho mọi loại agent; nút "Trao vai" xét cho mọi loại agent thoả mãn các điều kiện (không phải lead hiện tại, IDLE, `lead_eligible !== false`, `auth_status !== 'NEEDS_AUTH'`, máy ONLINE và `gh_auth !== 'NEEDS_AUTH'`).
- SCR-07 (Settings): hiển thị dòng trạng thái Telegram chỉ đọc dưới công tắc văn phòng ảo: "Telegram: đang bật (cấu hình trong .env)" hoặc "Telegram: đang tắt (cấu hình trong .env)".
- Đảm bảo tương thích ngược dữ liệu cũ không có trường auth/gh_auth; 100% test (10 file test, 112 tests) đều pass, typecheck và build đạt, không dùng `dangerouslySetInnerHTML`.

## File đã tạo/sửa
- `web/src/components/AuthBadge.tsx`: component huy hiệu đăng nhập dùng chung cho SCR-05 và SCR-11.
- `web/src/pages/Fleet.tsx`: hiển thị AuthBadge, băng gh, nút kiểm lại, mở rộng công tắc lead & nút trao vai cho mọi agent.
- `web/src/pages/MyMachines.tsx`: hiển thị AuthBadge, băng gh và nút kiểm lại cho máy cá nhân.
- `web/src/pages/Settings.tsx`: hiển thị dòng trạng thái Telegram cấu hình qua .env.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-034-web-agent-auth/verify.sh
 Test Files  10 passed (10)
      Tests  112 passed (112)
TASK-034 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
