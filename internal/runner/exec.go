package runner

import (
	"context"
	"errors"
	"io"
	"os/exec"
)

type Executor interface {
	Run(ctx context.Context, dir string, argv []string, out io.Writer) (exitCode int, err error)
}

type ExecExecutor struct{}

func (ExecExecutor) Run(ctx context.Context, dir string, argv []string, out io.Writer) (int, error) {
	if len(argv) == 0 {
		return -1, errors.New("empty argv")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out

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
