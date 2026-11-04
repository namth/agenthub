package projects

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Repo struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

type GitHub interface {
	ListRepos(ctx context.Context) ([]Repo, error)
	CreateWebhook(ctx context.Context, fullName, url, secret string) (int64, error)
	Permission(ctx context.Context, fullName, login string) (string, error)
}

type ghClient struct {
	apiURL string
	token  string
	hc     *http.Client
}

func NewGitHub(apiURL, token string, hc *http.Client) GitHub {
	if apiURL == "" {
		apiURL = "https://api.github.com"
	}
	apiURL = strings.TrimRight(apiURL, "/")
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &ghClient{
		apiURL: apiURL,
		token:  token,
		hc:     hc,
	}
}

func (g *ghClient) ListRepos(ctx context.Context) ([]Repo, error) {
	var allRepos []Repo
	// Dừng khi trang thiếu (< per_page) hoặc rỗng; tối đa 50 trang để không lặp vô hạn.
	for page := 1; page <= 50; page++ {
		reqURL := fmt.Sprintf("%s/user/repos?affiliation=owner,collaborator&per_page=100&page=%d", g.apiURL, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+g.token)
		req.Header.Set("Accept", "application/json")

		resp, err := g.hc.Do(req)
		if err != nil {
			return nil, err
		}
		var repos []Repo
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("github api error: %d", resp.StatusCode)
		}
		err = json.NewDecoder(resp.Body).Decode(&repos)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		allRepos = append(allRepos, repos...)
		if len(repos) < 100 {
			break
		}
	}
	return allRepos, nil
}

type webhookConfig struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type"`
	Secret      string `json:"secret"`
}

type webhookPayload struct {
	Name   string        `json:"name"`
	Active bool          `json:"active"`
	Events []string      `json:"events"`
	Config webhookConfig `json:"config"`
}

func (g *ghClient) CreateWebhook(ctx context.Context, fullName, targetURL, secret string) (int64, error) {
	reqURL := fmt.Sprintf("%s/repos/%s/hooks", g.apiURL, fullName)
	payload := webhookPayload{
		Name:   "web",
		Active: true,
		Events: []string{"issues", "pull_request", "workflow_run"},
		Config: webhookConfig{
			URL:         targetURL,
			ContentType: "json",
			Secret:      secret,
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := g.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("github api error: %d", resp.StatusCode)
	}

	var res struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return 0, err
	}
	return res.ID, nil
}

func (g *ghClient) Permission(ctx context.Context, fullName, login string) (string, error) {
	reqURL := fmt.Sprintf("%s/repos/%s/collaborators/%s/permission", g.apiURL, fullName, url.PathEscape(login))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/json")

	resp, err := g.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotCollaborator
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api error: %d", resp.StatusCode)
	}

	var res struct {
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.Permission, nil
}
