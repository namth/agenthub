# 14. NHẬT KÝ QUYẾT ĐỊNH KIẾN TRÚC (DECISIONS LOG)

> Mỗi quyết định có mã `DEC-NN`, không xoá; quyết định bị thay thế thì ghi `Trạng thái: THAY THẾ bởi DEC-XX`.

---

## DEC-01: Trưởng phòng = Job trí tuệ + một "lead trực" giữ lease
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-01 · **Trạng thái:** CHỐT
- **Bối cảnh:** Trưởng phòng AI (Claude) có thể hết rate limit bất cứ lúc nào; cần chuyển giao mà không mất việc.
- **Quyết định:**
  - Server Go là bộ điều phối duy nhất, luôn sống, không tiêu token — không bầu chọn phân tán (không Raft).
  - Việc trí tuệ (lập kế hoạch, review PR, nghiệm thu) là **job** trong hàng đợi, máy Claude nào `IDLE` cũng nhận được.
  - Có đúng **1 lead trực** tại một thời điểm, giữ bản ghi `lead_lease` (`worker_id`, `epoch`, `expires_at`), lo việc cần liên tục: gỡ kẹt cho worker, lập lại kế hoạch khi task fail nhiều lần, sắp ưu tiên hàng đợi.
  - Mọi trạng thái điều phối nằm ở DB/GitHub, không nằm trong context của phiên Claude.
- **Phương án loại:** chỉ job (thiếu góc nhìn toàn cảnh); phiên lead dài hạn (tốn token, chuyển giao phải chép context, mâu thuẫn Dispatcher Go).

## DEC-02: Phát hiện hết limit = phản ứng theo mẫu lỗi + hạn mềm chủ động
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-01 · **Trạng thái:** CHỐT
- **Quyết định:**
  - Runner so exit code/stderr của CLI (`claude`, `agy`) với `exhaust_patterns` cấu hình được; đọc giờ reset nếu thông báo có, không có thì dùng `default_cooldown_min` (mặc định 60).
  - Runner gửi `AGENT_EXHAUSTED { agent, resets_at, reason }`; server chuyển agent đó sang `COOLDOWN` đến `resets_at`.
  - Hạn mềm tuỳ chọn mỗi máy (`soft_limit.max_jobs` trong `window_hours`): chạm hạn mềm → lead trực **tự nhường vai** (graceful handoff) và không nhận job mới, nhưng vẫn làm xong job đang chạy.
- **Phương án loại:** chỉ phản ứng (lead luôn bị cắt giữa chừng); server đếm token (không có trần chính xác để so).

## DEC-03: Job dở được làm tiếp từ nhánh, có attempt + fencing
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-01 · **Trạng thái:** CHỐT
- **Quyết định:**
  - Khi hết limit: runner (code, không cần AI) `git commit` WIP + `git push` nhánh của task, tự sinh `handoff_note` từ `git diff --stat` + 50 dòng log cuối.
  - Khi mất kết nối: không push được → máy kế tiếp làm từ commit đã push gần nhất.
  - Task về `READY`, `attempt += 1`, cấp `assignment_id` mới. Máy kế tiếp checkout cùng nhánh, prompt kèm `handoff_note`.
  - **Fencing:** mọi bản tin của runner về một task phải mang `assignment_id` hiện hành; server từ chối (`STALE_ASSIGNMENT`) bản tin mang mã cũ — máy cũ kết nối lại không được nộp đè.
  - `attempt > 3` → sinh job `REPLAN` cho lead trực và báo Admin qua Telegram.
- **Phương án loại:** làm lại từ đầu (phí token); chờ máy cũ reset (trái mục tiêu máy rảnh hỗ trợ máy khác).

## DEC-04: Tách "máy" (workers) và "agent" (agents)
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-01 · **Trạng thái:** CHỐT
- **Quyết định:**
  - `workers` = máy vật lý: kết nối WebSocket, heartbeat, token; trạng thái kết nối `ONLINE | OFFLINE`.
  - `agents` = từng CLI trên máy (`CLAUDE | ANTIGRAVITY`), trạng thái riêng `IDLE | BUSY | COOLDOWN`, có `cooldown_until`, bộ đếm hạn mềm.
  - Máy `OFFLINE` ⇒ mọi agent của máy coi như không khả dụng.
  - Quy tắc "một việc tại một thời điểm" áp dụng **theo agent**, không theo máy.
  - Thay thế cột `workers.agent_type`, `workers.status`, `workers.capabilities` trong `docs/06`.

## DEC-05: Chế độ "trống lead" — phần code chạy tiếp, việc trí tuệ xếp hàng
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-01 · **Trạng thái:** CHỐT
- **Quyết định:**
  - Không còn agent Claude khả dụng ⇒ `lead_lease` rỗng (`VACANT`).
  - Server vẫn giao task code đã `READY` cho Antigravity; job `PLAN`/`REVIEW`/`REPLAN` nằm chờ trong hàng đợi.
  - Telegram báo Admin một lần khi vào và khi thoát chế độ trống lead, kèm giờ dự kiến (`MIN(cooldown_until)` của agent Claude).
  - Agent Claude đầu tiên trở lại `IDLE` tự nhận lease.
  - Antigravity **không** bao giờ giữ vai lead — giữ nguyên Claude là QA độc lập.

## DEC-06: Chọn lead tự động, Admin được ghim/thu hồi trên SCR-05
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-01 · **Trạng thái:** CHỐT
- **Quyết định:**
  - Thứ tự chọn lead kế nhiệm trong các agent Claude `IDLE` của máy `ONLINE`: `lead_priority` (Admin đặt, nhỏ hơn = ưu tiên hơn, NULL xếp cuối) → còn nhiều hạn mềm nhất → rảnh lâu nhất.
  - Admin có nút "Trao vai lead" / "Thu hồi vai" ngay trên SCR-05 Fleet; không thêm màn hình mới.
  - Thao tác thủ công cũng tăng `epoch` như chuyển giao tự động.

## DEC-07: Sơ đồ phối hợp = Văn phòng ảo động, nhúng trong dashboard
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-02 · **Trạng thái:** CHỐT
- **Bối cảnh:** Đã có văn phòng ảo pixel-art ở `namth/edusystem` nhánh `claude/office-simulation-gui-ca46c2`, thư mục `office/` — chỉ quan sát, 4 nhân vật cố định, toạ độ bàn viết cứng, nguồn là hook Claude + `handoff/tasks/*/state.json`.
- **Quyết định:**
  - Port thành màn hình tuỳ chọn SCR-08 trong dashboard AgentHub, chỉ quan sát, không điều khiển.
  - Bố cục sinh từ dữ liệu: mỗi máy một dãy bàn, mỗi agent một bàn; máy `OFFLINE` → cả dãy tắt đèn.
  - "Ghế Trưởng phòng" là vị trí cố định — agent giữ `lead_lease` đi tới ngồi; chuyển giao lead = trao hồ sơ cho người kế nhiệm.
  - Agent `COOLDOWN` về phòng nghỉ kèm đồng hồ đếm ngược.
- **Phương án loại:** đồ thị nút (mất cảm giác văn phòng); làm cả hai (gấp đôi công UI).

## DEC-08: Văn phòng ảo chỉ hiển thị việc thật, không có chế độ mô phỏng
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-02 · **Trạng thái:** CHỐT
- **Quyết định:** SCR-08 chỉ vẽ từ sự kiện thật của server. Không làm `agenthub sim`, không kịch bản demo. Khi chưa có máy/việc → hiện trạng thái rỗng có hướng dẫn đăng ký máy.
- **Hệ quả:** Kiểm thử chuyển giao lead (FEAT-01) dựa vào test tích hợp Go với runner giả trong code test, không dựa vào UI.

## DEC-09: Chỉ quyền `admin`/`maintain` trên GitHub mới thành `ADMIN`
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-01 (sửa lỗi phát hiện khi rà soát) · **Trạng thái:** CHỐT
- **Bối cảnh:** `docs/08` cũ map `write` → `ADMIN`, mâu thuẫn `docs/02` (ADMIN = Owner/Maintainer); mọi collaborator có quyền ghi sẽ duyệt được task và trao/thu hồi lead.
- **Quyết định:** `admin`, `maintain` → `ADMIN`; `write`, `triage`, `read` → `PARTNER`.

## DEC-10: Runner viết bằng Go, chung binary với server
- **Ngày:** 2026-11-03 · **Trạng thái:** CHỐT
- **Quyết định:** Runner daemon là lệnh con `agenthub runner --server <url> --token <token>` trong cùng binary; code ở `internal/runner/`. Bỏ phương án `runner/runner.py`.
- **Lý do:** cài máy con chỉ cần chép 1 file; dùng chung kiểu bản tin với gateway; test chung bộ Go; đúng tinh thần single binary (DEC/docs 05 §2).

## BACKLOG (chưa grill)
- **FEAT-03 Dây chuyền vai trò** (BA / Designer / Dev / Test): vai trò là loại job (`SPEC`, `DESIGN`, `TEST_WRITE`, `CODE`, `QA_REVIEW`), mẫu dây chuyền theo loại task, bảng ánh xạ vai trò → loại agent cấu hình được. Schema `jobs.kind` đã dành sẵn.
- **FEAT-04 Đa dự án song song:** GitHub App thay PAT, `project_members` (RBAC theo dự án), runner giữ clone theo repo, dispatcher chia lượt có trọng số + giới hạn đồng thời mỗi dự án, secrets/CI theo dự án; một lead trực toàn hệ thống. Cột `jobs.project_id` đã có. Gộp luôn việc thu hẹp scope OAuth của người dùng về `read:user`.
