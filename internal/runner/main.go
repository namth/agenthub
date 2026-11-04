package runner

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(val string) error {
	*s = append(*s, val)
	return nil
}

func Main(args []string) int {
	fs := flag.NewFlagSet("agenthub runner", flag.ContinueOnError)
	var (
		serverURL      string
		repoDir        string
		worktreeRoot   string
		osName         string
		keyFile        string
		baseBranch     string
		noPR           bool
		agentFlags     stringSlice
		exhaustPattern stringSlice
	)

	fs.StringVar(&serverURL, "server", "", "URL của WebSocket gateway (bắt buộc, vd ws://127.0.0.1:8080/ws/agent)")
	fs.StringVar(&repoDir, "repo", ".", "Đường dẫn repo git")
	fs.StringVar(&worktreeRoot, "worktrees", "", "Đường dẫn thư mục chứa worktree (mặc định <repo>/.worktrees)")
	fs.StringVar(&osName, "os", runtime.GOOS, "Tên hệ điều hành")
	fs.StringVar(&keyFile, "token-file", "", "Đường dẫn file chứa khoá xác thực")
	fs.StringVar(&baseBranch, "base", "main", "Nhánh cơ sở khi mở pull request")
	fs.BoolVar(&noPR, "no-pr", false, "Không tự động mở pull request cho job CODE")
	fs.Var(&agentFlags, "agent", "Cấu hình agent dạng kind=command (lặp được, vd --agent claude='claude -p {prompt}')")
	fs.Var(&exhaustPattern, "exhaust-pattern", "Mẫu regex/chuỗi nhận diện hết limit (lặp được)")

	fs.SetOutput(os.Stdout)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if serverURL == "" {
		fmt.Fprintln(os.Stderr, "agenthub runner: thiếu cờ bắt buộc --server")
		fs.Usage()
		return 1
	}

	if worktreeRoot == "" {
		worktreeRoot = filepath.Join(repoDir, ".worktrees")
	}

	authSecret := strings.TrimSpace(os.Getenv("AGENTHUB_RUNNER_TOKEN"))
	if authSecret == "" && keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err != nil {
			log.Printf("runner: không đọc được file khoá: %v", err)
			return 1
		}
		authSecret = strings.TrimSpace(string(data))
	}
	if authSecret == "" {
		log.Println("runner: thiếu khoá xác thực")
		return 1
	}

	var agents []AgentConfig
	for _, a := range agentFlags {
		parts := strings.SplitN(a, "=", 2)
		if len(parts) != 2 {
			log.Printf("runner: cờ --agent sai định dạng: %s", a)
			return 1
		}
		kind := strings.ToUpper(strings.TrimSpace(parts[0]))
		cmdStr := strings.TrimSpace(parts[1])
		cmdFields := strings.Fields(cmdStr)
		if len(cmdFields) == 0 {
			log.Printf("runner: lệnh agent rỗng: %s", a)
			return 1
		}
		agents = append(agents, AgentConfig{
			Kind:    kind,
			Command: cmdFields,
		})
	}
	if len(agents) == 0 {
		agents = append(agents, AgentConfig{
			Kind:    "ANTIGRAVITY",
			Command: []string{"agy", "-p", "{prompt}"},
		})
	}

	git := NewGit(repoDir, worktreeRoot)
	exec := ExecExecutor{}

	var openPR func(ctx context.Context, dir, branch, title, body string) (string, error)
	if !noPR {
		openPR = GhPROpener(exec, baseBranch)
	}

	cfg := Config{
		ServerURL:       serverURL,
		Token:           authSecret,
		OS:              osName,
		Agents:          agents,
		ExhaustPatterns: exhaustPattern,
		OpenPR:          openPR,
	}

	r := New(cfg, exec, git, time.Now)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := r.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("runner dừng với lỗi: %v", err)
		return 1
	}
	return 0
}
