package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GitHub là interface tương tác với GitHub REST API v3.
type GitHub interface {
	CreateIssue(ctx context.Context, title, body string, labels []string) (number int, url string, err error)
	SetLabels(ctx context.Context, number int, labels []string) error
	CloseIssue(ctx context.Context, number int, comment string) error
}

type ghClient struct {
	apiURL string
	owner  string
	repo   string
	token  string
	hc     *http.Client
}

// NewGitHubClient khởi tạo client REST v3; apiURL rỗng → https://api.github.com.
func NewGitHubClient(apiURL, owner, repo, token string, hc *http.Client) GitHub {
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	apiURL = strings.TrimRight(apiURL, "/")
	if hc == nil {
		hc = http.DefaultClient
	}
	return &ghClient{
		apiURL: apiURL,
		owner:  owner,
		repo:   repo,
		token:  token,
		hc:     hc,
	}
}

func (c *ghClient) CreateIssue(ctx context.Context, title, body string, labels []string) (int, string, error) {
	reqBody := map[string]any{
		"title": title,
		"body":  body,
	}
	if len(labels) > 0 {
		reqBody["labels"] = labels
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return 0, "", err
	}
	u := fmt.Sprintf("%s/repos/%s/%s/issues", c.apiURL, c.owner, c.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, "", fmt.Errorf("github api error: status %d", resp.StatusCode)
	}

	var res struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
		URL     string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, "", err
	}
	issueURL := res.HTMLURL
	if issueURL == "" {
		issueURL = res.URL
	}
	return res.Number, issueURL, nil
}

func (c *ghClient) SetLabels(ctx context.Context, number int, labels []string) error {
	reqBody := map[string]any{
		"labels": labels,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/repos/%s/%s/issues/%d/labels", c.apiURL, c.owner, c.repo, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github set labels error: status %d", resp.StatusCode)
	}
	return nil
}

func (c *ghClient) CloseIssue(ctx context.Context, number int, comment string) error {
	if comment != "" {
		commentBody := map[string]any{
			"body": comment,
		}
		b, err := json.Marshal(commentBody)
		if err != nil {
			return err
		}
		u := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.apiURL, c.owner, c.repo, number)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Content-Type", "application/json")
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("github comment error: status %d", resp.StatusCode)
		}
	}

	patchBody := map[string]any{
		"state": "closed",
	}
	b, err := json.Marshal(patchBody)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/repos/%s/%s/issues/%d", c.apiURL, c.owner, c.repo, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github close issue error: status %d", resp.StatusCode)
	}
	return nil
}

// Merger là interface gộp Pull Request trên GitHub.
type Merger interface {
	MergePR(ctx context.Context, number int) error
}

type ghMerger struct {
	apiURL string
	owner  string
	repo   string
	token  string
	hc     *http.Client
}

// NewGitHubMerger khởi tạo client gộp PR; apiURL rỗng → https://api.github.com.
func NewGitHubMerger(apiURL, owner, repo, token string, hc *http.Client) Merger {
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	apiURL = strings.TrimRight(apiURL, "/")
	if hc == nil {
		hc = http.DefaultClient
	}
	return &ghMerger{
		apiURL: apiURL,
		owner:  owner,
		repo:   repo,
		token:  token,
		hc:     hc,
	}
}

func (m *ghMerger) MergePR(ctx context.Context, number int) error {
	reqBody := map[string]any{
		"merge_method": "squash",
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	u := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/merge", m.apiURL, m.owner, m.repo, number)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	if m.token != "" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}

	resp, err := m.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("github merge error: status %d", resp.StatusCode)
	}
	return nil
}
