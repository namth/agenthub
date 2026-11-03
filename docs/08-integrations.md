# 08. TÀI LIỆU TÍCH HỢP BÊN THỨ BA (THIRD-PARTY INTEGRATIONS)

---

## 1. TÍCH HỢP GITHUB (OAUTH, REST API & WEBHOOKS)

GitHub đóng vai trò là nguồn dữ liệu gốc (Single Source of Truth) của toàn bộ hệ thống.

### 1.1 GitHub OAuth App
* **Cấu hình trên GitHub Developer Settings:**
  - Homepage URL: `https://your-agenthub-domain.com`
  - Authorization Callback URL: `https://your-agenthub-domain.com/api/auth/callback`
  - Scopes yêu cầu: `read:user`, `repo` (để đọc và tạo Issue, PR).
* **Quy tắc phân vai:**
  Khi người dùng đăng nhập, Go Server gọi GitHub API `GET /repos/{owner}/{repo}/collaborators/{username}/permission`:
  - `admin` hoặc `maintain` ➡️ Phân vai `ADMIN`.
  - `write`, `triage` hoặc `read` ➡️ Phân vai `PARTNER` (DEC-09).

### 1.2 GitHub Webhook Handlers
Cấu hình Webhook lắng nghe tại endpoint: `POST /api/webhooks/github` (kèm Webhook Secret xác thực qua HMAC-SHA256).
* **Sự kiện `issues` (opened, closed, labeled):**
  - Khi ai đó tạo Issue trực tiếp trên GitHub: Tự động sync vào bảng `tasks` của AgentHub.
* **Sự kiện `pull_request` (opened, closed, review_requested):**
  - Khi PR được mở bởi Runner: Kích hoạt GitHub Actions CI.
* **Sự kiện `workflow_run` (completed):**
  - Khi GitHub Actions CI chạy xong: Nếu kết quả `success` ➡️ Báo Go Server kích hoạt Claude Code vào review. Nếu `failure` ➡️ Báo lại cho Worker sửa lỗi.

---

## 2. TÍCH HỢP TELEGRAM BOT (ALERTS & 1-CLICK ACTIONS)

Telegram Bot đóng vai trò là trợ lý di động cho Bạn (Product Owner).

### 2.1 Cấu hình Bot
* Tạo Bot qua `@BotFather`, lấy `TELEGRAM_BOT_TOKEN`.
* Cấu hình biến môi trường:
  ```env
  TELEGRAM_BOT_TOKEN=123456789:ABCdefGHIjklMNOpqrSTUvwxYZ
  TELEGRAM_ADMIN_CHAT_ID=987654321
  ```

### 2.2 Định dạng Tin nhắn & Nút bấm Tương tác (Inline Keyboards)

**Tin nhắn 1: Khi có Yêu cầu mới từ Đối tác:**
```text
🔔 YÊU CẦU TÍNH NĂNG MỚI #105
Người yêu cầu: @doi-tac-a
Tiêu đề: Tích hợp xuất file báo cáo ra PDF
Mô tả: Khách hàng cần xuất bảng tiến độ thành file PDF...

[ ✅ Phê duyệt & Lập kế hoạch ]   [ ❌ Từ chối ]
```
* Bấm `Phê duyệt`: Callback data `task_approve:105` ➡️ Go Server cập nhật task thành `PLANNING` và gửi lệnh cho Claude Code ngay lập tức.

**Tin nhắn 2: Khi PR đã qua 2 vòng kiểm định (QA Passed):**
```text
🎉 PR ĐÃ SẴN SÀNG MERGE!
Task: #105 - Xuất file PDF
Pull Request: #18 (Branch: feat/task-105)
Kiểm định CI: ✅ PASSED
Claude AI Review: ✅ PASSED (Điểm chất lượng: 9.5/10)

[ 🚀 Merge vào Main ngay ]   [ 🔍 Xem chi tiết trên Web ]
```

**Tin nhắn 3–5: Trưởng phòng & chuyển giao (FEAT-01):**
```text
👑 ĐỔI TRƯỞNG PHÒNG (epoch 8)
mac-mini/claude ➡️ laptop/claude · Lý do: hết limit, reset 21:40
```
```text
⚠️ CHƯA CÓ TRƯỞNG PHÒNG
Mọi máy Claude đang nghỉ/mất kết nối. Việc code vẫn chạy; 3 việc lập kế hoạch/review đang chờ.
Dự kiến có trưởng phòng lại lúc 21:40.
```
```text
🔁 TASK #102 THẤT BẠI 4 LẦN
Đã chuyển về lập lại kế hoạch (REPLAN) cho trưởng phòng.
[ 🔍 Xem chi tiết trên Web ]
```
* Mỗi sự kiện vào/thoát trạng thái trống lead chỉ gửi **một** tin (không gửi lặp mỗi lần ticker chạy).
