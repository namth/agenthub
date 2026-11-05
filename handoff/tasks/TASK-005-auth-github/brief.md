# TASK-005 — Đăng nhập GitHub OAuth, phiên JWT, RBAC middleware

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> Phần auth của TASK-05 trong `docs/11-tasks.md`. Webhook GitHub làm ở task sau.

## 1. Mục tiêu
Người dùng đăng nhập bằng GitHub; vai trò `ADMIN`/`PARTNER` lấy từ quyền trên repo (DEC-09); phiên lưu bằng JWT trong cookie HttpOnly; middleware cho các API sau dùng để chặn theo vai trò.

## 2. Tài liệu bắt buộc đọc
- `docs/07-api-contracts.md` §1 (định dạng response), §1.1, §1.5 (mã lỗi)
- `docs/08-integrations.md` §1.1, `docs/02-modules-and-features.md` §2–3, `docs/14-decisions.md` DEC-09
- `docs/10-env-and-security.md`; `internal/auth/auth_test.go` (đặc tả chi tiết nhất)

## 3. Phạm vi file
**Được tạo/sửa:** các file `.go` (không phải `_test.go`) trong `internal/auth/`; `go.mod`/`go.sum` (chỉ thêm `github.com/golang-jwt/jwt/v5`).

**Không được sửa:** file trong `tests.lock`; mọi package khác trong `internal/`, `cmd/`, `web/`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
type User struct { ID, GitHubID, Username, AvatarURL, Role string }
type Config struct {
    ClientID, ClientSecret, BaseURL, JWTSecret string
    RepoOwner, RepoName string
    ServerToken string              // PAT của server, dùng tra quyền collaborator
    GitHubWebURL, GitHubAPIURL string // rỗng → https://github.com, https://api.github.com
    SessionTTL time.Duration          // 0 → 7 ngày
    HTTPClient *http.Client           // nil → client timeout 10s
    Now func() time.Time              // nil → time.Now
}
func New(db *sql.DB, cfg Config) *Service
func (s *Service) Routes() http.Handler                 // phục vụ các đường /api/auth/*
func (s *Service) Middleware(next http.Handler) http.Handler // đọc cookie, gắn User vào context nếu hợp lệ; không chặn
func UserFrom(ctx context.Context) (User, bool)
func RequireRole(roles ...string) func(http.Handler) http.Handler // 401 UNAUTHORIZED / 403 FORBIDDEN (JSON docs/07)
```
- `GET /api/auth/github`: sinh `state` ngẫu nhiên (`crypto/rand`, ≥ 16 ký tự), cookie `agenthub_oauth_state` (HttpOnly, SameSite=Lax, Secure nếu `BaseURL` là https, sống 10 phút), 302 tới `{web}/login/oauth/authorize?client_id&redirect_uri={BaseURL}/api/auth/callback&scope=read:user&state`.
- `GET /api/auth/callback`: state thiếu/không khớp cookie → 400 `VALIDATION_ERROR`; đổi `code` (POST `{web}/login/oauth/access_token`, `Accept: application/json`) lỗi → 401 `UNAUTHORIZED`; `GET {api}/user` bằng token người dùng; `GET {api}/repos/{owner}/{repo}/collaborators/{login}/permission` bằng `ServerToken` (`Authorization: Bearer …`); 404 → 403 `FORBIDDEN`, không tạo user. Ánh xạ: `admin|maintain` → `ADMIN`, còn lại → `PARTNER`. Upsert `users` theo `github_id` (id UUID khi tạo mới, cập nhật username/avatar/role khi đã có). Đặt cookie `agenthub_session` = JWT HS256 (claims: `sub`=users.id, `role`, `username`, `iat`, `exp`=Now+TTL), HttpOnly, SameSite=Lax, Path=/, Secure nếu https; xoá cookie state; 302 về `/`.
- `GET /api/auth/me`: 200 `{id, username, avatar_url, role}` (đọc lại từ DB theo `sub`); thiếu/sai/hết hạn → 401 `UNAUTHORIZED`.
- `POST /api/auth/logout`: xoá cookie (MaxAge âm), 200 `{"success":true,"data":null,"error":null}`.
- Xác minh JWT: chỉ chấp nhận HS256 (từ chối `alg=none` và thuật toán khác), kiểm `exp` theo `Now`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-005-auth-github/verify.sh` exit 0 (gồm `-race`)
- [ ] Không test nào bị skip; `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không log token GitHub, client secret, JWT.
- Không tin `role` từ client; `/me` luôn đọc DB.
- Gọi lệnh bằng tên ngắn (`go`, `bash`); không đọc file ngoài repo.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
