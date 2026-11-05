package contrib

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
)

type AuthzFunc func(*http.Request) (userID string, isOwner bool, ok bool)

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

type Row struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	Machines  int      `json:"machines"`
	Jobs      JobsStat `json:"jobs"`
	TasksDone int      `json:"tasks_done"`
	PRsMerged int      `json:"prs_merged"`
	BusyHours float64  `json:"busy_hours"`
}

type JobsStat struct {
	Done      int            `json:"done"`
	Failed    int            `json:"failed"`
	Exhausted int            `json:"exhausted"`
	ByKind    map[string]int `json:"by_kind"`
}

type TimelineItem struct {
	At        string `json:"at"`
	Worker    string `json:"worker"`
	Agent     string `json:"agent"`
	JobKind   string `json:"job_kind"`
	TaskID    string `json:"task_id"`
	TaskTitle string `json:"task_title"`
	Project   string `json:"project"`
	Result    string `json:"result"`
	Attempt   int    `json:"attempt"`
}

const dbTimeFormat = "2006-01-02 15:04:05"

func parseDBTime(s string) (time.Time, error) {
	if t, err := time.ParseInLocation(dbTimeFormat, s, time.UTC); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	return time.Parse(time.RFC3339, s)
}

func parseTimeParam(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	return time.Parse(time.RFC3339, s)
}

func Handler(db *sql.DB, authz func(*http.Request) (userID string, isOwner bool, ok bool)) http.Handler {
	r := chi.NewRouter()

	handleContrib := func(w http.ResponseWriter, req *http.Request) {
		userID, isOwner, ok := authz(req)
		if !ok || userID == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
			return
		}

		by := req.URL.Query().Get("by")
		if by != "partner" && by != "project" {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "tham số by không hợp lệ")
			return
		}

		fromStr := req.URL.Query().Get("from")
		toStr := req.URL.Query().Get("to")
		var tFrom, tTo time.Time
		if fromStr == "" || toStr == "" {
			tTo = time.Now().UTC()
			tFrom = tTo.AddDate(0, 0, -7)
		} else {
			var err error
			tFrom, err = parseTimeParam(fromStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", "from không đúng định dạng RFC3339")
				return
			}
			tTo, err = parseTimeParam(toStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", "to không đúng định dạng RFC3339")
				return
			}
		}

		fromDB := tFrom.UTC().Format(dbTimeFormat)
		toDB := tTo.UTC().Format(dbTimeFormat)

		var rows []Row
		if by == "partner" {
			var err error
			rows, err = getContributionsByPartner(db, userID, isOwner, fromDB, toDB)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
				return
			}
		} else {
			var err error
			rows, err = getContributionsByProject(db, userID, isOwner, fromDB, toDB)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
				return
			}
		}

		if rows == nil {
			rows = make([]Row, 0)
		}
		writeJSON(w, http.StatusOK, rows)
	}

	handleTimeline := func(w http.ResponseWriter, req *http.Request) {
		userID, isOwner, ok := authz(req)
		if !ok || userID == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
			return
		}

		partner := req.URL.Query().Get("partner")
		project := req.URL.Query().Get("project")

		if (partner == "" && project == "") || (partner != "" && project != "") {
			writeError(w, http.StatusBadRequest, "BAD_REQUEST", "phải cung cấp đúng một trong partner hoặc project")
			return
		}

		if partner != "" {
			if !isOwner && partner != userID {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "không có quyền xem")
				return
			}
		} else {
			if !isOwner {
				var count int
				err := db.QueryRow("SELECT COUNT(1) FROM project_members WHERE project_id = ? AND user_id = ?", project, userID).Scan(&count)
				if err != nil || count == 0 {
					writeError(w, http.StatusForbidden, "FORBIDDEN", "không có quyền xem")
					return
				}
			}
		}

		fromStr := req.URL.Query().Get("from")
		toStr := req.URL.Query().Get("to")
		var tFrom, tTo time.Time
		if fromStr == "" || toStr == "" {
			tTo = time.Now().UTC()
			tFrom = tTo.AddDate(0, 0, -7)
		} else {
			var err error
			tFrom, err = parseTimeParam(fromStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", "from không đúng định dạng RFC3339")
				return
			}
			tTo, err = parseTimeParam(toStr)
			if err != nil {
				writeError(w, http.StatusBadRequest, "BAD_REQUEST", "to không đúng định dạng RFC3339")
				return
			}
		}

		fromDB := tFrom.UTC().Format(dbTimeFormat)
		toDB := tTo.UTC().Format(dbTimeFormat)

		items, err := getTimeline(db, partner, project, fromDB, toDB)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}

		if items == nil {
			items = make([]TimelineItem, 0)
		}
		writeJSON(w, http.StatusOK, items)
	}

	r.Get("/api/contributions", handleContrib)
	r.Get("/api/contributions/timeline", handleTimeline)
	r.Get("/", handleContrib)
	r.Get("/timeline", handleTimeline)

	return r
}

type partnerGroup struct {
	key   string
	label string
}

func getContributionsByPartner(db *sql.DB, currentUserID string, isOwner bool, fromDB, toDB string) ([]Row, error) {
	q := `
		SELECT DISTINCT
			COALESCE(w.owner_user_id, 'system') AS p_key,
			COALESCE(u.username, 'Hệ thống') AS p_label
		FROM assignments a
		JOIN agents ag ON ag.id = a.agent_id
		JOIN workers w ON w.id = ag.worker_id
		LEFT JOIN users u ON u.id = w.owner_user_id
		WHERE a.started_at >= ? AND a.started_at < ?
	`
	var args []any
	args = append(args, fromDB, toDB)

	if !isOwner {
		q += " AND w.owner_user_id = ?"
		args = append(args, currentUserID)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []partnerGroup
	for rows.Next() {
		var g partnerGroup
		if err := rows.Scan(&g.key, &g.label); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}

	result := make([]Row, 0, len(groups))
	for _, g := range groups {
		var r Row
		r.Key = g.key
		r.Label = g.label
		r.Jobs.ByKind = make(map[string]int)

		// 1. Machines
		mQuery := `
			SELECT COUNT(DISTINCT w.id)
			FROM assignments a
			JOIN agents ag ON ag.id = a.agent_id
			JOIN workers w ON w.id = ag.worker_id
			WHERE a.started_at >= ? AND a.started_at < ?
			  AND ((? = 'system' AND w.owner_user_id IS NULL) OR (? != 'system' AND w.owner_user_id = ?))
		`
		if err := db.QueryRow(mQuery, fromDB, toDB, g.key, g.key, g.key).Scan(&r.Machines); err != nil {
			return nil, err
		}

		// 2. Jobs stats & busy hours
		jRows, err := db.Query(`
			SELECT
				a.status,
				j.kind,
				a.started_at,
				COALESCE(a.ended_at, '')
			FROM assignments a
			JOIN jobs j ON j.id = a.job_id
			JOIN agents ag ON ag.id = a.agent_id
			JOIN workers w ON w.id = ag.worker_id
			WHERE a.started_at >= ? AND a.started_at < ?
			  AND ((? = 'system' AND w.owner_user_id IS NULL) OR (? != 'system' AND w.owner_user_id = ?))
		`, fromDB, toDB, g.key, g.key, g.key)
		if err != nil {
			return nil, err
		}

		var totalBusySeconds float64
		for jRows.Next() {
			var status, kind, startedAt, endedAt string
			if err := jRows.Scan(&status, &kind, &startedAt, &endedAt); err != nil {
				jRows.Close()
				return nil, err
			}
			switch status {
			case "SUBMITTED":
				r.Jobs.Done++
				r.Jobs.ByKind[kind]++
			case "FAILED", "RECLAIMED", "ABORTED":
				r.Jobs.Failed++
			case "EXHAUSTED":
				r.Jobs.Exhausted++
			}

			if endedAt != "" && startedAt != "" {
				tStart, err1 := parseDBTime(startedAt)
				tEnd, err2 := parseDBTime(endedAt)
				if err1 == nil && err2 == nil && tEnd.After(tStart) {
					totalBusySeconds += tEnd.Sub(tStart).Seconds()
				}
			}
		}
		jRows.Close()
		r.BusyHours = math.Round((totalBusySeconds/3600.0)*100) / 100

		// 3. tasks_done and prs_merged
		tQuery := `
			SELECT
				COUNT(DISTINCT t.id),
				COUNT(DISTINCT CASE WHEN t.pr_url IS NOT NULL AND t.pr_url != '' THEN t.id END)
			FROM tasks t
			WHERE t.status = 'DONE'
			  AND t.updated_at >= ? AND t.updated_at < ?
			  AND EXISTS (
				  SELECT 1 FROM assignments a
				  JOIN jobs j ON j.id = a.job_id
				  JOIN agents ag ON ag.id = a.agent_id
				  JOIN workers w ON w.id = ag.worker_id
				  WHERE j.task_id = t.id
				    AND a.status = 'SUBMITTED'
				    AND ((? = 'system' AND w.owner_user_id IS NULL) OR (? != 'system' AND w.owner_user_id = ?))
			  )
		`
		if err := db.QueryRow(tQuery, fromDB, toDB, g.key, g.key, g.key).Scan(&r.TasksDone, &r.PRsMerged); err != nil {
			return nil, err
		}

		result = append(result, r)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Jobs.Done != result[j].Jobs.Done {
			return result[i].Jobs.Done > result[j].Jobs.Done
		}
		return result[i].Label < result[j].Label
	})

	return result, nil
}

type projectGroup struct {
	id         string
	githubRepo string
}

func getContributionsByProject(db *sql.DB, currentUserID string, isOwner bool, fromDB, toDB string) ([]Row, error) {
	q := `
		SELECT DISTINCT
			p.id,
			p.github_repo
		FROM assignments a
		JOIN jobs j ON j.id = a.job_id
		JOIN projects p ON p.id = j.project_id
		WHERE a.started_at >= ? AND a.started_at < ?
	`
	var args []any
	args = append(args, fromDB, toDB)

	if !isOwner {
		q += " AND p.id IN (SELECT project_id FROM project_members WHERE user_id = ?)"
		args = append(args, currentUserID)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []projectGroup
	for rows.Next() {
		var g projectGroup
		if err := rows.Scan(&g.id, &g.githubRepo); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}

	result := make([]Row, 0, len(groups))
	for _, g := range groups {
		var r Row
		r.Key = g.id
		r.Label = g.githubRepo
		r.Jobs.ByKind = make(map[string]int)

		// 1. Machines
		mQuery := `
			SELECT COUNT(DISTINCT w.id)
			FROM assignments a
			JOIN jobs j ON j.id = a.job_id
			JOIN agents ag ON ag.id = a.agent_id
			JOIN workers w ON w.id = ag.worker_id
			WHERE a.started_at >= ? AND a.started_at < ?
			  AND j.project_id = ?
		`
		if err := db.QueryRow(mQuery, fromDB, toDB, g.id).Scan(&r.Machines); err != nil {
			return nil, err
		}

		// 2. Jobs stats & busy hours
		jRows, err := db.Query(`
			SELECT
				a.status,
				j.kind,
				a.started_at,
				COALESCE(a.ended_at, '')
			FROM assignments a
			JOIN jobs j ON j.id = a.job_id
			WHERE a.started_at >= ? AND a.started_at < ?
			  AND j.project_id = ?
		`, fromDB, toDB, g.id)
		if err != nil {
			return nil, err
		}

		var totalBusySeconds float64
		for jRows.Next() {
			var status, kind, startedAt, endedAt string
			if err := jRows.Scan(&status, &kind, &startedAt, &endedAt); err != nil {
				jRows.Close()
				return nil, err
			}
			switch status {
			case "SUBMITTED":
				r.Jobs.Done++
				r.Jobs.ByKind[kind]++
			case "FAILED", "RECLAIMED", "ABORTED":
				r.Jobs.Failed++
			case "EXHAUSTED":
				r.Jobs.Exhausted++
			}

			if endedAt != "" && startedAt != "" {
				tStart, err1 := parseDBTime(startedAt)
				tEnd, err2 := parseDBTime(endedAt)
				if err1 == nil && err2 == nil && tEnd.After(tStart) {
					totalBusySeconds += tEnd.Sub(tStart).Seconds()
				}
			}
		}
		jRows.Close()
		r.BusyHours = math.Round((totalBusySeconds/3600.0)*100) / 100

		// 3. tasks_done and prs_merged
		tQuery := `
			SELECT
				COUNT(DISTINCT t.id),
				COUNT(DISTINCT CASE WHEN t.pr_url IS NOT NULL AND t.pr_url != '' THEN t.id END)
			FROM tasks t
			WHERE t.project_id = ?
			  AND t.status = 'DONE'
			  AND t.updated_at >= ? AND t.updated_at < ?
		`
		if err := db.QueryRow(tQuery, g.id, fromDB, toDB).Scan(&r.TasksDone, &r.PRsMerged); err != nil {
			return nil, err
		}

		result = append(result, r)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Jobs.Done != result[j].Jobs.Done {
			return result[i].Jobs.Done > result[j].Jobs.Done
		}
		return result[i].Label < result[j].Label
	})

	return result, nil
}

func getTimeline(db *sql.DB, partner, project, fromDB, toDB string) ([]TimelineItem, error) {
	q := `
		SELECT
			a.started_at,
			w.name,
			a.agent_id,
			j.kind,
			j.task_id,
			t.title,
			p.github_repo,
			a.status,
			a.attempt
		FROM assignments a
		JOIN jobs j ON j.id = a.job_id
		JOIN tasks t ON t.id = j.task_id
		JOIN projects p ON p.id = j.project_id
		JOIN agents ag ON ag.id = a.agent_id
		JOIN workers w ON w.id = ag.worker_id
		WHERE a.started_at >= ? AND a.started_at < ?
	`
	var args []any
	args = append(args, fromDB, toDB)

	if partner != "" {
		if partner == "system" {
			q += " AND w.owner_user_id IS NULL"
		} else {
			q += " AND w.owner_user_id = ?"
			args = append(args, partner)
		}
	} else if project != "" {
		q += " AND j.project_id = ?"
		args = append(args, project)
	}

	q += " ORDER BY a.started_at DESC LIMIT 500"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TimelineItem
	for rows.Next() {
		var it TimelineItem
		var startedAt string
		if err := rows.Scan(
			&startedAt,
			&it.Worker,
			&it.Agent,
			&it.JobKind,
			&it.TaskID,
			&it.TaskTitle,
			&it.Project,
			&it.Result,
			&it.Attempt,
		); err != nil {
			return nil, err
		}
		if t, err := parseDBTime(startedAt); err == nil {
			it.At = t.Format(time.RFC3339)
		} else {
			it.At = startedAt
		}
		items = append(items, it)
	}

	return items, nil
}
