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

## DEC-11: Ánh xạ 8 trạng thái task sang 5 cột Kanban
- **Ngày:** 2026-11-04 · **Trạng thái:** CHỐT
- **Quyết định:** `backlog` = PENDING_APPROVAL · `planning` = PLANNING, READY · `in_progress` = IN_PROGRESS · `review` = WAITING_QA, QA_PASSED · `done` = DONE. Task `REJECTED` không hiện trên Kanban (xem ở chi tiết/lọc).
- **Kèm theo:** Partner chỉ thấy task gốc (`parent_id IS NULL`), không có `branch_name`, `assigned_worker_id`. Tra quyền collaborator bằng token server, OAuth người dùng chỉ xin `read:user`.

## DEC-12: Agent trả kết quả có cấu trúc qua file `result.json`; server có bộ pipeline
- **Ngày:** 2026-11-04 · **Trạng thái:** CHỐT
- **Quyết định:**
  - Runner tạo thư mục tạm cho mỗi assignment, thêm vào prompt dòng `AGENTHUB_RESULT_FILE=<đường dẫn tuyệt đối>`; agent ghi JSON vào đó. Runner đọc, kiểm (object JSON, ≤ 64 KB) rồi gửi trong `TASK_SUBMITTED.result`.
  - Schema theo loại job: `SPEC` → `{"subtasks":[{"title","description","dod"}]}` (1–20 mục, bắt buộc); `QA_REVIEW` → `{"verdict":"PASS"|"CHANGES","comments":"..."}` (bắt buộc); `CODE` → `{"summary":"..."}` (tuỳ chọn).
  - Job `CODE` thành công: runner commit, push rồi mở PR (`gh pr create`), gửi `pr_url`.
  - Server: package `internal/pipeline` sinh prompt cho từng assignment (gateway gắn vào `ASSIGN_TASK.payload.prompt`) và chuyển trạng thái task theo `docs/12`: SPEC → tạo sub-task (issue GitHub + job CODE), task cha `READY`; CODE → `WAITING_QA` + job `QA_REVIEW`; QA `PASS` → `QA_PASSED`; QA `CHANGES` → `READY` + job CODE mới cùng nhánh, nhận xét ghi vào `lead_journal` và đưa vào prompt lần sau. Kết quả sai schema → job `REPLAN` cho lead.
- **Phương án loại:** khối JSON cuối stdout (dễ vỡ), agent tự gọi API (phải cấp token cho agent).

## DEC-13: Live terminal tự render ANSI, không dùng xterm.js
- **Ngày:** 2026-11-04 · **Trạng thái:** CHỐT
- **Quyết định:** SCR-06 hiển thị log bằng bộ tách màu SGR tự viết (`web/src/components/ansi.ts`) trong `<span data-color>`, giữ 5000 dòng mới nhất; không thêm thư viện xterm.js (nhẹ hơn, test được trong jsdom, không cần nhập liệu vào terminal). Cấm `dangerouslySetInnerHTML` cho log.

## DEC-14: Thêm dự án bằng cách chọn từ danh sách repo mà PAT thấy; server tự tạo webhook
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-04 · **Trạng thái:** CHỐT
- **Quyết định:** Giữ PAT fine-grained của server (cùng owner `namth`, cấp các repo cần dùng). Admin mở trang Dự án → server liệt kê repo PAT truy cập được → bấm Thêm. Server tự tạo webhook GitHub cho repo đó (PAT cần thêm quyền *Webhooks: Read and write*), dùng chung `GITHUB_WEBHOOK_SECRET`. Bỏ cặp env `GITHUB_REPO_OWNER/NAME` khỏi vai trò "repo duy nhất" (chỉ còn dùng để tạo dự án đầu tiên khi DB trống).
- **Phương án loại:** GitHub App (chuẩn hơn cho nhiều owner nhưng phải làm lại OAuth/PAT), gõ tay owner/repo (dễ sai, quên webhook).

## DEC-15: Một runner mỗi máy, tự clone repo theo job
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-04, FEAT-05 · **Trạng thái:** CHỐT
- **Quyết định:** `ASSIGN_TASK` mang `repo` (`owner/name`). Runner giữ bản clone ở `~/.agenthub/repos/<owner>/<repo>`, tự clone khi gặp repo mới, worktree ở `~/.agenthub/worktrees/<repo>/<nhánh>`. Mỗi agent có danh sách dự án được nhận (NULL = mọi dự án), Admin chỉnh trên SCR-05. Bỏ cờ `--repo`.
- **Phương án loại:** mỗi repo một runner (cùng CLI bị đếm thành nhiều agent, phá BR-17 và hạn mềm).

## DEC-16: Chia việc công bằng giữa các dự án
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-04 · **Trạng thái:** CHỐT
- **Quyết định:** Thứ tự chọn job cho agent rảnh: job `lead_only` (nếu là lead) → `priority` tăng dần → trong cùng priority, dự án có tỉ lệ `RUNNING / weight` nhỏ nhất → `created_at`. Bỏ qua dự án đã đạt `max_parallel` (NULL = không giới hạn) hoặc ngoài danh sách dự án của agent. `weight` 1–5 (mặc định 1), Admin chỉnh ở SCR-09.

## DEC-17: Quản trị hệ thống = owner; vai trong dự án theo quyền trên từng repo
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-04 · **Trạng thái:** CHỐT · Mở rộng DEC-09
- **Quyết định:** Quản trị hệ thống (SCR-05 Cụm máy, SCR-08, SCR-07, SCR-09 Dự án, trao/thu hồi lead) chỉ dành cho tài khoản GitHub trùng `GITHUB_OWNER`. Mỗi dự án: vai `ADMIN`/`PARTNER` tra quyền collaborator trên đúng repo đó theo DEC-09, làm mới khi đăng nhập và khi thêm dự án, lưu `project_members`. Không là collaborator → không thấy dự án. Sidebar có ô chọn dự án; Kanban/Tạo yêu cầu theo dự án đang chọn; quản trị hệ thống có thêm "Tất cả dự án".

## DEC-18: Thêm máy bằng mã tham gia một lần + script cài + khối hướng dẫn cho agent local
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-05 · **Trạng thái:** CHỐT
- **Quyết định:**
  - Quản trị hệ thống bấm "Thêm máy mới" (SCR-05) → nhập tên máy → server tạo **mã tham gia** dùng một lần, hết hạn 15 phút (chỉ lưu SHA-256).
  - Trang hiện **hai thứ**: (1) lệnh một dòng `curl -fsSL <BASE_URL>/install.sh | sh -s -- --join <mã>`; (2) **khối hướng dẫn cho agent** (Markdown, nút Sao chép) để dán vào Claude Code / Antigravity đang chạy trên máy đó: mục tiêu, lệnh cài, cách kiểm kết quả, danh sách công cụ cần có và cách cài theo OS, ràng buộc an toàn (không in token, không dùng chế độ bỏ qua quyền, dừng lại nhờ người dùng ở mọi bước đăng nhập), và tiêu chí xong (máy ONLINE trên SCR-05).
  - `install.sh`: nhận diện OS/CPU, tải binary từ server và kiểm SHA-256, đổi mã lấy `runner_token` (file 600), tự phát hiện agent có trên máy (`claude`, `agy`), cài dịch vụ tự chạy (LaunchAgent / systemd user), khởi động, rồi in bảng kiểm `git/gh/claude/agy/go/node` + trạng thái đăng nhập kèm lệnh cài đặt; **không tự cài phần mềm khác** (việc đó để agent local hoặc người dùng làm, có xác nhận).
- **Phương án loại:** script tự cài mọi công cụ (rủi ro, dễ vỡ theo OS), chỉ in hướng dẫn (chậm, dễ sai).

## DEC-19: Runner tự cập nhật khi rảnh
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-05 · **Trạng thái:** CHỐT
- **Quyết định:** `HANDSHAKE` gửi `version`, `HANDSHAKE_OK` trả `server_version`. Khác nhau → runner chờ mọi agent rảnh, tải `/download/agenthub-<os>-<arch>`, kiểm SHA-256, thay file rồi thoát để dịch vụ khởi động lại. SCR-05 hiện phiên bản từng máy. Cờ `--no-update` để tắt.

## DEC-20: Máy có chủ sở hữu; partner tự thêm máy của mình
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-05 · **Trạng thái:** CHỐT · Mở rộng DEC-18
- **Quyết định:**
  - `workers.owner_user_id` = người tạo mã tham gia. Mọi người dùng đã đăng nhập tạo được mã cho máy của chính mình.
  - Partner có trang **SCR-11 Máy của tôi**: tạo mã, xem trạng thái/agent/dự án đang chạy, tạm dừng hoặc gỡ máy, chọn dự án máy được nhận — chỉ trong các dự án họ là thành viên (DEC-17).
  - Quản trị hệ thống thấy mọi máy trên SCR-05, **nhóm theo chủ máy**, mỗi agent hiện dự án + task đang chạy.
  - Agent trên máy của partner **không được giữ vai trưởng phòng** trừ khi quản trị bật `lead_eligible` cho agent đó (mặc định: máy của owner bật, máy partner tắt).

## DEC-21: Trang Đóng góp tính từ dữ liệu sẵn có
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-05 · **Trạng thái:** CHỐT
- **Quyết định:** **SCR-10 Đóng góp** — lọc khoảng thời gian (7/30/90 ngày, tuỳ chọn), xem theo Partner (chủ máy) hoặc theo Dự án: số máy, job xong / lỗi / hết limit theo loại (SPEC/CODE/QA_REVIEW), task tạo và DONE, PR đã merge, giờ máy chạy việc (tổng `ended_at − started_at` của assignment). Bấm một dòng → dòng thời gian chi tiết (thời điểm, máy/agent, loại job, task, kết quả, attempt). Tính bằng truy vấn trên `assignments`, `jobs`, `tasks`, `workers` — không thêm bảng. Quản trị xem tất cả; partner chỉ xem phần của mình và các dự án mình là thành viên. Không ghi nhận token/chi phí (CLI không báo nhất quán).

## BACKLOG (chưa grill)
- **FEAT-03 Dây chuyền vai trò** (BA / Designer / Dev / Test): vai trò là loại job (`SPEC`, `DESIGN`, `TEST_WRITE`, `CODE`, `QA_REVIEW`), mẫu dây chuyền theo loại task, bảng ánh xạ vai trò → loại agent cấu hình được. Schema `jobs.kind` đã dành sẵn.
- ~~**FEAT-04 Đa dự án song song:**~~ → đã grill, xem DEC-14 → DEC-17 và `docs/features/multi-project.md`. Ghi chú gốc: GitHub App thay PAT, `project_members` (RBAC theo dự án), runner giữ clone theo repo, dispatcher chia lượt có trọng số + giới hạn đồng thời mỗi dự án, secrets/CI theo dự án; một lead trực toàn hệ thống. Cột `jobs.project_id` đã có. Gộp luôn việc thu hẹp scope OAuth của người dùng về `read:user`.
