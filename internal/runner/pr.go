package runner

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

// GhPROpener trả về hàm mở PR bằng CLI gh (DEC-12).
func GhPROpener(exec Executor, base string) func(ctx context.Context, dir, branch, title, body string) (string, error) {
	if exec == nil {
		exec = ExecExecutor{}
	}
	if base == "" {
		base = "main"
	}
	return func(ctx context.Context, dir, branch, title, body string) (string, error) {
		var buf bytes.Buffer
		argv := []string{
			"gh", "pr", "create",
			"--head", branch,
			"--base", base,
			"--title", title,
			"--body", body,
		}
		exitCode, err := exec.Run(ctx, dir, argv, &buf)
		if err != nil {
			return "", err
		}
		if exitCode != 0 {
			msg := strings.TrimSpace(buf.String())
			if msg == "" {
				msg = fmt.Sprintf("exit code %d", exitCode)
			}
			return "", fmt.Errorf("gh pr create thất bại (%d): %s", exitCode, msg)
		}
		var prURL string
		for _, line := range strings.Split(buf.String(), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "https://") {
				prURL = trimmed
			}
		}
		if prURL == "" {
			return "", fmt.Errorf("gh pr create: không tìm thấy URL PR trong kết quả: %s", buf.String())
		}
		return prURL, nil
	}
}
