package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/namth/agenthub/internal/dispatcher"
)

var (
	ErrNotFound          = errors.New("task not found")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrValidation        = errors.New("validation error")
	ErrGitHub            = errors.New("github error")
	ErrNotConfigured     = errors.New("not configured")
	ErrForbidden         = errors.New("FORBIDDEN")
)

type ClarifyQuestion struct {
	ID          string   `json:"id"`
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Recommended string   `json:"recommended"`
	Reason      string   `json:"reason"`
}

type ClarifyRound struct {
	Round       int               `json:"round"`
	Status      string            `json:"status"`
	Questions   []ClarifyQuestion `json:"questions"`
	Answers     map[string]string `json:"answers,omitempty"`
	AnsweredBy  string            `json:"answered_by,omitempty"`
	AnsweredVia string            `json:"answered_via,omitempty"`
	AnsweredAt  *time.Time        `json:"answered_at,omitempty"`
}

type ClarifyView struct {
	Rounds    []ClarifyRound `json:"rounds"`
	CanAnswer bool           `json:"can_answer"`
	CanAuto   bool           `json:"can_auto"`
}

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

func WithOnApproved(fn func(taskID, userID string)) Option {
	return func(s *Service) {
		s.onApproved = fn
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

func WithOnClarified(fn func(taskID string, round int, byLogin, via string)) Option {
	return func(s *Service) {
		s.onClarified = fn
	}
}

type Service struct {
	db          *sql.DB
	gh          GitHub
	disp        dispatcher.API
	projectID   string
	onCreated   func(Created)
	onApproved  func(taskID, userID string)
	merger      Merger
	onMerged    func(string)
	onClarified func(taskID string, round int, byLogin, via string)

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

func (s *Service) checkClarifyPermissions(ctx context.Context, userID, role, taskProjectID, taskCreatedBy string) (canAnswer bool, canAuto bool) {
	if role == "" && userID != "" {
		_ = s.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, userID).Scan(&role)
	}

	var isAdmin bool
	if s.isMulti {
		if s.IsOwner(userID) {
			isAdmin = true
		} else if s.roleIn != nil {
			if r, ok := s.roleIn(userID, taskProjectID); ok && r == "ADMIN" {
				isAdmin = true
			}
		}
	} else {
		if role == "ADMIN" || s.IsOwner(userID) {
			isAdmin = true
		}
	}

	canAuto = isAdmin
	canAnswer = isAdmin || (userID != "" && userID == taskCreatedBy)
	return canAnswer, canAuto
}

func (s *Service) Clarifications(ctx context.Context, userID, role, taskID string) (ClarifyView, error) {
	var projectID, createdBy string
	err := s.db.QueryRowContext(ctx, `SELECT project_id, created_by FROM tasks WHERE id = ?`, taskID).Scan(&projectID, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return ClarifyView{}, ErrNotFound
	}
	if err != nil {
		return ClarifyView{}, err
	}

	if s.isMulti {
		if !s.IsOwner(userID) {
			if s.roleIn != nil {
				if _, ok := s.roleIn(userID, projectID); !ok {
					return ClarifyView{}, ErrForbidden
				}
			} else {
				return ClarifyView{}, ErrForbidden
			}
		}
	} else {
		if role != "ADMIN" && role != "PARTNER" && !s.IsOwner(userID) {
			return ClarifyView{}, ErrForbidden
		}
		if s.projectID != "" && projectID != s.projectID {
			return ClarifyView{}, ErrNotFound
		}
	}

	canAnswer, canAuto := s.checkClarifyPermissions(ctx, userID, role, projectID, createdBy)

	rows, err := s.db.QueryContext(ctx, `
		SELECT tc.round, tc.status, tc.questions, tc.answers, COALESCE(u.username, tc.answered_by, ''), COALESCE(tc.answered_via, ''), tc.answered_at
		FROM task_clarifications tc
		LEFT JOIN users u ON tc.answered_by = u.id
		WHERE tc.task_id = ?
		ORDER BY tc.round ASC
	`, taskID)
	if err != nil {
		return ClarifyView{}, err
	}
	defer rows.Close()

	rounds := make([]ClarifyRound, 0)
	for rows.Next() {
		var round int
		var status, questionsJSON string
		var answersJSON, answeredBy, answeredVia, answeredAtStr sql.NullString

		if err := rows.Scan(&round, &status, &questionsJSON, &answersJSON, &answeredBy, &answeredVia, &answeredAtStr); err != nil {
			return ClarifyView{}, err
		}

		var qs []ClarifyQuestion
		if questionsJSON != "" {
			_ = json.Unmarshal([]byte(questionsJSON), &qs)
		}
		if qs == nil {
			qs = []ClarifyQuestion{}
		}

		var ans map[string]string
		if answersJSON.Valid && answersJSON.String != "" {
			_ = json.Unmarshal([]byte(answersJSON.String), &ans)
		}

		var answeredAt *time.Time
		if answeredAtStr.Valid && answeredAtStr.String != "" {
			for _, layout := range []string{
				time.RFC3339,
				"2006-01-02 15:04:05",
				"2006-01-02T15:04:05Z",
				"2006-01-02T15:04:05",
			} {
				if t, err := time.Parse(layout, answeredAtStr.String); err == nil {
					answeredAt = &t
					break
				}
			}
		}

		by := ""
		if answeredBy.Valid {
			by = answeredBy.String
		}
		via := ""
		if answeredVia.Valid {
			via = answeredVia.String
		}

		rounds = append(rounds, ClarifyRound{
			Round:       round,
			Status:      status,
			Questions:   qs,
			Answers:     ans,
			AnsweredBy:  by,
			AnsweredVia: via,
			AnsweredAt:  answeredAt,
		})
	}

	return ClarifyView{
		Rounds:    rounds,
		CanAnswer: canAnswer,
		CanAuto:   canAuto,
	}, nil
}

func (s *Service) Clarify(ctx context.Context, userID, role, taskID string, answers map[string]string, via string) error {
	var projectID, currentStatus, createdBy string
	err := s.db.QueryRowContext(ctx, `SELECT project_id, status, created_by FROM tasks WHERE id = ?`, taskID).Scan(&projectID, &currentStatus, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	canAnswer, _ := s.checkClarifyPermissions(ctx, userID, role, projectID, createdBy)
	if !canAnswer {
		return ErrForbidden
	}

	if currentStatus != "WAITING_USER" {
		return fmt.Errorf("%w: task status is %s", ErrInvalidTransition, currentStatus)
	}

	var roundID string
	var roundNum int
	var roundStatus, questionsJSON string
	err = s.db.QueryRowContext(ctx, `
		SELECT id, round, status, questions
		FROM task_clarifications
		WHERE task_id = ?
		ORDER BY round DESC
		LIMIT 1
	`, taskID).Scan(&roundID, &roundNum, &roundStatus, &questionsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: no clarification round found", ErrInvalidTransition)
	}
	if err != nil {
		return err
	}

	if roundStatus != "WAITING" {
		return fmt.Errorf("%w: latest round is not WAITING (%s)", ErrInvalidTransition, roundStatus)
	}

	var questions []ClarifyQuestion
	if err := json.Unmarshal([]byte(questionsJSON), &questions); err != nil {
		return fmt.Errorf("unmarshal questions: %w", err)
	}

	cleanAnswers := make(map[string]string)
	for _, q := range questions {
		val, ok := answers[q.ID]
		valTrim := strings.TrimSpace(val)
		if !ok || valTrim == "" {
			return fmt.Errorf("%w: missing answer for question %s", ErrValidation, q.ID)
		}
		if utf8.RuneCountInString(val) > 1000 {
			return fmt.Errorf("%w: answer for %s exceeds 1000 chars", ErrValidation, q.ID)
		}
		cleanAnswers[q.ID] = val
	}

	if notes, ok := answers["_notes"]; ok && strings.TrimSpace(notes) != "" {
		if utf8.RuneCountInString(notes) > 1000 {
			return fmt.Errorf("%w: _notes exceeds 1000 chars", ErrValidation)
		}
		cleanAnswers["_notes"] = notes
	}

	rawAnswers, err := json.Marshal(cleanAnswers)
	if err != nil {
		return err
	}

	if via == "" {
		via = "WEB"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res1, err := tx.ExecContext(ctx, `
		UPDATE task_clarifications
		SET status = 'ANSWERED', answers = ?, answered_by = ?, answered_via = ?, answered_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'WAITING'
	`, string(rawAnswers), userID, via, roundID)
	if err != nil {
		return err
	}
	if rows, _ := res1.RowsAffected(); rows == 0 {
		return fmt.Errorf("%w: round already answered", ErrInvalidTransition)
	}

	res2, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'PLANNING', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'WAITING_USER'
	`, taskID)
	if err != nil {
		return err
	}
	if rows, _ := res2.RowsAffected(); rows == 0 {
		return fmt.Errorf("%w: task not in WAITING_USER", ErrInvalidTransition)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	if s.disp != nil {
		_, _ = s.disp.EnqueueJob(dispatcher.NewJob{
			ProjectID:     projectID,
			TaskID:        taskID,
			Kind:          dispatcher.JobSpec,
			RequiredAgent: dispatcher.KindClaude,
		})
	}

	if s.onClarified != nil {
		var byLogin string
		if userID != "" {
			_ = s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, userID).Scan(&byLogin)
		}
		if byLogin == "" {
			byLogin = userID
		}
		s.onClarified(taskID, roundNum, byLogin, via)
	}

	return nil
}

func (s *Service) ClarifyAuto(ctx context.Context, userID, role, taskID string) error {
	var projectID, currentStatus, createdBy string
	err := s.db.QueryRowContext(ctx, `SELECT project_id, status, created_by FROM tasks WHERE id = ?`, taskID).Scan(&projectID, &currentStatus, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	_, canAuto := s.checkClarifyPermissions(ctx, userID, role, projectID, createdBy)
	if !canAuto {
		return ErrForbidden
	}

	if currentStatus != "WAITING_USER" {
		return fmt.Errorf("%w: task status is %s", ErrInvalidTransition, currentStatus)
	}

	var roundID string
	var roundNum int
	var roundStatus, questionsJSON string
	err = s.db.QueryRowContext(ctx, `
		SELECT id, round, status, questions
		FROM task_clarifications
		WHERE task_id = ?
		ORDER BY round DESC
		LIMIT 1
	`, taskID).Scan(&roundID, &roundNum, &roundStatus, &questionsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: no clarification round found", ErrInvalidTransition)
	}
	if err != nil {
		return err
	}

	if roundStatus != "WAITING" {
		return fmt.Errorf("%w: latest round is not WAITING (%s)", ErrInvalidTransition, roundStatus)
	}

	var questions []ClarifyQuestion
	if err := json.Unmarshal([]byte(questionsJSON), &questions); err != nil {
		return fmt.Errorf("unmarshal questions: %w", err)
	}

	answers := make(map[string]string)
	for _, q := range questions {
		if strings.TrimSpace(q.Recommended) != "" {
			answers[q.ID] = q.Recommended
		} else {
			answers[q.ID] = "Trưởng phòng tự chọn phương án hợp lý nhất, ghi rõ giả định"
		}
	}

	rawAnswers, err := json.Marshal(answers)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res1, err := tx.ExecContext(ctx, `
		UPDATE task_clarifications
		SET status = 'AUTO_DECIDED', answers = ?, answered_by = ?, answered_via = 'AUTO', answered_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'WAITING'
	`, string(rawAnswers), userID, roundID)
	if err != nil {
		return err
	}
	if rows, _ := res1.RowsAffected(); rows == 0 {
		return fmt.Errorf("%w: round already answered", ErrInvalidTransition)
	}

	res2, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'PLANNING', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'WAITING_USER'
	`, taskID)
	if err != nil {
		return err
	}
	if rows, _ := res2.RowsAffected(); rows == 0 {
		return fmt.Errorf("%w: task not in WAITING_USER", ErrInvalidTransition)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	if s.disp != nil {
		_, _ = s.disp.EnqueueJob(dispatcher.NewJob{
			ProjectID:     projectID,
			TaskID:        taskID,
			Kind:          dispatcher.JobSpec,
			RequiredAgent: dispatcher.KindClaude,
		})
	}

	if s.onClarified != nil {
		var byLogin string
		if userID != "" {
			_ = s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, userID).Scan(&byLogin)
		}
		if byLogin == "" {
			byLogin = userID
		}
		s.onClarified(taskID, roundNum, byLogin, "AUTO")
	}

	return nil
}

func (s *Service) AnswerByLogin(ctx context.Context, login, taskID string, answers map[string]string) error {
	var userID, role string
	err := s.db.QueryRowContext(ctx, `SELECT id, role FROM users WHERE LOWER(username) = LOWER(?)`, strings.TrimSpace(login)).Scan(&userID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}

	return s.Clarify(ctx, userID, role, taskID, answers, "GITHUB")
}

func (s *Service) PickOption(ctx context.Context, userID, taskID string, round int, questionID string, optionIdx int) (done bool, err error) {
	var projectID, currentStatus, createdBy string
	err = s.db.QueryRowContext(ctx, `SELECT project_id, status, created_by FROM tasks WHERE id = ?`, taskID).Scan(&projectID, &currentStatus, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}

	var userRole string
	_ = s.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = ?`, userID).Scan(&userRole)

	canAnswer, _ := s.checkClarifyPermissions(ctx, userID, userRole, projectID, createdBy)
	if !canAnswer {
		return false, ErrForbidden
	}

	if currentStatus != "WAITING_USER" {
		return false, fmt.Errorf("%w: task status is %s", ErrInvalidTransition, currentStatus)
	}

	var roundID string
	var roundStatus, questionsJSON string
	var answersJSON sql.NullString
	err = s.db.QueryRowContext(ctx, `
		SELECT id, status, questions, answers
		FROM task_clarifications
		WHERE task_id = ? AND round = ?
	`, taskID, round).Scan(&roundID, &roundStatus, &questionsJSON, &answersJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("%w: round %d not found", ErrInvalidTransition, round)
	}
	if err != nil {
		return false, err
	}

	if roundStatus != "WAITING" {
		return false, fmt.Errorf("%w: round %d status is %s", ErrInvalidTransition, round, roundStatus)
	}

	var questions []ClarifyQuestion
	if err := json.Unmarshal([]byte(questionsJSON), &questions); err != nil {
		return false, fmt.Errorf("unmarshal questions: %w", err)
	}

	var targetQ *ClarifyQuestion
	for i := range questions {
		if questions[i].ID == questionID {
			targetQ = &questions[i]
			break
		}
	}
	if targetQ == nil {
		return false, fmt.Errorf("%w: question %s not found", ErrValidation, questionID)
	}

	if optionIdx < 0 || optionIdx >= len(targetQ.Options) {
		return false, fmt.Errorf("%w: option index %d out of bounds (0..%d)", ErrValidation, optionIdx, len(targetQ.Options)-1)
	}

	pickedVal := targetQ.Options[optionIdx]

	ans := make(map[string]string)
	if answersJSON.Valid && answersJSON.String != "" {
		_ = json.Unmarshal([]byte(answersJSON.String), &ans)
	}
	ans[questionID] = pickedVal

	allAnswered := true
	for _, q := range questions {
		if v, ok := ans[q.ID]; !ok || strings.TrimSpace(v) == "" {
			allAnswered = false
			break
		}
	}

	rawAnswers, err := json.Marshal(ans)
	if err != nil {
		return false, err
	}

	if !allAnswered {
		_, err = s.db.ExecContext(ctx, `
			UPDATE task_clarifications
			SET answers = ?
			WHERE id = ? AND status = 'WAITING'
		`, string(rawAnswers), roundID)
		if err != nil {
			return false, err
		}
		return false, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	res1, err := tx.ExecContext(ctx, `
		UPDATE task_clarifications
		SET status = 'ANSWERED', answers = ?, answered_by = ?, answered_via = 'TELEGRAM', answered_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'WAITING'
	`, string(rawAnswers), userID, roundID)
	if err != nil {
		return false, err
	}
	if rows, _ := res1.RowsAffected(); rows == 0 {
		return false, fmt.Errorf("%w: round already answered", ErrInvalidTransition)
	}

	res2, err := tx.ExecContext(ctx, `
		UPDATE tasks
		SET status = 'PLANNING', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = 'WAITING_USER'
	`, taskID)
	if err != nil {
		return false, err
	}
	if rows, _ := res2.RowsAffected(); rows == 0 {
		return false, fmt.Errorf("%w: task not in WAITING_USER", ErrInvalidTransition)
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	if s.disp != nil {
		_, _ = s.disp.EnqueueJob(dispatcher.NewJob{
			ProjectID:     projectID,
			TaskID:        taskID,
			Kind:          dispatcher.JobSpec,
			RequiredAgent: dispatcher.KindClaude,
		})
	}

	if s.onClarified != nil {
		var byLogin string
		if userID != "" {
			_ = s.db.QueryRowContext(ctx, `SELECT username FROM users WHERE id = ?`, userID).Scan(&byLogin)
		}
		if byLogin == "" {
			byLogin = userID
		}
		s.onClarified(taskID, round, byLogin, "TELEGRAM")
	}

	return true, nil
}

func ParseAnswerComment(body string) (map[string]string, bool) {
	lines := strings.Split(body, "\n")
	res := make(map[string]string)
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "/answer ") || strings.HasPrefix(trimmed, "/answer\t") {
			rest := strings.TrimSpace(trimmed[len("/answer"):])
			parts := strings.SplitN(rest, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				if key != "" {
					res[key] = val
				}
			}
		}
	}
	if len(res) == 0 {
		return nil, false
	}
	return res, true
}
