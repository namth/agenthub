package tasks

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/namth/agenthub/internal/dispatcher"
)

// Authz là hàm xác thực & phân quyền.
type Authz func(r *http.Request) (userID, role string, ok bool)

type apiResponse struct {
	Success bool    `json:"success"`
	Data    any     `json:"data"`
	Error   *apiErr `json:"error"`
}

type apiErr struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Success: true,
		Data:    data,
		Error:   nil,
	})
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Success: false,
		Data:    nil,
		Error: &apiErr{
			Code:    code,
			Message: msg,
		},
	})
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrValidation):
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, ErrInvalidTransition):
		writeError(w, http.StatusConflict, "INVALID_TRANSITION", err.Error())
	case errors.Is(err, ErrGitHub):
		writeError(w, http.StatusBadGateway, "GITHUB_ERROR", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // RFC 4122 version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Handler trả về http.Handler phục vụ các API liên quan đến Task và Kanban.
func Handler(db *sql.DB, gh GitHub, d dispatcher.API, projectID string, authz Authz) http.Handler {
	return HandlerWithService(NewService(db, gh, d, projectID), authz)
}

// HandlerWithService nhận Service và Authz để khởi tạo http.Handler.
func HandlerWithService(s *Service, authz Authz) http.Handler {
	r := chi.NewRouter()

	createH := handleCreate(s, authz)
	getTasksH := handleGetTasks(s.db, s.projectID, authz)
	getDetailH := handleGetTaskDetail(s.db, s.projectID, authz)
	approveH := handleApprove(s, authz)
	rejectH := handleReject(s, authz)

	// Hỗ trợ cả tiền tố /api/tasks và /tasks
	r.Post("/api/tasks", createH)
	r.Post("/api/tasks/", createH)
	r.Get("/api/tasks", getTasksH)
	r.Get("/api/tasks/", getTasksH)
	r.Get("/api/tasks/{id}", getDetailH)
	r.Post("/api/tasks/{id}/approve", approveH)
	r.Post("/api/tasks/{id}/reject", rejectH)

	r.Post("/tasks", createH)
	r.Post("/tasks/", createH)
	r.Get("/tasks", getTasksH)
	r.Get("/tasks/", getTasksH)
	r.Get("/tasks/{id}", getDetailH)
	r.Post("/tasks/{id}/approve", approveH)
	r.Post("/tasks/{id}/reject", rejectH)

	return r
}

func handleCreate(s *Service, authz Authz) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, role, ok := authz(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
			return
		}
		if role != "ADMIN" && role != "PARTNER" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "không đủ quyền hạn")
			return
		}

		var req struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "body không đúng định dạng json")
			return
		}

		task, err := s.Create(r.Context(), userID, req.Title, req.Description)
		if err != nil {
			writeServiceError(w, err)
			return
		}

		taskData := map[string]any{
			"id":              task.ID,
			"project_id":      s.projectID,
			"parent_id":       task.ParentID,
			"github_issue_id": task.GitHubIssueID,
			"title":           task.Title,
			"description":     task.Description,
			"status":          task.Status,
			"created_by":      task.CreatedBy,
		}
		if role == "ADMIN" {
			taskData["branch_name"] = nil
			taskData["pr_url"] = nil
			taskData["assigned_worker_id"] = nil
		}

		writeJSON(w, http.StatusCreated, taskData)
	}
}

func handleApprove(s *Service, authz Authz) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, role, ok := authz(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
			return
		}
		if role != "ADMIN" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "chỉ admin mới có quyền duyệt")
			return
		}

		taskID := chi.URLParam(r, "id")
		if err := s.Approve(r.Context(), taskID); err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":     taskID,
			"status": "PLANNING",
		})
	}
}

func handleReject(s *Service, authz Authz) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, role, ok := authz(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
			return
		}
		if role != "ADMIN" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "chỉ admin mới có quyền từ chối")
			return
		}

		taskID := chi.URLParam(r, "id")
		var req struct {
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "body không đúng định dạng json")
			return
		}

		if err := s.Reject(r.Context(), taskID, req.Reason); err != nil {
			writeServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"id":     taskID,
			"status": "REJECTED",
		})
	}
}

type kanbanColumn struct {
	Key   string           `json:"key"`
	Title string           `json:"title"`
	Tasks []map[string]any `json:"tasks"`
}

func handleGetTasks(db *sql.DB, projectID string, authz Authz) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, role, ok := authz(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
			return
		}
		if role != "ADMIN" && role != "PARTNER" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "không đủ quyền hạn")
			return
		}
		isAdmin := role == "ADMIN"

		query := `SELECT id, project_id, parent_id, github_issue_id, title, status, assigned_worker_id, branch_name, pr_url, created_at FROM tasks WHERE project_id = ?`
		args := []any{projectID}

		if !isAdmin {
			query += ` AND parent_id IS NULL`
		}

		statusParam := r.URL.Query().Get("status")
		if statusParam != "" {
			query += ` AND status = ?`
			args = append(args, statusParam)
		} else {
			query += ` AND status != 'REJECTED'`
		}
		query += ` ORDER BY created_at ASC`

		rows, err := db.QueryContext(r.Context(), query, args...)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "lỗi truy vấn danh sách task")
			return
		}
		defer rows.Close()

		cols := []kanbanColumn{
			{Key: "backlog", Title: "Chờ duyệt", Tasks: []map[string]any{}},
			{Key: "planning", Title: "Lập kế hoạch", Tasks: []map[string]any{}},
			{Key: "in_progress", Title: "Đang thực hiện", Tasks: []map[string]any{}},
			{Key: "review", Title: "Kiểm định", Tasks: []map[string]any{}},
			{Key: "done", Title: "Hoàn thành", Tasks: []map[string]any{}},
		}

		for rows.Next() {
			var id, pID, title, st, createdAt string
			var parentID, assignedWorker, branch, pr sql.NullString
			var ghIssueID sql.NullInt64

			if err := rows.Scan(&id, &pID, &parentID, &ghIssueID, &title, &st, &assignedWorker, &branch, &pr, &createdAt); err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "lỗi đọc dòng task")
				return
			}

			colIdx := -1
			switch st {
			case "PENDING_APPROVAL":
				colIdx = 0
			case "PLANNING", "READY":
				colIdx = 1
			case "IN_PROGRESS":
				colIdx = 2
			case "WAITING_QA", "QA_PASSED":
				colIdx = 3
			case "DONE":
				colIdx = 4
			}

			if colIdx >= 0 {
				card := map[string]any{
					"id":         id,
					"title":      title,
					"status":     st,
					"created_at": createdAt,
				}
				if ghIssueID.Valid {
					card["github_issue_id"] = ghIssueID.Int64
				} else {
					card["github_issue_id"] = nil
				}
				if parentID.Valid {
					card["parent_id"] = parentID.String
				} else {
					card["parent_id"] = nil
				}

				if isAdmin {
					if branch.Valid {
						card["branch_name"] = branch.String
					} else {
						card["branch_name"] = nil
					}
					if pr.Valid {
						card["pr_url"] = pr.String
					} else {
						card["pr_url"] = nil
					}
					if assignedWorker.Valid {
						card["assigned_worker_id"] = assignedWorker.String
					} else {
						card["assigned_worker_id"] = nil
					}
				}

				cols[colIdx].Tasks = append(cols[colIdx].Tasks, card)
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"columns": cols,
		})
	}
}

func handleGetTaskDetail(db *sql.DB, projectID string, authz Authz) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, role, ok := authz(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
			return
		}
		if role != "ADMIN" && role != "PARTNER" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "không đủ quyền hạn")
			return
		}
		isAdmin := role == "ADMIN"
		taskID := chi.URLParam(r, "id")

		var id, pID, title, desc, st, createdBy, createdAt, updatedAt string
		var parentID, assignedWorker, branch, pr sql.NullString
		var ghIssueID sql.NullInt64

		err := db.QueryRowContext(r.Context(), `
			SELECT id, project_id, parent_id, github_issue_id, title, description, status, assigned_worker_id, branch_name, pr_url, created_by, created_at, updated_at
			FROM tasks
			WHERE id = ? AND project_id = ?
		`, taskID, projectID).Scan(&id, &pID, &parentID, &ghIssueID, &title, &desc, &st, &assignedWorker, &branch, &pr, &createdBy, &createdAt, &updatedAt)
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "không tìm thấy task")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "lỗi truy vấn task")
			return
		}

		// Partner không được xem sub-task
		if !isAdmin && parentID.Valid && parentID.String != "" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "đối tác không được xem sub-task")
			return
		}

		taskMap := map[string]any{
			"id":          id,
			"project_id":  pID,
			"title":       title,
			"description": desc,
			"status":      st,
			"created_by":  createdBy,
			"created_at":  createdAt,
			"updated_at":  updatedAt,
		}
		if ghIssueID.Valid {
			taskMap["github_issue_id"] = ghIssueID.Int64
		} else {
			taskMap["github_issue_id"] = nil
		}
		if parentID.Valid {
			taskMap["parent_id"] = parentID.String
		} else {
			taskMap["parent_id"] = nil
		}
		if isAdmin {
			if branch.Valid {
				taskMap["branch_name"] = branch.String
			} else {
				taskMap["branch_name"] = nil
			}
			if pr.Valid {
				taskMap["pr_url"] = pr.String
			} else {
				taskMap["pr_url"] = nil
			}
			if assignedWorker.Valid {
				taskMap["assigned_worker_id"] = assignedWorker.String
			} else {
				taskMap["assigned_worker_id"] = nil
			}
		}

		// Lấy danh sách subtasks
		subRows, err := db.QueryContext(r.Context(), `
			SELECT id, project_id, parent_id, github_issue_id, title, description, status, assigned_worker_id, branch_name, pr_url, created_by, created_at, updated_at
			FROM tasks
			WHERE parent_id = ? AND project_id = ?
			ORDER BY created_at ASC
		`, taskID, projectID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "lỗi truy vấn sub-task")
			return
		}
		defer subRows.Close()

		subtasksList := make([]map[string]any, 0)
		for subRows.Next() {
			var sid, spID, stitle, sdesc, sst, screatedBy, screatedAt, supdatedAt string
			var sparentID, sassignedWorker, sbranch, spr sql.NullString
			var sghIssueID sql.NullInt64

			if err := subRows.Scan(&sid, &spID, &sparentID, &sghIssueID, &stitle, &sdesc, &sst, &sassignedWorker, &sbranch, &spr, &screatedBy, &screatedAt, &supdatedAt); err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "lỗi đọc sub-task")
				return
			}

			sMap := map[string]any{
				"id":          sid,
				"project_id":  spID,
				"title":       stitle,
				"description": sdesc,
				"status":      sst,
				"created_by":  screatedBy,
				"created_at":  screatedAt,
				"updated_at":  supdatedAt,
			}
			if sghIssueID.Valid {
				sMap["github_issue_id"] = sghIssueID.Int64
			} else {
				sMap["github_issue_id"] = nil
			}
			if sparentID.Valid {
				sMap["parent_id"] = sparentID.String
			} else {
				sMap["parent_id"] = nil
			}
			if isAdmin {
				if sbranch.Valid {
					sMap["branch_name"] = sbranch.String
				} else {
					sMap["branch_name"] = nil
				}
				if spr.Valid {
					sMap["pr_url"] = spr.String
				} else {
					sMap["pr_url"] = nil
				}
				if sassignedWorker.Valid {
					sMap["assigned_worker_id"] = sassignedWorker.String
				} else {
					sMap["assigned_worker_id"] = nil
				}
			}
			subtasksList = append(subtasksList, sMap)
		}

		taskMap["subtasks"] = subtasksList
		writeJSON(w, http.StatusOK, taskMap)
	}
}
