package projects

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type apiResponse struct {
	Success bool      `json:"success"`
	Data    any       `json:"data"`
	Error   *apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeSuccess(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Success: true,
		Data:    data,
		Error:   nil,
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Success: false,
		Data:    nil,
		Error: &apiError{
			Code:    code,
			Message: message,
		},
	})
}

type projectListItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	GitHubRepo    string `json:"github_repo"`
	DefaultBranch string `json:"default_branch"`
	Status        string `json:"status"`
	Role          string `json:"role"`
	Weight        int    `json:"weight"`
	MaxParallel   *int   `json:"max_parallel"`
	WebhookOK     bool   `json:"webhook_ok"`
}

type adminRepoItem struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
	Added         bool   `json:"added"`
}

func Handler(s *Service, authz func(*http.Request) (userID, login string, ok bool)) http.Handler {
	r := chi.NewRouter()

	adminAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			_, login, ok := authz(req)
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
				return
			}
			if !s.IsOwner(login) {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "forbidden")
				return
			}
			next.ServeHTTP(w, req)
		})
	}

	userAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			_, _, ok := authz(req)
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
				return
			}
			next.ServeHTTP(w, req)
		})
	}

	// GET /api/projects & /projects
	for _, p := range []string{"/api/projects", "/projects"} {
		r.With(userAuth).Get(p, func(w http.ResponseWriter, req *http.Request) {
			userID, login, _ := authz(req)
			s.handleListProjects(w, req, userID, login)
		})
	}

	// GET /api/admin/github/repos & /admin/github/repos
	for _, p := range []string{"/api/admin/github/repos", "/admin/github/repos"} {
		r.With(adminAuth).Get(p, s.handleListGitHubRepos)
	}

	// POST /api/admin/projects & /admin/projects
	for _, p := range []string{"/api/admin/projects", "/admin/projects"} {
		r.With(adminAuth).Post(p, s.handleAddProject)
	}

	// PATCH /api/admin/projects/{id} & /admin/projects/{id}
	for _, p := range []string{"/api/admin/projects/{id}", "/admin/projects/{id}"} {
		r.With(adminAuth).Patch(p, s.handlePatchProject)
	}

	return r
}

func (s *Service) handleListProjects(w http.ResponseWriter, r *http.Request, userID, login string) {
	var rows *sql.Rows
	var err error

	if s.IsOwner(login) {
		rows, err = s.db.QueryContext(r.Context(), `
			SELECT id, name, github_repo, default_branch, status, 'ADMIN' as role, weight, max_parallel, webhook_id
			FROM projects
			ORDER BY created_at ASC
		`)
	} else {
		rows, err = s.db.QueryContext(r.Context(), `
			SELECT p.id, p.name, p.github_repo, p.default_branch, p.status, pm.role, p.weight, p.max_parallel, p.webhook_id
			FROM projects p
			JOIN project_members pm ON p.id = pm.project_id
			WHERE pm.user_id = ?
			ORDER BY p.created_at ASC
		`, userID)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "database error")
		return
	}
	defer rows.Close()

	items := make([]projectListItem, 0)
	for rows.Next() {
		var it projectListItem
		var mp sql.NullInt64
		var wid sql.NullInt64
		if err := rows.Scan(&it.ID, &it.Name, &it.GitHubRepo, &it.DefaultBranch, &it.Status, &it.Role, &it.Weight, &mp, &wid); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "scan error")
			return
		}
		if mp.Valid {
			v := int(mp.Int64)
			it.MaxParallel = &v
		}
		it.WebhookOK = wid.Valid && wid.Int64 > 0
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "database error")
		return
	}

	writeSuccess(w, http.StatusOK, items)
}

func (s *Service) handleListGitHubRepos(w http.ResponseWriter, r *http.Request) {
	repos, err := s.gh.ListRepos(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "GITHUB_ERROR", err.Error())
		return
	}

	rows, err := s.db.QueryContext(r.Context(), "SELECT github_repo FROM projects")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "database error")
		return
	}
	defer rows.Close()

	addedMap := make(map[string]bool)
	for rows.Next() {
		var repo string
		if err := rows.Scan(&repo); err == nil {
			addedMap[repo] = true
		}
	}

	items := make([]adminRepoItem, 0, len(repos))
	for _, repo := range repos {
		items = append(items, adminRepoItem{
			FullName:      repo.FullName,
			DefaultBranch: repo.DefaultBranch,
			Private:       repo.Private,
			Added:         addedMap[repo.FullName],
		})
	}

	writeSuccess(w, http.StatusOK, items)
}

func (s *Service) handleAddProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		GitHubRepo string `json:"github_repo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}
	if req.GitHubRepo == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "github_repo is required")
		return
	}

	proj, err := s.AddProject(r.Context(), req.GitHubRepo)
	if err != nil {
		switch {
		case errors.Is(err, ErrValidation):
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		case errors.Is(err, ErrConflict):
			writeError(w, http.StatusConflict, "CONFLICT", err.Error())
		case errors.Is(err, ErrNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		}
		return
	}

	writeSuccess(w, http.StatusCreated, proj)
}

func (s *Service) handlePatchProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "project id required")
		return
	}

	var curWeight int
	var curMaxParallel sql.NullInt64
	var curStatus string
	err := s.db.QueryRowContext(r.Context(), "SELECT weight, max_parallel, status FROM projects WHERE id = ?", id).Scan(&curWeight, &curMaxParallel, &curStatus)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "project not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "database error")
		return
	}

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid json body")
		return
	}

	for k := range body {
		if k != "weight" && k != "max_parallel" && k != "status" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "unknown field: "+k)
			return
		}
	}

	setClauses := make([]string, 0)
	args := make([]any, 0)

	if v, ok := body["weight"]; ok {
		wFloat, isFloat := v.(float64)
		if !isFloat || wFloat != float64(int(wFloat)) || int(wFloat) < 1 || int(wFloat) > 5 {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "weight must be integer between 1 and 5")
			return
		}
		setClauses = append(setClauses, "weight = ?")
		args = append(args, int(wFloat))
	}

	if v, ok := body["max_parallel"]; ok {
		if v == nil {
			setClauses = append(setClauses, "max_parallel = NULL")
		} else {
			mpFloat, isFloat := v.(float64)
			if !isFloat || mpFloat != float64(int(mpFloat)) || int(mpFloat) < 1 || int(mpFloat) > 50 {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "max_parallel must be null or integer between 1 and 50")
				return
			}
			setClauses = append(setClauses, "max_parallel = ?")
			args = append(args, int(mpFloat))
		}
	}

	if v, ok := body["status"]; ok {
		sStr, isStr := v.(string)
		if !isStr || (sStr != "ACTIVE" && sStr != "ARCHIVED") {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "status must be ACTIVE or ARCHIVED")
			return
		}
		setClauses = append(setClauses, "status = ?")
		args = append(args, sStr)
	}

	if len(setClauses) > 0 {
		args = append(args, id)
		query := fmt.Sprintf("UPDATE projects SET %s WHERE id = ?", strings.Join(setClauses, ", "))
		if _, err := s.db.ExecContext(r.Context(), query, args...); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update project")
			return
		}
	}

	writeSuccess(w, http.StatusOK, map[string]any{"id": id})
}
