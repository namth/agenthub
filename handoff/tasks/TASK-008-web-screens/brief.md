# TASK-008 — Giao diện SCR-02 Kanban, SCR-03 Tạo yêu cầu, SCR-05 Fleet

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Ứng với TASK-06 (Kanban + form) và TASK-13 (SCR-05) trong `docs/11-tasks.md`.

## 1. Mục tiêu
Ba màn hình đầu tiên chạy trên API thật: bảng Kanban 5 cột (Admin duyệt được), form tạo yêu cầu, màn hình quản trị cụm máy với thẻ Trưởng phòng và nút Trao/Thu hồi vai.

## 2. Tài liệu bắt buộc đọc
- `docs/03-screens-and-ui.md` §1, §3 (4 trạng thái UI), §4 (validate); `docs/features/lead-rotation.md` §4 (bố cục SCR-05)
- `docs/14-decisions.md` DEC-11 (cột Kanban); `docs/07-api-contracts.md` §1 (định dạng response)
- Hình dạng dữ liệu API: `internal/api/fleet_test.go`, `internal/tasks/tasks_test.go`
- `web/src/pages/screens.test.tsx` — đặc tả chi tiết nhất; `web/src/components/StateView.tsx`, `web/src/nav.ts`

## 3. Phạm vi file
**Được tạo/sửa:** `web/src/pages/Kanban.tsx`, `TaskCreate.tsx`, `Fleet.tsx`; file mới trong `web/src/components/`, `web/src/hooks/`, `web/src/api/`; `web/src/App.tsx` (truyền `role` của user cho Kanban).

**Không được sửa:** file trong `tests.lock`; `web/src/nav.ts`, `web/package.json` (không thêm thư viện); mọi thứ ngoài `web/src`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- Gọi API bằng `fetch` đường tương đối (`/api/...`), `credentials: 'include'`; đọc `{success, data, error}`; lỗi lấy `error.code`. Gom vào một helper `web/src/api/client.ts`.
- 4 trạng thái mỗi màn hình (dùng `StateView`): tải → phần tử `aria-busy="true"`; lỗi → `ErrorBanner` (`role="alert"`, nút "Thử lại" gọi lại API); rỗng → thông điệp đúng chữ trong test; thành công.
- **Fleet (`export default function Fleet()`):** `GET /api/fleet`. Thẻ Lead `data-testid="lead-card"`: HELD → `agent_id`, chữ `epoch <n>`, nút "Thu hồi vai" (hỏi `window.confirm`, đồng ý mới `POST /api/admin/lead/revoke`); VACANT → "Chưa có trưởng phòng", giờ dự kiến (`eta`), số việc chờ (`waiting_jobs`), không có nút thu hồi. Mỗi máy `data-testid="worker-<id>"` hiện `conn_status`; mỗi agent `data-testid="agent-<id>"` hiện `status`, `current_job.task_id` nếu có, giờ hết nghỉ nếu COOLDOWN. Nút "Trao vai" chỉ ở agent `CLAUDE`, `IDLE`, máy `ONLINE`, không phải lead → `confirm` → `POST /api/admin/lead/assign {agent_id}`. Sau thao tác thành công tải lại `/api/fleet`; lỗi `AGENT_NOT_ELIGIBLE` → thông báo "Agent không còn đủ điều kiện nhận vai (đang bận/nghỉ/mất kết nối)". Rỗng: "Chưa có máy nào kết nối" + nút "Thêm máy mới". LED màu theo `--color-led-*` trong `index.css`.
- **Kanban (`export default function Kanban({ role }: { role: 'ADMIN' | 'PARTNER' })`):** `GET /api/tasks`; mỗi cột `data-testid="column-<key>"` với `title`; thẻ hiện `title`. Thẻ ở cột `backlog` có nút "Duyệt" khi `role === 'ADMIN'` → `POST /api/tasks/<id>/approve` rồi tải lại. Mọi cột rỗng → "Chưa có tác vụ nào trong hàng đợi" + link "Tạo yêu cầu đầu tiên" tới `/tasks/new`.
- **TaskCreate:** hai trường có label "Tiêu đề", "Mô tả chi tiết"; nút "Gửi yêu cầu". Validate trước khi gửi (trim tiêu đề; 6–120 ký tự → "Tiêu đề tối thiểu 6 ký tự" / "Tiêu đề tối đa 120 ký tự"; mô tả "Mô tả tối thiểu 20 ký tự"). Gửi `POST /api/tasks {title (đã trim), description}` → thành công điều hướng `/tasks/<id>`; lỗi → `ErrorBanner`, giữ nguyên dữ liệu đã nhập.
- Tailwind cho giao diện, gọn, tiếng Việt.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-008-web-screens/verify.sh` exit 0 (typecheck, vitest, build)
- [ ] Không test nào bị skip; `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không hard-code dữ liệu mẫu trong component; không thêm nhánh riêng cho test.
- `ls`/`cat`/`grep` không được phép qua run_command — dùng công cụ đọc/liệt kê file có sẵn. Lệnh được phép: `node`, `npm`/`npx` qua `bash -c`, `bash`, `git`, `python3`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
