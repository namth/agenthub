# Báo cáo TASK-005

## Đã làm
- Hiện thực hoá cấu trúc dữ liệu `User`, `Config`, `Service` cho package `auth`.
- Cung cấp constructor `New(db *sql.DB, cfg Config) *Service` với các giá trị mặc định cho `GitHubWebURL`, `GitHubAPIURL`, `SessionTTL` (7 ngày), `HTTPClient` (timeout 10s), `Now` (`time.Now`).
- `GET /api/auth/github`: sinh `state` ngẫu nhiên bằng `crypto/rand` (32 ký tự hex), đặt cookie `agenthub_oauth_state` (HttpOnly, SameSite=Lax, Secure nếu `BaseURL` là https, MaxAge 10 phút), chuyển hướng 302 đến `{web}/login/oauth/authorize` với query params `client_id`, `redirect_uri={BaseURL}/api/auth/callback`, `scope=read:user`, `state`.
- `GET /api/auth/callback`:
  - Xác thực `state` trong query so với cookie `agenthub_oauth_state` qua so sánh thời gian không đổi `subtle.ConstantTimeCompare`, trả về 400 `VALIDATION_ERROR` nếu thiếu hoặc không khớp.
  - Đổi authorization `code` lấy user access token qua `POST {web}/login/oauth/access_token`, trả về 401 `UNAUTHORIZED` nếu thất bại hoặc code không hợp lệ.
  - Lấy thông tin user profile qua `GET {api}/user` với header `Authorization: Bearer <user_token>`.
  - Tra quyền collaborator qua `GET {api}/repos/{owner}/{repo}/collaborators/{login}/permission` bằng `ServerToken` (`Authorization: Bearer <ServerToken>`). Trả về 403 `FORBIDDEN` nếu là 404 hoặc thất bại, không tạo user.
  - Ánh xạ vai trò chuẩn DEC-09: `admin` hoặc `maintain` phân vai `ADMIN`, còn lại phân vai `PARTNER`.
  - Upsert vào bảng `users` theo `github_id`: sinh UUID mới nếu tạo mới; cập nhật `username`, `avatar_url`, `role` nếu user đã tồn tại.
  - Sinh JWT HS256 với claims (`sub`=user.id, `role`, `username`, `iat`, `exp`=Now+TTL), đặt cookie phiên `agenthub_session` (HttpOnly, SameSite=Lax, Path=/, Secure nếu https), xoá cookie state và chuyển hướng 302 về `/`.
- `GET /api/auth/me`: đọc lại user từ DB theo `sub` (claims.Subject), trả về 200 `{id, username, avatar_url, role}`; trả về 401 `UNAUTHORIZED` nếu thiếu cookie, cookie sai chữ ký, sai thuật toán (`alg=none`), hoặc hết hạn theo `Now`.
- `POST /api/auth/logout`: xoá cookie `agenthub_session` (MaxAge âm), trả về 200 `{"success":true,"data":null,"error":null}`.
- `Middleware(next http.Handler)`: giải mã cookie phiên `agenthub_session`, kiểm tra thuật toán HS256 và hạn dùng theo `Now`, truy vấn thông tin user mới nhất từ database và gắn vào context qua `WithUser`. Không chặn request nếu cookie thiếu hoặc không hợp lệ.
- `UserFrom(ctx context.Context) (User, bool)`: lấy thông tin `User` từ context.
- `RequireRole(roles ...string)`: middleware kiểm tra vai trò người dùng trong context; trả về 401 `UNAUTHORIZED` nếu chưa đăng nhập, 403 `FORBIDDEN` nếu vai trò không nằm trong danh sách được phép.

## File đã tạo/sửa
- `internal/auth/auth.go`: Hiện thực toàn bộ luồng GitHub OAuth, JWT session, DB upsert, context helper, Service routes và RBAC middleware theo đặc tả.
- `go.mod`, `go.sum`: Thêm dependency `github.com/golang-jwt/jwt/v5`.

## Kết quả tự kiểm
```text
$ bash handoff/tasks/TASK-005-auth-github/verify.sh
== 1. gofmt + vet
== 2. Không log bí mật
== 3. Test auth (-race)
--- PASS: TestLoginRedirectsToGitHub (0.05s)
--- PASS: TestCallbackAdminCreatesUserAndSession (0.04s)
--- PASS: TestRoleMappingDEC09 (0.20s)
--- PASS: TestRoleUpdatedOnNextLogin (0.04s)
--- PASS: TestNonCollaboratorRejected (0.04s)
--- PASS: TestCallbackRejectsBadStateOrCode (0.04s)
--- PASS: TestSecureCookieOnHTTPS (0.04s)
--- PASS: TestMeRejectsMissingTamperedExpired (0.04s)
--- PASS: TestLogoutClearsSession (0.04s)
--- PASS: TestRequireRole (0.04s)
ok  	github.com/namth/agenthub/internal/auth	2.016s
== 4. Không phá test cũ
ok  	github.com/namth/agenthub/internal/auth	0.841s
ok  	github.com/namth/agenthub/internal/database	0.406s
ok  	github.com/namth/agenthub/internal/dispatcher	0.987s
ok  	github.com/namth/agenthub/internal/gateway	2.220s
ok  	github.com/namth/agenthub/internal/runner	5.643s
ok  	github.com/namth/agenthub/internal/server	0.980s
ok  	github.com/namth/agenthub/web	1.604s
TASK-005 ĐẠT

KẾT QUẢ: khoá test OK, verify.sh exit 0
```

## Điểm lệch / câu hỏi cho trưởng phòng
Không.
