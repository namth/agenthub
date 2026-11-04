# TASK-024 — UI thêm máy: hộp thoại Thêm máy, SCR-11 Máy của tôi, SCR-05 nhóm theo chủ

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-05: DEC-18…20, BR-30…34; FEAT-04 §4 (agent chọn dự án)

## 1. Mục tiêu
Ai cũng thêm được máy của mình bằng một lệnh (hoặc dán hướng dẫn cho agent local), quản lý máy của mình; owner thấy cụm máy nhóm theo chủ, phiên bản, dự án/việc đang chạy, bật/tắt quyền làm trưởng phòng và chọn dự án cho từng agent.

## 2. Tài liệu bắt buộc đọc
- `docs/features/join-machine.md` §2, §4, §6; `docs/features/multi-project.md` §4, §6 (`PATCH /api/admin/agents/{id}`)
- Test đặc tả (chi tiết nhất): `web/src/pages/machines.test.tsx`; test cũ phải giữ xanh: `web/src/pages/screens.test.tsx` (SCR-05)
- `web/src/api/client.ts`, `web/src/components/StateView.tsx`, `web/src/pages/Fleet.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** `web/src/components/AddMachineDialog.tsx` (mới), `web/src/pages/MyMachines.tsx` (đang là trang khung — viết lại), `web/src/pages/Fleet.tsx`, `web/src/api/client.ts` (chỉ thêm `api.del` và xử lý 204).

**Không được sửa:** file trong `tests.lock`; `web/src/nav.ts`, `web/src/App.tsx`, `web/package.json`; `Kanban.tsx`, `TaskCreate.tsx`, `Projects.tsx`, `layout/`, `web/src/project.ts` (TASK-022 làm song song — **không import** từ đó); mọi file Go; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**`api/client.ts`** — `request` trả `undefined` khi `res.status === 204` (không parse JSON); thêm `api.del<T>(url)` (method DELETE).

**`AddMachineDialog`** (`export default function AddMachineDialog({ open, onClose }: { open: boolean; onClose: () => void })`) — `open=false` → không render gì. Hộp thoại (`role="dialog"`) có nút "Đóng" (gọi `onClose`).
- Bước 1: ô `<input>` nhãn "Tên máy" (`<label>`), nút "Tạo mã". Tên phải khớp `^[a-zA-Z0-9-]{2,40}$`, sai → "Tên máy 2–40 ký tự: chữ không dấu, số, gạch ngang", không gọi API.
- `POST /api/fleet/join-codes {name}` → `{code, expires_at, command, agent_prompt}`. Lỗi → `ErrorBanner` với message server.
- Bước 2: (1) "Lệnh cài một dòng" trong `<pre data-testid="join-command">` + nút "Sao chép lệnh"; (2) "Hướng dẫn cho agent (Claude Code / Antigravity)" trong `<pre data-testid="join-prompt">` + nút "Sao chép hướng dẫn"; sao chép bằng `navigator.clipboard.writeText`. Đồng hồ "Còn mm:ss" đến `expires_at` (cập nhật mỗi giây). Hết hạn → "Mã đã hết hạn" + nút "Tạo mã mới" (gọi lại POST với cùng tên), ngừng thăm dò.
- Thăm dò: ngay sau khi có mã và sau đó mỗi 3 giây gọi `GET /api/my/machines`; khi có máy `name` trùng tên đã nhập và `conn_status === 'ONLINE'` → "Đã kết nối ✓" và **ngừng thăm dò**; trước đó hiện "Đang chờ máy kết nối…". Dọn interval khi đóng/unmount.

**SCR-11 `MyMachines`** (`export default function MyMachines()`) — tiêu đề "Máy của tôi", nút "Thêm máy mới" mở `AddMachineDialog` (đóng hộp thoại → tải lại danh sách). `GET /api/my/machines` (mảng `{id, name, os, conn_status, version, paused, agents:[{id, kind, status, projects: string[], current_job}]}`) và `GET /api/projects` (lỗi thì bỏ qua). Mỗi máy `data-testid="machine-<id>"`: tên, `conn_status`, phiên bản, "Tạm dừng" nếu paused, từng agent (kind, status, `task_id` của `current_job` nếu có). Nút "Tạm dừng" (khi chưa paused) → `PATCH /api/my/machines/<id> {paused:true}`; "Tiếp tục" → `{paused:false}`; "Gỡ" (`confirm`) → `DELETE /api/my/machines/<id>` (204). `<select aria-label="Dự án nhận việc">`: option `""` "Mọi dự án" + mỗi dự án (nhãn `github_repo`); giá trị hiện tại = dự án chung của các agent nếu đúng 1 (agent `projects` rỗng → `""`); đổi → `PATCH {projects: [id]}` hoặc `{projects: null}` với `""`. Thao tác thành công → tải lại; lỗi → `ErrorBanner` (message server). 4 trạng thái: tải (`Loading`), lỗi (`ErrorBanner` + Thử lại), rỗng "Chưa có máy nào" + nút Thêm máy mới, có dữ liệu.

**SCR-05 `Fleet`** — giữ hành vi cũ (thẻ lead, Trao vai, Thu hồi vai, `data-testid` cũ). Thêm:
- Dữ liệu mới của `GET /api/fleet`: worker `owner: {id, login} | null`, `version`, `paused`; agent `lead_eligible`, `projects: string[] | null`; `current_job` thêm `project_id`, `project`, `task_title`. Thiếu trường (dữ liệu cũ) → coi như `owner=null`, `lead_eligible=true`, `projects=null`.
- Nhóm worker theo chủ: khối `data-testid="owner-group-<owner.id>"` tiêu đề `owner.login`; `owner=null` → `data-testid="owner-group-system"` tiêu đề "Hệ thống" (đặt đầu).
- Mỗi worker hiện phiên bản và nhãn "Tạm dừng" khi paused; agent có `current_job` hiện `project` và `task_title` (giữ hiện `task_id`).
- Agent `CLAUDE`: checkbox `aria-label="Cho làm trưởng phòng"` (checked = `lead_eligible`) → `PATCH /api/admin/agents/<encodeURIComponent(id)> {lead_eligible: <giá trị mới>}` rồi tải lại. Agent khác không có công tắc.
- Mỗi agent: `<select aria-label="Dự án được nhận">` (`""` "Mọi dự án" + dự án từ `GET /api/projects`, lỗi thì bỏ qua); giá trị = phần tử đầu của `projects` hoặc `""`; đổi → `PATCH /api/admin/agents/<id> {projects:[id]}` hoặc `{projects:null}`.
- Nút "Thêm máy mới" (luôn **đúng một** nút trên trang, cả lúc rỗng) mở `AddMachineDialog`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-024-web-machines/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không dùng thư viện mới; Tailwind như trang sẵn có; `tsc -b` sạch.
- Không in mã tham gia ra `console`.
- Lệnh được phép: `bash`, `node`, `npx` (qua `bash`), `go`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
