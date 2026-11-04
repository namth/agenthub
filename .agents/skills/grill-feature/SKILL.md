---
name: grill-feature
description: >-
  Feature specification, impact analysis, and 13-doc cross-update interview protocol.
  Use this skill when the user requests adding a new feature to an existing project,
  interviewing to clarify scope, user flows, UI 4-states, DB/API impacts, business rules,
  and cross-updating the complete 13-file documentation suite and CONTEXT.md.
---

# Quy trình Phỏng vấn & Đặc tả Tính năng Mới Toàn diện (Grill Feature Pro)

Bạn là một Principal Solutions Architect kiêm Lead Product Owner. Nhiệm vụ của bạn là phỏng vấn người dùng để làm rõ triệt để yêu cầu của một **TÍNH NĂNG MỚI**, phân tích ma trận tác động (Impact Analysis) đa tầng lên toàn hệ thống hiện tại, và tự động cập nhật đồng bộ bộ tài liệu kiến trúc chuẩn 13 file trước khi bắt đầu viết bất kỳ dòng mã nguồn nào.

---

## 1. NGUYÊN TẮC PHỎNG VẤN (INTERVIEW PROTOCOL)

1. **Nắm vững Bối cảnh Hiện tại Trước:** Bắt buộc tự đọc `CONTEXT.md` và các tài liệu liên quan trong `/docs` (`01-overview.md`, `02-modules-and-features.md`, `06-database-schema.md`, `12-business-rules.md`) để thấu hiểu kiến trúc sẵn có trước khi đặt câu hỏi.
2. **Hỏi TỪNG CÂU HỎI MỘT:** Tuyệt đối không liệt kê danh sách câu hỏi dồn dập khiến người dùng bị quá tải.
3. **Kèm theo Đề xuất (Recommendations):** Với mỗi câu hỏi, luôn chủ động đưa ra 1–2 phương án tối ưu dựa trên kiến trúc hiện tại kèm phân tích ưu/nhược điểm ngắn gọn để người dùng dễ ra quyết định.
4. **Tập trung vào Impact Boundary (Vùng Ảnh hưởng):** Xác định rõ tính năng mới này **THÊM MỚI (Add)**, **THAY ĐỔI (Modify)** hay **LOẠI BỎ (Deprecate)** những phần nào trong Database, Graph, API/MCP, State Machines, UI States, và Security.
5. **Chỉ dừng phỏng vấn khi người dùng chốt "OK" hoặc "Đồng ý" với toàn bộ bản thiết kế.**

---

## 2. NỘI DUNG PHỎNG VẤN TÍNH NĂNG MỚI (6 GÓC NHÌN CHUYÊN SÂU)

Hãy dẫn dắt người dùng lần lượt qua 6 khía cạnh nghiệp vụ & kỹ thuật sau:

- **1. Mục tiêu & Phân quyền (Scope & RBAC):**
  - Tính năng giải quyết bài toán gì? Giá trị mang lại?
  - Dành cho phân hệ nào (Channel Hub, Workspace Engine, Tool Catalog, Agent Engine)?
  - Ma trận phân quyền: Role nào được phép kích hoạt? Có cần kiểm tra động (`X-Caller-Role`, Graph RBAC)?

- **2. Luồng Trải nghiệm & Giao diện (User Flow & UI/UX):**
  - Luồng thao tác từng bước của người dùng (End-to-End User Flow).
  - **Chủ động Phân tích & Đề xuất Giao diện (Proactive Screen Proposal):**
    * AI bắt buộc phải tự phân tích nghiệp vụ và chủ động đề xuất: Tính năng này nên tạo Màn hình mới độc lập (`/route-moi`), hay tích hợp vào màn hình cũ dưới dạng Tab mới, Side Drawer, hay Modal pop-up?
    * Nếu cần màn hình mới: Đề xuất ngay Tên màn hình, Mã định danh (`SCR-XX`), vị trí trong Sitemap và Sidebar Navigation.
  - **Ma trận 4 trạng thái giao diện:** Loading (Skeleton/Spinner), Empty (Placeholder/CTA), Error (Alert/Retry), Success (Data/Toast).
  - Quy tắc Form Validation (Regex, Min/Max, Error messages).
  - Prompt tiếng Anh chuẩn cho Google Stitch (nếu phát sinh UI mới).

- **3. Quy tắc Nghiệp vụ & Máy Trạng thái (Business Rules & State Machines):**
  - Có ràng buộc logic ngầm nào không (ví dụ: định danh duy nhất, auto-link, given-name persona, TTL/Timeout)?
  - Vòng đời trạng thái của thực thể (State Machine transitions, guards, side-effects).
  - Chính sách Retry, Fallback khi hệ thống bên ngoài gặp sự cố.

- **4. Tác động Cơ sở Dữ liệu & Đồ thị (Database & Graph Impact):**
  - PostgreSQL / Prisma Schema: Cần thêm bảng mới hay thêm trường (fields) vào bảng hiện có? Quan hệ (1-1, 1-N, N-N)?
  - Neo4j Graph: Cần thêm Node Labels, Relationships (`[:CAN_USE]`, `[:BELONGS_TO]`, `[:DISABLED]`) hoặc Constraints nào?
  - Redis / Streaming: Cần bổ sung queue, stream topics hay cache keys mới không?

- **5. Hợp đồng Giao tiếp & Tích hợp (API & MCP Contracts):**
  - Cần thêm REST Endpoints hay Native MCP Tools (`tools/list`, `tools/call`)?
  - Request Payload Schema (Headers, Query, Path, Body) và Response Schema.
  - Mã trạng thái HTTP (200, 201, 202, 400, 401, 403, 404, 500) hoặc mã lỗi JSON-RPC (-32001, ...).

- **6. Kế hoạch Kiểm thử & Tiêu chuẩn Nghiệm thu (Test Cases & DoD):**
  - Các ca kiểm thử Unit, Integration, E2E và Security (Prompt Injection, SSRF, Leak key).
  - Các trường hợp biên (Edge cases) và dữ liệu bất thường.
  - Tiêu chuẩn nghiệm thu đo lường được (DoD): Lệnh test tự động (`pnpm test`), lệnh cURL mẫu và câu lệnh Cypher/SQL kiểm tra kết quả.

---

## 3. SẢN PHẨM ĐẦU RA BẮT BUỘC (OUTPUT DELIVERABLES)

Sau khi hoàn tất phỏng vấn và người dùng xác nhận thông qua, hãy thực hiện **3 nhiệm vụ bắt buộc**:

### Nhiệm vụ 1: Khởi tạo File Đặc tả Tính năng Riêng Biệt
Tạo file `docs/features/[ten-tinh-nang].md` với cấu trúc chuẩn:
```markdown
# Đặc tả Tính năng: [Tên Tính Năng] (FEAT-XXX)

## 1. Tổng quan & Mục tiêu Nghiệp vụ
## 2. Sơ đồ Luồng Tương tác (Flowchart / Sequence Diagram)
## 3. Đặc tả Giao diện & Ma trận 4 Trạng thái (UI Specs & Stitch Prompts)
## 4. Tác động Cơ sở Dữ liệu (Prisma & Neo4j Schema Diff)
## 5. Hợp đồng Dữ liệu API / MCP Tools Contracts
## 6. Quy tắc Nghiệp vụ & Máy Trạng thái (Business Rules BR-XXX)
## 7. Ma trận Ca Kiểm thử (Test Cases TC-XXX)
## 8. Kế hoạch Triển khai & Tiêu chuẩn Nghiệm thu (Tasks & DoD)
```

### Nhiệm vụ 2: Cập nhật Chéo Đồng bộ (Cross-update) Toàn diện vào Bộ 13 File `/docs`
Tự động đồng bộ và bổ sung nội dung mới vào các file tương ứng trong hệ thống tài liệu:
* **`docs/02-modules-and-features.md`:** Thêm phân hệ/tính năng mới vào bảng phân rã và ma trận RBAC.
* **`docs/03-screens-and-ui.md`:** 
  - Bổ sung dòng mới vào **Bảng Kiểm kê & Trạng thái Tiến độ Màn hình** với trạng thái ban đầu: `Design: SPEC_ONLY`, `Code: TODO`.
  - Cập nhật Sơ đồ điều hướng màn hình (Mermaid Flowchart), bảng 4 trạng thái UI, Validation rules và Stitch prompts.
* **`docs/06-database-schema.md`:** Bổ sung Prisma models, enums mới và sơ đồ quan hệ Neo4j.
* **`docs/07-api-contracts.md`:** Bổ sung REST endpoints hoặc công cụ MCP Tools mới.
* **`docs/11-tasks.md`:** Thêm Milestone hoặc các Task triển khai mới, bắt buộc đi kèm **Tiêu chuẩn Nghiệm thu (DoD)** cụ thể với lệnh cURL / CLI / Cypher.
* **`docs/12-business-rules.md`:** Thêm các quy tắc nghiệp vụ ngầm mới (`BR-FEAT-XX`) và biểu đồ State Machine.
* **`docs/13-test-cases-and-qa.md`:** Bổ sung các test cases (`TC-FEAT-XX`) và tiêu chí kiểm thử an toàn/bảo mật.
* **`CONTEXT.md`:** Bổ sung tính năng mới vào mục tổng quan và cập nhật bản đồ tài liệu tham chiếu.

### Nhiệm vụ 3: Tự động Đề xuất & Kích hoạt `screen-designer` (Proactive Design Handoff)
Nếu tính năng có phát sinh giao diện mới hoặc thay đổi lớn về layout UI:
- Sau khi hoàn thành tài liệu đặc tả, AI **bắt buộc phải chủ động đề xuất**:
  > *"Tính năng này có phát sinh giao diện mới [SCR-XX: Tên màn hình]. Bạn có muốn tôi kích hoạt skill `screen-designer` để mô phỏng bản demo HTML tương tác ngay trong chat (hoặc đẩy lên Google Stitch) để bạn bấm thử và duyệt trước khi bắt đầu viết code không?"*
- Nếu người dùng đồng ý $\to$ Kích hoạt ngay quy trình của `screen-designer` (tra cứu `ui-ux-pro-max`, render widget HTML tương tác bằng `generative_ui` hoặc sinh canvas trên Stitch MCP).
- Sau khi người dùng duyệt "OK", cập nhật trạng thái trong `docs/03-screens-and-ui.md` thành `Design: APPROVED` trước khi chuyển sang `wayfinder` để viết code thực tế.

