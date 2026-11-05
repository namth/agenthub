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
  - ~~Antigravity **không** bao giờ giữ vai lead — giữ nguyên Claude là QA độc lập.~~ → thay bởi DEC-29 (agent bất kỳ có `lead_eligible`; QA độc lập giữ bằng quy tắc không tự review).

## DEC-06: Chọn lead tự động, Admin được ghim/thu hồi trên SCR-05
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-01 · **Trạng thái:** CHỐT
- **Quyết định:**
  - Thứ tự chọn lead kế nhiệm trong các agent Claude `IDLE` của máy `ONLINE` *(→ agent bất kỳ `lead_eligible`, DEC-29)*: `lead_priority` (Admin đặt, nhỏ hơn = ưu tiên hơn, NULL xếp cuối) → còn nhiều hạn mềm nhất → rảnh lâu nhất.
  - Admin có nút "Trao vai lead" / "Thu hồi vai" ngay trên SCR-05 Fleet; không thêm màn hình mới.
  - Thao tác thủ công cũng tăng `epoch` như chuyển giao tự động.

## DEC-07: Sơ đồ phối hợp = Văn phòng ảo động, nhúng trong dashboard
- **Ngày:** 2026-11-03 · **Tính năng:** FEAT-02 · **Trạng thái:** CHỐT
- **Bối cảnh:** Đã có văn phòng ảo pixel-art ở `namth/edusystem` nhánh `claude/office-simulation-gui-ca46c2`, thư mục `office/` — chỉ quan sát, 4 nhân vật cố định, toạ độ bàn viết cứng, nguồn là hook Claude + `handoff/tasks/*/state.json`.
- **Quyết định:**
  - Port thành màn hình tuỳ chọn SCR-08 trong dashboard AgentHub, chỉ quan sát, không điều khiển.
  - ~~Bố cục sinh từ dữ liệu: mỗi máy một dãy bàn, mỗi agent một bàn; máy `OFFLINE` → cả dãy tắt đèn.~~ → thay bởi DEC-25 (chỗ ngồi cố định, hai cụm, mỗi agent một bàn; máy `OFFLINE` → hàng bàn tắt đèn).
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
- **Quyết định:** `backlog` = PENDING_APPROVAL · `planning` = PLANNING, READY, WAITING_USER *(thêm theo DEC-33, nhãn "Cần làm rõ")* · `in_progress` = IN_PROGRESS · `review` = WAITING_QA, QA_PASSED · `done` = DONE. Task `REJECTED` không hiện trên Kanban (xem ở chi tiết/lọc).
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
- **Quyết định:** Quản trị hệ thống (SCR-05 Cụm máy, SCR-08 *(→ mở cho partner theo DEC-27)*, SCR-07, SCR-09 Dự án, trao/thu hồi lead) chỉ dành cho tài khoản GitHub trùng `GITHUB_OWNER`. Mỗi dự án: vai `ADMIN`/`PARTNER` tra quyền collaborator trên đúng repo đó theo DEC-09, làm mới khi đăng nhập và khi thêm dự án, lưu `project_members`. Không là collaborator → không thấy dự án. Sidebar có ô chọn dự án; Kanban/Tạo yêu cầu theo dự án đang chọn; quản trị hệ thống có thêm "Tất cả dự án".

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

## DEC-22: Trạng thái đăng nhập là cột riêng `auth_status`, không thêm giá trị vào `status`
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-06 · **Trạng thái:** CHỐT
- **Bối cảnh:** Máy iMac-8GB nối vào hiện IDLE dù agent có thể chưa đăng nhập; server không có khái niệm đăng nhập, dispatcher chỉ kiểm `status = 'IDLE'`.
- **Quyết định:** Thêm `agents.auth_status OK | NEEDS_AUTH | UNKNOWN` (+ `auth_checked_at`, `auth_detail`) và `workers.gh_auth` cùng miền giá trị, độc lập với `status IDLE | BUSY | COOLDOWN` (giữ nguyên DEC-04). Nhận job/làm lead đòi `auth_status <> 'NEEDS_AUTH'` và `gh_auth <> 'NEEDS_AUTH'`; `UNKNOWN` vẫn được nhận.
- **Phương án loại:** thêm `NEEDS_AUTH` vào `status` (phá DEC-04, chồng chéo với COOLDOWN).

## DEC-23: Kiểm đăng nhập chủ động + phản ứng theo mẫu lỗi
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-06 · **Trạng thái:** CHỐT
- **Quyết định:** Runner chạy `claude auth status`, `gh auth status` lúc bắt tay, 10 phút/lần khi rảnh, và trước `REQUEST_TASK` nếu lần kiểm cũ hơn 60 giây (timeout 15 giây → `UNKNOWN`). `agy` chưa có lệnh kiểm → `UNKNOWN`. Thêm `auth_patterns` (như `exhaust_patterns` của DEC-02): job lỗi khớp mẫu → `AGENT_AUTH NEEDS_AUTH` kèm `assignment_id`, job về hàng đợi, **không tăng `attempt`**. Chủ máy bấm "Đã đăng nhập, kiểm lại" → `AUTH_RECHECK`.
- **Phương án loại:** chỉ chủ động (agy không bao giờ phát hiện được, job lỗi bị tính attempt); chỉ phản ứng (máy mới vẫn hiện IDLE tới job đầu tiên).

## DEC-24: Hiển thị "Cần đăng nhập" trên web, văn phòng và Telegram
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-06, FEAT-02 · **Trạng thái:** CHỐT
- **Quyết định:** SCR-05/SCR-11 nhãn đỏ + lệnh gợi ý + nút kiểm lại; SCR-08 nhân vật đứng cạnh bàn với 🔑; Telegram báo Admin một tin khi vào `NEEDS_AUTH`, một tin khi trở lại `OK`.

## DEC-25: Văn phòng ảo bản 2 — chỗ ngồi cố định, hai cụm, lối đi giữa
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-02 · **Trạng thái:** CHỐT · **Thay phần bố cục của DEC-07** ("mỗi máy một dãy bàn")
- **Quyết định:** Văn phòng hoàn chỉnh liên thông (tường, vách/cửa kính, bảng tin, cây xanh, sàn gỗ, phòng nghỉ, sảnh) theo phong cách bản tham khảo edusystem. Khu làm việc tối thiểu 8 bàn, **1 bàn = 1 agent**, hai cụm trái/phải, mỗi hàng 2 bàn (một máy 2 agent = một hàng), lối đi giữa; xếp máy theo tên trái-1 → phải-1 → trái-2 → …, quá 4 máy kéo dài xuống. Bàn Trưởng phòng phía trên hai cụm. Phòng nghỉ + sảnh ở đáy.
- **Phương án loại:** 1 bàn = 1 máy (khó thấy agent nào làm gì).

## DEC-26: Khu chỉ huy — mỗi tài khoản GitHub một bàn, xếp dọc, sáng khi online
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-02 · **Trạng thái:** CHỐT
- **Quyết định:** Khu chỉ huy bên phải khu làm việc, ngăn bằng vách kính có cửa thông. Mỗi bản ghi `users` một bàn có biển tên. Thứ tự: owner đầu → partner online → partner vắng (ghế trống, vẫn có biển tên). Online = request web trong 5 phút (`users.last_seen_at`) hoặc đang mở SSE văn phòng. Hoạt cảnh duyệt task diễn ra tại bàn người duyệt.
- **Phương án loại:** chỉ hiện người online (phòng thay đổi liên tục); ai cũng luôn ngồi (không phản ánh thực tế).

## DEC-27: Partner được xem SCR-08 đầy đủ
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-02 · **Trạng thái:** CHỐT · **Sửa DEC-17** (SCR-08 không còn là màn hình chỉ owner)
- **Quyết định:** Owner và partner mở SCR-08 với cùng nội dung, kể cả task của dự án partner không tham gia (người dùng chấp nhận rủi ro lộ thông tin giữa dự án). Công tắc bật/tắt vẫn chỉ owner. Luồng SSE chuyển sang `GET /api/office/stream` (đường `/api/admin/office/stream` trả `308`). Giới hạn SSE: 3 kết nối/người, 30 tổng (thay BR-23 cũ: 5 tổng).
- **Phương án loại:** partner xem có lọc theo dự án (an toàn hơn, người dùng không chọn); giữ chỉ owner.

## DEC-28: Logo INOVA trên tường sau, icon trên cửa kính và thảm sảnh
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-02 · **Trạng thái:** CHỐT
- **Quyết định:** Logo đầy đủ (`web/public/brand/inova-logo.png`) trên ô tường tối phía trên bàn Trưởng phòng; icon chim (`web/public/brand/inova-icon.png`) trên cửa kính khu chỉ huy và thảm sảnh. Vẽ bằng lớp DOM phủ canvas để giữ nét; lỗi tải → chữ "INOVA".
- **Phương án loại:** logo ở sảnh lễ tân đầu văn phòng (tốn thêm một dải chiều cao).

## DEC-29: Trưởng phòng có thể là agent bất kỳ loại nếu được bật `lead_eligible`
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-01 (sửa đổi), FEAT-06 · **Trạng thái:** CHỐT · **Thay mục "Antigravity không bao giờ giữ vai lead" của DEC-05; thay cụm "agent Claude `IDLE`" ở DEC-06**
- **Quyết định:** Ứng viên lead = agent bất kỳ `kind` có `lead_eligible = 1`, `IDLE`, `auth_status <> NEEDS_AUTH`, máy `ONLINE`; thứ tự giữ nguyên DEC-06 (`lead_priority` → hạn mềm → rảnh lâu nhất). Mặc định chỉ Claude trên máy owner bật; Admin bật cho agent khác trên SCR-05. Job `lead_only` giao cho người giữ lease bất kể `required_agent`. Giữ QA độc lập: `QA_REVIEW` không giao cho agent đã viết code của chính task đó. `VACANT` = không còn ứng viên nào.
- **Phương án loại:** agy chỉ làm lead khi Admin bấm "Trao vai" thủ công.

## DEC-30: Cấu hình Telegram giữ hoàn toàn trong `.env`
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-06 · **Trạng thái:** CHỐT
- **Bối cảnh:** `docs/03` cũ ghi SCR-07 "cấu hình Telegram Bot Token, Chat ID" nhưng code chỉ đọc `TELEGRAM_BOT_TOKEN`, `TELEGRAM_ADMIN_CHAT_ID`, `TELEGRAM_WEBHOOK_SECRET` từ môi trường (`cmd/server/main.go`).
- **Quyết định:** Giữ nguyên `.env`; SCR-07 chỉ hiện dòng chỉ đọc "Telegram: đang bật / tắt (cấu hình trong `.env`)" từ `GET /api/admin/settings.telegram_enabled`. Sửa `docs/03` cho khớp code.
- **Phương án loại:** Chat ID + công tắc loại tin trên SCR-07; nhập cả token trên web (phải mã hoá secret trong DB).

## DEC-31: Vòng lặp phỏng vấn làm rõ yêu cầu (FEAT-07) & Kỹ năng dự án dùng chung (FEAT-08)
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-07, FEAT-08 · **Trạng thái:** CHỐT
- **Quyết định:**
  - **FEAT-07:** Trưởng phòng khi nhận job `SPEC` có thể trả về `{"status":"NEED_INFO","questions":[...]}` thay vì chỉ sinh `subtasks`. Task chuyển sang trạng thái `WAITING_USER` (đóng băng timeout/attempts).
  - Tương tác đa kênh đồng bộ: Tab Q&A trên Web SCR-04 + GitHub Issue Comments + Telegram Bot inline buttons. Bất kỳ câu trả lời nào gửi về đều nạp vào prompt của `SPEC` để Trưởng phòng hoàn thiện và chốt việc.
  - Bổ sung bảng `task_clarifications` (`id`, `task_id`, `round`, `questions`, `answers`, `status`, `answered_by`).
  - ~~**FEAT-08:** Đặt toàn bộ kỹ năng chuẩn (`grill-master`, `grill-feature`) trong `.agents/skills/` của repository (kèm symlink `.claude/skills/`). Prompt của `SPEC` trong Server Go bắt buộc dẫn hướng Trưởng phòng đọc và tuân thủ `.agents/skills/grill-feature/SKILL.md`.~~ → thay bởi DEC-32 (job SPEC chạy trong repo dự án đích nên skill phải đi theo runner).

## DEC-32: Kỹ năng trưởng phòng nhúng trong binary runner
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-08 · **Trạng thái:** CHỐT · **Thay phần FEAT-08 của DEC-31**
- **Bối cảnh:** Job SPEC chạy trong worktree của repo dự án đích (DEC-15); skill đặt trong repo AgentHub không có ở đó. `grill-feature` là skill tương tác (AskUserQuestion) và đòi bộ `docs/`, không chạy được ở chế độ `-p`.
- **Quyết định:** Skill không tương tác `agenthub-spec` (phỏng vấn → `NEED_INFO`) nhúng bằng `go:embed` (`internal/skills`), runner chép vào `<worktree>/.agenthub/skills/` và `.git/info/exclude`; dự án có `.agents/skills/agenthub-spec/` riêng thì ưu tiên. Cập nhật theo phiên bản server (DEC-19).
- **Phương án loại:** mỗi dự án tự mang skill.

## DEC-33: Ai trả lời câu hỏi làm rõ; WAITING_USER ở cột Planning
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-07 · **Trạng thái:** CHỐT · **Sửa DEC-11, sửa BR-52**
- **Quyết định:** Người trả lời = owner, ADMIN dự án, người tạo task; trả lời trước thì chốt vòng. Kênh: web cho mọi người, comment GitHub `/answer`, Telegram chỉ owner. Job SPEC sau khi trả lời giao cho agent Claude bất kỳ (không bắt buộc lead). Kanban: `WAITING_USER` ở cột Planning, nhãn "Cần làm rõ (n câu)"; nhắc 24h/72h, Admin có "Cho trưởng phòng tự quyết".
- **Phương án loại:** chỉ Admin trả lời; cột Kanban thứ 6.

## DEC-34: Quyền CLI không tương tác = allowlist theo dự án
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-09 · **Trạng thái:** CHỐT
- **Quyết định:** Mặc định hệ thống (`git`, `gh pr create/view`, đọc file) ∪ lệnh build/test riêng dự án do owner khai trên SCR-09; gửi trong `ASSIGN_TASK.payload.allowed_commands`. Claude: `--permission-mode acceptEdits --allowedTools …`; agy: cờ phiên `--settings '{"permissions":{"allow":["command(…)"]}}'`. Không bao giờ dùng chế độ bỏ qua quyền.
- **Phương án loại:** allowlist chung cố định; chạy trong container.

## DEC-35: Kiểm đăng nhập bằng lời gọi thật
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-09 (sửa FEAT-06) · **Trạng thái:** CHỐT · Mở rộng DEC-23
- **Bối cảnh:** `claude auth status` báo `loggedIn: true` trong khi `claude -p` lỗi "OAuth session expired and could not be refreshed" (iMac-8GB, 2026-11-04).
- **Quyết định:** Thêm probe gọi prompt cực ngắn lúc bắt tay và 6 giờ/lần khi rảnh; thêm mẫu lỗi OAuth vào `auth_patterns`.

## DEC-36: Dự án sandbox + nút Chạy thử dây chuyền
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-09 · **Trạng thái:** CHỐT
- **Quyết định:** Repo `namth/agenthub-sandbox`, cờ `projects.is_sandbox`; owner bấm Chạy thử trên SCR-09 → yêu cầu mẫu tự duyệt, theo dõi SPEC → CODE → PR → QA, đóng PR/issue sau khi xong, lưu `smoke_runs`.

## DEC-37: Máy partner chỉ nhận việc dự án chủ máy là thành viên
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-09 · **Trạng thái:** CHỐT · Sửa DEC-15 ("NULL = mọi dự án" chỉ còn đúng với máy owner/hệ thống)
- **Bối cảnh:** máy partner mới nối không có `agent_projects` nên nhận việc mọi dự án, kể cả repo partner không được xem.
- **Quyết định:** Dispatcher lọc cứng theo `project_members` của chủ máy; mất tư cách thành viên → thu hồi assignment đang chạy (không tăng attempt).

## DEC-38: Không cấp secret cho máy partner; test cần secret chạy ở CI
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-09 · **Trạng thái:** CHỐT
- **Quyết định:** Runner không bao giờ nhận secret. Dự án bật `needs_ci_for_integration` → prompt dặn agent dựa vào CI gate (GitHub Actions giữ secret).
- **Phương án loại:** server mã hoá và cấp secret theo dự án.

## DEC-39: Telegram và MCP chỉ cho owner ở đợt này
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-10 · **Trạng thái:** CHỐT
- **Quyết định:** Bot chỉ nhận lệnh từ `TELEGRAM_ADMIN_CHAT_ID` (`/new`, `/tasks`, `/status`, Duyệt, Merge, trả lời làm rõ, xác nhận đề xuất huỷ). MCP `/mcp` (Streamable HTTP) với token cá nhân chỉ owner tạo được (SCR-12). Partner chỉ dùng web; mở rộng partner để sau.
- **Phương án loại:** liên kết Telegram từng người ngay đợt này; CLI `agenthub task …`.

## DEC-40: Web dùng được trên điện thoại; REPLAN/UNBLOCK có hành động
- **Ngày:** 2026-11-04 · **Tính năng:** FEAT-10 · **Trạng thái:** CHỐT · Sửa DEC-12 (REPLAN/UNBLOCK có schema kết quả)
- **Quyết định:** Màn chính dùng được ở 375px. `result.json` của REPLAN/UNBLOCK: `retry` | `replan` | `ask_user` | `cancel` (cancel chờ Admin xác nhận qua `lead_proposals`).

## DEC-41: Agent = một tài khoản CLI; một máy có nhiều agent cùng loại
- **Ngày:** 2026-11-05 · **Tính năng:** FEAT-11 · **Trạng thái:** CHỐT · Sửa ràng buộc `UNIQUE(worker_id, kind)` (FEAT-01)
- **Bối cảnh:** có máy chỉ có một loại CLI nhưng nhiều tài khoản; id cố định `<máy>/<kind>` chỉ cho 1 tài khoản/loại.
- **Quyết định:** id `<máy>/<name>`; thêm tài khoản bằng lệnh tại máy `agenthub account add <claude|antigravity>` (thư mục cấu hình riêng: `CLAUDE_CONFIG_DIR`, `HOME` cho agy), lưu ở `~/.agenthub/agents.json`. Mỗi agent một slot, hạn mức riêng.
- **Phương án loại:** nút thêm trên web (vẫn phải đăng nhập tại máy, thêm bước); cài nhiều runner trên một máy (tốn tài nguyên, văn phòng hiện như nhiều máy).

## DEC-42: Agent chỉ hiện sau lần đăng nhập OK đầu tiên
- **Ngày:** 2026-11-05 · **Tính năng:** FEAT-11 · **Trạng thái:** CHỐT · Sửa FEAT-06 (agent cài CLI nhưng chưa có tài khoản không còn hiện `NEEDS_AUTH`)
- **Quyết định:** cờ `ever_ok` (runner và server); runner chỉ báo agent `ever_ok`; server ẩn và không giao việc cho agent `ever_ok = 0`. `NEEDS_AUTH` đỏ chỉ dành cho agent từng OK rồi mất phiên.
- **Phương án loại:** hiện dòng xám "chưa đăng nhập" ở trang máy.

## DEC-43: Owner và chủ máy xoá agent trên web
- **Ngày:** 2026-11-05 · **Tính năng:** FEAT-11 · **Trạng thái:** CHỐT
- **Quyết định:** `DELETE /api/admin/agents/{id}` (owner mọi máy), `DELETE /api/my/machines/{id}/agents/{name}` (chủ máy); agent đang chạy việc → `409`. Server giữ tombstone `removed_at` và gửi `AGENT_REMOVE` để runner gỡ khỏi `agents.json`, nên không tự đăng ký lại; thêm lại thì `account add` sinh tên mới.
- **Phương án loại:** chỉ owner được xoá.

## DEC-44: Văn phòng ảo bản 3 — chỉ vẽ agent có thật, trưởng phòng quay mặt ra, phòng nghỉ có giường
- **Ngày:** 2026-11-05 · **Tính năng:** FEAT-02 bản 3 · **Trạng thái:** CHỐT · **Thay phần "tối thiểu 8 chỗ, ghế trống vẫn vẽ" của DEC-25**
- **Quyết định:** chỉ vẽ bàn cho agent đang có; chữ vẽ theo `devicePixelRatio` cho nét; bàn trưởng phòng đặt trên cùng, nhân vật quay mặt về phía người xem, đối diện các bàn nhân viên (nhân viên quay lưng về người xem, nhìn vào màn hình); tường sau: giữa là cửa sổ kính, logo INOVA chuyển sang mảng tường bên phải; mỗi chỗ ngồi gồm bàn gọn + màn hình + ghế; khu chỉ huy giãn cách bàn owner/partner; phòng nghỉ có giường, agent `COOLDOWN` nằm ngủ kèm đồng hồ đếm ngược.

## BACKLOG (chưa grill)
- **FEAT-03 Dây chuyền vai trò** (BA / Designer / Dev / Test): vai trò là loại job (`SPEC`, `DESIGN`, `TEST_WRITE`, `CODE`, `QA_REVIEW`), mẫu dây chuyền theo loại task, bảng ánh xạ vai trò → loại agent cấu hình được. Schema `jobs.kind` đã dành sẵn.
- ~~**FEAT-04 Đa dự án song song:**~~ → đã grill, xem DEC-14 → DEC-17 và `docs/features/multi-project.md`. Ghi chú gốc: GitHub App thay PAT, `project_members` (RBAC theo dự án), runner giữ clone theo repo, dispatcher chia lượt có trọng số + giới hạn đồng thời mỗi dự án, secrets/CI theo dự án; một lead trực toàn hệ thống. Cột `jobs.project_id` đã có. Gộp luôn việc thu hẹp scope OAuth của người dùng về `read:user`.
