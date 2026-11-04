package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/namth/agenthub/internal/dispatcher"
)

var (
	ErrNotFound          = errors.New("task not found")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrValidation        = errors.New("validation error")
	ErrGitHub            = errors.New("github error")
)

type Task struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	Status        string  `json:"status"`
	GitHubIssueID int     `json:"github_issue_id"`
	ParentID      *string `json:"parent_id"`
	CreatedBy     string  `json:"created_by"`
}

type Created struct {
	TaskID      string
	Issue       int
	Title       string
	Requester   string
	Description string
}

type Option func(*Service)

func WithOnCreated(fn func(Created)) Option {
	return func(s *Service) {
		s.onCreated = fn
	}
}

type Service struct {
	db        *sql.DB
	gh        GitHub
	disp      dispatcher.API
	projectID string
	onCreated func(Created)
}

func NewService(db *sql.DB, gh GitHub, d dispatcher.API, projectID string, opts ...Option) *Service {
	s := &Service{
		db:        db,
		gh:        gh,
		disp:      d,
		projectID: projectID,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) Create(ctx context.Context, userID, title, description string) (Task, error) {
	titleTrim := strings.TrimSpace(title)
	titleLen := utf8.RuneCountInString(titleTrim)
	if titleLen < 6 || titleLen > 120 {
		return Task{}, fmt.Errorf("%w: tiêu đề sau khi trim phải từ 6 đến 120 ký tự", ErrValidation)
	}

	descTrim := strings.TrimSpace(description)
	if utf8.RuneCountInString(descTrim) < 20 {
		return Task{}, fmt.Errorf("%w: mô tả phải từ 20 ký tự trở lên", ErrValidation)
	}

	var issueNum int
	if s.gh != nil {
		var err error
		issueNum, _, err = s.gh.CreateIssue(ctx, titleTrim, description, []string{"status:pending-approval", "type:feature"})
		if err != nil {
			return Task{}, fmt.Errorf("%w: lỗi tạo issue trên GitHub: %v", ErrGitHub, err)
		}
	}

	taskID := newUUID()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, project_id, github_issue_id, title, description, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'PENDING_APPROVAL', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, taskID, s.projectID, issueNum, titleTrim, description, userID)
	if err != nil {
		return Task{}, fmt.Errorf("lỗi lưu task: %w", err)
	}

	var username string
	err = s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, userID).Scan(&username)
	if err != nil {
		username = userID
	}

	task := Task{
		ID:            taskID,
		Title:         titleTrim,
		Description:   description,
		Status:        "PENDING_APPROVAL",
		GitHubIssueID: issueNum,
		ParentID:      nil,
		CreatedBy:     userID,
	}

	if s.onCreated != nil {
		s.onCreated(Created{
			TaskID:      taskID,
			Issue:       issueNum,
			Title:       titleTrim,
			Requester:   username,
			Description: description,
		})
	}

	return task, nil
}

func (s *Service) Approve(ctx context.Context, taskID string) error {
	var currentStatus string
	var ghIssueID sql.NullInt64
	var pID string
	err := s.db.QueryRowContext(ctx, "SELECT status, github_issue_id, project_id FROM tasks WHERE id = ?", taskID).Scan(&currentStatus, &ghIssueID, &pID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: task %s", ErrNotFound, taskID)
	}
	if err != nil {
		return err
	}

	if currentStatus != "PENDING_APPROVAL" {
		return fmt.Errorf("%w: task %s có trạng thái %s", ErrInvalidTransition, taskID, currentStatus)
	}

	res, err := s.db.ExecContext(ctx, "UPDATE tasks SET status = 'PLANNING', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'PENDING_APPROVAL'", taskID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: chuyển trạng thái không hợp lệ cho task %s", ErrInvalidTransition, taskID)
	}

	if ghIssueID.Valid && s.gh != nil {
		_ = s.gh.SetLabels(ctx, int(ghIssueID.Int64), []string{"status:planning", "type:feature"})
	}

	if s.disp != nil {
		_, err := s.disp.EnqueueJob(dispatcher.NewJob{
			ProjectID:     pID,
			TaskID:        taskID,
			Kind:          dispatcher.JobSpec,
			RequiredAgent: dispatcher.KindClaude,
		})
		if err != nil {
			return fmt.Errorf("enqueue spec job: %w", err)
		}
	}

	return nil
}

func (s *Service) Reject(ctx context.Context, taskID, reason string) error {
	reasonTrim := strings.TrimSpace(reason)
	if reasonTrim == "" {
		return fmt.Errorf("%w: lý do từ chối không được để trống", ErrValidation)
	}

	var currentStatus string
	var ghIssueID sql.NullInt64
	err := s.db.QueryRowContext(ctx, "SELECT status, github_issue_id FROM tasks WHERE id = ?", taskID).Scan(&currentStatus, &ghIssueID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: task %s", ErrNotFound, taskID)
	}
	if err != nil {
		return err
	}

	if currentStatus != "PENDING_APPROVAL" {
		return fmt.Errorf("%w: task %s có trạng thái %s", ErrInvalidTransition, taskID, currentStatus)
	}

	res, err := s.db.ExecContext(ctx, "UPDATE tasks SET status = 'REJECTED', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'PENDING_APPROVAL'", taskID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: chuyển trạng thái không hợp lệ cho task %s", ErrInvalidTransition, taskID)
	}

	if ghIssueID.Valid && s.gh != nil {
		_ = s.gh.CloseIssue(ctx, int(ghIssueID.Int64), "Từ chối: "+reasonTrim)
	}

	return nil
}
