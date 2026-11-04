package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/namth/agenthub/internal/dispatcher"
)

// Authz trả vai trò người gọi; ok=false nghĩa là chưa đăng nhập.
type Authz func(r *http.Request) (role string, ok bool)

type apiResponse struct {
	Success bool      `json:"success"`
	Data    any       `json:"data"`
	Error   *apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Success: true,
		Data:    data,
		Error:   nil,
	})
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Success: false,
		Data:    nil,
		Error: &apiError{
			Code:    code,
			Message: msg,
		},
	})
}

const dbTimeFormat = "2006-01-02 15:04:05"

func formatRFC3339(val any) *string {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case time.Time:
		s := v.UTC().Format(time.RFC3339)
		return &s
	case string:
		if v == "" {
			return nil
		}
		if t, err := time.ParseInLocation(dbTimeFormat, v, time.UTC); err == nil {
			s := t.Format(time.RFC3339)
			return &s
		}
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			s := t.UTC().Format(time.RFC3339)
			return &s
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			s := t.UTC().Format(time.RFC3339)
			return &s
		}
	}
	return nil
}

type FleetLead struct {
	State       string  `json:"state"`
	AgentID     *string `json:"agent_id"`
	Epoch       int64   `json:"epoch"`
	ExpiresAt   *string `json:"expires_at"`
	VacantSince *string `json:"vacant_since"`
	ETA         *string `json:"eta"`
	WaitingJobs int     `json:"waiting_jobs"`
}

type CurrentJobItem struct {
	JobID     string `json:"job_id"`
	TaskID    string `json:"task_id"`
	Kind      string `json:"kind"`
	Attempt   int    `json:"attempt"`
	ProjectID string `json:"project_id"`
	Project   string `json:"project"`
	TaskTitle string `json:"task_title"`
}

type AgentItem struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"`
	Status          string          `json:"status"`
	CooldownUntil   *string         `json:"cooldown_until"`
	LeadPriority    *int            `json:"lead_priority"`
	SoftMaxJobs     *int            `json:"soft_max_jobs"`
	SoftWindowHours int             `json:"soft_window_hours"`
	JobsInWindow    int             `json:"jobs_in_window"`
	LeadEligible    bool            `json:"lead_eligible"`
	AuthStatus      string          `json:"auth_status"`
	AuthDetail      *string         `json:"auth_detail"`
	AuthCheckedAt   *string         `json:"auth_checked_at"`
	Projects        *[]string       `json:"projects"`
	CurrentJob      *CurrentJobItem `json:"current_job"`
}

type WorkerOwner struct {
	ID    string `json:"id"`
	Login string `json:"login"`
}

type WorkerItem struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	OS           string       `json:"os"`
	ConnStatus   string       `json:"conn_status"`
	OfflineSince *string      `json:"offline_since"`
	Owner        *WorkerOwner `json:"owner"`
	Version      *string      `json:"version"`
	Paused       bool         `json:"paused"`
	GhAuth       string       `json:"gh_auth"`
	Agents       []AgentItem  `json:"agents"`
}

type FleetResponse struct {
	Lead    FleetLead    `json:"lead"`
	Workers []WorkerItem `json:"workers"`
}

type AdminLeadActionResponse struct {
	State     string  `json:"state"`
	AgentID   *string `json:"agent_id"`
	Epoch     int64   `json:"epoch"`
	ExpiresAt *string `json:"expires_at"`
}

// FleetHandler khởi tạo HTTP handler phục vụ các API Fleet và quản trị trưởng phòng.
func FleetHandler(db *sql.DB, d dispatcher.API, authz Authz) http.Handler {
	r := chi.NewRouter()

	// RBAC Middleware: mọi đường chỉ cho ADMIN
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			role, ok := authz(req)
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
				return
			}
			if role != "ADMIN" {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "yêu cầu quyền ADMIN")
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	r.Get("/api/fleet", func(w http.ResponseWriter, req *http.Request) {
		handleGetFleet(w, req, db, d)
	})

	r.Post("/api/admin/lead/assign", func(w http.ResponseWriter, req *http.Request) {
		handlePostLeadAssign(w, req, d)
	})

	r.Post("/api/admin/lead/revoke", func(w http.ResponseWriter, req *http.Request) {
		handlePostLeadRevoke(w, req, d)
	})

	r.Patch("/api/admin/agents/{id}", func(w http.ResponseWriter, req *http.Request) {
		handlePatchAgent(w, req, db)
	})

	return r
}

func handleGetFleet(w http.ResponseWriter, req *http.Request, db *sql.DB, d dispatcher.API) {
	lease, err := d.Lead()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi truy vấn lead lease")
		return
	}

	fleetLead := FleetLead{
		State: lease.State,
		Epoch: lease.Epoch,
	}
	if lease.State != "VACANT" && lease.AgentID != "" {
		fleetLead.AgentID = &lease.AgentID
	}
	if lease.ExpiresAt != nil && lease.State == "HELD" {
		expStr := lease.ExpiresAt.UTC().Format(time.RFC3339)
		fleetLead.ExpiresAt = &expStr
	}

	var vacantSinceVal any
	_ = db.QueryRow("SELECT vacant_since FROM lead_lease WHERE id = 1").Scan(&vacantSinceVal)
	if lease.State == "VACANT" {
		fleetLead.VacantSince = formatRFC3339(vacantSinceVal)
	}

	// ETA = MIN(cooldown_until) của agent CLAUDE đang COOLDOWN
	var minCdVal any
	_ = db.QueryRow(
		"SELECT MIN(cooldown_until) FROM agents WHERE kind = 'CLAUDE' AND status = 'COOLDOWN' AND cooldown_until IS NOT NULL",
	).Scan(&minCdVal)
	fleetLead.ETA = formatRFC3339(minCdVal)

	// Waiting jobs = số job required_agent='CLAUDE' đang QUEUED
	var waitingJobs int
	_ = db.QueryRow("SELECT COUNT(*) FROM jobs WHERE required_agent = 'CLAUDE' AND status = 'QUEUED'").Scan(&waitingJobs)
	fleetLead.WaitingJobs = waitingJobs

	// Workers
	workers := make([]WorkerItem, 0)
	rows, err := db.Query(`
		SELECT w.id, w.name, w.os, w.conn_status, w.offline_since, w.version, w.paused, u.id, u.username, w.gh_auth
		FROM workers w
		LEFT JOIN users u ON w.owner_user_id = u.id
		ORDER BY w.id ASC
	`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi truy vấn workers")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var wk WorkerItem
		var offVal any
		var verVal sql.NullString
		var pausedInt int
		var uID sql.NullString
		var uName sql.NullString
		if err := rows.Scan(&wk.ID, &wk.Name, &wk.OS, &wk.ConnStatus, &offVal, &verVal, &pausedInt, &uID, &uName, &wk.GhAuth); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi đọc workers")
			return
		}
		wk.OfflineSince = formatRFC3339(offVal)
		if verVal.Valid {
			wk.Version = &verVal.String
		}
		wk.Paused = (pausedInt != 0)
		if uID.Valid && uName.Valid {
			wk.Owner = &WorkerOwner{
				ID:    uID.String,
				Login: uName.String,
			}
		}
		wk.Agents = make([]AgentItem, 0)

		// Lấy agents của worker
		agentRows, err := db.Query(
			"SELECT id FROM agents WHERE worker_id = ? ORDER BY id ASC",
			wk.ID,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi truy vấn agents")
			return
		}

		var agentIDs []string
		for agentRows.Next() {
			var agID string
			if err := agentRows.Scan(&agID); err == nil {
				agentIDs = append(agentIDs, agID)
			}
		}
		agentRows.Close()

		for _, agID := range agentIDs {
			ag, err := getAgentDetail(db, agID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi lấy thông tin agent")
				return
			}
			wk.Agents = append(wk.Agents, *ag)
		}

		workers = append(workers, wk)
	}

	writeJSON(w, http.StatusOK, FleetResponse{
		Lead:    fleetLead,
		Workers: workers,
	})
}

func getAgentDetail(db *sql.DB, agentID string) (*AgentItem, error) {
	var ag AgentItem
	var cdVal any
	var lp sql.NullInt64
	var sm sql.NullInt64
	var leadEligibleInt int
	var authStatus string
	var authDetail, authCheckedAt sql.NullString

	err := db.QueryRow(
		`SELECT id, kind, status, cooldown_until, lead_priority, soft_max_jobs, soft_window_hours, lead_eligible, auth_status, auth_detail, auth_checked_at
		 FROM agents WHERE id = ?`,
		agentID,
	).Scan(&ag.ID, &ag.Kind, &ag.Status, &cdVal, &lp, &sm, &ag.SoftWindowHours, &leadEligibleInt, &authStatus, &authDetail, &authCheckedAt)
	if err != nil {
		return nil, err
	}

	ag.AuthStatus = authStatus
	if authDetail.Valid && authDetail.String != "" {
		ag.AuthDetail = &authDetail.String
	}
	if authCheckedAt.Valid && authCheckedAt.String != "" {
		ag.AuthCheckedAt = formatRFC3339(authCheckedAt.String)
	}

	ag.LeadEligible = (leadEligibleInt != 0)
	ag.CooldownUntil = formatRFC3339(cdVal)
	if lp.Valid {
		v := int(lp.Int64)
		ag.LeadPriority = &v
	}
	if sm.Valid {
		v := int(sm.Int64)
		ag.SoftMaxJobs = &v
	}
	if ag.SoftWindowHours <= 0 {
		ag.SoftWindowHours = 5
	}

	pRows, err := db.Query("SELECT project_id FROM agent_projects WHERE agent_id = ? ORDER BY project_id ASC", agentID)
	if err != nil {
		return nil, err
	}
	defer pRows.Close()

	var pList []string
	for pRows.Next() {
		var pid string
		if err := pRows.Scan(&pid); err == nil {
			pList = append(pList, pid)
		}
	}
	if err := pRows.Err(); err != nil {
		return nil, err
	}
	if len(pList) > 0 {
		ag.Projects = &pList
	} else {
		ag.Projects = nil
	}

	// Tính jobs_in_window theo thời gian thật hiện tại
	windowStart := time.Now().UTC().Add(-time.Duration(ag.SoftWindowHours) * time.Hour)
	windowStartStr := windowStart.Format(dbTimeFormat)
	err = db.QueryRow(
		"SELECT COUNT(*) FROM assignments WHERE agent_id = ? AND started_at >= ?",
		agentID, windowStartStr,
	).Scan(&ag.JobsInWindow)
	if err != nil {
		return nil, err
	}

	// Current job của assignment ACTIVE
	var cj CurrentJobItem
	err = db.QueryRow(
		`SELECT a.job_id, j.task_id, j.kind, a.attempt, j.project_id, p.github_repo, t.title
		 FROM assignments a
		 JOIN jobs j ON a.job_id = j.id
		 JOIN projects p ON j.project_id = p.id
		 JOIN tasks t ON j.task_id = t.id
		 WHERE a.agent_id = ? AND a.status = 'ACTIVE'
		 ORDER BY a.started_at DESC
		 LIMIT 1`,
		agentID,
	).Scan(&cj.JobID, &cj.TaskID, &cj.Kind, &cj.Attempt, &cj.ProjectID, &cj.Project, &cj.TaskTitle)
	if err == nil {
		ag.CurrentJob = &cj
	} else if errors.Is(err, sql.ErrNoRows) {
		ag.CurrentJob = nil
	} else {
		return nil, err
	}

	return &ag, nil
}

func handlePostLeadAssign(w http.ResponseWriter, req *http.Request, d dispatcher.API) {
	var body struct {
		AgentID *string `json:"agent_id"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.AgentID == nil || strings.TrimSpace(*body.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "thiếu agent_id")
		return
	}

	lease, err := d.AdminAssign(*body.AgentID)
	if err != nil {
		switch {
		case errors.Is(err, dispatcher.ErrNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "không tìm thấy agent")
		case errors.Is(err, dispatcher.ErrAgentNotEligible):
			writeError(w, http.StatusConflict, "AGENT_NOT_ELIGIBLE", "agent không đủ điều kiện")
		case errors.Is(err, dispatcher.ErrNoLead):
			writeError(w, http.StatusConflict, "NO_LEAD", "không có lead")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi hệ thống")
		}
		return
	}

	res := AdminLeadActionResponse{
		State: lease.State,
		Epoch: lease.Epoch,
	}
	if lease.AgentID != "" {
		res.AgentID = &lease.AgentID
	}
	if lease.ExpiresAt != nil {
		expStr := lease.ExpiresAt.UTC().Format(time.RFC3339)
		res.ExpiresAt = &expStr
	}

	writeJSON(w, http.StatusOK, res)
}

func handlePostLeadRevoke(w http.ResponseWriter, req *http.Request, d dispatcher.API) {
	lease, err := d.AdminRevoke()
	if err != nil {
		switch {
		case errors.Is(err, dispatcher.ErrNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "không tìm thấy lead")
		case errors.Is(err, dispatcher.ErrAgentNotEligible):
			writeError(w, http.StatusConflict, "AGENT_NOT_ELIGIBLE", "agent không đủ điều kiện")
		case errors.Is(err, dispatcher.ErrNoLead):
			writeError(w, http.StatusConflict, "NO_LEAD", "không có lead để thu hồi")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi hệ thống")
		}
		return
	}

	res := AdminLeadActionResponse{
		State: lease.State,
		Epoch: lease.Epoch,
	}
	if lease.AgentID != "" {
		res.AgentID = &lease.AgentID
	}
	if lease.ExpiresAt != nil {
		expStr := lease.ExpiresAt.UTC().Format(time.RFC3339)
		res.ExpiresAt = &expStr
	}

	writeJSON(w, http.StatusOK, res)
}

func handlePatchAgent(w http.ResponseWriter, req *http.Request, db *sql.DB) {
	rawID := chi.URLParam(req, "id")
	agentID, err := url.PathUnescape(rawID)
	if err != nil || agentID == "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "không tìm thấy agent")
		return
	}

	var kind string
	err = db.QueryRow("SELECT kind FROM agents WHERE id = ?", agentID).Scan(&kind)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "không tìm thấy agent")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi kiểm tra agent")
		return
	}

	var rawMap map[string]json.RawMessage
	dec := json.NewDecoder(req.Body)
	if err := dec.Decode(&rawMap); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "body JSON không hợp lệ")
		return
	}

	for k := range rawMap {
		if k != "lead_priority" && k != "soft_max_jobs" && k != "soft_window_hours" && k != "lead_eligible" && k != "projects" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "khoá lạ: "+k)
			return
		}
	}

	var updateLeadPriority bool
	var newLeadPriority *int
	if raw, ok := rawMap["lead_priority"]; ok {
		if kind != "CLAUDE" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "lead_priority chỉ áp dụng cho agent CLAUDE")
			return
		}
		updateLeadPriority = true
		if string(raw) == "null" {
			newLeadPriority = nil
		} else {
			var rawVal any
			if err := json.Unmarshal(raw, &rawVal); err != nil {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "lead_priority sai kiểu")
				return
			}
			f, ok := rawVal.(float64)
			if !ok || f != float64(int(f)) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "lead_priority phải là số nguyên")
				return
			}
			val := int(f)
			if val < 1 || val > 99 {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "lead_priority phải từ 1 đến 99")
				return
			}
			newLeadPriority = &val
		}
	}

	var updateSoftMaxJobs bool
	var newSoftMaxJobs *int
	if raw, ok := rawMap["soft_max_jobs"]; ok {
		updateSoftMaxJobs = true
		if string(raw) == "null" {
			newSoftMaxJobs = nil
		} else {
			var rawVal any
			if err := json.Unmarshal(raw, &rawVal); err != nil {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "soft_max_jobs sai kiểu")
				return
			}
			f, ok := rawVal.(float64)
			if !ok || f != float64(int(f)) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "soft_max_jobs phải là số nguyên")
				return
			}
			val := int(f)
			if val < 1 || val > 500 {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "soft_max_jobs phải từ 1 đến 500")
				return
			}
			newSoftMaxJobs = &val
		}
	}

	var updateSoftWindowHours bool
	var newSoftWindowHours int
	if raw, ok := rawMap["soft_window_hours"]; ok {
		updateSoftWindowHours = true
		if string(raw) == "null" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "soft_window_hours không được null")
			return
		}
		var rawVal any
		if err := json.Unmarshal(raw, &rawVal); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "soft_window_hours sai kiểu")
			return
		}
		f, ok := rawVal.(float64)
		if !ok || f != float64(int(f)) {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "soft_window_hours phải là số nguyên")
			return
		}
		val := int(f)
		if val < 1 || val > 24 {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "soft_window_hours phải từ 1 đến 24")
			return
		}
		newSoftWindowHours = val
	}

	var updateLeadEligible bool
	var newLeadEligible int
	if raw, ok := rawMap["lead_eligible"]; ok {
		if string(raw) == "null" {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "lead_eligible không được null")
			return
		}
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "lead_eligible phải là boolean")
			return
		}
		updateLeadEligible = true
		if b {
			newLeadEligible = 1
		} else {
			newLeadEligible = 0
		}
	}

	var updateProjects bool
	var newProjectIDs []string
	if raw, ok := rawMap["projects"]; ok {
		updateProjects = true
		if string(raw) == "null" {
			newProjectIDs = nil
		} else {
			var rawList []any
			if err := json.Unmarshal(raw, &rawList); err != nil {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "projects phải là mảng hoặc null")
				return
			}
			for _, item := range rawList {
				s, ok := item.(string)
				if !ok {
					writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "mỗi project id phải là chuỗi")
					return
				}
				newProjectIDs = append(newProjectIDs, s)
			}
		}
	}

	var uniqueProjects []string
	if updateProjects && len(newProjectIDs) > 0 {
		seen := make(map[string]bool)
		for _, pid := range newProjectIDs {
			var dummy int
			err := db.QueryRow("SELECT 1 FROM projects WHERE id = ?", pid).Scan(&dummy)
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "project không tồn tại: "+pid)
				return
			}
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi kiểm tra dự án")
				return
			}
			if !seen[pid] {
				seen[pid] = true
				uniqueProjects = append(uniqueProjects, pid)
			}
		}
	}

	var setClauses []string
	var args []any
	if updateLeadPriority {
		setClauses = append(setClauses, "lead_priority = ?")
		args = append(args, newLeadPriority)
	}
	if updateSoftMaxJobs {
		setClauses = append(setClauses, "soft_max_jobs = ?")
		args = append(args, newSoftMaxJobs)
	}
	if updateSoftWindowHours {
		setClauses = append(setClauses, "soft_window_hours = ?")
		args = append(args, newSoftWindowHours)
	}
	if updateLeadEligible {
		setClauses = append(setClauses, "lead_eligible = ?")
		args = append(args, newLeadEligible)
	}

	tx, err := db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi bắt đầu transaction")
		return
	}
	defer tx.Rollback()

	if len(setClauses) > 0 {
		query := "UPDATE agents SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
		args = append(args, agentID)
		if _, err := tx.Exec(query, args...); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi cập nhật agent")
			return
		}
	}

	if updateProjects {
		if _, err := tx.Exec("DELETE FROM agent_projects WHERE agent_id = ?", agentID); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi xoá agent_projects")
			return
		}
		for _, pid := range uniqueProjects {
			if _, err := tx.Exec("INSERT INTO agent_projects (agent_id, project_id) VALUES (?, ?)", agentID, pid); err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi thêm agent_projects")
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi commit")
		return
	}

	updatedAgent, err := getAgentDetail(db, agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "lỗi lấy thông tin agent sau khi sửa")
		return
	}

	writeJSON(w, http.StatusOK, updatedAgent)
}
