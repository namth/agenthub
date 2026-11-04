# Báo cáo TASK-020

## Đã làm
- Hiện thực hoá package `internal/projects`:
  - `GitHub` interface và HTTP client (`NewGitHub`, `ListRepos`, `CreateWebhook`, `Permission`) giao tiếp với GitHub API bằng Bearer token.
  - `Service` quản lý dự án (`New`, `IsOwner`, `AddProject`, `SyncUser`, `RoleIn`).
  - Hỗ trợ thêm dự án: kiểm tra định dạng `owner/repo`, trùng lặp `CONFLICT`, repo tồn tại trong `ListRepos`, tự động tạo webhook GitHub cho dự án (`url = BaseURL + "/api/webhooks/github"`), nếu webhook lỗi vẫn lưu dự án với `webhook_ok = false`, đồng bộ vai trò cho người dùng hiện có.
  - Phân quyền theo dự án: Owner luôn có quyền ADMIN ở mọi dự án; thành viên khác tra quyền collaborator qua GitHub (`admin|maintain` -> `ADMIN`, còn lại -> `PARTNER`, 404 / `ErrNotCollaborator` -> xoá khỏi `project_members`).
  - HTTP handlers (`Handler`): `GET /api/projects`, `GET /api/admin/github/repos`, `POST /api/admin/projects`, `PATCH /api/admin/projects/{id}` với xác thực đăng nhập và kiểm tra quyền owner cho `/api/admin/*`, hỗ trợ chuẩn JSON docs/07.
- Mở rộng `internal/auth`:
  - Thêm `Owner` và `OnLogin` callback vào `auth.Config`.
  - Khi `OnLogin != nil`: chạy ở chế độ đa dự án, không tra quyền repo cố định, gán ADMIN nếu login khớp `Owner` (không phân biệt hoa thường) ngược lại PARTNER, gọi `OnLogin`, nếu không phải owner và thuộc 0 dự án thì trả về 403 FORBIDDEN không đặt cookie phiên.
  - Khi `OnLogin == nil`: giữ nguyên hoàn toàn hành vi cũ.

## File đã tạo/sửa
- `internal/projects/github.go`: Client GitHub API (`ListRepos`, `CreateWebhook`, `Permission`).
- `internal/projects/service.go`: Logic nghiệp vụ dự án, đồng bộ vai trò `SyncUser`, kiểm tra vai `RoleIn`, thêm dự án `AddProject`.
- `internal/projects/handler.go`: HTTP endpoints cho dự án và admin quản lý dự án/repo.
- `internal/auth/auth.go`: Thêm `Owner` và `OnLogin` trong `Config` cùng luồng callback hỗ trợ chế độ đa dự án.

## Kết quả tự kiểm
```
$ bash handoff/tasks/TASK-020-projects/verify.sh
--- PASS: TestSyncUserRolesPerProject (0.10s)
--- PASS: TestAddProjectCreatesWebhookAndMembers (0.09s)
--- PASS: TestAddProjectWebhookFailureStillSaves (0.10s)
--- PASS: TestAddProjectErrors (0.10s)
--- PASS: TestAdminEndpointsOwnerOnly (0.10s)
--- PASS: TestListReposMarksAdded (0.12s)
--- PASS: TestListProjectsByMembership (0.15s)
--- PASS: TestPatchProject (0.11s)
--- PASS: TestGitHubClientRequests (0.00s)
ok  	github.com/namth/agenthub/internal/projects	2.535s
--- PASS: TestLoginRedirectsToGitHub (0.10s)
--- PASS: TestCallbackAdminCreatesUserAndSession (0.09s)
--- PASS: TestRoleMappingDEC09 (0.48s)
--- PASS: TestRoleUpdatedOnNextLogin (0.10s)
--- PASS: TestNonCollaboratorRejected (0.13s)
--- PASS: TestCallbackRejectsBadStateOrCode (0.14s)
--- PASS: TestSecureCookieOnHTTPS (0.10s)
--- PASS: TestMeRejectsMissingTamperedExpired (0.10s)
--- PASS: TestLogoutClearsSession (0.10s)
--- PASS: TestRequireRole (0.10s)
--- PASS: TestMultiProjectLoginOwnerIsAdmin (0.10s)
--- PASS: TestMultiProjectLoginPartnerNeedsMembership (0.10s)
ok  	github.com/namth/agenthub/internal/auth	3.020s
TASK-020 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không
