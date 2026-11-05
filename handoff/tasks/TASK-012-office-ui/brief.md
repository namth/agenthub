# TASK-012 — Giao diện SCR-08 Văn phòng ảo

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Ứng với TASK-16 (FEAT-02, DEC-07, DEC-08).

## 1. Mục tiêu
Văn phòng pixel-art sống trong dashboard: mỗi máy một dãy bàn, ghế Trưởng phòng theo lead hiện tại, phòng nghỉ cho agent COOLDOWN, máy OFFLINE tắt đèn — vẽ từ luồng SSE `/api/admin/office/stream`.

## 2. Tài liệu bắt buộc đọc
- `docs/features/virtual-office.md` (toàn bộ); `docs/03-screens-and-ui.md` §3
- Bản canvas cũ để chuyển sang: `web/src/office/legacy/index.html` + `README.md` (chỉ đọc, không import, không sửa)
- Hình dạng SSE: `internal/office/office_test.go` (đang làm song song ở nhánh khác — chỉ dùng làm tham chiếu định dạng)
- `web/src/office/office.test.tsx` — đặc tả chi tiết nhất

## 3. Phạm vi file
**Được tạo/sửa:** `web/src/office/model.ts` (logic thuần), `web/src/office/*.ts(x)` khác (vẽ canvas, sprite), `web/src/pages/Office.tsx`.

**Không được sửa:** file trong `tests.lock`; `web/src/office/legacy/`; `web/package.json`; các trang khác; mọi thứ ngoài `web/src`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**`model.ts` (thuần, không DOM):**
```ts
export type Snapshot = { lead: { state: string; agent_id: string | null; epoch: number }; workers: { id: string; conn_status: string; agents: { id: string; kind: string; status: string; cooldown_until?: string | null }[] }[]; recent: OfficeEvent[] };
export type OfficeEvent = { id: number; ts: string; who: string; type: string; to?: string; task?: string; text?: string };
export type Place = 'desk' | 'lead-seat' | 'break' | 'offline';
export type OfficeState = { lead: string | null; vacantNote?: string; dark: Set<string>; chars: Record<string, { worker: string; kind: string; place: Place; task?: string; note?: string }> };
export function computeLayout(workers: Snapshot['workers']): { width: number; height: number; zones: { director: Rect; leadSeat: Rect; breakRoom: Rect }; rows: { workerId: string; desks: (Rect & { agentId: string })[] }[] };
export function initialState(s: Snapshot): OfficeState;
export function applyEvent(s: OfficeState, e: OfficeEvent): OfficeState; // bất biến: trả state mới
```
- Bố cục: hàng trên cố định (Giám đốc, Ghế Trưởng phòng, Phòng nghỉ); dưới là lưới dãy bàn, sắp theo `worker.id`, mỗi dãy chứa bàn của các agent của máy; ≤ 8 máy × 2 agent không chồng lấn; nhiều hơn thì tăng `height`.
- `initialState`: lead HELD → agent đó `lead-seat`; agent COOLDOWN → `break`, `note` = giờ hết nghỉ `HH:MM`; máy OFFLINE → mọi agent `offline`, worker vào `dark`.
- `applyEvent` theo bảng mục 3 của `virtual-office.md`: `handover` (lead cũ về `desk`, `to` lên `lead-seat`), `vacant` (lead=null, `vacantNote` = text), `cooldown` (`break` + note = text), `idle` (về `desk`, hoặc `lead-seat` nếu là lead), `assign` (đặt `task` cho `to`), `submit` (xoá `task` của `who`), `offline`/`online` (`who` là worker), loại khác → trả state tương đương.

**`Office.tsx`:** mở `new EventSource('/api/admin/office/stream')` (cookie phiên tự gửi); lắng nghe `snapshot` (dựng state từ snapshot, không phát lại hoạt cảnh) và `office` (applyEvent); `onerror` → `ErrorBanner` "Mất kết nối — đang thử lại" (giữ cảnh), `onopen` → ẩn banner; unmount → `close()`. Trước snapshot: skeleton `aria-busy="true"`. Không có máy: "Chưa có máy nào kết nối" + link "Đăng ký máy" tới `/fleet`. Có dữ liệu: `<canvas data-testid="office-canvas">` vẽ pixel-art (chuyển từ bản cũ: nhân vật, bàn, hoạt cảnh đi lại; 12 fps khi có hoạt động, 4 fps khi yên, dừng khi tab ẩn; `getContext` có thể null trong test → không lỗi) **và** một lớp danh sách truy cập được: mỗi agent một phần tử `data-testid="char-<agentId>"` với `data-place` (có thể `sr-only`). Tooltip khi di chuột: agent_id, trạng thái, task.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-012-office-ui/verify.sh` exit 0 (typecheck, vitest, build)
- [ ] Không test nào bị skip; `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không gọi `fetch` trong Office (chỉ SSE); không hard-code nhân vật/máy cố định như bản cũ.
- `ls`/`cat`/`grep` không được phép qua run_command — dùng công cụ đọc/liệt kê file có sẵn. Lệnh được phép: `node`, `bash` (để chạy `npx`), `git`, `python3`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
