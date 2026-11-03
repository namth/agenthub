# 01. TỔNG QUAN DỰ ÁN (PROJECT OVERVIEW)

---

## 1. TÊN DỰ ÁN & ĐỊNH DANH
* **Tên thương mại:** AgentHub
* **Tên mã kỹ thuật:** `agenthub`
* **Khẩu hiệu (Tagline):** Distributed Multi-Agent Software Engineering Studio (Studio kỹ thuật phần mềm đa Agent phân tán).

---

## 2. BỐI CẢNH & BÀI TOÁN CỐT LÕI (PROBLEM STATEMENT)
1. **Rào cản Token & Rate Limit của LLM:** Khi giao toàn bộ dự án cho một AI Agent duy nhất (như Claude Code), công việc nhanh chóng bị ngắt quãng do chạm trần rate limit và tràn context window.
2. **Chi phí điều phối thủ công (Coordination Tax):** Người dùng phải liên tục copy-paste mã nguồn, chỉ thị và ngữ cảnh giữa các phiên AI, giữa các tài khoản và giữa các máy tính khác nhau.
3. **Mô hình làm việc nhóm phân tán chưa có lời giải:** Thiếu một hệ thống điều phối cho phép Đối tác (Khách hàng) đề xuất tính năng, Trưởng nhóm (Product Owner) phê duyệt, và dàn máy AI phân tán tự động nhận việc, viết code, kiểm thử và mở Pull Request mà không xung đột mã nguồn.

---

## 3. MỤC TIÊU DỰ ÁN (GOALS & SUCCESS METRICS)
* **Tự động hóa chu trình khép kín:** Từ khâu tiếp nhận ý tưởng -> lập kế hoạch kiến trúc -> phân rã việc -> thi công song song trên nhiều máy vật lý -> kiểm thử chất lượng -> sẵn sàng xuất bản.
* **Tối ưu tài nguyên cực đại (Ultra-lightweight):** 
  - Toàn bộ máy chủ điều phối được đóng gói thành **1 file nhị phân duy nhất (Single Binary)** viết bằng Golang.
  - Sử dụng **SQLite nhúng**, không cần Docker/Postgres.
  - Chiếm dưới **30MB RAM** khi vận hành.
* **Tách biệt vai trò thông minh:**
  - **Claude Code:** Làm Kiến trúc sư trưởng (Tech Lead) và Trưởng ban Kiểm định (QA/Code Reviewer).
  - **Dàn máy Antigravity:** Làm Kỹ sư thi công (Engineers) giải quyết khối lượng code lớn song song.
* **An toàn & Minh bạch mã nguồn:** Dùng **Git Worktrees** cô lập hoàn toàn môi trường làm việc trên từng máy con; lấy **GitHub Issues & Pull Requests** làm chân lý dữ liệu duy nhất.

---

## 4. ĐỐI TƯỢNG SỬ DỤNG (TARGET PERSONAS)
1. **Bạn (Product Owner / Lead Engineer):** 
   - Quản trị toàn bộ cụm máy và dự án.
   - Phê duyệt các yêu cầu tính năng từ đối tác.
   - Giám sát live logs terminal của các máy con và bấm Merge vào nhánh chính `main`.
2. **Đối tác (Partner / Stakeholder):**
   - Đưa ra yêu cầu tính năng qua giao diện Web thân thiện hoặc tạo Issue trên GitHub.
   - Theo dõi tiến độ trực quan theo thời gian thực (Kanban) mà không cần hiểu sâu về kỹ thuật.
3. **AI Agents (Autonomous Workers):**
   - **Claude Code:** Nhận lệnh qua CLI headless để lập kế hoạch hoặc review PR.
   - **Antigravity Instances:** Nhận task qua CLI headless để viết code trên branch riêng.
