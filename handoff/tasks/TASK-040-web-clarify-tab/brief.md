# TASK-040 — Web: phần "Làm rõ" ở SCR-04 và nhãn Kanban

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-07: DEC-33, BR-53, BR-54 · TC-82
> API (TASK-039) chưa có: test dùng fetch giả theo đúng hợp đồng trong đặc tả.

## 1. Mục tiêu
Người ra đề / Admin trả lời câu hỏi làm rõ của trưởng phòng AI ngay trên trang chi tiết task; Admin có nút "Cho trưởng phòng tự quyết"; Kanban hiện nhãn "Cần làm rõ (n câu)".

## 2. Tài liệu bắt buộc đọc
- `docs/features/clarification-and-shared-skills.md` §2, §4, §6 (`GET …/clarifications` trả `{rounds, can_answer, can_auto}`; `POST …/clarify`; `POST …/clarify/auto`)
- Test đặc tả: `web/src/pages/clarify.test.tsx`; code: `web/src/pages/TaskDetail.tsx`, `web/src/pages/Kanban.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** `web/src/pages/TaskDetail.tsx`, `web/src/pages/Kanban.tsx`, file mới `web/src/components/ClarifyPanel.tsx`.
**Không được sửa:** file trong `tests.lock` và mọi file test; trang khác; Go; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- `TaskDetail` gọi `GET /api/tasks/{id}/clarifications` khi task ở `WAITING_USER` **hoặc** sau khi tải task (để hiện lịch sử); `404`/rỗng → không hiện phần làm rõ.
- Phần `section[data-testid="clarify"]`:
  - Task `WAITING_USER` → băng `role="status"` chứa "Trưởng phòng cần bạn trả lời".
  - Vòng `WAITING` mới nhất: mỗi câu một `fieldset` có `legend` = câu hỏi, đoạn lý do; mỗi phương án là radio (nhãn = phương án, phương án bằng `recommended` thêm " (Đề xuất)"), thêm radio "Khác" kèm ô nhập `aria-label="Khác: <câu hỏi>"` (hiện khi chọn Khác hoặc câu hỏi không có phương án).
  - Ô `textarea` `aria-label="Ghi chú thêm"` (≤ 1000 ký tự, tuỳ chọn).
  - Nút **Gửi câu trả lời** (chỉ khi `can_answer`): thiếu câu nào → `role="alert"` "Hãy trả lời đủ các câu hỏi", không gọi API. Body: `{ "answers": { <id>: <phương án | chữ ở ô Khác>, "_notes": <ghi chú nếu có> } }`. Đang gửi → nút `disabled`, chữ "Đang gửi…". `200` → `role="status"` "Đã gửi — trưởng phòng đang hoàn thiện kế hoạch", tải lại task + clarifications. `409` → `role="alert"` chứa "đã được trả lời", tải lại. Lỗi khác → `role="alert"` + nội dung đã nhập giữ nguyên.
  - Nút **Cho trưởng phòng tự quyết** (chỉ khi `can_auto` và vòng đang `WAITING`) → `confirm` → `POST …/clarify/auto`.
  - `can_answer = false` → hiển thị chỉ đọc (không radio có thể bấm, không nút gửi).
  - Vòng `ANSWERED`/`AUTO_DECIDED` → danh sách lịch sử `data-testid="clarify-history"`: câu hỏi → câu trả lời, người trả lời, kênh.
- **Kanban:** thẻ có `status === 'WAITING_USER'` hiển thị nhãn cam "Cần làm rõ (<clarify_pending> câu)".

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-040-web-clarify-tab/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi test web cũ phải xanh; không đổi `data-testid`/chữ đang có. Không `dangerouslySetInnerHTML` (câu hỏi do AI sinh). Dùng được ở 375px (nút `min-h-11`).
- Lệnh được phép: `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`, `git`, `bash`, `node`, `python3` — mỗi lần một lệnh; không `test`, `cp`, `npm`, `npx`; chạy kiểm tra bằng `bash handoff/tasks/TASK-040-web-clarify-tab/verify.sh`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
