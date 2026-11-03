# AgentHub

Nền tảng điều phối đa AI agent phân tán: server Go (single binary + SQLite + WebSocket) giao việc cho dàn máy Claude Code / Antigravity, GitHub là nguồn sự thật, có luân chuyển trưởng phòng khi hết limit.

- Đọc `CONTEXT.md` trước, sau đó tài liệu liên quan trong `docs/` (01–14) và `docs/features/`.
- Quyết định kiến trúc ghi ở `docs/14-decisions.md` (`DEC-NN`); không làm trái DEC đã chốt khi chưa hỏi người dùng.
- Tính năng lớn mới → `/grill-feature` trước khi code.

## Mô hình phân vai Claude ↔ Antigravity

- **Claude Code = Trưởng phòng / Giám đốc sản phẩm:** phân tích, thiết kế kiến trúc, viết đặc tả, viết test, giao việc, nghiệm thu, giữ `docs/`.
- **Antigravity (Gemini) = Kỹ sư thực thi:** viết code sản phẩm theo `brief.md`. Luật ở `.agents/rules/`, quy trình là skill `.agents/skills/thuc-hien-task/`, hook khoá test ở `.agents/hooks.json`. Claude gọi nó qua Antigravity CLI `agy` (chạy ngầm), dự phòng `antigravity-ide chat` hoặc clipboard.
- Giao thức & vòng đời task: `handoff/README.md`, công cụ `scripts/handoff.py` (`doctor` để kiểm tra), cấu hình riêng dự án ở `handoff/config.json`.
- Hạng mục code sản phẩm → mặc định giao qua `/giao-viec`, nghiệm thu qua `/nghiem-thu`. Claude chỉ tự code khi người dùng yêu cầu rõ, hoặc với việc nhỏ (sửa docs, script nội bộ, test).
- Test trong `tests.lock` thuộc về Claude; Antigravity không được sửa.
- Chỉ-đọc với Antigravity: `docs/`, `CONTEXT.md`, `CLAUDE.md` (khai báo trong `handoff/config.json`).

## Cách làm việc

- Trả lời và viết tài liệu bằng tiếng Việt có dấu; giữ nguyên thuật ngữ kỹ thuật, tên biến, lệnh.
- Mã TASK/TC/BR/SCR/DEC/FEAT phải khớp chéo giữa `docs/11`, `docs/12`, `docs/13`, `docs/14` và `docs/features/`.
- Không commit/push khi người dùng chưa đồng ý.
