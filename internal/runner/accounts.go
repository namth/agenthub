package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type AccountEntry struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Command   []string `json:"command"`
	ConfigDir string   `json:"config_dir"`
	EverOK    bool     `json:"ever_ok"`
}

type AccountFile struct {
	Agents  []AccountEntry `json:"agents"`
	Removed []string       `json:"removed"`
}

func LoadAccounts(path string) (AccountFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return AccountFile{
				Agents:  []AccountEntry{},
				Removed: []string{},
			}, nil
		}
		return AccountFile{}, err
	}
	var f AccountFile
	if err := json.Unmarshal(data, &f); err != nil {
		return AccountFile{}, err
	}
	if f.Agents == nil {
		f.Agents = []AccountEntry{}
	}
	if f.Removed == nil {
		f.Removed = []string{}
	}
	return f, nil
}

func SaveAccounts(path string, f AccountFile) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if f.Agents == nil {
		f.Agents = []AccountEntry{}
	}
	if f.Removed == nil {
		f.Removed = []string{}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmpFile := filepath.Join(dir, fmt.Sprintf(".agents-%d.tmp", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(tmpFile, 0o600)
	if err := os.Rename(tmpFile, path); err != nil {
		_ = os.Remove(tmpFile)
		return err
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

func (f AccountFile) NextName(kind string) string {
	base := strings.ToLower(strings.TrimSpace(kind))
	used := make(map[string]bool)
	for _, a := range f.Agents {
		used[strings.ToLower(a.Name)] = true
	}
	for _, r := range f.Removed {
		used[strings.ToLower(r)] = true
	}

	if !used[base] {
		return base
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if !used[cand] {
			return cand
		}
	}
}

func (f AccountFile) AgentConfigs() []AgentConfig {
	res := make([]AgentConfig, len(f.Agents))
	for i, a := range f.Agents {
		everOK := a.EverOK
		res[i] = AgentConfig{
			Kind:      a.Kind,
			Name:      a.Name,
			Command:   a.Command,
			ConfigDir: a.ConfigDir,
			EverOK:    &everOK,
		}
	}
	return res
}

func expandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	} else if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func agentEnv(kind, configDir string) []string {
	if configDir == "" {
		return nil
	}
	dir := expandHome(configDir)
	switch strings.ToUpper(kind) {
	case "CLAUDE":
		return []string{"CLAUDE_CONFIG_DIR=" + dir}
	case "ANTIGRAVITY":
		return []string{"HOME=" + dir}
	}
	return nil
}

func defaultAgentCommand(kind string) []string {
	switch strings.ToUpper(kind) {
	case "CLAUDE":
		return []string{"claude", "-p", "{prompt}"}
	case "ANTIGRAVITY":
		return []string{"agy", "-p", "{prompt}"}
	default:
		return []string{strings.ToLower(kind), "-p", "{prompt}"}
	}
}

func defaultProbeCommand(kind string) []string {
	switch strings.ToUpper(kind) {
	case "CLAUDE":
		return []string{"claude", "-p", "Trả lời đúng một từ: OK"}
	case "ANTIGRAVITY":
		return []string{"agy", "-p", "Trả lời đúng một từ: OK"}
	default:
		return []string{strings.ToLower(kind), "-p", "Trả lời đúng một từ: OK"}
	}
}

func runInteractiveLogin(binary string, env []string) error {
	cmd := exec.Command(binary)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = env
	}
	return cmd.Run()
}

func runProbeWithEnv(probeArgv []string, env []string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, probeArgv[0], probeArgv[1:]...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if len(env) > 0 {
		cmd.Env = env
	}
	err := cmd.Run()
	exit := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exit = exitErr.ExitCode()
		} else {
			exit = -1
		}
	}
	st, _ := ParseProbeResult(exit, buf.String(), err)
	return st, err
}

func restartInstruction() string {
	if runtime.GOOS == "darwin" {
		return "launchctl kickstart -k gui/$(id -u)/io.agenthub.runner"
	}
	return "systemctl --user restart agenthub-runner"
}

func AccountMain(args []string) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printAccountHelp()
		return 0
	}

	home := os.Getenv("AGENTHUB_HOME")
	if home == "" {
		home = resolveHome("~/.agenthub")
	} else {
		home = resolveHome(home)
	}
	accountsPath := filepath.Join(home, "agents.json")

	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "list":
		return accountList(accountsPath)
	case "add":
		return accountAdd(accountsPath, home, subArgs)
	case "login":
		return accountLogin(accountsPath, subArgs)
	case "remove":
		return accountRemove(accountsPath, subArgs)
	default:
		fmt.Fprintf(os.Stderr, "Lệnh không xác định: %s\n\n", subcmd)
		printAccountHelp()
		return 1
	}
}

func printAccountHelp() {
	fmt.Println(`agenthub account - Quản lý tài khoản agent trên máy

Sử dụng:
  agenthub account <lệnh> [tham số]

Các lệnh:
  add <claude|antigravity>   Thêm tài khoản agent mới
  login <name>               Đăng nhập lại tài khoản agent
  list                       Liệt kê các tài khoản agent
  remove <name>              Xoá tài khoản agent`)
}

func accountList(accountsPath string) int {
	f, err := LoadAccounts(accountsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lỗi đọc %s: %v\n", accountsPath, err)
		return 1
	}
	if len(f.Agents) == 0 {
		fmt.Println("Chưa có tài khoản agent nào được cấu hình.")
		return 0
	}
	fmt.Printf("%-16s %-14s %-9s %s\n", "NAME", "KIND", "EVER_OK", "CONFIG_DIR")
	for _, a := range f.Agents {
		fmt.Printf("%-16s %-14s %-9v %s\n", a.Name, a.Kind, a.EverOK, a.ConfigDir)
	}
	return 0
}

func accountAdd(accountsPath, home string, args []string) int {
	fs := flag.NewFlagSet("account add", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "Sử dụng: agenthub account add <claude|antigravity>")
		return 1
	}
	kindInput := strings.ToLower(strings.TrimSpace(fs.Arg(0)))
	var kind string
	var binary string
	switch kindInput {
	case "claude":
		kind = "CLAUDE"
		binary = "claude"
	case "antigravity", "agy":
		kind = "ANTIGRAVITY"
		binary = "agy"
	default:
		fmt.Fprintf(os.Stderr, "Loại agent không hợp lệ: %s (chỉ hỗ trợ claude hoặc antigravity)\n", kindInput)
		return 1
	}

	f, err := LoadAccounts(accountsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lỗi đọc %s: %v\n", accountsPath, err)
		return 1
	}

	name := f.NextName(kind)
	countKind := 0
	for _, a := range f.Agents {
		if strings.EqualFold(a.Kind, kind) {
			countKind++
		}
	}

	var configDir string
	if countKind > 0 {
		configDir = filepath.Join(home, "accounts", name)
		if err := os.MkdirAll(expandHome(configDir), 0o700); err != nil {
			fmt.Fprintf(os.Stderr, "Không thể tạo thư mục %s: %v\n", configDir, err)
			return 1
		}
	}

	extraEnv := agentEnv(kind, configDir)
	var fullEnv []string
	if len(extraEnv) > 0 {
		fullEnv = append(os.Environ(), extraEnv...)
	}

	fmt.Printf("Bắt đầu cấu hình tài khoản %s (%s)...\n", name, kind)
	fmt.Println("Đăng nhập xong thì thoát CLI.")
	_ = runInteractiveLogin(binary, fullEnv)

	probeArgv := defaultProbeCommand(kind)
	st, _ := runProbeWithEnv(probeArgv, fullEnv, 60*time.Second)
	everOK := (st == AuthOK)

	if everOK {
		fmt.Printf("Probe thành công: tài khoản %s đã sẵn sàng.\n", name)
	} else {
		fmt.Printf("Probe chưa thành công (trạng thái: %s). Tài khoản %s đã được lưu nhưng chưa kích hoạt.\n", st, name)
	}

	entry := AccountEntry{
		Name:      name,
		Kind:      kind,
		Command:   defaultAgentCommand(kind),
		ConfigDir: configDir,
		EverOK:    everOK,
	}
	f.Agents = append(f.Agents, entry)
	if err := SaveAccounts(accountsPath, f); err != nil {
		fmt.Fprintf(os.Stderr, "Lỗi lưu %s: %v\n", accountsPath, err)
		return 1
	}

	fmt.Printf("Đã lưu tài khoản vào %s\n", accountsPath)
	fmt.Printf("Khởi động lại runner: %s\n", restartInstruction())
	return 0
}

func accountLogin(accountsPath string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Sử dụng: agenthub account login <name>")
		return 1
	}
	name := strings.TrimSpace(args[0])

	f, err := LoadAccounts(accountsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lỗi đọc %s: %v\n", accountsPath, err)
		return 1
	}

	var foundIdx = -1
	for i, a := range f.Agents {
		if strings.EqualFold(a.Name, name) {
			foundIdx = i
			break
		}
	}
	if foundIdx == -1 {
		fmt.Fprintf(os.Stderr, "Không tìm thấy tài khoản: %s\n", name)
		return 1
	}

	target := &f.Agents[foundIdx]
	binary := "claude"
	if strings.EqualFold(target.Kind, "ANTIGRAVITY") {
		binary = "agy"
	}

	extraEnv := agentEnv(target.Kind, target.ConfigDir)
	var fullEnv []string
	if len(extraEnv) > 0 {
		fullEnv = append(os.Environ(), extraEnv...)
	}

	fmt.Printf("Đăng nhập lại tài khoản %s (%s)...\n", target.Name, target.Kind)
	fmt.Println("Đăng nhập xong thì thoát CLI.")
	_ = runInteractiveLogin(binary, fullEnv)

	probeArgv := defaultProbeCommand(target.Kind)
	st, _ := runProbeWithEnv(probeArgv, fullEnv, 60*time.Second)
	target.EverOK = (st == AuthOK)

	if target.EverOK {
		fmt.Printf("Probe thành công: tài khoản %s đã đăng nhập OK.\n", target.Name)
	} else {
		fmt.Printf("Probe chưa thành công (trạng thái: %s).\n", st)
	}

	if err := SaveAccounts(accountsPath, f); err != nil {
		fmt.Fprintf(os.Stderr, "Lỗi lưu %s: %v\n", accountsPath, err)
		return 1
	}

	fmt.Printf("Khởi động lại runner: %s\n", restartInstruction())
	return 0
}

func accountRemove(accountsPath string, args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Sử dụng: agenthub account remove <name>")
		return 1
	}
	name := strings.TrimSpace(args[0])

	f, err := LoadAccounts(accountsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Lỗi đọc %s: %v\n", accountsPath, err)
		return 1
	}

	var newAgents []AccountEntry
	var removedName string
	for _, a := range f.Agents {
		if strings.EqualFold(a.Name, name) {
			removedName = a.Name
		} else {
			newAgents = append(newAgents, a)
		}
	}
	if removedName == "" {
		fmt.Fprintf(os.Stderr, "Không tìm thấy tài khoản: %s\n", name)
		return 1
	}

	f.Agents = newAgents
	alreadyRemoved := false
	for _, r := range f.Removed {
		if strings.EqualFold(r, removedName) {
			alreadyRemoved = true
			break
		}
	}
	if !alreadyRemoved {
		f.Removed = append(f.Removed, removedName)
	}

	if err := SaveAccounts(accountsPath, f); err != nil {
		fmt.Fprintf(os.Stderr, "Lỗi lưu %s: %v\n", accountsPath, err)
		return 1
	}

	fmt.Printf("Đã xoá tài khoản %s khỏi %s.\n", removedName, accountsPath)
	fmt.Println("Máy chủ vẫn giữ agent tới khi owner xoá trên web.")
	fmt.Printf("Khởi động lại runner: %s\n", restartInstruction())
	return 0
}
