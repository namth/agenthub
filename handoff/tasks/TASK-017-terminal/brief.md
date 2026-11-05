# TASK-017 — Giao diện SCR-06 Live terminal

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity · TASK-07 trong `docs/11-tasks.md`

## 1. Mục tiêu
Admin xem log thời gian thực của một máy con (lịch sử + log sống), giữ màu ANSI, lọc theo task.

## 2. Tài liệu bắt buộc đọc
- `docs/03-screens-and-ui.md` §1 (SCR-06), §3; định dạng SSE: `internal/logs/logs_test.go` (đang làm ở nhánh khác — chỉ tham chiếu)
- `web/src/pages/terminal.test.tsx` (đặc tả chi tiết nhất); `web/src/components/StateView.tsx`; `web/src/pages/Fleet.tsx`

## 3. Phạm vi file
**Được tạo/sửa:** `web/src/pages/LiveTerminal.tsx`, `web/src/components/ansi.ts`, file mới trong `web/src/components/`; `web/src/pages/Fleet.tsx` chỉ thêm link "Xem log" mỗi máy tới `/fleet/<id>/terminal`.

**Không được sửa:** file trong `tests.lock`; `web/package.json` (không thêm xterm — tự render ANSI, nhẹ và test được; quyết định này trưởng phòng ghi vào docs); mọi thứ ngoài `web/src`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
- `ansiToSegments(text): { text: string; color?: string; bold?: boolean }[]` — hỗ trợ SGR 0 (reset), 1 (bold), 30–37/90–97 (màu: black, red, green, yellow, blue, magenta, cyan, white; 9x → cùng tên), 39 (bỏ màu); mọi chuỗi ESC khác (`\x1b[...]` bất kỳ, `\x1b[?…`) bị bỏ; không để lọt ký tự `\x1b`. Gộp các đoạn liền kề cùng kiểu là tuỳ.
- `LiveTerminal`: `:id` từ route; tiêu đề có tên máy; `new EventSource('/api/admin/workers/<id>/logs/stream')`; `history` (mảng dòng) thay toàn bộ; `log` (một dòng) nối thêm. Vùng `data-testid="terminal-output"` nền tối, font mono; mỗi dòng log một phần tử có thuộc tính `data-line`; đoạn màu là `<span data-color="<tên>">` (+ đậm nếu bold). Giữ tối đa 5000 dòng mới nhất. Ô `<select>` nhãn "Lọc theo task" (tất cả + các `task_id` đã thấy). Tự cuộn xuống cuối khi có log mới (trừ khi người dùng bật "Tạm dừng cuộn"). Trước `history`: `aria-busy="true"`; history rỗng và chưa có log: "Chưa có log"; `onerror` → `ErrorBanner` "Mất kết nối — đang thử lại" (giữ log), `onopen` → ẩn; unmount → `close()`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-017-terminal/verify.sh` exit 0 (typecheck, vitest, build)
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không dùng `dangerouslySetInnerHTML` cho log (chống XSS).
- Lệnh được phép: `node`, `bash` (chạy `npx`), `git`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
