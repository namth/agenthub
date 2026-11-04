package runner

import (
	"context"
	"encoding/json"
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

func resolveHome(h string) string {
	if h == "" {
		h = "~/.agenthub"
	}
	if h == "~" {
		if dir, err := os.UserHomeDir(); err == nil {
			return dir
		}
		if env := os.Getenv("HOME"); env != "" {
			return env
		}
		return h
	}
	if strings.HasPrefix(h, "~/") {
		dir, err := os.UserHomeDir()
		if err != nil {
			dir = os.Getenv("HOME")
		}
		if dir != "" {
			return filepath.Join(dir, h[2:])
		}
	}
	return h
}

func checkStatus(home, statusFile string) int {
	if statusFile == "" {
		statusFile = filepath.Join(resolveHome(home), "status.json")
	}
	data, err := os.ReadFile(statusFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OFFLINE không đọc được status file: %v\n", err)
		return 1
	}
	var st struct {
		Connected bool   `json:"connected"`
		Server    string `json:"server"`
		Version   string `json:"version"`
		Agents    []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"agents"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		fmt.Fprintf(os.Stderr, "OFFLINE status không đúng định dạng JSON: %v\n", err)
		return 1
	}
	if !st.Connected {
		fmt.Printf("OFFLINE runner mất kết nối (%s)\n", st.Server)
		return 1
	}
	if st.UpdatedAt == "" {
		fmt.Println("OFFLINE thiếu updated_at")
		return 1
	}
	t, err := time.Parse(time.RFC3339, st.UpdatedAt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OFFLINE updated_at sai định dạng RFC3339: %v\n", err)
		return 1
	}
	diff := time.Since(t)
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Minute {
		fmt.Printf("OFFLINE status cũ quá 2 phút (%s)\n", st.UpdatedAt)
		return 1
	}

	agentParts := make([]string, len(st.Agents))
	for i, a := range st.Agents {
		agentParts[i] = fmt.Sprintf("%s:%s", a.ID, a.Status)
	}
	agentsStr := strings.Join(agentParts, ",")
	if len(agentParts) == 0 {
		agentsStr = "none"
	}
	fmt.Printf("ONLINE %s agents=%s\n", st.Server, agentsStr)
	return 0
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
		home           string
		statusOnly     bool
		statusFile     string
		noUpdate       bool
		agentFlags     stringSlice
		exhaustPattern stringSlice
	)

	fs.StringVar(&serverURL, "server", "", "URL của WebSocket gateway (bắt buộc, vd ws://127.0.0.1:8080/ws/agent)")
	fs.StringVar(&repoDir, "repo", "", "Đường dẫn repo git (tuỳ chọn)")
	fs.StringVar(&worktreeRoot, "worktrees", "", "Đường dẫn thư mục chứa worktree")
	fs.StringVar(&osName, "os", runtime.GOOS, "Tên hệ điều hành")
	fs.StringVar(&keyFile, "token-file", "", "Đường dẫn file chứa khoá xác thực")
	fs.StringVar(&baseBranch, "base", "main", "Nhánh cơ sở khi mở pull request")
	fs.BoolVar(&noPR, "no-pr", false, "Không tự động mở pull request cho job CODE")
	fs.StringVar(&home, "home", "~/.agenthub", "Đường dẫn thư mục home của agenthub")
	fs.BoolVar(&statusOnly, "status", false, "Kiểm tra trạng thái runner local")
	fs.StringVar(&statusFile, "status-file", "", "Đường dẫn file status.json (mặc định <home>/status.json)")
	fs.BoolVar(&noUpdate, "no-update", false, "Tắt tính năng tự cập nhật")
	fs.Var(&agentFlags, "agent", "Cấu hình agent dạng kind=command (lặp được, vd --agent claude='claude -p {prompt}')")
	fs.Var(&exhaustPattern, "exhaust-pattern", "Mẫu regex/chuỗi nhận diện hết limit (lặp được)")

	fs.SetOutput(os.Stdout)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}

	if statusOnly {
		return checkStatus(home, statusFile)
	}

	if serverURL == "" {
		fmt.Fprintln(os.Stderr, "agenthub runner: thiếu cờ bắt buộc --server")
		fs.Usage()
		return 1
	}

	homeDir := resolveHome(home)
	if statusFile == "" {
		statusFile = filepath.Join(homeDir, "status.json")
	}

	if worktreeRoot == "" {
		if repoDir != "" {
			worktreeRoot = filepath.Join(repoDir, ".worktrees")
		} else {
			worktreeRoot = filepath.Join(homeDir, "worktrees")
		}
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
		defaultTokenPath := filepath.Join(homeDir, "token")
		if data, err := os.ReadFile(defaultTokenPath); err == nil {
			authSecret = strings.TrimSpace(string(data))
		}
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

	gitRepo := repoDir
	if gitRepo == "" {
		gitRepo = "."
	}
	git := NewGit(gitRepo, worktreeRoot)
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
		GitFor:          NewRepoCache(homeDir, func(r string) string { return "https://github.com/" + r + ".git" }, exec),
		NoUpdate:        noUpdate,
		StatusFile:      statusFile,
	}

	r := New(cfg, exec, git, time.Now)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := r.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		if errors.Is(err, ErrUpdated) {
			return 0
		}
		log.Printf("runner dừng với lỗi: %v", err)
		return 1
	}
	return 0
}
