package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Git interface {
	Prepare(ctx context.Context, branch string) (dir string, err error)
	CommitWIP(ctx context.Context, dir, msg string) (sha string, err error)
	Push(ctx context.Context, dir, branch string) error
	DiffStat(ctx context.Context, dir string) string
	Cleanup(ctx context.Context, dir string) error
}

type realGit struct {
	repoDir      string
	worktreeRoot string
}

func NewGit(repoDir, worktreeRoot string) Git {
	if repoDir == "" {
		repoDir = "."
	}
	if worktreeRoot == "" {
		worktreeRoot = filepath.Join(repoDir, ".worktrees")
	}
	return &realGit{
		repoDir:      repoDir,
		worktreeRoot: worktreeRoot,
	}
}

func (g *realGit) gitCmd(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("git %v: %w (stderr: %s)", args, err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (g *realGit) Prepare(ctx context.Context, branch string) (string, error) {
	if err := os.MkdirAll(g.worktreeRoot, 0o755); err != nil {
		return "", err
	}

	sanitized := strings.ReplaceAll(branch, "/", "-")
	wtDir := filepath.Join(g.worktreeRoot, sanitized)

	// Xoá worktree cũ nếu còn tồn tại
	if _, err := os.Stat(wtDir); err == nil {
		_ = g.Cleanup(ctx, wtDir)
	}

	// Fetch từ origin
	_, _ = g.gitCmd(ctx, g.repoDir, "fetch", "origin")

	// 1. Nhánh đã có trên origin -> checkout nó
	_, errRemote := g.gitCmd(ctx, g.repoDir, "rev-parse", "--verify", "refs/remotes/origin/"+branch)
	if errRemote == nil {
		_, err := g.gitCmd(ctx, g.repoDir, "worktree", "add", "-B", branch, wtDir, "origin/"+branch)
		if err != nil {
			return "", err
		}
		return wtDir, nil
	}

	// 2. Nhánh đã có ở local
	_, errLocal := g.gitCmd(ctx, g.repoDir, "rev-parse", "--verify", "refs/heads/"+branch)
	if errLocal == nil {
		_, err := g.gitCmd(ctx, g.repoDir, "worktree", "add", wtDir, branch)
		if err != nil {
			return "", err
		}
		return wtDir, nil
	}

	// 3. Nhánh chưa có -> tạo mới từ HEAD
	_, err := g.gitCmd(ctx, g.repoDir, "worktree", "add", "-b", branch, wtDir, "HEAD")
	if err != nil {
		return "", err
	}
	return wtDir, nil
}

func (g *realGit) CommitWIP(ctx context.Context, dir, msg string) (string, error) {
	if _, err := g.gitCmd(ctx, dir, "add", "-A"); err != nil {
		return "", err
	}

	st, err := g.gitCmd(ctx, dir, "status", "--porcelain")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(st) == "" {
		return "", nil
	}

	_, err = g.gitCmd(ctx, dir, "-c", "user.name=AgentHub Runner", "-c", "user.email=runner@agenthub.local", "commit", "-q", "-m", msg)
	if err != nil {
		return "", err
	}

	sha, err := g.gitCmd(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sha), nil
}

func (g *realGit) Push(ctx context.Context, dir, branch string) error {
	_, err := g.gitCmd(ctx, dir, "push", "-u", "origin", branch)
	return err
}

func (g *realGit) DiffStat(ctx context.Context, dir string) string {
	_, _ = g.gitCmd(ctx, dir, "add", "-N", ".")
	out, err := g.gitCmd(ctx, dir, "diff", "--stat", "HEAD")
	if err != nil {
		out, _ = g.gitCmd(ctx, dir, "diff", "--stat")
	}
	return out
}

func (g *realGit) Cleanup(ctx context.Context, dir string) error {
	_, _ = g.gitCmd(ctx, g.repoDir, "worktree", "remove", "--force", dir)
	if _, err := os.Stat(dir); err == nil {
		_ = os.RemoveAll(dir)
	}
	_, _ = g.gitCmd(ctx, g.repoDir, "worktree", "prune")
	return nil
}
