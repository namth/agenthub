package tasks

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type webhookConfig struct {
	onWorkflowRun       func(branch, conclusion, url string)
	onPullRequestMerged func(prURL string)
}

// WebhookOption cấu hình cho WebhookHandler.
type WebhookOption func(*webhookConfig)

// OnWorkflowRun đăng ký callback khi có sự kiện workflow_run với action "completed".
func OnWorkflowRun(fn func(branch, conclusion, url string)) WebhookOption {
	return func(cfg *webhookConfig) {
		cfg.onWorkflowRun = fn
	}
}

// OnPullRequestMerged đăng ký callback khi có sự kiện pull_request với action "closed" và merged=true.
func OnPullRequestMerged(fn func(prURL string)) WebhookOption {
	return func(cfg *webhookConfig) {
		cfg.onPullRequestMerged = fn
	}
}

// WebhookHandler xử lý webhook từ GitHub (POST /api/webhooks/github).
func WebhookHandler(db *sql.DB, secret, projectID string, opts ...WebhookOption) http.Handler {
	return newWebhookHandler(db, secret, func(string) (string, bool) {
		return projectID, true
	}, opts...)
}

// WebhookHandlerMulti xử lý webhook từ GitHub cho nhiều dự án theo repository.full_name.
func WebhookHandlerMulti(db *sql.DB, secret string, projectOf func(fullName string) (projectID string, ok bool), opts ...WebhookOption) http.Handler {
	return newWebhookHandler(db, secret, projectOf, opts...)
}

func newWebhookHandler(db *sql.DB, secret string, projectOf func(fullName string) (projectID string, ok bool), opts ...WebhookOption) http.Handler {
	var cfg webhookConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	fn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "chỉ hỗ trợ method POST")
			return
		}

		// Đọc body tối đa 1 MB
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "không thể đọc nội dung body")
			return
		}

		sigHeader := r.Header.Get("X-Hub-Signature-256")
		if !strings.HasPrefix(sigHeader, "sha256=") {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chữ ký thiếu hoặc sai tiền tố sha256=")
			return
		}

		sigHex := strings.TrimPrefix(sigHeader, "sha256=")
		sigBytes, err := hex.DecodeString(sigHex)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "định dạng chữ ký hex không hợp lệ")
			return
		}

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expectedMAC := mac.Sum(nil)
		if !hmac.Equal(sigBytes, expectedMAC) {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chữ ký không khớp")
			return
		}

		var repoPayload struct {
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		}
		_ = json.Unmarshal(body, &repoPayload)

		projectID, ok := projectOf(repoPayload.Repository.FullName)
		if !ok || projectID == "" {
			writeJSON(w, http.StatusAccepted, map[string]string{"message": "bỏ qua repo không quản lý"})
			return
		}

		event := r.Header.Get("X-GitHub-Event")
		if event == "workflow_run" {
			var payload struct {
				Action      string `json:"action"`
				WorkflowRun struct {
					HeadBranch string  `json:"head_branch"`
					Conclusion *string `json:"conclusion"`
					HTMLURL    string  `json:"html_url"`
				} `json:"workflow_run"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "payload json không hợp lệ")
				return
			}
			if payload.Action == "completed" {
				if cfg.onWorkflowRun != nil {
					conclusion := ""
					if payload.WorkflowRun.Conclusion != nil {
						conclusion = *payload.WorkflowRun.Conclusion
					}
					cfg.onWorkflowRun(payload.WorkflowRun.HeadBranch, conclusion, payload.WorkflowRun.HTMLURL)
				}
				writeJSON(w, http.StatusOK, map[string]string{"message": "workflow_run completed"})
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]string{"message": "bỏ qua workflow_run không phải completed"})
			return
		}

		if event == "pull_request" {
			var payload struct {
				Action      string `json:"action"`
				PullRequest struct {
					Merged  bool   `json:"merged"`
					HTMLURL string `json:"html_url"`
				} `json:"pull_request"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "payload json không hợp lệ")
				return
			}
			if payload.Action == "closed" && payload.PullRequest.Merged {
				if cfg.onPullRequestMerged != nil {
					cfg.onPullRequestMerged(payload.PullRequest.HTMLURL)
				}
				writeJSON(w, http.StatusOK, map[string]string{"message": "pull_request merged"})
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]string{"message": "bỏ qua pull_request chưa merge"})
			return
		}

		if event != "issues" {
			writeJSON(w, http.StatusAccepted, map[string]string{"message": "bỏ qua sự kiện không phải issues"})
			return
		}

		var payload struct {
			Action string `json:"action"`
			Issue  struct {
				Number int    `json:"number"`
				Title  string `json:"title"`
				Body   string `json:"body"`
				User   struct {
					Login string `json:"login"`
				} `json:"user"`
			} `json:"issue"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "payload json không hợp lệ")
			return
		}

		if payload.Action != "opened" {
			writeJSON(w, http.StatusAccepted, map[string]string{"message": "bỏ qua hành động không phải opened"})
			return
		}

		// Idempotent: kiểm tra task đã tồn tại với github_issue_id hay chưa
		var existingID string
		err = db.QueryRowContext(r.Context(), "SELECT id FROM tasks WHERE github_issue_id = ? AND project_id = ?", payload.Issue.Number, projectID).Scan(&existingID)
		if err == nil {
			writeJSON(w, http.StatusOK, map[string]any{"id": existingID, "status": "already_exists"})
			return
		}
		if err != sql.ErrNoRows {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "lỗi truy vấn cơ sở dữ liệu")
			return
		}

		// Tạo user hệ thống system-github nếu chưa có
		_, err = db.ExecContext(r.Context(), `
			INSERT OR IGNORE INTO users (id, github_id, username, role)
			VALUES ('system-github', 'system-github', 'system-github', 'PARTNER')
		`)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "lỗi tạo user hệ thống")
			return
		}

		taskID := newUUID()
		_, err = db.ExecContext(r.Context(), `
			INSERT INTO tasks (id, project_id, github_issue_id, title, description, status, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'PENDING_APPROVAL', 'system-github', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		`, taskID, projectID, payload.Issue.Number, payload.Issue.Title, payload.Issue.Body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "lỗi tạo task")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":     taskID,
			"status": "PENDING_APPROVAL",
		})
	})

	return fn
}
