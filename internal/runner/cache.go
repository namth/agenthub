package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

type repoCache struct {
	home     string
	cloneURL func(repo string) string
	exec     Executor
	mu       sync.Mutex
	locks    map[string]*sync.Mutex
}

func (c *repoCache) getLock(repo string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.locks[repo]
	if !ok {
		l = &sync.Mutex{}
		c.locks[repo] = l
	}
	return l
}

func (c *repoCache) gitFor(repo string) (Git, error) {
	if strings.Contains(repo, "..") || !repoPattern.MatchString(repo) {
		return nil, fmt.Errorf("tên repo không hợp lệ: %s", repo)
	}

	l := c.getLock(repo)
	l.Lock()
	defer l.Unlock()

	parts := strings.SplitN(repo, "/", 2)
	owner, name := parts[0], parts[1]

	repoDir := filepath.Join(c.home, "repos", owner, name)
	gitDir := filepath.Join(repoDir, ".git")

	if _, err := os.Stat(gitDir); err != nil {
		if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
			return nil, fmt.Errorf("tạo thư mục cha thất bại: %w", err)
		}
		url := c.cloneURL(repo)
		var buf bytes.Buffer
		code, err := c.exec.Run(context.Background(), "", []string{"git", "clone", url, repoDir}, &buf)
		if err != nil {
			_ = os.RemoveAll(repoDir)
			return nil, fmt.Errorf("git clone: %w (output: %s)", err, buf.String())
		}
		if code != 0 {
			_ = os.RemoveAll(repoDir)
			return nil, fmt.Errorf("git clone thoát mã %d (output: %s)", code, buf.String())
		}
	}

	wtDir := filepath.Join(c.home, "worktrees", owner+"-"+name)
	return NewGit(repoDir, wtDir), nil
}

// NewRepoCache tạo một hàm lấy Git cho từng repo, tự clone khi repo chưa có.
func NewRepoCache(home string, cloneURL func(repo string) string, exec Executor) func(repo string) (Git, error) {
	if exec == nil {
		exec = ExecExecutor{}
	}
	if cloneURL == nil {
		cloneURL = func(r string) string { return "https://github.com/" + r + ".git" }
	}
	c := &repoCache{
		home:     home,
		cloneURL: cloneURL,
		exec:     exec,
		locks:    make(map[string]*sync.Mutex),
	}
	return c.gitFor
}
