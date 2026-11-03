# 12. QUY TẮC NGHIỆP VỤ & MÁY TRẠNG THÁI (BUSINESS RULES)

---

## 1. MÁY TRẠNG THÁI CỦA TÁC VỤ (TASK STATE MACHINE)

Mỗi tác vụ (Task/Issue) trong hệ thống đều trải qua một vòng đời nghiêm ngặt và không được phép nhảy cóc trạng thái:

```mermaid
stateDiagram-v2
    [*] --> PENDING_APPROVAL: Đối tác tạo yêu cầu
    
    PENDING_APPROVAL --> PLANNING: Bạn bấm "Duyệt" (Approve)
    PENDING_APPROVAL --> REJECTED: Bạn từ chối (Reject)
    
    PLANNING --> READY: Claude Code bẻ nhỏ xong thành các Sub-tasks
    
    READY --> IN_PROGRESS: Máy Antigravity nhận việc (Assign)
    
    IN_PROGRESS --> WAITING_QA: Antigravity mở Pull Request
    IN_PROGRESS --> READY: Máy mất kết nối 120s / Agent hết limit (giao lại, attempt+1)
    IN_PROGRESS --> PLANNING: Vượt MAX_ATTEMPTS (REPLAN cho lead)
    
    WAITING_QA --> IN_PROGRESS: CI Failed hoặc Claude yêu cầu sửa đổi
    WAITING_QA --> QA_PASSED: CI Pass & Claude Code xác nhận đạt
    
    QA_PASSED --> DONE: Bạn bấm "Merge" vào main
    
    REJECTED --> [*]
    DONE --> [*]
```

### Bảng Điều kiện Chuyển trạng thái (Transition Guards)

| Trạng thái Nguồn | Trạng thái Đích | Người/Tác nhân kích hoạt | Điều kiện kiểm tra (Guard Condition) |
| :--- | :--- | :--- | :--- |
| `PENDING_APPROVAL` | `PLANNING` | Bạn (Admin) | Chỉ Admin mới có quyền duyệt; Issue phải có tiêu đề và mô tả hợp lệ. |
| `PENDING_APPROVAL` | `REJECTED` | Bạn (Admin) | Phải kèm theo lý do từ chối để thông báo cho đối tác. |
| `PLANNING` | `READY` | Claude Code (Runner) | Phải tạo thành công ít nhất 1 Sub-issue trên GitHub kèm checklist DoD. |
| `READY` | `IN_PROGRESS` | Task Dispatcher | Có ít nhất 1 agent `ANTIGRAVITY` ở trạng thái `IDLE` trên máy `ONLINE`. |
| `IN_PROGRESS` | `WAITING_QA` | Antigravity (Runner) | Đã push code lên branch và mở thành công PR trên GitHub. |
| `IN_PROGRESS` | `READY` | Hệ thống (Timeout Engine) | Máy `OFFLINE` liên tục 120s (BR-13), hoặc agent gửi `AGENT_EXHAUSTED` hợp lệ (BR-08). Không có log 15 phút **không** thu hồi mà sinh job `UNBLOCK` (BR-16). |
| `IN_PROGRESS` | `PLANNING` | Hệ thống | `attempt > MAX_ATTEMPTS` (3) → job `REPLAN` cho lead (BR-15). |
| `WAITING_QA` | `QA_PASSED` | Claude Code (QA Lead) | GitHub Actions CI trả về `success` và Claude Code gắn tag review hợp lệ. |
| `QA_PASSED` | `DONE` | Bạn (Admin) | PR đã được gộp (Merged) thành công vào nhánh chính `main`. |

---

## 2. QUY TẮC RÀNG BUỘC CỦA CỤM MÁY CON (WORKER FLEET RULES)

1. **Ràng buộc Một việc tại một thời điểm (One-Job-Per-Agent):** Mỗi agent (một CLI trên một máy) chỉ có tối đa 1 job đang chạy (DEC-04, BR-17). Một máy có cả Claude và Antigravity có thể chạy song song 2 job.
2. **Cơ chế Thu hồi Tác vụ (Task Reclaim & Failover):**
   - 45 giây không có `PING` → máy `OFFLINE` (chỉ hiển thị, chưa thu hồi).
   - `OFFLINE` liên tục 120 giây → mọi assignment đang chạy của máy thành `RECLAIMED`, job về hàng đợi với `attempt + 1`; máy rảnh khác nhận và làm tiếp từ commit đã push gần nhất.
   - Máy cũ kết nối lại và gửi bản tin mang `assignment_id` cũ → bị từ chối `STALE_ASSIGNMENT` (BR-04).
3. **Quy tắc Dọn dẹp Worktree (Clean-up Guarantee):**
   - Trước khi nhận task mới, Runner bắt buộc phải chạy lệnh kiểm tra dọn dẹp các thư mục worktree cũ để tránh rò rỉ dung lượng ổ cứng.
4. **Quy tắc trưởng phòng trực & hết limit:** BR-01 → BR-18 ở [features/lead-rotation.md](features/lead-rotation.md) mục 7 (lease, epoch, chọn lead, trống lead, hạn mềm, cooldown, bàn giao WIP).
5. **Quy tắc văn phòng ảo:** BR-19 → BR-23 ở [features/virtual-office.md](features/virtual-office.md) mục 7.
