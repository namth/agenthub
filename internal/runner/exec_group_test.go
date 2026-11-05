package runner_test

// Test do trưởng phòng viết: huỷ CLI phải giết cả tiến trình cháu.

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/runner"
)

func TestCancelKillsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	var out strings.Builder
	runner.ExecExecutor{}.Run(ctx, t.TempDir(), []string{"sh", "-c", "sleep 30 & echo $! > " + pidFile + "; wait"}, &out)
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	time.Sleep(200 * time.Millisecond)
	if err := syscall.Kill(pid, 0); err == nil {
		syscall.Kill(pid, syscall.SIGKILL)
		t.Fatalf("tiến trình cháu %d vẫn sống sau khi huỷ", pid)
	}
}
