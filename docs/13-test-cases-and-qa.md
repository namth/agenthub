# 13. KẾ HOẠCH KIỂM THỬ TOÀN DIỆN & TIÊU CHUẨN CHẤT LƯỢNG (TEST CASES & QA)

---

## 1. MA TRẬN TEST CASES CHI TIẾT (TEST SUITE MATRIX)

### 1.1 Kiểm thử Backend & WebSocket (Unit & Integration Tests)
| Mã Test | Phân hệ kiểm thử | Kịch bản kiểm thử (Test Scenario) | Kết quả kỳ vọng (Expected Output) | Lệnh chạy kiểm tra |
| :--- | :--- | :--- | :--- | :--- |
| **TC-01** | Database | Khởi tạo bảng SQLite lần đầu | Tạo đủ mọi bảng trong `schema.sql` (gồm bảng FEAT-01/02), index và không báo lỗi syntax | `go test ./internal/database/...` |
| **TC-02** | WebSocket Hub | 10 máy con kết nối đồng thời | Cả 10 máy nhận bắt tay `HANDSHAKE_OK` | `go test ./internal/gateway/...` |
| **TC-03** | Heartbeat | Máy con ngắt kết nối quá 45 giây | Trạng thái máy tự đổi thành `OFFLINE` | `go test ./internal/gateway -run TestHeartbeat` |
| **TC-04** | Dispatcher | Phân job theo `required_agent` | Job SPEC/QA_REVIEW vào agent Claude, job CODE vào agent Antigravity | `go test ./internal/dispatcher/...` |
| **TC-05** | GitHub Webhook | Giả lập Webhook với chữ ký HMAC sai | Trả về mã lỗi `401 Unauthorized` | `go test ./internal/github -run TestWebhookSig` |

### 1.2 Kiểm thử Runner & Git Worktree (Client Tests)
| Mã Test | Phân hệ kiểm thử | Kịch bản kiểm thử (Test Scenario) | Kết quả kỳ vọng |
| :--- | :--- | :--- | :--- |
| **TC-06** | Git Isolation | Runner tạo worktree cho Task #102 | Thư mục `.worktrees/task-102` xuất hiện độc lập, nhánh `feat/task-102` được checkout |
| **TC-07** | Log Streaming | AI in ra chuỗi màu ANSI | Server nhận được log chunk trong vòng < 200ms |
| **TC-08** | Task Cleanup | Task hoàn thành và PR được tạo | Thư mục worktree bị xóa sạch sẽ, `git worktree list` không còn nhánh đó |

### 1.3 Kiểm thử Phân quyền (RBAC Security Tests)
| Mã Test | Phân hệ kiểm thử | Kịch bản kiểm thử | Kết quả kỳ vọng |
| :--- | :--- | :--- | :--- |
| **TC-09** | Quyền Đối tác | Đối tác gọi API `POST /api/tasks/1/approve` | Trả về `403 Forbidden` (Chỉ Admin mới được duyệt) |
| **TC-10** | Dữ liệu nhạy cảm | Đối tác gọi `GET /api/fleet` | Trả về `403 Forbidden` hoặc dữ liệu rỗng |

### 1.4 Chuyển giao & Trưởng phòng trực (FEAT-01)
TC-11 → TC-27: xem [features/lead-rotation.md](features/lead-rotation.md) mục 8 (lease, fencing, trống lead, thu hồi 45s/120s, hạn mềm, giả mạo agent, RBAC, E2E SCR-05).

### 1.5 Văn phòng ảo (FEAT-02)
TC-28 → TC-35: xem [features/virtual-office.md](features/virtual-office.md) mục 8 (chuyển sự kiện, che bí mật, RBAC, bật/tắt, nối lại SSE, bố cục động).

---

## 2. QUY TRÌNH KIỂM ĐỊNH KÉP (2-LAYER QA GATE) KHI MERGE CODE

Trước khi bất kỳ một Pull Request nào được phép gộp vào `main`, hệ thống tự động thực thi 2 chốt chặn:

```
                  ┌──────────────────────────────┐
                  │    Worker tạo Pull Request   │
                  └──────────────┬───────────────┘
                                 │
                                 ▼
         [ CHỐT CHẶN 1: GITHUB ACTIONS CI ]
         - Build kiểm tra biên dịch (go build / npm build)
         - Chạy toàn bộ Unit Tests & Linter
                                 │
                   ┌─────────────┴─────────────┐
                   │                           │
              ❌ THẤT BẠI                  ✅ THÀNH CÔNG
                   │                           │
                   ▼                           ▼
        Báo Worker sửa lỗi         [ CHỐT CHẶN 2: CLAUDE AI CODE REVIEW ]
                                   - Kiểm tra Clean Code & Kiến trúc
                                   - Kiểm tra bảo mật (SQL Injection, Secrets)
                                   - Kiểm tra Edge cases & Logic nghiệp vụ
                                               │
                                 ┌─────────────┴─────────────┐
                                 │                           │
                            ❌ CẦN SỬA                  ✅ ĐẠT YÊU CẦU
                                 │                           │
                                 ▼                           ▼
                        Comment yêu cầu sửa      Gắn nhãn `qa-passed`
                                                 & Bắn Telegram cho Bạn
                                                             │
                                                             ▼
                                                    [ BẠN BẤM MERGE VÀO MAIN ]
```

---

## 3. TIÊU CHUẨN CHẤT LƯỢNG MÃ NGUỒN CỦA CLAUDE AI REVIEW

Claude Code sẽ đánh giá PR dựa trên thang điểm 10 với checklist sau:
1. **Kiến trúc & Tính nhất quán (Weight: 30%):** Code có đặt đúng thư mục quy định không? Có tuân thủ naming convention không?
2. **Độ tin cậy & Test kèm theo (Weight: 30%):** Code mới có viết kèm Unit Test tương ứng không? Các trường hợp biên (null, empty, error) đã được xử lý chưa?
3. **Bảo mật & Hiệu năng (Weight: 20%):** Có hardcode mật khẩu/token không? Truy vấn database có an toàn không? Có nguy cơ rò rỉ bộ nhớ (goroutine leak) không?
4. **Độ sạch & Chú thích (Weight: 20%):** Code có dễ đọc không? Chú thích bằng tiếng Việt rõ ràng ở các hàm phức tạp.

### 1.6 Đa dự án & thêm máy (FEAT-04, FEAT-05)
TC-36 → TC-56: xem [features/multi-project.md](features/multi-project.md) §8 và [features/join-machine.md](features/join-machine.md) §8.
