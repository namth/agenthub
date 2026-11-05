# TASK-035 — Office UI bản 2: hai cụm bàn, khu chỉ huy, nội thất, logo INOVA

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-02 bản 2: DEC-25 → DEC-28; BR-43 → BR-46; TC-33 (sửa), TC-34, TC-71 → TC-73

## 1. Mục tiêu
Vẽ lại SCR-08 thành **một văn phòng hoàn chỉnh, liên thông**: tường, vách/cửa kính, sàn gỗ, bảng tin, cây xanh, đồng hồ; khu làm việc ≥ 8 bàn chia hai cụm, lối đi giữa; bàn trưởng phòng phía trên; khu chỉ huy bên phải (mỗi tài khoản một bàn, xếp dọc); phòng nghỉ + sảnh ở đáy; logo INOVA.

## 2. Tài liệu bắt buộc đọc
- `docs/features/virtual-office.md` **§4.1 (sơ đồ bố cục — làm đúng sơ đồ này)**, §4.2, §6 (snapshot bản 2), §7 BR-43 → BR-46
- **Phong cách vẽ bắt buộc bám theo** `web/src/office/legacy/index.html` (hàm vẽ sàn gỗ, tường, vách kính, bảng công việc, cây `plant`, đồng hồ, bàn, ghế, nhân vật pixel-art) — chép/chuyển các hàm vẽ đó sang `renderer.ts`.
- Ảnh: `web/public/brand/inova-logo.png` (900×356, nền trong suốt, chữ sáng → đặt trên ô tường **tối màu**), `web/public/brand/inova-icon.png` (411×417).
- Test đặc tả: `web/src/office/office_v2.test.tsx`, `web/src/office/office.test.tsx` (đã sửa), `web/src/nav.test.ts`.

## 3. Phạm vi file
**Được tạo/sửa:** `web/src/office/model.ts`, `web/src/office/renderer.ts`, `web/src/pages/Office.tsx`, `web/src/nav.ts`, `web/src/App.tsx` (chỉ phần đọc trạng thái văn phòng), file mới trong `web/src/office/` (không phải test).

**Không được sửa:** file trong `tests.lock` và mọi file test; `web/src/office/legacy/`; `web/public/`; trang khác; Go; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**`model.ts`** — kiểu `Snapshot` thêm `users?: {login, avatar_url, is_owner, online}[]`, worker `gh_auth?`, agent `auth_status?`. `Place` thêm `'auth'`. `OfficeState` thêm `ghNeeds: Set<string>`, `users: Record<login, {online: boolean; isOwner: boolean}>`.
```ts
export type Seat = Rect & { index: number; cluster: 'left' | 'right'; row: number; col: 0 | 1; agentId: string | null; workerId: string | null };
export type UserDesk = Rect & { login: string; isOwner: boolean; online: boolean };
export function computeLayout(workers: Snapshot['workers'], users: NonNullable<Snapshot['users']>): {
  width: number; height: number;
  zones: { backWall: Rect; logo: Rect; board: Rect; leadDesk: Rect; work: Rect; aisle: Rect; command: Rect; commandDoor: Rect; breakRoom: Rect; lobby: Rect; lobbyRug: Rect };
  seats: Seat[]; machineLabels: { workerId: string; x: number; y: number }[]; userDesks: UserDesk[];
}
```
- Xếp bàn đúng **BR-43** (sắp tên không phân biệt hoa thường; trái-1 → phải-1 → trái-2 …; máy n agent chiếm `ceil(n/2)` hàng liền trong một cụm; mỗi cụm ≥ 2 hàng; agent `CLAUDE` trước `ANTIGRAVITY`). Chiều cao canvas không đổi khi ≤ 4 máy hai agent và ≤ số người vừa khu chỉ huy; tăng khi cần thêm hàng hoặc thêm bàn người dùng (giữ đáy phòng nghỉ/sảnh dưới cùng).
- `userDesks` giữ **đúng thứ tự** mảng `users` từ server, cùng `x`, `y` tăng dần, nằm trong `zones.command`.
- `initialState`/`applyEvent`: thứ tự ưu tiên vị trí `offline` > `lead-seat` > `break` > `auth` (agent `NEEDS_AUTH`) > `desk`; sự kiện `auth` (`who` = agent → đổi `auth`/`desk`; `who` = máy → `ghNeeds`), `presence` (`who` = `user:<login>`, text `online`/`offline`). Giữ nguyên hành vi cũ cho các sự kiện khác (test cũ phải xanh).

**`Office.tsx`** — mở `EventSource('/api/office/stream')`. Lớp DOM phủ canvas (toạ độ theo hệ logic, co giãn cùng canvas):
- mỗi ghế: `data-testid="seat-<index>"`, `data-agent`, `data-empty="true"` khi trống;
- nhân vật agent `char-<agentId>` (`data-place`), `NEEDS_AUTH` hiện "🔑"; biển máy `machine-label-<workerId>` (thêm "🔑" khi `ghNeeds`);
- bàn người dùng `user-<login>` (`data-online`, hiện login);
- `<img alt="INOVA" src="/brand/inova-logo.png">` trên ô tường; `onError` → thay bằng `<span data-testid="logo-fallback">INOVA</span>` (màu `#c8102e`, không xê dịch bố cục); icon `<img data-testid="inova-icon-door|inova-icon-rug" src="/brand/inova-icon.png">`;
- thanh dưới `data-testid="office-status"`: "Lead …", "Đang chạy N", "Nghỉ N", "Cần đăng nhập N", "Online x/y";
- trạng thái rỗng: "Chưa có máy nào kết nối" + link "Thêm máy" → `/my/machines`.
- `renderer.ts` vẽ nền tĩnh vào canvas phụ, chỉ vẽ lại khi bố cục đổi; nhân vật đi theo lối đi (không xuyên tường); 12/4 fps, dừng khi tab ẩn (giữ từ bản 1). jsdom không có canvas 2D → không được ném lỗi khi `getContext` trả `null`.

**`nav.ts` / `App.tsx`** — SCR-08 `roles: ['ADMIN','PARTNER']`; `App` đọc trạng thái bật/tắt qua `GET /api/office/status` → `{enabled}` cho **mọi** vai (thay cho `/api/admin/settings`).

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-035-office-ui-v2/verify.sh` exit 0
- [ ] `report.md` dán nguyên văn output verify; ghi rõ đã chuyển những hàm vẽ nào từ `legacy/index.html`

## 6. Ràng buộc & bẫy cần tránh
- **Chỉ dùng lệnh được phép:** `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`, `git`, `bash`, `node`, `python3`. **Không** gọi `test`, `npm`, `npx` trực tiếp — chạy test/typecheck/build bằng `bash handoff/tasks/TASK-035-office-ui-v2/verify.sh`. `node_modules` đã cài sẵn.
- Không dùng `dangerouslySetInnerHTML`; không thêm thư viện; không sửa ảnh logo.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
