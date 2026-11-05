package machines_test

// Test nghiệm thu TASK-050 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-accounts.md BR-67: install.sh chỉ đăng ký CLI đã đăng nhập OK, ghi ~/.agenthub/agents.json.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptRegistersOnlyLoggedIn(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("không có sh")
	}
	r := newRig(t)
	res, _ := http.Get(r.srv.URL + "/install.sh")
	script := new(bytes.Buffer)
	script.ReadFrom(res.Body)
	res.Body.Close()

	// CLI giả: claude đăng nhập OK, agy chưa đăng nhập.
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\necho OK\nexit 0\n"), 0o755)
	os.WriteFile(filepath.Join(bin, "agy"), []byte("#!/bin/sh\necho 'Not logged in · Please run /login'\nexit 1\n"), 0o755)

	home := filepath.Join(t.TempDir(), "home")
	c := r.newCode("u-a", "may-mot-loai")
	cmd := exec.Command("sh", "-s", "--", "--join", c.Code, "--name", "may-mot-loai", "--no-service")
	cmd.Stdin = strings.NewReader(script.String())
	cmd.Env = append(os.Environ(), "AGENTHUB_HOME="+home, "AGENTHUB_UNAME_S=Darwin", "AGENTHUB_UNAME_M=arm64",
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh lỗi: %v\n%s", err, out)
	}

	b, err := os.ReadFile(filepath.Join(home, "agents.json"))
	if err != nil {
		t.Fatalf("install.sh phải ghi $AGENTHUB_HOME/agents.json: %v\n%s", err, out)
	}
	st, _ := os.Stat(filepath.Join(home, "agents.json"))
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("agents.json phải 0600, có %v", st.Mode().Perm())
	}
	var f struct {
		Agents []struct {
			Name    string   `json:"name"`
			Kind    string   `json:"kind"`
			Command []string `json:"command"`
			EverOK  bool     `json:"ever_ok"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("agents.json không phải JSON: %v\n%s", err, b)
	}
	if len(f.Agents) != 1 || f.Agents[0].Name != "claude" || f.Agents[0].Kind != "CLAUDE" || !f.Agents[0].EverOK || len(f.Agents[0].Command) == 0 {
		t.Fatalf("chỉ claude (đã đăng nhập) được ghi, ever_ok=true: %s", b)
	}

	// Server chỉ có agent claude cho máy này.
	rows, _ := r.db.Query(`SELECT id FROM agents WHERE worker_id = 'may-mot-loai'`)
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	if len(ids) != 1 || ids[0] != "may-mot-loai/claude" {
		t.Fatalf("join chỉ đăng ký CLI đã đăng nhập: %v", ids)
	}
	if !strings.Contains(string(out), "agenthub account add") {
		t.Fatalf("install.sh phải gợi ý `agenthub account add antigravity` cho CLI chưa đăng nhập:\n%s", out)
	}
}
