package projects

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrNotCollaborator = errors.New("NOT_COLLABORATOR")
	ErrValidation      = errors.New("VALIDATION_ERROR")
	ErrConflict        = errors.New("CONFLICT")
	ErrNotFound        = errors.New("NOT_FOUND")
)

type Config struct {
	Owner         string
	BaseURL       string
	WebhookSecret string
}

type Project struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	GitHubRepo    string `json:"github_repo"`
	DefaultBranch string `json:"default_branch"`
	Status        string `json:"status"`
	Weight        int    `json:"weight"`
	MaxParallel   *int   `json:"max_parallel"`
	WebhookOK     bool   `json:"webhook_ok"`
}

type Service struct {
	db  *sql.DB
	gh  GitHub
	cfg Config
}

func New(db *sql.DB, gh GitHub, cfg Config) *Service {
	return &Service{
		db:  db,
		gh:  gh,
		cfg: cfg,
	}
}

func (s *Service) IsOwner(login string) bool {
	if login == "" || s.cfg.Owner == "" {
		return false
	}
	return strings.EqualFold(login, s.cfg.Owner)
}

func (s *Service) AddProject(ctx context.Context, fullName string) (Project, error) {
	parts := strings.Split(fullName, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Project{}, ErrValidation
	}

	var exists int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE github_repo = ?", fullName).Scan(&exists)
	if err == nil {
		return Project{}, ErrConflict
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Project{}, err
	}

	repos, err := s.gh.ListRepos(ctx)
	if err != nil {
		return Project{}, err
	}

	var found *Repo
	for i := range repos {
		if repos[i].FullName == fullName {
			found = &repos[i]
			break
		}
	}
	if found == nil {
		return Project{}, ErrNotFound
	}

	id := uuid.NewString()
	name := parts[1]
	defaultBranch := found.DefaultBranch
	if defaultBranch == "" {
		defaultBranch = "main"
	}

	webhookURL := strings.TrimRight(s.cfg.BaseURL, "/") + "/api/webhooks/github"
	hookID, hookErr := s.gh.CreateWebhook(ctx, fullName, webhookURL, s.cfg.WebhookSecret)
	webhookOK := hookErr == nil

	if webhookOK {
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO projects (id, name, github_repo, default_branch, status, weight, webhook_id)
			VALUES (?, ?, ?, ?, 'ACTIVE', 1, ?)
		`, id, name, fullName, defaultBranch, hookID)
	} else {
		_, err = s.db.ExecContext(ctx, `
			INSERT INTO projects (id, name, github_repo, default_branch, status, weight, webhook_id)
			VALUES (?, ?, ?, ?, 'ACTIVE', 1, NULL)
		`, id, name, fullName, defaultBranch)
	}
	if err != nil {
		return Project{}, err
	}

	rows, err := s.db.QueryContext(ctx, "SELECT id, username FROM users")
	if err == nil {
		type uinfo struct {
			id       string
			username string
		}
		var ulist []uinfo
		for rows.Next() {
			var u uinfo
			if err := rows.Scan(&u.id, &u.username); err == nil {
				ulist = append(ulist, u)
			}
		}
		rows.Close()
		for _, u := range ulist {
			_, _ = s.SyncUser(ctx, u.id, u.username)
		}
	}

	return Project{
		ID:            id,
		Name:          name,
		GitHubRepo:    fullName,
		DefaultBranch: defaultBranch,
		Status:        "ACTIVE",
		Weight:        1,
		MaxParallel:   nil,
		WebhookOK:     webhookOK,
	}, nil
}

func (s *Service) SyncUser(ctx context.Context, userID, login string) (int, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, github_repo FROM projects WHERE status = 'ACTIVE'")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type proj struct {
		id   string
		repo string
	}
	var projs []proj
	for rows.Next() {
		var p proj
		if err := rows.Scan(&p.id, &p.repo); err != nil {
			return 0, err
		}
		projs = append(projs, p)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	memberCount := 0
	isOwner := s.IsOwner(login)

	for _, p := range projs {
		var role string
		if isOwner {
			role = "ADMIN"
		} else {
			perm, err := s.gh.Permission(ctx, p.repo, login)
			if errors.Is(err, ErrNotCollaborator) {
				_, _ = s.db.ExecContext(ctx, "DELETE FROM project_members WHERE project_id = ? AND user_id = ?", p.id, userID)
				continue
			}
			if err != nil {
				return 0, err
			}
			switch perm {
			case "admin", "maintain":
				role = "ADMIN"
			default:
				role = "PARTNER"
			}
		}

		_, err = s.db.ExecContext(ctx, `
			INSERT INTO project_members (project_id, user_id, role, synced_at)
			VALUES (?, ?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(project_id, user_id) DO UPDATE SET
				role = excluded.role,
				synced_at = excluded.synced_at
		`, p.id, userID, role)
		if err != nil {
			return 0, err
		}
		memberCount++
	}

	return memberCount, nil
}

func (s *Service) RoleIn(ctx context.Context, userID, projectID string) (string, bool) {
	var dummy int
	if err := s.db.QueryRowContext(ctx, "SELECT 1 FROM projects WHERE id = ?", projectID).Scan(&dummy); err != nil {
		return "", false
	}

	var username string
	if err := s.db.QueryRowContext(ctx, "SELECT username FROM users WHERE id = ?", userID).Scan(&username); err == nil && s.IsOwner(username) {
		return "ADMIN", true
	}

	var role string
	if err := s.db.QueryRowContext(ctx, "SELECT role FROM project_members WHERE project_id = ? AND user_id = ?", projectID, userID).Scan(&role); err == nil {
		return role, true
	}

	return "", false
}
