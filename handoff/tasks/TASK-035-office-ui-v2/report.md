# Báo cáo TASK-035: Office UI bản 2: hai cụm bàn, khu chỉ huy, nội thất, logo INOVA

## Đã làm
- **`model.ts`**:
  - Bổ sung các kiểu: `Snapshot` mở rộng (`users`, `gh_auth`, `auth_status`), `Seat`, `UserDesk`, `Zones`, `Place` (thêm `'auth'`), `OfficeState` (thêm `ghNeeds`, `users`).
  - Hàm `computeLayout` bản 2: sắp xếp bàn theo BR-43 (tên máy không phân biệt hoa thường, CLAUDE trước ANTIGRAVITY, mỗi máy chiếm `ceil(n/2)` hàng liên tiếp trong cụm, tối thiểu 8 bàn chia 2 cụm trái/phải với lối đi ở giữa, bàn trưởng phòng ở trên sát tường, phòng nghỉ và sảnh ở đáy); khu chỉ huy bên phải xếp dọc theo đúng thứ tự mảng `users` từ server; chiều cao tự động giãn ra khi vượt quá sức chứa cơ sở.
  - `initialState` / `applyEvent`: thứ tự ưu tiên vị trí `offline` > `lead-seat` > `break` > `auth` (khi agent `NEEDS_AUTH`) > `desk`. Xử lý các sự kiện `auth` (agent đổi `auth`/`desk`; máy đổi `ghNeeds`), `presence` (người dùng `online`/`offline`), `handover` (cho cả Claude và Antigravity).
- **`renderer.ts`**:
  - Chuyển và tích hợp các hàm vẽ phong cách pixel-art từ `web/src/office/legacy/index.html`:
    1. Sàn gỗ khu làm việc (`drawWoodFloor`): thanh sàn đan xen hai tông màu `#c79f77` / `#cba47c` cùng chỉ sàn `#b38b63`.
    2. Đồng hồ treo tường (`drawClock`): đồng hồ tròn pixel với kim giờ/phút chuyển động theo thời gian thực.
    3. Bảng công việc trên tường (`drawBoard`): bảng tin viền xám, 4 dải màu trạng thái nhiệm vụ.
    4. Cây xanh (`drawPlant`): chậu cây cảnh pixel với tán lá 3 sắc xanh và chậu đất nung.
    5. Vách kính & cửa thông (`drawGlassPartition`): kính trong suốt ngăn khu làm việc với khu chỉ huy, chừa lối đi qua cửa kính.
    6. Quầy cà phê & máy pha trong phòng nghỉ: quầy gỗ, máy pha cà phê pixel, đèn báo đỏ và ly cà phê.
    7. Thảm sảnh & cửa ra vào sảnh đón.
    8. Bàn ghế làm việc & bàn trưởng phòng gắn sao vàng ★; bàn làm việc trong khu chỉ huy.
    9. Màn hình máy tính động: chạy code khi đang làm việc, tắt khi offline, hiện ổ khoá khi cần đăng nhập.
    10. Nhân vật pixel-art (`drawSprite`): chuyển động chân so le khi bước đi, tư thế gõ phím, trang phục theo vai (Claude cam, Antigravity xanh, Owner đỏ INOVA `#c8102e`, Partner xám), bong bóng trạng thái / mã nhiệm vụ.
    11. Thuật toán định tuyến (`routeTo`): di chuyển qua lối đi trung tâm và cửa kính, không đi xuyên tường.
    12. Bộ đệm nền tĩnh (background caching): vẽ nền vào canvas phụ offscreen và tái sử dụng, chỉ vẽ lại khi bố cục thay đổi.
- **`Office.tsx`**:
  - Kết nối SSE qua `EventSource('/api/office/stream')`.
  - Lớp DOM phủ canvas:
    - Ghế: `data-testid="seat-<index>"`, `data-agent`, `data-empty="true"` khi trống.
    - Nhân vật agent: `char-<agentId>`, `data-place`, biểu tượng 🔑 khi `NEEDS_AUTH`.
    - Biển tên máy: `machine-label-<workerId>`, thêm 🔑 khi máy có `ghNeeds`.
    - Bàn người dùng: `user-<login>`, `data-online`, hiển thị tên và chấm trạng thái.
    - Logo INOVA: `<img>` thẻ DOM phủ trên tường tối màu, `onError` chuyển sang `<span data-testid="logo-fallback">INOVA</span>` màu `#c8102e`.
    - Biểu tượng chim INOVA: `inova-icon-door` ở cửa kính và `inova-icon-rug` ở thảm sảnh.
    - Thanh trạng thái dưới đáy `office-status`: đếm Lead, Đang chạy, Nghỉ, Cần đăng nhập, Online x/y.
    - Trạng thái rỗng: thông báo "Chưa có máy nào kết nối" + link "Thêm máy" trỏ về `/my/machines`.
- **`nav.ts` / `App.tsx`**:
  - `SCR-08`: phân quyền `roles: ['ADMIN', 'PARTNER']`.
  - `App.tsx`: đọc trạng thái văn phòng qua `GET /api/office/status` (`{ enabled }`) cho mọi vai đã đăng nhập.

## File đã tạo/sửa
- `web/src/office/model.ts`: đặc tả kiểu dữ liệu, `computeLayout` bản 2, `initialState`, `applyEvent`.
- `web/src/office/renderer.ts`: hiện thực toàn bộ các hàm vẽ pixel-art, cây cảnh, vách kính, nhân vật và định tuyến.
- `web/src/pages/Office.tsx`: giao diện SCR-08 bản 2 với lớp DOM phủ canvas, logo INOVA, thanh trạng thái và 4 trạng thái UI.
- `web/src/nav.ts`: cấp quyền xem SCR-08 cho cả Admin và Partner khi văn phòng bật.
- `web/src/App.tsx`: gọi `GET /api/office/status` để kiểm tra trạng thái bật/tắt văn phòng cho mọi vai.

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-035-office-ui-v2/verify.sh
 Test Files  11 passed (11)
      Tests  124 passed (124)
TASK-035 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
