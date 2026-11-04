# FEAT-02: Văn phòng ảo (Sơ đồ phối hợp trực quan)

> Trạng thái: **Bản 2 — SPEC_APPROVED (2026-11-04)** · Bản 1 DONE (2026-11-04).
> Quyết định: DEC-07 (phần bố cục thay bởi DEC-25), DEC-08, DEC-24, DEC-25, DEC-26, DEC-27, DEC-28 (`docs/14-decisions.md`).
> Nguồn tham khảo phong cách: `namth/edusystem`, nhánh `claude/office-simulation-gui-ca46c2`, thư mục `office/` — bản chép ở `web/src/office/legacy/` (sàn gỗ, tường, vách kính, bảng công việc, cây xanh, đồng hồ).

## 1. Tổng quan & mục tiêu nghiệp vụ

**Bài toán:** Bảng Fleet (SCR-05) cho biết *trạng thái*, nhưng khó cảm nhận *sự phối hợp*: ai đang giao việc cho ai, ai vừa nghỉ, lead vừa đổi sang máy nào, ai trong nhóm đang online.

**Mục tiêu:** Một màn hình vẽ **văn phòng hoàn chỉnh** kiểu pixel-art, các khu liên thông nhau, nhìn là hiểu dây chuyền đang chạy ra sao. **Chỉ quan sát**, không điều khiển; không có chế độ mô phỏng/demo — chỉ vẽ việc thật (DEC-08).

**Bản 2 thay đổi so với bản 1:**
| Bản 1 (đã code) | Bản 2 |
| :--- | :--- |
| Hàng zone trên + lưới "mỗi máy một dãy", số bàn co giãn theo số agent | Khu làm việc có **chỗ ngồi cố định tối thiểu 8**, hai cụm trái/phải, mỗi hàng 2 bàn, lối đi giữa; ghế trống vẫn vẽ máy trống |
| "Phòng Giám đốc" một người (Admin) | **Khu chỉ huy**: mỗi tài khoản GitHub một bàn, xếp dọc, sáng khi online |
| Nền đơn giản | Văn phòng đầy đủ: tường, vách/cửa kính, sàn gỗ, bảng tin, cây xanh, đồng hồ, sảnh, **logo INOVA** |
| Chỉ owner xem | Owner **và partner** xem đầy đủ (DEC-27) |
| Không biết agent chưa đăng nhập | Nhân vật `NEEDS_AUTH` đứng cạnh bàn với 🔑 (FEAT-06) |

## 2. Phân quyền

| Hành động | Owner (quản trị hệ thống) | PARTNER | WORKER_AGENT |
| :--- | :---: | :---: | :---: |
| Xem SCR-08 và luồng `GET /api/office/stream` | ✅ | ✅ (đầy đủ, DEC-27) | ❌ |
| Bật/tắt văn phòng ảo ở SCR-07 | ✅ | ❌ | ❌ |
| Có bàn trong khu chỉ huy | ✅ (đầu hàng) | ✅ | ❌ |

Partner thấy mọi máy, mọi task đang chạy kể cả của dự án họ không tham gia — rủi ro lộ thông tin giữa dự án đã được người dùng chấp nhận (DEC-27). `text` vẫn qua bộ che secret BR-20.

## 3. Luồng tương tác

```mermaid
sequenceDiagram
    autonumber
    participant D as Dispatcher / Lease Manager / GitHub sync / Auth middleware
    participant B as Office Event Bus (in-memory ring 400)
    participant API as GET /api/office/stream (SSE)
    participant UI as SCR-08 Office.tsx

    UI->>API: Mở trang (cookie JWT, owner hoặc partner)
    API->>API: users.last_seen_at = now; đếm kết nối (BR-23)
    API-->>UI: event: snapshot (lead + workers + users + 400 sự kiện gần nhất)
    UI->>UI: Dựng lại cảnh theo BR-43/BR-44, KHÔNG phát lại hoạt cảnh
    loop Khi có sự kiện miền
        D->>B: DomainEvent (ASSIGN_TASK, AGENT_EXHAUSTED, LEASE_GRANTED, AGENT_AUTH, presence…)
        B->>B: Chuyển thành OfficeEvent {ts, who, type, to?, task?, text?}
        B-->>API: phát cho mọi kết nối SSE
        API-->>UI: event: office
        UI->>UI: Xếp hàng hoạt cảnh cho nhân vật `who`
    end
    Note over UI,API: Mất kết nối → EventSource tự nối lại, gửi Last-Event-ID
```

### Bảng chuyển sự kiện miền → sự kiện văn phòng

`who`/`to` là `user:<login>` (người trong khu chỉ huy), `agent_id` (vd `mac-mini/claude`), `worker_id` hoặc `system`; `lead` là bí danh của agent đang giữ lease tại thời điểm phát.

| Sự kiện miền | OfficeEvent | Hoạt cảnh |
| :--- | :--- | :--- |
| Người dùng duyệt task (web/Telegram) | `{who: user:<login>, type: approve, task}` | Người đó đóng dấu hồ sơ **tại bàn mình** trong khu chỉ huy; lead đi qua cửa kính vào nhận |
| `ASSIGN_TASK` | `{who: lead, type: assign, to: agent, task}` | Lead mang hồ sơ tới bàn agent (nếu VACANT: hồ sơ tự bay từ "khay việc") |
| Agent bắt đầu làm (`LOG_CHUNK` đầu tiên) | `{who: agent, type: work, task, text}` | Agent gõ phím, màn hình bàn chạy code |
| `TASK_SUBMITTED` | `{who: agent, type: submit, task}` | Agent giơ tay 🙋 |
| Job `QA_REVIEW` bắt đầu | `{who: reviewer, type: review, to: tác giả, task}` | Reviewer cầm bảng kẹp đi tới bàn tác giả, màn hình chạy test |
| QA đạt / cần sửa | `approve` / `reject` (text = lý do) | Kết luận tại bàn rồi về chỗ |
| `AGENT_EXHAUSTED` / chạm hạn mềm | `{who: agent, type: cooldown, text: "đến 21:40"}` | Agent đi xuống phòng nghỉ, ☕ + đồng hồ đếm ngược |
| `COOLDOWN → IDLE` | `{who: agent, type: idle}` | Agent rời phòng nghỉ về bàn |
| `LEASE_GRANTED` (đổi người) | `{who: lead cũ, type: handover, to: lead mới}` | Lead cũ trao hồ sơ "Trưởng phòng"; lead mới (Claude **hoặc Antigravity**, DEC-29) đi tới bàn Trưởng phòng ★, bàn cũ của nó hiện "→ ★" |
| Lease `VACANT` | `{who: system, type: vacant, text: "dự kiến 21:40"}` | Bàn Trưởng phòng trống, biển "Đang tuyển trưởng phòng — 21:40" |
| Máy `OFFLINE` / `ONLINE` | `{who: worker, type: offline/online}` | Hàng bàn của máy tắt/bật đèn 📴 |
| Job `REPLAN`/`UNBLOCK` | `{who: lead, type: work, task, text: "Gỡ kẹt #102"}` | Lead đi tới bàn agent đang kẹt rồi về ghế |
| Lỗi job | `{who: agent, type: error, text}` | ⚠️ trên đầu agent |
| **(mới)** `AGENT_AUTH` vào/ra `NEEDS_AUTH` | `{who: agent, type: auth, text: "NEEDS_AUTH"\|"OK"}` | Đứng dậy cạnh bàn với 🔑, màn hình ổ khoá / ngồi lại làm việc |
| **(mới)** `gh_auth` của máy vào/ra `NEEDS_AUTH` | `{who: worker, type: auth, text: …}` | 🔑 trên biển tên máy |
| **(mới)** Người dùng online/offline (BR-44) | `{who: user:<login>, type: presence, text: "online"\|"offline"}` | Người đi từ sảnh vào ngồi bàn / rời bàn ra sảnh; khu chỉ huy xếp lại thứ tự |
| **(mới)** Máy mới bắt tay lần đầu | `{who: worker, type: online}` + snapshot cập nhật | Nhân vật mới đi từ sảnh vào ngồi đúng ghế (BR-43) |

## 4. Giao diện & 4 trạng thái UI

**SCR-08 "Văn phòng ảo"** — route `/office`, file `src/pages/Office.tsx`, owner và partner, mục sidebar "Văn phòng"; **chỉ hiện khi bật** ở SCR-07 (mặc định tắt).

### 4.1 Bố cục (canvas logic rộng 480 px, cao động)

```
┌── TƯỜNG SAU: 🌿 [ LOGO INOVA đầy đủ ] [Bảng tin] 🕰 🌿 ──────────────┬── KHU CHỈ HUY ────────┐
│                 [ BÀN TRƯỞNG PHÒNG ★ ]      [Khay việc]            │ [🐦 icon] cửa kính    │
│                                                                    ║ ★ namth (owner)  🟢   │
│   CỤM TRÁI                     lối đi                CỤM PHẢI       ║ partner-a        🟢   │
│  [claude][agy]                   │                [claude][agy]    ║ partner-b        🟢   │
│   iMac-8GB                       │                  mac-mini       ║ partner-c (vắng) ░░   │
│  [claude][agy]                   │                [trống][trống]   ║ …                     │
│   laptop                         │                                 │                       │
│  [trống][trống]                  │                [trống][trống]   │ 🌿                    │
│  [trống][trống]                  │                [trống][trống]   │                       │
│   … (máy thứ 9+ kéo dài xuống, mỗi bên 1 hàng 2 bàn) …             │                       │
├── PHÒNG NGHỈ ☕ (COOLDOWN) ──────────┬── SẢNH: thảm [🐦] · cửa ra vào kính ──┴─────────────────┤
└──────────────────────────────────────┴──────────────────────────────────────────────────────────┘
Thanh dưới: Lead · Đang chạy 3 job · Chờ 2 · Nghỉ 1 · Cần đăng nhập 1 · Online 2/4 · [⏸ Tạm dừng hoạt cảnh]
```

- **Khu làm việc (sàn gỗ):** hai cụm trái/phải, lối đi ở giữa; mỗi hàng 2 bàn, **1 bàn = 1 agent**; hai agent cùng máy ngồi cùng hàng, biển tên máy dưới hàng. Luôn vẽ **tối thiểu 8 bàn** (2 cụm × 2 hàng × 2 bàn); bàn chưa có ai → máy tính tắt, ghế trống. Quy tắc xếp chỗ: BR-43.
- **Bàn Trưởng phòng ★** ở chính giữa phía trên hai cụm, sát tường sau; dưới tường là logo INOVA. Agent giữ lease ngồi ở đây; bàn gốc của nó để trống kèm "→ ★".
- **Tường sau:** ô tường tối màu gắn **logo INOVA đầy đủ** (`/brand/inova-logo.png`), cạnh đó bảng tin (hiện số job đang chạy/chờ, như bảng công việc của bản tham khảo), đồng hồ, cây xanh; góc phải có "khay việc".
- **Khu chỉ huy (bên phải):** ngăn với khu làm việc bằng **vách kính** có **cửa thông** (lead đi qua khi nhận việc duyệt); **icon chim INOVA** (`/brand/inova-icon.png`) dán trên cửa kính. Bàn xếp **theo hàng dọc**, mỗi tài khoản GitHub một bàn có biển tên + avatar; thứ tự BR-44. Người vắng: ghế trống, bàn mờ, vẫn giữ biển tên. Nhiều người hơn chiều cao khu làm việc → khu chỉ huy kéo dài xuống cùng canvas.
- **Đáy văn phòng:** phòng nghỉ ☕ (agent `COOLDOWN`, đồng hồ đếm ngược) và sảnh (thảm có icon chim, cửa ra vào kính, cây xanh). Khu làm việc hoặc khu chỉ huy dài thêm thì đáy bị đẩy xuống.
- **Liên thông:** lối đi giữa nối bàn Trưởng phòng ↔ hai cụm ↔ sảnh/phòng nghỉ; cửa kính nối khu làm việc ↔ khu chỉ huy. Nhân vật di chuyển theo lối đi (định tuyến như bản tham khảo `routeTo`), không xuyên tường.
- **Nhân vật:** màu áo theo `kind` (Claude cam, Antigravity xanh); người dùng mặc áo theo vai (owner đỏ INOVA `#c8102e`, partner xám). `NEEDS_AUTH` → đứng cạnh bàn, 🔑 trên đầu, màn hình hiện ổ khoá. Di chuột → tooltip `agent_id`/login, trạng thái, `auth_status`, job hiện tại, `attempt`.
- **Logo vẽ bằng lớp DOM** (`<img>` phủ lên canvas, toạ độ theo hệ logic, co giãn cùng canvas) để giữ nét; ảnh lỗi tải → vẽ chữ "INOVA" bằng canvas (BR-46).
- **Hiệu năng (giữ từ bản 1):** nền tĩnh vẽ một lần vào canvas phụ, chỉ vẽ lại khi bố cục đổi (thêm máy/người); 12 fps khi có hoạt động, 4 fps khi yên, dừng hẳn khi tab ẩn; hàng đợi hoạt cảnh mỗi nhân vật tối đa 10 bước.

### 4.2 Trạng thái UI

| Trạng thái | Hiển thị |
| :--- | :--- |
| **Empty** | Văn phòng đầy đủ nội thất, 8 bàn trống, khu chỉ huy chỉ có bàn của người đang xem; thông điệp "Chưa có máy nào kết nối" + nút "Thêm máy" → SCR-11 (partner) / SCR-05 (owner). |
| **Loading** | Khung văn phòng mờ (skeleton) đến khi nhận `snapshot`. |
| **Error** | Mất SSE: băng hổ phách "Mất kết nối — đang thử lại", giữ cảnh cuối, nhân vật đứng yên; `403` → về SCR-02; `429` → "Bạn đã mở văn phòng ở quá nhiều tab". |
| **Success** | Văn phòng sống như trên. |

**SCR-07 Cài đặt:** công tắc "Bật văn phòng ảo" (lưu `settings.office_enabled`) — chỉ owner. Tắt → mục sidebar ẩn với mọi người, `/office` và `/api/office/stream` trả `404`.

## 5. Tác động dữ liệu

```sql
-- Bản 1 (đã có)
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT OR IGNORE INTO settings (key, value) VALUES ('office_enabled', 'false');

-- Bản 2 (trong migration 004_agent_auth_office.sql, FEAT-06 §5)
ALTER TABLE users ADD COLUMN last_seen_at DATETIME;
```

- `users.last_seen_at` cập nhật bởi middleware xác thực, **tối đa 1 lần/60 giây/người** (so trong RAM trước khi ghi).
- Thứ tự ghế máy **không lưu DB**: tính lại từ danh sách máy sắp theo tên mỗi lần dựng (BR-43).
- Sự kiện văn phòng vẫn **không lưu DB**: bộ đệm vòng 400 trong RAM.
- Ảnh logo: `web/public/brand/inova-logo.png` (900×356, PNG trong suốt), `web/public/brand/inova-icon.png` (411×417) — nhúng vào binary cùng `web/dist`.

## 6. Hợp đồng API

| Endpoint | Quyền | Mô tả | Lỗi |
| :--- | :--- | :--- | :--- |
| `GET /api/office/stream` | owner, PARTNER | **Đổi đường dẫn** từ `/api/admin/office/stream` (đường cũ giữ làm bí danh, trả `308` tới đường mới). `text/event-stream`; sự kiện đầu `event: snapshot`, sau đó `event: office`; `id` tăng dần; hỗ trợ `Last-Event-ID`; heartbeat `: ping` mỗi 20 giây. | `401` · `404 NOT_FOUND` khi tắt · `429 TOO_MANY_CONNECTIONS` (BR-23) |
| `GET /api/admin/settings` | owner | mọi key, gồm `office_enabled`, `telegram_enabled` (chỉ đọc) | `403` |
| `PATCH /api/admin/settings` | owner | `{ "office_enabled": true }` | `400 VALIDATION_ERROR` · `403` |

```json
// event: snapshot (bản 2)
{ "lead": { "state": "HELD", "agent_id": "mac-mini/claude", "epoch": 8 },
  "workers": [ { "id": "iMac-8GB", "conn_status": "ONLINE", "gh_auth": "OK", "owner": "namth",
                 "agents": [ { "id": "iMac-8GB/claude", "kind": "CLAUDE", "status": "IDLE", "auth_status": "NEEDS_AUTH" },
                             { "id": "iMac-8GB/antigravity", "kind": "ANTIGRAVITY", "status": "BUSY", "auth_status": "UNKNOWN",
                               "job": { "task": 101, "kind": "CODE" } } ] } ],
  "users": [ { "login": "namth", "avatar_url": "https://…", "is_owner": true, "online": true },
             { "login": "partner-a", "avatar_url": "https://…", "is_owner": false, "online": false } ],
  "recent": [ /* ≤ 400 OfficeEvent */ ] }
```

```text
id: 1902
event: office
data: {"ts":"2026-11-04T09:02:11Z","who":"user:partner-a","type":"presence","text":"online"}
```

## 7. Quy tắc nghiệp vụ

| Mã | Quy tắc |
| :--- | :--- |
| **BR-19** | Văn phòng ảo chỉ đọc: SCR-08 không có nút điều khiển nào; mọi thao tác vẫn ở SCR-05/SCR-04/SCR-11. |
| **BR-20** | `text` của OfficeEvent tối đa 80 ký tự, đã loại chuỗi giống secret (token, `ghp_`, `sk-`, khoá dài ≥ 32 ký tự liên tục). Không đưa nội dung log thô hay prompt vào văn phòng. |
| **BR-21** | Khi dựng lại từ `snapshot`, chỉ đặt nhân vật vào vị trí cuối, **không** phát lại hoạt cảnh. |
| **BR-22** | `office_enabled = false` → server không giữ kết nối SSE nào (đóng các kết nối đang mở) và bộ chuyển sự kiện ngừng ghi bộ đệm. |
| **BR-23** | *(sửa ở bản 2 — DEC-27)* Tối đa **3 kết nối SSE mỗi người dùng** và **30 kết nối tổng**; vượt → `429 TOO_MANY_CONNECTIONS`. |
| **BR-43** | **Xếp chỗ khu làm việc:** máy sắp theo `name` (không phân biệt hoa thường). Mỗi máy chiếm `ceil(số agent / 2)` hàng liền nhau trong một cụm; agent trong máy theo thứ tự `CLAUDE` rồi `ANTIGRAVITY`. Máy lần lượt đặt vào hàng trống kế tiếp theo thứ tự **trái-1 → phải-1 → trái-2 → phải-2 → …**; máy cần 2 hàng mà cụm hiện tại không còn hàng liền → đặt sang cụm còn lại hoặc kéo dài xuống. Số hàng mỗi cụm = `max(2, số hàng cần)`, tức luôn ≥ 8 bàn. Máy `OFFLINE` vẫn giữ ghế (đèn tắt); máy bị gỡ thì ghế được giải phóng ở lần dựng sau. |
| **BR-44** | **Khu chỉ huy:** mỗi bản ghi `users` một bàn. Thứ tự: owner (`GITHUB_OWNER`) đầu tiên → partner **online** (theo `username`) → partner **vắng** (theo `username`). Online = `last_seen_at ≥ now − 5 phút` **hoặc** đang mở ít nhất một kết nối SSE văn phòng. Server kiểm mỗi 60 giây và phát `presence` khi đổi; client đổi chỗ bằng hoạt cảnh đi bộ, không nhảy cóc. |
| **BR-45** | Nhân vật agent theo `auth_status`: `NEEDS_AUTH` → đứng cạnh bàn với 🔑 (không gõ phím, không nhận hồ sơ mới); máy `gh_auth = NEEDS_AUTH` → 🔑 trên biển tên máy; `UNKNOWN`/`OK` → hiển thị bình thường. |
| **BR-46** | Logo dùng đúng hai ảnh trong `web/public/brand/`; không cho tải logo qua UI. Logo đầy đủ chỉ xuất hiện một lần (tường sau), icon ở cửa kính khu chỉ huy và thảm sảnh. Ảnh lỗi tải → chữ "INOVA" màu `#c8102e`, bố cục không xê dịch. |

## 8. Test case

| Mã | Loại | Kịch bản | Kỳ vọng |
| :--- | :--- | :--- | :--- |
| **TC-28** | Unit | Bộ chuyển nhận từng sự kiện miền trong bảng mục 3 | Ra đúng OfficeEvent (`who`, `type`, `to`) — gồm `auth`, `presence`, `approve` theo `user:<login>` |
| **TC-29** | Unit | `text` chứa `ghp_abc…` hoặc dài 200 ký tự | Bị che và cắt còn ≤ 80 ký tự (BR-20) |
| **TC-30** | Security | *(sửa)* Partner gọi `GET /api/office/stream`; người chưa đăng nhập gọi | Partner `200` nhận `snapshot`; chưa đăng nhập `401` |
| **TC-31** | API | `office_enabled = false` rồi gọi stream và `/office` | `404`; bật lại → `200` và nhận `snapshot` |
| **TC-32** | API | Nối lại với `Last-Event-ID` còn trong bộ đệm | Nhận đúng các sự kiện bị lỡ, không trùng |
| **TC-33** | UI (vitest) | *(sửa)* `computeLayout` với 0, 1, 3, 4, 5 và 9 máy (2 agent/máy) | 0–4 máy: đúng 8 bàn, thứ tự trái-1/phải-1/trái-2/phải-2; 5 máy: thêm hàng thứ 3 bên trái; 9 máy: 5 hàng trái, 4 hàng phải; không bàn nào chồng lấn lối đi |
| **TC-34** | UI (vitest) | Sự kiện `handover` tới một agent Antigravity | Nhân vật mới ở bàn Trưởng phòng, bàn gốc trống "→ ★" |
| **TC-35** | UI (vitest) | Agent `COOLDOWN` | Nhân vật ở phòng nghỉ, đồng hồ đúng `cooldown_until` |
| **TC-68** | Unit (Go) | Danh sách `users` với owner, 2 partner online, 1 partner vắng | Snapshot `users` đúng thứ tự BR-44; phát `presence` khi `last_seen_at` quá 5 phút |
| **TC-69** | Unit (Go) | Middleware nhận 10 request trong 30 giây của cùng người | `users.last_seen_at` chỉ ghi 1 lần |
| **TC-70** | API | Một người mở kết nối SSE thứ 4; tổng 31 kết nối | `429 TOO_MANY_CONNECTIONS` cho cả hai trường hợp (BR-23) |
| **TC-71** | UI (vitest) | `computeLayout` khu chỉ huy với 1, 4, 12 người | Bàn xếp dọc, owner đầu; 12 người làm canvas cao thêm, không chồng sảnh |
| **TC-72** | UI (vitest) | Agent `NEEDS_AUTH`, máy `gh_auth = NEEDS_AUTH` | Nhân vật đứng cạnh bàn có 🔑; biển máy có 🔑 (BR-45) |
| **TC-73** | UI (vitest) | Ảnh logo lỗi tải (mock `onerror`) | Hiện chữ "INOVA", toạ độ các khu không đổi (BR-46) |

## 9. Task triển khai & DoD

| Task | Milestone | Nội dung | DoD / lệnh xác minh |
| :--- | :--- | :--- | :--- |
| ~~TASK-15~~ | M4 | Bản 1: `internal/office`, SSE, `settings` | DONE |
| ~~TASK-16~~ | M4 | Bản 1: `Office.tsx` | DONE |
| ~~TASK-17~~ | M4 | Công tắc SCR-07 | DONE |
| **TASK-033** | Bản 2 | `internal/office`: snapshot thêm `users`, `auth_status`, `gh_auth`, `owner`; sự kiện `auth`, `presence`, `approve` theo người duyệt; middleware `last_seen_at` (BR-44, TC-69); route `GET /api/office/stream` cho owner + partner, bí danh `308`; giới hạn BR-23; `GET /api/admin/settings` thêm `telegram_enabled` (DEC-30) | `go test ./internal/office/... ./internal/auth/...` qua TC-28 → TC-32, TC-68 → TC-70 |
| **TASK-034** | Bản 2 | `web/src/office/model.ts` (`computeLayout` bản 2: khu làm việc BR-43, khu chỉ huy BR-44, đáy phòng nghỉ/sảnh), `renderer.ts` (nội thất theo bản tham khảo legacy: sàn gỗ, tường, vách/cửa kính, bảng tin, cây, đồng hồ; nhân vật người dùng; 🔑 BR-45), lớp DOM logo (BR-46), `Office.tsx` (route/stream mới, thanh dưới, 4 trạng thái); sidebar hiện "Văn phòng" cho partner khi bật | `cd web && npx vitest run src/office` qua TC-33 → TC-35, TC-71 → TC-73; `npm run typecheck && npm run build`; ảnh chụp màn hình 0/3/9 máy gửi kèm bản nộp |

Thứ tự: TASK-030 (migration) → TASK-033 → TASK-034.
