# TASK-022 — UI đa dự án: ô chọn dự án, SCR-09 Dự án, Kanban / Tạo yêu cầu theo dự án

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-04: DEC-14…17, BR-24…27

## 1. Mục tiêu
Người dùng chọn dự án ở sidebar; Kanban và Tạo yêu cầu làm việc theo dự án đó. Owner có "Tất cả dự án" và trang SCR-09 để thêm repo, chỉnh trọng số, lưu trữ dự án, tạo lại webhook.

## 2. Tài liệu bắt buộc đọc
- `docs/features/multi-project.md` §2, §4, §6; `docs/03` (SCR-02, SCR-03, SCR-09, 4 trạng thái UI)
- Test đặc tả (chi tiết nhất): `web/src/pages/projects.test.tsx`; test cũ phải giữ xanh: `web/src/pages/screens.test.tsx` (Kanban, TaskCreate không chọn dự án)
- `web/src/api/client.ts`, `web/src/components/StateView.tsx`, `web/src/hooks/useAuth.ts`, `web/src/layout/AppShell.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** `web/src/project.ts` (mới), `web/src/components/ProjectSwitcher.tsx` (mới), `web/src/layout/AppShell.tsx`, `web/src/pages/Kanban.tsx`, `web/src/pages/TaskCreate.tsx`, `web/src/pages/Projects.tsx` (đang là trang khung — viết lại).

**Không được sửa:** file trong `tests.lock`; `web/src/api/client.ts`, `web/src/nav.ts`, `web/src/App.tsx`, `web/package.json`; `Fleet.tsx`, `MyMachines.tsx` (TASK-024 đang làm song song); mọi file Go; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**`web/src/project.ts`** — `export const PROJECT_KEY = 'agenthub.project'`, `export const ALL_PROJECTS = '*'`; `getSelectedProject(): string | null` (đọc localStorage, bọc try/catch); `setSelectedProject(id: string)` ghi localStorage rồi `window.dispatchEvent(new Event('agenthub:project'))`; hook `useSelectedProject()` trả giá trị hiện tại và cập nhật khi có sự kiện `agenthub:project` (và `storage`). Kiểu `Project` = phần tử `GET /api/projects` (`id, name, github_repo, default_branch, status, role, weight, max_parallel, webhook_ok, open_tasks`). Có thể thêm hook `useProjects()`.

**`ProjectSwitcher`** (`export default function ProjectSwitcher({ isOwner }: { isOwner: boolean })`) — gọi `GET /api/projects`; `<select aria-label="Dự án">`, option value = `id`, nhãn = `github_repo`; owner có option đầu `value="*"` "Tất cả dự án". Lựa chọn đã lưu không hợp lệ (không có trong danh sách, hoặc `*` với người không phải owner) hoặc chưa có → mặc định (owner `*`, người khác dự án đầu tiên) và **ghi lại** bằng `setSelectedProject`. Danh sách rỗng → chữ "Chưa có dự án". Đổi → `setSelectedProject`.

**AppShell** — đặt `ProjectSwitcher` trong `<aside>`, `isOwner = user.role === 'ADMIN'`.

**Kanban** — theo `useSelectedProject()`: id cụ thể → `GET /api/tasks?project_id=<id>`; `*` hoặc `null` → `GET /api/tasks` (không tham số). Đổi dự án → tải lại. Tải thêm `GET /api/projects` (lỗi thì bỏ qua, không hiện banner) để: (a) khi xem `*` ghi `github_repo` của `task.project_id` trên thẻ; (b) nút Duyệt hiện khi `role === 'ADMIN'` **hoặc** vai của dự án đang chọn là `ADMIN`; (c) dự án đang chọn `ARCHIVED` → ghi chú "Dự án đã lưu trữ — chỉ xem" và không nút Duyệt.

**TaskCreate** — chỉ khi `getSelectedProject()` khác `null` mới gọi `GET /api/projects` (giữ test cũ: không chọn dự án thì không gọi API nào trước khi gửi, body đúng `{title, description}`). Id cụ thể → hiện "Dự án: <github_repo>" và gửi `{project_id, title, description}`. `*` → hiện `<select aria-label="Dự án của yêu cầu">` (option rỗng "— Chọn dự án —" + dự án ACTIVE); gửi khi chưa chọn → lỗi "Chọn dự án cho yêu cầu", không gọi API.

**SCR-09 `Projects`** (`export default function Projects()`) — tiêu đề "Dự án". Gọi song song `GET /api/projects` và `GET /api/admin/github/repos`.
- Bảng dự án: mỗi dòng `data-testid="project-<id>"`: `github_repo`, `default_branch`, webhook ✓ hoặc ⚠ + nút "Tạo lại webhook" (`POST /api/admin/projects/<id>/webhook`, xong tải lại), `open_tasks`, ô số `aria-label="Trọng số"` (1–5, sai → "Trọng số 1–5"), ô số `aria-label="Song song tối đa"` (trống = `null`, ngoài 1–50 → "Song song tối đa 1–50 hoặc để trống"), nút "Lưu" → `PATCH /api/admin/projects/<id>` body `{weight, max_parallel}`; nút "Lưu trữ" (`confirm`) → `PATCH {status:'ARCHIVED'}`; dự án ARCHIVED hiện "Đã lưu trữ" và nút "Khôi phục" → `PATCH {status:'ACTIVE'}`. Sau PATCH thành công tải lại.
- Khối `data-testid="repos-available"` "Repo chưa thêm": repo `added=false`, mỗi repo nút "Thêm" → `POST /api/admin/projects {github_repo}`; kết quả `webhook_ok=false` → thông báo (`role="status"`) có chữ "webhook" (vd "Đã thêm, nhưng chưa tạo được webhook — bấm Tạo lại webhook"); lỗi (409…) → `ErrorBanner` với message server; xong tải lại.
- 4 trạng thái: tải (`Loading`), lỗi tải dự án (`ErrorBanner` + Thử lại), rỗng ("Chưa có dự án" + vẫn hiện repo chưa thêm), thành công. Lỗi riêng repo (502 `GITHUB_ERROR`) → `ErrorBanner` nêu PAT cần quyền (vd "Không đọc được repo từ GitHub — kiểm tra PAT: cần Metadata Read, Contents/Issues/Pull requests RW, Webhooks RW"), bảng dự án vẫn hiện.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-022-web-projects/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không dùng thư viện mới. Tailwind như các trang sẵn có. Không `any` bừa bãi (tsc -b phải sạch).
- localStorage có thể ném lỗi → bọc try/catch.
- Lệnh được phép: `bash`, `node`, `npx` (qua `bash`), `go`, `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
