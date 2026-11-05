package main

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/namth/agenthub/internal/tasks"
	"github.com/namth/agenthub/internal/telegram"
)

type TelegramCommander struct {
	db    *sql.DB
	svc   *tasks.Service
	owner string
}

func NewTelegramCommander(db *sql.DB, svc *tasks.Service, owner string) *TelegramCommander {
	return &TelegramCommander{
		db:    db,
		svc:   svc,
		owner: owner,
	}
}

func (c *TelegramCommander) getOwnerUserID(ctx context.Context) (string, error) {
	var id string
	if c.owner != "" {
		err := c.db.QueryRowContext(ctx, `SELECT id FROM users WHERE LOWER(username) = LOWER(?)`, c.owner).Scan(&id)
		if err == nil {
			return id, nil
		}
	}
	err := c.db.QueryRowContext(ctx, `SELECT id FROM users WHERE role = 'ADMIN' LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	err = c.db.QueryRowContext(ctx, `SELECT id FROM users LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	username := c.owner
	if username == "" {
		username = "owner"
	}
	id = "u-" + username
	_, err = c.db.ExecContext(ctx, `INSERT INTO users (id, username, role) VALUES (?, ?, 'ADMIN')`, id, username)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (c *TelegramCommander) CreateTask(ctx context.Context, project, title, description string) (string, error) {
	var projectID string
	err := c.db.QueryRowContext(ctx, `
		SELECT id FROM projects
		WHERE LOWER(name) = LOWER(?) OR LOWER(github_repo) = LOWER(?)
		LIMIT 1
	`, project, project).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", telegram.ErrNotFound
	}
	if err != nil {
		return "", err
	}

	userID, err := c.getOwnerUserID(ctx)
	if err != nil {
		return "", err
	}

	t, err := c.svc.CreateIn(ctx, userID, projectID, title, description)
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

func (c *TelegramCommander) ListTasks(ctx context.Context, project string, limit int) ([]telegram.TaskBrief, error) {
	if limit <= 0 {
		limit = 10
	}

	var query string
	var args []any
	if strings.TrimSpace(project) == "" {
		query = `
			SELECT t.id, t.title, t.status, p.name, COALESCE(t.pr_url, '')
			FROM tasks t
			JOIN projects p ON t.project_id = p.id
			WHERE t.parent_id IS NULL
			ORDER BY t.created_at DESC
			LIMIT ?
		`
		args = []any{limit}
	} else {
		query = `
			SELECT t.id, t.title, t.status, p.name, COALESCE(t.pr_url, '')
			FROM tasks t
			JOIN projects p ON t.project_id = p.id
			WHERE t.parent_id IS NULL
			  AND (LOWER(p.name) = LOWER(?) OR LOWER(p.github_repo) = LOWER(?))
			ORDER BY t.created_at DESC
			LIMIT ?
		`
		args = []any{project, project, limit}
	}

	rows, err := c.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []telegram.TaskBrief
	for rows.Next() {
		var tb telegram.TaskBrief
		if err := rows.Scan(&tb.ID, &tb.Title, &tb.Status, &tb.Project, &tb.PRURL); err != nil {
			return nil, err
		}
		result = append(result, tb)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *TelegramCommander) TaskStatus(ctx context.Context, id string) (telegram.TaskBrief, error) {
	var tb telegram.TaskBrief
	err := c.db.QueryRowContext(ctx, `
		SELECT t.id, t.title, t.status, p.name, COALESCE(t.pr_url, '')
		FROM tasks t
		JOIN projects p ON t.project_id = p.id
		WHERE t.id = ?
	`, id).Scan(&tb.ID, &tb.Title, &tb.Status, &tb.Project, &tb.PRURL)
	if errors.Is(err, sql.ErrNoRows) {
		return telegram.TaskBrief{}, telegram.ErrNotFound
	}
	if err != nil {
		return telegram.TaskBrief{}, err
	}

	rows, err := c.db.QueryContext(ctx, `
		SELECT id, title, status, COALESCE(pr_url, '')
		FROM tasks
		WHERE parent_id = ?
		ORDER BY created_at ASC
	`, id)
	if err != nil {
		return telegram.TaskBrief{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var sub telegram.TaskBrief
		if err := rows.Scan(&sub.ID, &sub.Title, &sub.Status, &sub.PRURL); err != nil {
			return telegram.TaskBrief{}, err
		}
		tb.Subtasks = append(tb.Subtasks, sub)
	}
	if err := rows.Err(); err != nil {
		return telegram.TaskBrief{}, err
	}

	return tb, nil
}

func (c *TelegramCommander) Merge(ctx context.Context, id string) error {
	return c.svc.Merge(ctx, id)
}

func (c *TelegramCommander) PickOption(ctx context.Context, taskID string, round int, questionID string, optionIdx int) (bool, error) {
	userID, err := c.getOwnerUserID(ctx)
	if err != nil {
		return false, err
	}
	return c.svc.PickOption(ctx, userID, taskID, round, questionID, optionIdx)
}

func (c *TelegramCommander) DecideProposal(ctx context.Context, taskID, proposalID string, accept bool) error {
	userID, err := c.getOwnerUserID(ctx)
	if err != nil {
		return err
	}
	return c.svc.DecideProposal(ctx, userID, "ADMIN", taskID, proposalID, accept)
}
