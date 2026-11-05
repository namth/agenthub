# TASK-034 — Web: huy hiệu đăng nhập SCR-05/SCR-11, lead mọi agent, dòng Telegram SCR-07

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-06: DEC-24, DEC-29, DEC-30; TC-67

## 1. Mục tiêu
Owner và chủ máy thấy ngay agent nào "Cần đăng nhập", biết lệnh cần chạy và bấm "Đã đăng nhập, kiểm lại"; owner bật vai trưởng phòng cho mọi loại agent; SCR-07 cho biết Telegram đang bật hay tắt.

## 2. Tài liệu bắt buộc đọc
- `docs/features/agent-auth.md` §4 (giao diện + 4 trạng thái của nút kiểm lại), §6 (trường `auth_status`, `auth_detail`, `gh_auth`; `POST /api/my/machines/{id}/auth-recheck`)
- Test đặc tả: `web/src/pages/agentauth.test.tsx`; code: `web/src/pages/Fleet.tsx`, `web/src/pages/MyMachines.tsx`, `web/src/pages/Settings.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** `web/src/pages/Fleet.tsx`, `web/src/pages/MyMachines.tsx`, `web/src/pages/Settings.tsx`, file mới `web/src/components/AuthBadge.tsx`.

**Không được sửa:** file trong `tests.lock` và mọi file test; `web/src/office/`, `web/src/pages/Office.tsx`; Go; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- **`AuthBadge`** (dùng chung SCR-05/SCR-11): `auth_status = 'NEEDS_AUTH'` → nhãn chữ đúng **"Cần đăng nhập"** (màu đỏ), `title` = `auth_detail` + lệnh gợi ý theo `kind`: `CLAUDE` → "Chạy `claude` rồi gõ /login", `ANTIGRAVITY` → "Mở `agy` và đăng nhập"; hiển thị luôn dòng lệnh gợi ý dưới nhãn. `'UNKNOWN'` hoặc thiếu → nhãn xám **"Chưa kiểm được"**. `'OK'` → không hiển thị gì.
- **Băng gh:** máy `gh_auth = 'NEEDS_AUTH'` → phần tử `role="status"` trong khung máy chứa "GitHub CLI chưa đăng nhập" và lệnh `gh auth login`.
- **Nút "Đã đăng nhập, kiểm lại"** (chỉ hiện cạnh agent `NEEDS_AUTH`, và ở băng gh với `agent_id` rỗng): `POST /api/my/machines/{worker_id}/auth-recheck` body `{ agent_id }` (băng gh: `{}`) — **cả SCR-05 và SCR-11 dùng đường này**. Đang gửi → nút `disabled` + chữ "Đang kiểm…"; `202` → thông báo "Đã gửi yêu cầu kiểm lại", sau 3 giây tải lại danh sách; `409` → `role="alert"` chứa "Máy đang offline"; lỗi khác → `role="alert"` chứa thông điệp lỗi.
- **SCR-05 (Fleet):** công tắc "Cho làm trưởng phòng" hiện cho **mọi** agent (bỏ điều kiện `kind === 'CLAUDE'`); nút **"Trao vai"** hiện khi: không phải lead hiện tại, `status === 'IDLE'`, `lead_eligible !== false`, `auth_status !== 'NEEDS_AUTH'`, máy `ONLINE`, máy `gh_auth !== 'NEEDS_AUTH'` — mọi loại agent. Ô ưu tiên lead trong drawer sửa agent nhận mọi loại agent.
- **SCR-07 (Settings):** dưới công tắc văn phòng, dòng chữ "Telegram: đang bật (cấu hình trong .env)" khi `telegram_enabled === true`, ngược lại "Telegram: đang tắt (cấu hình trong .env)". Không có ô nhập token/chat ID.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-034-web-agent-auth/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi test web cũ phải xanh (`screens.test.tsx`, `machines.test.tsx`, `detail.test.tsx`…). Dữ liệu cũ không có trường `auth_status`/`gh_auth`/`lead_eligible` phải hiển thị như trước (không thêm nhãn đỏ, nút Trao vai giữ nguyên quy tắc).
- Không dùng `dangerouslySetInnerHTML`. Không thêm thư viện.
- Lệnh được phép: `npm`, `npx`, `node`, `bash`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
