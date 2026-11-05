package smoke

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	ErrNotSandbox = errors.New("project is not a sandbox")
	ErrConflict   = errors.New("smoke run already running")
	ErrNotFound   = errors.New("not found")
)

type Step struct {
	Step      string     `json:"step"`
	AgentID   string     `json:"agent_id"`
	StartedAt *time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	OK        *bool      `json:"ok"`
	Note      string     `json:"note"`
}

type Run struct {
	ID        string     `json:"id"`
	ProjectID string     `json:"project_id"`
	TaskID    string     `json:"task_id"`
	Status    string     `json:"status"`
	Steps     []Step     `json:"steps"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
}

type TaskOps interface {
	CreateIn(ctx context.Context, userID, projectID, title, description string) (taskID string, err error)
	Approve(ctx context.Context, taskID string) error
	Cleanup(ctx context.Context, taskID string) error // đóng PR (không merge) + issue mẫu
}

type Smoke struct {
	db  *sql.DB
	ops TaskOps
	now func() time.Time
}

func New(db *sql.DB, ops TaskOps, now func() time.Time) *Smoke {
	if now == nil {
		now = time.Now
	}
	return &Smoke{
		db:  db,
		ops: ops,
		now: now,
	}
}

func parseTime(val any) *time.Time {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case time.Time:
		t := v.UTC()
		return &t
	case *time.Time:
		if v == nil {
			return nil
		}
		t := v.UTC()
		return &t
	case string:
		if v == "" {
			return nil
		}
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		} {
			if t, err := time.ParseInLocation(layout, v, time.UTC); err == nil {
				return &t
			}
		}
	}
	return nil
}

func (s *Smoke) Start(ctx context.Context, projectID, userID string) (string, error) {
	var isSandbox int
	err := s.db.QueryRowContext(ctx, "SELECT is_sandbox FROM projects WHERE id = ?", projectID).Scan(&isSandbox)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if isSandbox != 1 {
		return "", ErrNotSandbox
	}

	var count int
	err = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM smoke_runs WHERE project_id = ? AND status = 'RUNNING'", projectID).Scan(&count)
	if err != nil {
		return "", err
	}
	if count > 0 {
		return "", ErrConflict
	}

	taskID, err := s.ops.CreateIn(ctx, userID, projectID, "Smoke test: hàm Add(a, b) + test", "Thêm hàm cộng hai số Add(a, b) và kiểm thử bảng cho dự án sandbox.")
	if err != nil {
		return "", err
	}

	if err := s.ops.Approve(ctx, taskID); err != nil {
		return "", err
	}

	b := make([]byte, 8)
	_, _ = rand.Read(b)
	runID := fmt.Sprintf("smk-%x", b)

	steps := []Step{
		{Step: "SPEC"},
		{Step: "CODE"},
		{Step: "QA"},
	}
	stepsJSON, err := json.Marshal(steps)
	if err != nil {
		return "", err
	}

	now := s.now().UTC()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO smoke_runs (id, project_id, task_id, status, steps, started_by, started_at)
		VALUES (?, ?, ?, 'RUNNING', ?, ?, ?)
	`, runID, projectID, taskID, string(stepsJSON), userID, now.Format(time.RFC3339))
	if err != nil {
		return "", err
	}

	return runID, nil
}

func (s *Smoke) Track(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, task_id, status, steps, started_at
		FROM smoke_runs
		WHERE status = 'RUNNING'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type activeRun struct {
		id        string
		projectID string
		taskID    string
		steps     []Step
		startedAt *time.Time
	}

	var activeRuns []activeRun
	for rows.Next() {
		var ar activeRun
		var stepsStr, startedAtStr string
		if err := rows.Scan(&ar.id, &ar.projectID, &ar.taskID, new(string), &stepsStr, &startedAtStr); err != nil {
			return err
		}
		ar.startedAt = parseTime(startedAtStr)
		_ = json.Unmarshal([]byte(stepsStr), &ar.steps)
		if len(ar.steps) < 3 {
			ar.steps = []Step{
				{Step: "SPEC"},
				{Step: "CODE"},
				{Step: "QA"},
			}
		}
		activeRuns = append(activeRuns, ar)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	now := s.now().UTC()

	for _, ar := range activeRuns {
		// 1. Nâng priority = 50 cho job QUEUED của task và sub-task
		_, _ = s.db.ExecContext(ctx, `
			UPDATE jobs
			SET priority = 50
			WHERE (task_id = ? OR task_id IN (SELECT id FROM tasks WHERE parent_id = ?))
			  AND status = 'QUEUED'
			  AND priority != 50
		`, ar.taskID, ar.taskID)

		// 2. Kiểm tra TIMEOUT (quá 30 phút)
		if ar.startedAt != nil && now.Sub(*ar.startedAt) > 30*time.Minute {
			for i := range ar.steps {
				if ar.steps[i].OK == nil {
					isOK := false
					ar.steps[i].OK = &isOK
					ar.steps[i].EndedAt = &now
					break
				}
			}
			stepsJSON, _ := json.Marshal(ar.steps)
			_, _ = s.db.ExecContext(ctx, `
				UPDATE smoke_runs
				SET status = 'TIMEOUT', steps = ?, ended_at = ?
				WHERE id = ?
			`, string(stepsJSON), now.Format(time.RFC3339), ar.id)
			s.finish(ctx, ar.taskID)
			continue
		}

		// 3. Kiểm tra FAILED (task REJECTED hoặc job FAILED)
		var taskStatus string
		_ = s.db.QueryRowContext(ctx, "SELECT status FROM tasks WHERE id = ?", ar.taskID).Scan(&taskStatus)

		var failedJobID, failedJobKind, failedJobTaskID string
		errFailed := s.db.QueryRowContext(ctx, `
			SELECT id, kind, task_id
			FROM jobs
			WHERE (task_id = ? OR task_id IN (SELECT id FROM tasks WHERE parent_id = ?))
			  AND status = 'FAILED'
			LIMIT 1
		`, ar.taskID, ar.taskID).Scan(&failedJobID, &failedJobKind, &failedJobTaskID)

		if taskStatus == "REJECTED" || errFailed == nil {
			targetTask := failedJobTaskID
			if targetTask == "" {
				targetTask = ar.taskID
			}

			logRows, err := s.db.QueryContext(ctx, `
				SELECT log_chunk
				FROM task_logs
				WHERE task_id = ? OR task_id = ?
				ORDER BY id ASC
			`, targetTask, ar.taskID)
			var allLogs []string
			if err == nil {
				for logRows.Next() {
					var chunk string
					if err := logRows.Scan(&chunk); err == nil {
						allLogs = append(allLogs, chunk)
					}
				}
				logRows.Close()
			}
			joined := strings.Join(allLogs, "\n")
			lines := strings.Split(joined, "\n")
			var cleanLines []string
			for _, l := range lines {
				trimmed := strings.TrimSpace(l)
				if trimmed != "" {
					cleanLines = append(cleanLines, trimmed)
				}
			}
			if len(cleanLines) > 20 {
				cleanLines = cleanLines[len(cleanLines)-20:]
			}
			note := strings.Join(cleanLines, "\n")

			stepIdx := -1
			switch failedJobKind {
			case "SPEC":
				stepIdx = 0
			case "CODE":
				stepIdx = 1
			case "QA", "QA_REVIEW":
				stepIdx = 2
			default:
				for i := range ar.steps {
					if ar.steps[i].OK == nil || !*ar.steps[i].OK {
						stepIdx = i
						break
					}
				}
			}
			if stepIdx >= 0 && stepIdx < len(ar.steps) {
				isOK := false
				ar.steps[stepIdx].OK = &isOK
				ar.steps[stepIdx].Note = note
				ar.steps[stepIdx].EndedAt = &now
			}

			stepsJSON, _ := json.Marshal(ar.steps)
			_, _ = s.db.ExecContext(ctx, `
				UPDATE smoke_runs
				SET status = 'FAILED', steps = ?, ended_at = ?
				WHERE id = ?
			`, string(stepsJSON), now.Format(time.RFC3339), ar.id)
			s.finish(ctx, ar.taskID)
			continue
		}

		// 4. Theo dõi từng bước: SPEC
		var specJobID, specJobStatus string
		if err := s.db.QueryRowContext(ctx, `
			SELECT id, status FROM jobs WHERE task_id = ? AND kind = 'SPEC'
		`, ar.taskID).Scan(&specJobID, &specJobStatus); err == nil {
			var agentID string
			var aStartedAt, aEndedAt sql.NullString
			_ = s.db.QueryRowContext(ctx, `
				SELECT agent_id, started_at, ended_at
				FROM assignments
				WHERE job_id = ?
				ORDER BY CASE status WHEN 'SUBMITTED' THEN 1 WHEN 'ACTIVE' THEN 2 ELSE 3 END, started_at DESC
				LIMIT 1
			`, specJobID).Scan(&agentID, &aStartedAt, &aEndedAt)

			if agentID != "" {
				ar.steps[0].AgentID = agentID
			}
			if aStartedAt.Valid {
				ar.steps[0].StartedAt = parseTime(aStartedAt.String)
			}
			if specJobStatus == "DONE" {
				isOK := true
				ar.steps[0].OK = &isOK
				if aEndedAt.Valid {
					ar.steps[0].EndedAt = parseTime(aEndedAt.String)
				} else if ar.steps[0].EndedAt == nil {
					ar.steps[0].EndedAt = &now
				}
			}
		}

		// 5. Theo dõi từng bước: CODE
		codeRows, err := s.db.QueryContext(ctx, `
			SELECT j.id, j.status, COALESCE(t.pr_url, '')
			FROM jobs j
			JOIN tasks t ON j.task_id = t.id
			WHERE t.parent_id = ? AND j.kind = 'CODE'
		`, ar.taskID)
		var totalCode, doneCode int
		var prURL string
		var codeJobIDs []string
		if err == nil {
			for codeRows.Next() {
				var jid, jst, pr string
				if err := codeRows.Scan(&jid, &jst, &pr); err == nil {
					totalCode++
					codeJobIDs = append(codeJobIDs, jid)
					if jst == "DONE" {
						doneCode++
					}
					if pr != "" && prURL == "" {
						prURL = pr
					}
				}
			}
			codeRows.Close()
		}
		if prURL == "" {
			_ = s.db.QueryRowContext(ctx, `
				SELECT pr_url FROM tasks
				WHERE (parent_id = ? OR id = ?) AND pr_url IS NOT NULL AND pr_url != ''
				LIMIT 1
			`, ar.taskID, ar.taskID).Scan(&prURL)
		}

		for _, cjid := range codeJobIDs {
			var cAgent string
			var caStart, caEnd sql.NullString
			if err := s.db.QueryRowContext(ctx, `
				SELECT agent_id, started_at, ended_at
				FROM assignments
				WHERE job_id = ?
				ORDER BY attempt DESC, started_at DESC
				LIMIT 1
			`, cjid).Scan(&cAgent, &caStart, &caEnd); err == nil {
				if ar.steps[1].AgentID == "" {
					ar.steps[1].AgentID = cAgent
				}
				if ar.steps[1].StartedAt == nil && caStart.Valid {
					ar.steps[1].StartedAt = parseTime(caStart.String)
				}
				if caEnd.Valid {
					ar.steps[1].EndedAt = parseTime(caEnd.String)
				}
			}
		}

		if totalCode > 0 && doneCode == totalCode {
			isOK := true
			ar.steps[1].OK = &isOK
			ar.steps[1].Note = prURL
			if ar.steps[1].EndedAt == nil {
				ar.steps[1].EndedAt = &now
			}
		}

		// 6. Theo dõi từng bước: QA
		if taskStatus == "QA_PASSED" {
			isOK := true
			ar.steps[2].OK = &isOK
			if ar.steps[2].EndedAt == nil {
				ar.steps[2].EndedAt = &now
			}

			var qaAgent string
			var qaStart sql.NullString
			_ = s.db.QueryRowContext(ctx, `
				SELECT a.agent_id, a.started_at
				FROM assignments a
				JOIN jobs j ON a.job_id = j.id
				WHERE (j.task_id = ? OR j.task_id IN (SELECT id FROM tasks WHERE parent_id = ?))
				  AND j.kind = 'QA_REVIEW'
				ORDER BY a.started_at DESC LIMIT 1
			`, ar.taskID, ar.taskID).Scan(&qaAgent, &qaStart)
			if qaAgent != "" {
				ar.steps[2].AgentID = qaAgent
				if qaStart.Valid {
					ar.steps[2].StartedAt = parseTime(qaStart.String)
				}
			}

			stepsJSON, _ := json.Marshal(ar.steps)
			_, _ = s.db.ExecContext(ctx, `
				UPDATE smoke_runs
				SET status = 'PASSED', steps = ?, ended_at = ?
				WHERE id = ?
			`, string(stepsJSON), now.Format(time.RFC3339), ar.id)
			s.finish(ctx, ar.taskID)
			continue
		}

		// Vẫn đang chạy, lưu cập nhật steps
		stepsJSON, _ := json.Marshal(ar.steps)
		_, _ = s.db.ExecContext(ctx, `UPDATE smoke_runs SET steps = ? WHERE id = ?`, string(stepsJSON), ar.id)
	}

	return nil
}

func (s *Smoke) List(projectID string, limit int) ([]Run, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.db.Query(`
		SELECT id, project_id, task_id, status, steps, started_at, ended_at
		FROM smoke_runs
		WHERE project_id = ?
		ORDER BY started_at DESC
		LIMIT ?
	`, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := make([]Run, 0)
	for rows.Next() {
		var r Run
		var stepsStr, startedAtStr string
		var endedAtStr sql.NullString
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.TaskID, &r.Status, &stepsStr, &startedAtStr, &endedAtStr); err != nil {
			return nil, err
		}
		if t := parseTime(startedAtStr); t != nil {
			r.StartedAt = *t
		}
		if endedAtStr.Valid {
			r.EndedAt = parseTime(endedAtStr.String)
		}
		_ = json.Unmarshal([]byte(stepsStr), &r.Steps)
		if r.Steps == nil {
			r.Steps = []Step{}
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

func (s *Smoke) Get(projectID, runID string) (Run, error) {
	var r Run
	var stepsStr, startedAtStr string
	var endedAtStr sql.NullString
	err := s.db.QueryRow(`
		SELECT id, project_id, task_id, status, steps, started_at, ended_at
		FROM smoke_runs
		WHERE project_id = ? AND id = ?
	`, projectID, runID).Scan(&r.ID, &r.ProjectID, &r.TaskID, &r.Status, &stepsStr, &startedAtStr, &endedAtStr)
	if err == sql.ErrNoRows {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	if t := parseTime(startedAtStr); t != nil {
		r.StartedAt = *t
	}
	if endedAtStr.Valid {
		r.EndedAt = parseTime(endedAtStr.String)
	}
	_ = json.Unmarshal([]byte(stepsStr), &r.Steps)
	if r.Steps == nil {
		r.Steps = []Step{}
	}
	return r, nil
}

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

func extractProjectID(req *http.Request) string {
	if id := chi.URLParam(req, "id"); id != "" {
		return id
	}
	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func extractRunID(req *http.Request) string {
	if run := chi.URLParam(req, "run"); run != "" {
		return run
	}
	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

func Handler(s *Smoke, authz func(*http.Request) (userID, role string, ok bool)) http.Handler {
	r := chi.NewRouter()

	requireAdmin := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			_, role, ok := authz(req)
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
				return
			}
			if role != "ADMIN" {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "forbidden")
				return
			}
			next(w, req)
		}
	}

	handleStart := func(w http.ResponseWriter, req *http.Request) {
		userID, _, _ := authz(req)
		projectID := extractProjectID(req)
		runID, err := s.Start(req.Context(), projectID, userID)
		if errors.Is(err, ErrNotSandbox) {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "project is not a sandbox")
			return
		}
		if errors.Is(err, ErrConflict) {
			writeError(w, http.StatusConflict, "CONFLICT", "smoke run already running")
			return
		}
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "project not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		writeSuccess(w, http.StatusAccepted, map[string]any{"run_id": runID})
	}

	handleList := func(w http.ResponseWriter, req *http.Request) {
		projectID := extractProjectID(req)
		runs, err := s.List(projectID, 5)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		writeSuccess(w, http.StatusOK, runs)
	}

	handleGet := func(w http.ResponseWriter, req *http.Request) {
		projectID := extractProjectID(req)
		runID := extractRunID(req)
		run, err := s.Get(projectID, runID)
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		writeSuccess(w, http.StatusOK, run)
	}

	for _, p := range []string{
		"/api/admin/projects/{id}/smoke-run",
		"/admin/projects/{id}/smoke-run",
		"/{id}/smoke-run",
		"/projects/{id}/smoke-run",
	} {
		r.Post(p, requireAdmin(handleStart))
	}

	for _, p := range []string{
		"/api/admin/projects/{id}/smoke-runs",
		"/admin/projects/{id}/smoke-runs",
		"/{id}/smoke-runs",
		"/projects/{id}/smoke-runs",
	} {
		r.Get(p, requireAdmin(handleList))
	}

	for _, p := range []string{
		"/api/admin/projects/{id}/smoke-runs/{run}",
		"/admin/projects/{id}/smoke-runs/{run}",
		"/{id}/smoke-runs/{run}",
		"/projects/{id}/smoke-runs/{run}",
	} {
		r.Get(p, requireAdmin(handleGet))
	}

	return r
}

// finish dọn lần chạy đã kết thúc (PASSED/FAILED/TIMEOUT): huỷ job còn QUEUED,
// đóng task gốc + sub-task chưa DONE (REJECTED), rồi đóng issue/PR mẫu trên GitHub.
// Job đang RUNNING để nguyên (server không dừng được runner); nếu nó nộp sau đó,
// pipeline chưa chặn job mới cho task REJECTED — giới hạn đã ghi ở BR-60.
func (s *Smoke) finish(ctx context.Context, rootID string) {
	_, _ = s.db.ExecContext(ctx, `
		UPDATE jobs SET status = 'CANCELLED', updated_at = CURRENT_TIMESTAMP
		WHERE status = 'QUEUED'
		  AND task_id IN (SELECT id FROM tasks WHERE id = ? OR parent_id = ?)
	`, rootID, rootID)
	_, _ = s.db.ExecContext(ctx, `
		UPDATE tasks SET status = 'REJECTED', updated_at = CURRENT_TIMESTAMP
		WHERE status <> 'DONE' AND (id = ? OR parent_id = ?)
	`, rootID, rootID)
	_ = s.ops.Cleanup(ctx, rootID)
}
