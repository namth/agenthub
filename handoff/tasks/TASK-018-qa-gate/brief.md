# TASK-018 — Chốt chặn CI trước QA, Merge PR → DONE, webhook workflow_run / pull_request

> Người giao: Claude (Trưởng phòng / Giám đốc sản phẩm) · Người thực hiện: Antigravity
> `docs/13` §2 (QA 2 lớp), `docs/12` §1 (QA_PASSED → DONE), TASK-09 trong `docs/11`.

## 1. Mục tiêu
CI (GitHub Actions) phải đạt trước khi Claude review; Admin bấm Merge (web/Telegram) hoặc merge trên GitHub thì task thành DONE, task cha DONE khi mọi sub-task DONE.

## 2. Tài liệu bắt buộc đọc
- `docs/13-test-cases-and-qa.md` §2; `docs/12-business-rules.md` §1; `docs/08-integrations.md` §1.2
- `internal/pipeline/`, `internal/tasks/` hiện có; `internal/pipeline/ci_test.go`, `internal/tasks/merge_test.go` (đặc tả chi tiết nhất)

## 3. Phạm vi file
**Được tạo/sửa:** các file `.go` (không phải `_test.go`) trong `internal/pipeline/` và `internal/tasks/`.

**Không được sửa:** file trong `tests.lock` (mọi test cũ của hai package giữ nguyên hành vi); package khác, `cmd/`, `web/`, `go.mod`; `docs/`, `CONTEXT.md`, `CLAUDE.md`, `handoff/`, `scripts/`, `.agents/`.

## 4. Đặc tả chi tiết
**pipeline:**
```go
type Option func(*Pipeline)
func WithCIGate(on bool) Option                       // mặc định false: giữ hành vi cũ
func New(db *sql.DB, d dispatcher.API, gh tasks.GitHub, projectID string, opts ...Option) *Pipeline
func (p *Pipeline) OnCI(ctx context.Context, branch, conclusion, runURL string) error
func (p *Pipeline) OnMerged(ctx context.Context, prURL string) error
```
- Bật CI gate: CODE nộp → `WAITING_QA` nhưng **chưa** tạo QA_REVIEW. `OnCI` tìm task `WAITING_QA` có job CODE gần nhất cùng `branch`: `success` → tạo QA_REVIEW (idempotent: không tạo nếu đã có QA_REVIEW QUEUED/RUNNING cho task); `failure`/`timed_out` → task `READY`, job CODE mới cùng nhánh, `lead_journal` NOTE có `runURL`; kết luận khác hoặc không tìm thấy task → bỏ qua, nil.
- `OnMerged`: task `QA_PASSED` có `pr_url` khớp → `DONE`; nếu task có cha và mọi task con `DONE` → cha `DONE` (từ bất kỳ trạng thái nào trừ REJECTED). Không khớp → nil.

**tasks:**
```go
type Merger interface { MergePR(ctx context.Context, number int) error }
func NewGitHubMerger(apiURL, owner, repo, token string, hc *http.Client) Merger // PUT /repos/{o}/{r}/pulls/{n}/merge {"merge_method":"squash"}, Bearer; HTTP ≠ 2xx → lỗi
func WithMerger(m Merger) Option
func WithOnMerged(fn func(prURL string)) Option
func (s *Service) Merge(ctx context.Context, taskID string) error
type WebhookOption func(*webhookConfig)
func OnWorkflowRun(fn func(branch, conclusion, url string)) WebhookOption   // chỉ action "completed"
func OnPullRequestMerged(fn func(prURL string)) WebhookOption              // chỉ action "closed" + merged=true
func WebhookHandler(db *sql.DB, secret, projectID string, opts ...WebhookOption) http.Handler
```
- `POST /api/tasks/{id}/merge` (ADMIN): không cấu hình Merger → 501 `NOT_CONFIGURED`; task không `QA_PASSED` → 409 `INVALID_TRANSITION`; số PR lấy từ đuôi `pr_url` (`/pull/<n>`); `MergePR` lỗi → 502 `GITHUB_ERROR`, giữ QA_PASSED; thành công → `DONE` rồi gọi `OnMerged(pr_url)` (main nối vào `pipeline.OnMerged` để gộp cha).
- Webhook giữ nguyên hành vi `issues`.

## 5. Tiêu chí nghiệm thu
- [ ] `bash handoff/tasks/TASK-018-qa-gate/verify.sh` exit 0 (gồm `-race`)
- [ ] `report.md` dán nguyên văn output verify

## 6. Ràng buộc & bẫy cần tránh
- Mọi chuyển trạng thái `UPDATE … WHERE status = <nguồn>`; không log token.
- Lệnh được phép: `go`, `gofmt`, `bash`, `git`, `curl`, `python3`, `ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`.
- Xong thì **tự chạy verify, viết `report.md` và `submit`** — không dừng giữa chừng.
