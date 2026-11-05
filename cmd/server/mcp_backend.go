package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/mcp"
	"github.com/namth/agenthub/internal/tasks"
)

type serverMCPBackend struct {
	db       *sql.DB
	tasksSvc *tasks.Service
	disp     dispatcher.API
}

func newMCPBackend(db *sql.DB, tasksSvc *tasks.Service, disp dispatcher.API) mcp.Backend {
	return &serverMCPBackend{
		db:       db,
		tasksSvc: tasksSvc,
		disp:     disp,
	}
}

func (b *serverMCPBackend) resolveProjectID(project string) (string, error) {
	if project == "" {
		var id string
		err := b.db.QueryRow(`SELECT id FROM projects ORDER BY created_at ASC LIMIT 1`).Scan(&id)
		if err != nil {
			return "", fmt.Errorf("không tìm thấy dự án mặc định: %w", err)
		}
		return id, nil
	}
	var id string
	err := b.db.QueryRow(`SELECT id FROM projects WHERE id = ? OR name = ? OR github_repo = ? LIMIT 1`, project, project, project).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("dự án %q không tồn tại", project)
	}
	if err != nil {
		return "", err
	}
	return id, nil
}

func (b *serverMCPBackend) CreateTask(ctx context.Context, userID, project, title, description string) (map[string]any, error) {
	pID, err := b.resolveProjectID(project)
	if err != nil {
		return nil, err
	}
	t, err := b.tasksSvc.CreateIn(ctx, userID, pID, title, description)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":     t.ID,
		"status": t.Status,
	}, nil
}

func (b *serverMCPBackend) ListTasks(ctx context.Context, userID, project, status string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	query := `SELECT t.id, t.project_id, p.name, t.title, t.status, t.created_at,
		(SELECT COUNT(*) FROM task_clarifications tc WHERE tc.task_id = t.id AND tc.status = 'WAITING') as waiting_clarifications
		FROM tasks t
		LEFT JOIN projects p ON p.id = t.project_id
		WHERE 1=1`
	var args []any
	if project != "" {
		pID, err := b.resolveProjectID(project)
		if err == nil {
			query += ` AND t.project_id = ?`
			args = append(args, pID)
		}
	}
	if status != "" {
		query += ` AND t.status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY t.created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := b.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		var id, pID, title, st, createdAt string
		var pName sql.NullString
		var waitingClarifications int
		if err := rows.Scan(&id, &pID, &pName, &title, &st, &createdAt, &waitingClarifications); err != nil {
			return nil, err
		}
		item := map[string]any{
			"id":         id,
			"project_id": pID,
			"project":    pName.String,
			"title":      title,
			"status":     st,
			"created_at": createdAt,
		}
		if waitingClarifications > 0 {
			item["needs_clarification"] = true
		}
		result = append(result, item)
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result, nil
}

func (b *serverMCPBackend) TaskStatus(ctx context.Context, userID, id string) (map[string]any, error) {
	var projectID, title, desc, status string
	var prURL, branchName sql.NullString
	var ghIssueID sql.NullInt64
	err := b.db.QueryRowContext(ctx, `
		SELECT project_id, title, description, status, pr_url, branch_name, github_issue_id
		FROM tasks WHERE id = ?
	`, id).Scan(&projectID, &title, &desc, &status, &prURL, &branchName, &ghIssueID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("task not found")
	}
	if err != nil {
		return nil, err
	}

	subRows, err := b.db.QueryContext(ctx, `SELECT id, title, status FROM tasks WHERE parent_id = ? ORDER BY created_at ASC`, id)
	var subtasks []map[string]any
	if err == nil {
		defer subRows.Close()
		for subRows.Next() {
			var sid, stitle, sstatus string
			if err := subRows.Scan(&sid, &stitle, &sstatus); err == nil {
				subtasks = append(subtasks, map[string]any{"id": sid, "title": stitle, "status": sstatus})
			}
		}
	}
	if subtasks == nil {
		subtasks = []map[string]any{}
	}

	cRows, err := b.db.QueryContext(ctx, `SELECT round, questions, answers, status FROM task_clarifications WHERE task_id = ? ORDER BY round ASC`, id)
	var clarifications []map[string]any
	if err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var round int
			var q, st string
			var a sql.NullString
			if err := cRows.Scan(&round, &q, &a, &st); err == nil {
				clarifications = append(clarifications, map[string]any{
					"round":     round,
					"questions": q,
					"answers":   a.String,
					"status":    st,
				})
			}
		}
	}
	if clarifications == nil {
		clarifications = []map[string]any{}
	}

	jRows, err := b.db.QueryContext(ctx, `SELECT id, kind, status, attempt FROM jobs WHERE task_id = ? ORDER BY created_at DESC`, id)
	var jobs []map[string]any
	if err == nil {
		defer jRows.Close()
		for jRows.Next() {
			var jid, jkind, jst string
			var jatt int
			if err := jRows.Scan(&jid, &jkind, &jst, &jatt); err == nil {
				jobs = append(jobs, map[string]any{
					"id":      jid,
					"kind":    jkind,
					"status":  jst,
					"attempt": jatt,
				})
			}
		}
	}
	if jobs == nil {
		jobs = []map[string]any{}
	}

	var logs []string
	logRows, err := b.db.QueryContext(ctx, `
		SELECT chunk FROM task_logs 
		WHERE assignment_id IN (SELECT id FROM assignments WHERE job_id IN (SELECT id FROM jobs WHERE task_id = ?))
		ORDER BY id DESC LIMIT 20
	`, id)
	if err == nil {
		defer logRows.Close()
		for logRows.Next() {
			var chunk string
			if err := logRows.Scan(&chunk); err == nil {
				lines := strings.Split(chunk, "\n")
				for _, line := range lines {
					if trimmed := strings.TrimSpace(line); trimmed != "" {
						logs = append(logs, trimmed)
						if len(logs) >= 20 {
							break
						}
					}
				}
				if len(logs) >= 20 {
					break
				}
			}
		}
	}
	if logs == nil {
		logs = []string{}
	}

	res := map[string]any{
		"id":             id,
		"project_id":     projectID,
		"title":          title,
		"description":    desc,
		"status":         status,
		"pr_url":         prURL.String,
		"branch_name":    branchName.String,
		"subtasks":       subtasks,
		"clarifications": clarifications,
		"jobs":           jobs,
		"recent_logs":    logs,
	}
	if ghIssueID.Valid {
		res["github_issue_id"] = ghIssueID.Int64
	}
	return res, nil
}

func (b *serverMCPBackend) AnswerClarification(ctx context.Context, userID, id string, answers map[string]string) (map[string]any, error) {
	return nil, errors.New("chưa hỗ trợ")
}

func (b *serverMCPBackend) ApproveTask(ctx context.Context, userID, id string) (map[string]any, error) {
	if err := b.tasksSvc.Approve(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{
		"id":     id,
		"status": "PLANNING",
	}, nil
}

func (b *serverMCPBackend) MergeTask(ctx context.Context, userID, id string) (map[string]any, error) {
	if err := b.tasksSvc.Merge(ctx, id); err != nil {
		return nil, err
	}
	return map[string]any{
		"id":     id,
		"status": "DONE",
	}, nil
}

func (b *serverMCPBackend) DecideProposal(ctx context.Context, userID, taskID, proposalID string, accept bool) (map[string]any, error) {
	if err := b.tasksSvc.DecideProposal(ctx, userID, "ADMIN", taskID, proposalID, accept); err != nil {
		return nil, err
	}
	return map[string]any{
		"task_id":     taskID,
		"proposal_id": proposalID,
		"accepted":    accept,
	}, nil
}

func (b *serverMCPBackend) FleetStatus(ctx context.Context, userID string) (map[string]any, error) {
	var workers []map[string]any
	wRows, err := b.db.QueryContext(ctx, `SELECT id, name, os, conn_status FROM workers ORDER BY name ASC`)
	if err == nil {
		defer wRows.Close()
		for wRows.Next() {
			var wid, wname, wos, wstatus string
			if err := wRows.Scan(&wid, &wname, &wos, &wstatus); err == nil {
				workers = append(workers, map[string]any{
					"id":          wid,
					"name":        wname,
					"os":          wos,
					"conn_status": wstatus,
				})
			}
		}
	}
	if workers == nil {
		workers = []map[string]any{}
	}

	var agents []map[string]any
	aRows, err := b.db.QueryContext(ctx, `SELECT id, worker_id, kind, status, auth_status FROM agents ORDER BY id ASC`)
	if err == nil {
		defer aRows.Close()
		for aRows.Next() {
			var aid, awid, akind, astatus string
			var authStatus sql.NullString
			if err := aRows.Scan(&aid, &awid, &akind, &astatus, &authStatus); err == nil {
				agents = append(agents, map[string]any{
					"id":          aid,
					"worker_id":   awid,
					"kind":        akind,
					"status":      astatus,
					"auth_status": authStatus.String,
				})
			}
		}
	}
	if agents == nil {
		agents = []map[string]any{}
	}

	var leadInfo any
	if b.disp != nil {
		if lease, err := b.disp.Lead(); err == nil {
			leadInfo = lease
		}
	}

	return map[string]any{
		"workers": workers,
		"agents":  agents,
		"lead":    leadInfo,
	}, nil
}
