package main

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/namth/agenthub/internal/tasks"
)

// repoClients tạo và nhớ GitHub client / Merger theo dự án (BR-24: client theo projects.github_repo).
type repoClients struct {
	db     *sql.DB
	apiURL string
	token  string

	mu      sync.Mutex
	gh      map[string]tasks.GitHub
	mergers map[string]tasks.Merger
}

func newRepoClients(db *sql.DB, apiURL, token string) *repoClients {
	return &repoClients{db: db, apiURL: apiURL, token: token, gh: map[string]tasks.GitHub{}, mergers: map[string]tasks.Merger{}}
}

func (c *repoClients) repoOf(projectID string) (owner, name string, err error) {
	var full string
	if err = c.db.QueryRow(`SELECT github_repo FROM projects WHERE id = ?`, projectID).Scan(&full); err != nil {
		return "", "", fmt.Errorf("dự án %s: %w", projectID, err)
	}
	i := strings.Index(full, "/")
	if i <= 0 || i == len(full)-1 {
		return "", "", fmt.Errorf("dự án %s: github_repo %q sai dạng owner/name", projectID, full)
	}
	return full[:i], full[i+1:], nil
}

func (c *repoClients) GitHub(projectID string) (tasks.GitHub, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if g, ok := c.gh[projectID]; ok {
		return g, nil
	}
	owner, name, err := c.repoOf(projectID)
	if err != nil {
		return nil, err
	}
	g := tasks.NewGitHubClient(c.apiURL, owner, name, c.token, nil)
	c.gh[projectID] = g
	return g, nil
}

// GitHubOrNil cho pipeline.WithGitHubFor: lỗi → nil để pipeline dùng client mặc định.
func (c *repoClients) GitHubOrNil(projectID string) tasks.GitHub {
	g, err := c.GitHub(projectID)
	if err != nil {
		return nil
	}
	return g
}

func (c *repoClients) Merger(projectID string) tasks.Merger {
	c.mu.Lock()
	defer c.mu.Unlock()
	if m, ok := c.mergers[projectID]; ok {
		return m
	}
	owner, name, err := c.repoOf(projectID)
	if err != nil {
		return nil
	}
	m := tasks.NewGitHubMerger(c.apiURL, owner, name, c.token, nil)
	c.mergers[projectID] = m
	return m
}

// projectOfRepo định tuyến webhook theo repository.full_name (chỉ dự án đang quản lý).
func projectOfRepo(db *sql.DB) func(fullName string) (string, bool) {
	return func(fullName string) (string, bool) {
		var id string
		if err := db.QueryRow(`SELECT id FROM projects WHERE lower(github_repo) = lower(?)`, fullName).Scan(&id); err != nil {
			return "", false
		}
		return id, true
	}
}

// ownerCheck: quản trị hệ thống = user có username trùng GITHUB_OWNER (DEC-17).
func ownerCheck(db *sql.DB, owner string) func(userID string) bool {
	return func(userID string) bool {
		if owner == "" || userID == "" {
			return false
		}
		var login string
		if err := db.QueryRow(`SELECT username FROM users WHERE id = ?`, userID).Scan(&login); err != nil {
			return false
		}
		return strings.EqualFold(login, owner)
	}
}

// memberRole đọc vai trong project_members.
func memberRole(db *sql.DB) func(userID, projectID string) (string, bool) {
	return func(userID, projectID string) (string, bool) {
		var role string
		if err := db.QueryRow(`SELECT role FROM project_members WHERE user_id = ? AND project_id = ?`, userID, projectID).Scan(&role); err != nil {
			return "", false
		}
		return role, true
	}
}

// baseURLOf: BASE_URL nếu có, không thì http://localhost:<PORT> (chạy thử tại chỗ).
func baseURLOf(port string) func() string {
	return func() string {
		if v := strings.TrimRight(envOr("BASE_URL", ""), "/"); v != "" {
			return v
		}
		return "http://localhost:" + port
	}
}
