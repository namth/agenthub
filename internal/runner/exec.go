package runner

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"syscall"
	"time"
)

type Executor interface {
	Run(ctx context.Context, dir string, argv []string, out io.Writer) (exitCode int, err error)
}

type EnvExecutor interface {
	RunEnv(ctx context.Context, dir string, argv, env []string, out io.Writer) (int, error)
}

type ExecExecutor struct{}

func (e ExecExecutor) Run(ctx context.Context, dir string, argv []string, out io.Writer) (int, error) {
	return e.RunEnv(ctx, dir, argv, nil, out)
}

func (ExecExecutor) RunEnv(ctx context.Context, dir string, argv, env []string, out io.Writer) (int, error) {
	if len(argv) == 0 {
		return -1, errors.New("empty argv")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out
	if len(env) > 0 {
		cmd.Env = env
	}
	// CLI (claude/agy) tự sinh tiến trình con: chạy trong nhóm riêng và giết cả nhóm khi huỷ.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second

	err := cmd.Run()
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}
