package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/namth/agenthub/internal/dispatcher"
)

var (
	ErrNotFound          = errors.New("task not found")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrValidation        = errors.New("validation error")
	ErrGitHub            = errors.New("github error")
	ErrNotConfigured     = errors.New("not configured")
)

type Task struct {
	ID            string  `json:"id"`
	ProjectID     string  `json:"project_id"`
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

func WithMerger(m Merger) Option {
	return func(s *Service) {
		s.merger = m
	}
}

func WithOnMerged(fn func(prURL string)) Option {
	return func(s *Service) {
		s.onMerged = fn
	}
}

func WithRoleIn(fn func(userID, projectID string) (role string, ok bool)) Option {
	return func(s *Service) {
		s.roleIn = fn
	}
}

func WithOwner(fn func(userID string) bool) Option {
	return func(s *Service) {
		s.isOwner = fn
	}
}

func WithMergerFor(fn func(projectID string) Merger) Option {
	return func(s *Service) {
		s.mergerFor = fn
	}
}

type Service struct {
	db        *sql.DB
	gh        GitHub
	disp      dispatcher.API
	projectID string
	onCreated func(Created)
	merger    Merger
	onMerged  func(string)

	isMulti   bool
	ghFor     func(projectID string) (GitHub, error)
	roleIn    func(userID, projectID string) (role string, ok bool)
	isOwner   func(userID string) bool
	mergerFor func(projectID string) Merger
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

func NewMultiService(db *sql.DB, ghFor func(projectID string) (GitHub, error), d dispatcher.API, opts ...Option) *Service {
	s := &Service{
		db:      db,
		ghFor:   ghFor,
		disp:    d,
		isMulti: true,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Service) githubFor(projectID string) (GitHub, error) {
	if s.ghFor != nil {
		return s.ghFor(projectID)
	}
	return s.gh, nil
}

func (s *Service) mergerForProject(projectID string) Merger {
	if s.mergerFor != nil {
		return s.mergerFor(projectID)
	}
	return s.merger
}

func (s *Service) UserRoleIn(userID, projectID string) (role string, ok bool) {
	if !s.isMulti {
		return "", true
	}
	if s.isOwner != nil && s.isOwner(userID) {
		return "ADMIN", true
	}
	if s.roleIn != nil {
		return s.roleIn(userID, projectID)
	}
	return "", false
}

func (s *Service) IsOwner(userID string) bool {
	if s.isOwner != nil {
		return s.isOwner(userID)
	}
	return false
}

func (s *Service) Create(ctx context.Context, userID, title, description string) (Task, error) {
	return s.CreateIn(ctx, userID, s.projectID, title, description)
}

func (s *Service) CreateIn(ctx context.Context, userID, projectID, title, description string) (Task, error) {
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
	gh, err := s.githubFor(projectID)
	if err != nil {
		return Task{}, fmt.Errorf("%w: %v", ErrGitHub, err)
	}
	if gh != nil {
		issueNum, _, err = gh.CreateIssue(ctx, titleTrim, description, []string{"status:pending-approval", "type:feature"})
		if err != nil {
			return Task{}, fmt.Errorf("%w: lỗi tạo issue trên GitHub: %v", ErrGitHub, err)
		}
	}

	taskID := newUUID()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO tasks (id, project_id, github_issue_id, title, description, status, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'PENDING_APPROVAL', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, taskID, projectID, issueNum, titleTrim, description, userID)
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
		ProjectID:     projectID,
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

	gh, _ := s.githubFor(pID)
	if ghIssueID.Valid && gh != nil {
		_ = gh.SetLabels(ctx, int(ghIssueID.Int64), []string{"status:planning", "type:feature"})
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

	res, err := s.db.ExecContext(ctx, "UPDATE tasks SET status = 'REJECTED', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'PENDING_APPROVAL'", taskID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: chuyển trạng thái không hợp lệ cho task %s", ErrInvalidTransition, taskID)
	}

	gh, _ := s.githubFor(pID)
	if ghIssueID.Valid && gh != nil {
		_ = gh.CloseIssue(ctx, int(ghIssueID.Int64), "Từ chối: "+reasonTrim)
	}

	return nil
}

func (s *Service) Merge(ctx context.Context, taskID string) error {
	var status string
	var prURL sql.NullString
	var pID string
	err := s.db.QueryRowContext(ctx, "SELECT status, pr_url, project_id FROM tasks WHERE id = ?", taskID).Scan(&status, &prURL, &pID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: task %s", ErrNotFound, taskID)
	}
	if err != nil {
		return err
	}

	merger := s.mergerForProject(pID)
	if merger == nil {
		return ErrNotConfigured
	}

	if status != "QA_PASSED" {
		return fmt.Errorf("%w: task %s có trạng thái %s (chỉ merge khi QA_PASSED)", ErrInvalidTransition, taskID, status)
	}

	if !prURL.Valid || strings.TrimSpace(prURL.String) == "" {
		return fmt.Errorf("%w: task %s không có pr_url", ErrValidation, taskID)
	}

	prNum, err := parsePRNumber(prURL.String)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrValidation, err)
	}

	if err := merger.MergePR(ctx, prNum); err != nil {
		return fmt.Errorf("%w: %v", ErrGitHub, err)
	}

	res, err := s.db.ExecContext(ctx, "UPDATE tasks SET status = 'DONE', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'QA_PASSED'", taskID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: task %s không ở trạng thái QA_PASSED", ErrInvalidTransition, taskID)
	}

	if s.onMerged != nil {
		s.onMerged(prURL.String)
	}

	return nil
}

func parsePRNumber(rawURL string) (int, error) {
	u := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	idx := strings.LastIndex(u, "/pull/")
	if idx == -1 {
		idx = strings.LastIndex(u, "/pulls/")
		if idx == -1 {
			return 0, fmt.Errorf("không tìm thấy /pull/<số> trong URL: %s", rawURL)
		}
		idx += len("/pulls/") - 1
	} else {
		idx += len("/pull/") - 1
	}
	part := u[idx+1:]
	num, err := strconv.Atoi(part)
	if err != nil {
		return 0, fmt.Errorf("số PR không hợp lệ (%s): %w", part, err)
	}
	return num, nil
}
