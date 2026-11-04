# TASK-020 — Dự án: liệt kê repo, thêm dự án + webhook, vai theo dự án, đăng nhập đa dự án

> Người giao: Claude (Trưởng phòng) · Người thực hiện: Antigravity · FEAT-04: DEC-14, DEC-17, BR-26, BR-27

## 1. Mục tiêu
Owner thêm repo làm dự án ngay trên AgentHub (server tự tạo webhook); vai Admin/Partner tính theo từng repo; partner chỉ thấy dự án mình là collaborator.

## 2. Tài liệu bắt buộc đọc
- `docs/14-decisions.md` DEC-09, DEC-14, DEC-17; `docs/features/multi-project.md` §2, §3, §5, §6, §7
- `internal/database/migrations/002_multi_project.sql`; `internal/auth/auth.go`
- `internal/projects/projects_test.go`, `internal/auth/multi_test.go` (đặc tả chi tiết nhất)

## 3. Phạm vi file
**Được tạo/sửa:** các file `.go` (không phải `_test.go`) trong `internal/projects/` (mới); `internal/auth/auth.go` (thêm chế độ đa dự án, giữ nguyên hành vi cũ khi `OnLogin == nil`).

**Không được sửa:** file trong `tests.lock` (gồm mọi test auth cũ); package khác; `cmd/`, `web/`, `go.mod`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
```go
// projects
var ErrNotCollaborator = errors.New("NOT_COLLABORATOR")
type Repo struct { FullName, DefaultBranch string; Private bool }
type GitHub interface {
    ListRepos(ctx context.Context) ([]Repo, error)                          // GET /user/repos?affiliation=owner,collaborator&per_page=100&page=N tới khi trang rỗng
    CreateWebhook(ctx context.Context, fullName, url, secret string) (int64, error) // POST /repos/{full}/hooks {name:"web",active:true,events:["issues","pull_request","workflow_run"],config:{url,content_type:"json",secret}}
    Permission(ctx context.Context, fullName, login string) (string, error)  // GET …/collaborators/{login}/permission; 404 → ErrNotCollaborator
}
func NewGitHub(apiURL, token string, hc *http.Client) GitHub               // apiURL rỗng → https://api.github.com; Bearer token
type Config struct { Owner, BaseURL, WebhookSecret string }
type Project struct { ID, Name, GitHubRepo, DefaultBranch, Status string; Weight int; MaxParallel *int; WebhookOK bool }
func New(db *sql.DB, gh GitHub, cfg Config) *Service
func (s *Service) IsOwner(login string) bool                               // so khớp không phân biệt hoa thường, login rỗng → false
func (s *Service) AddProject(ctx context.Context, fullName string) (Project, error)
func (s *Service) SyncUser(ctx context.Context, userID, login string) (int, error) // tra mọi dự án ACTIVE, upsert/xoá project_members; trả số dự án là thành viên
func (s *Service) RoleIn(ctx context.Context, userID, projectID string) (string, bool) // owner luôn ADMIN mọi dự án
func Handler(s *Service, authz func(*http.Request) (userID, login string, ok bool)) http.Handler
```
- `AddProject`: `fullName` phải dạng `owner/repo` → `ErrValidation`; đã có → `ErrConflict`; không có trong `ListRepos` → `ErrNotFound`. Tạo `projects` (`id` ngẫu nhiên, `name` = phần repo, `default_branch` từ danh sách), tạo webhook (`url = BaseURL + "/api/webhooks/github"`); lỗi webhook → vẫn lưu, `webhook_id NULL`, `WebhookOK=false`. Sau đó `SyncUser` cho mọi user trong bảng `users`.
- Vai: `admin|maintain` → ADMIN, quyền khác → PARTNER (DEC-09); `ErrNotCollaborator` → không là thành viên.
- **HTTP** (JSON chuẩn docs/07; 401 chưa đăng nhập; 403 không phải owner cho đường `/api/admin/*`): `GET /api/projects` (owner: mọi dự án ACTIVE+ARCHIVED kèm `role:"ADMIN"`; người khác: dự án trong `project_members`) → `[{id, name, github_repo, default_branch, status, role, weight, max_parallel, webhook_ok}]`; `GET /api/admin/github/repos` → `[{full_name, default_branch, private, added}]`, lỗi GitHub → 502 `GITHUB_ERROR`; `POST /api/admin/projects {github_repo}` → 201 / 400 / 404 / 409; `PATCH /api/admin/projects/{id}` nhận `weight` (1–5), `max_parallel` (1–50 hoặc null), `status` (`ACTIVE|ARCHIVED`), khoá lạ → 400, không có → 404.
- **auth** — `Config` thêm `Owner string` và `OnLogin func(ctx context.Context, userID, login string) (int, error)`. Khi `OnLogin != nil` (chế độ đa dự án): **không** tra quyền repo cố định; upsert user với `role = ADMIN` nếu login trùng `Owner` (không phân biệt hoa thường) ngược lại `PARTNER`; gọi `OnLogin`; không phải owner và kết quả 0 → 403 `FORBIDDEN`, không đặt phiên. `OnLogin == nil` → hành vi cũ y nguyên.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-020-projects/verify.sh` exit 0 (gồm `-race`)
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Không log PAT, webhook secret.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
