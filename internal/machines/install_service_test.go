package machines_test

// Test do trưởng phòng viết (sửa lỗi phát hiện khi triển khai 2026-11-04):
// dịch vụ runner do install.sh tạo phải có PATH của lúc cài và cờ --agent dạng kind=lệnh,
// nếu không runner không tìm thấy claude/gh/agy và ghép nhầm agent Claude vào lệnh agy.

import (
	"bytes"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptWritesService(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("không có sh")
	}
	r := newRig(t)
	res, _ := http.Get(r.srv.URL + "/install.sh")
	script := new(bytes.Buffer)
	script.ReadFrom(res.Body)
	res.Body.Close()

	// Công cụ giả: claude, agy có mặt; launchctl/systemctl không làm gì (không đụng dịch vụ thật).
	fake := t.TempDir()
	for _, name := range []string{"claude", "agy", "launchctl", "systemctl"} {
		if err := os.WriteFile(filepath.Join(fake, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	install := func(osName, user string) string {
		t.Helper()
		home := t.TempDir()
		c := r.newCode(user, "svc-"+strings.ToLower(osName))
		cmd := exec.Command("sh", "-s", "--", "--join", c.Code, "--name", "svc-"+strings.ToLower(osName))
		cmd.Stdin = strings.NewReader(script.String())
		cmd.Env = []string{
			"HOME=" + home,
			"PATH=" + fake + ":/usr/bin:/bin:/usr/sbin:/sbin",
			"AGENTHUB_HOME=" + filepath.Join(home, ".agenthub"),
			"AGENTHUB_UNAME_S=" + osName,
			"AGENTHUB_UNAME_M=arm64",
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("install.sh (%s) lỗi: %v\n%s", osName, err, out)
		}
		return home
	}

	home := install("Darwin", "u-a")
	plist, err := os.ReadFile(filepath.Join(home, "Library", "LaunchAgents", "io.agenthub.runner.plist"))
	if err != nil {
		t.Fatalf("không tạo plist: %v", err)
	}
	p := string(plist)
	for _, want := range []string{
		"<string>--agent</string>",
		"<string>claude=claude -p {prompt}</string>",
		"<string>antigravity=agy -p {prompt}</string>",
		"<key>EnvironmentVariables</key>",
		"<key>PATH</key>",
		fake,
	} {
		if !strings.Contains(p, want) {
			t.Fatalf("plist thiếu %q:\n%s", want, p)
		}
	}
	if plutil, err := exec.LookPath("plutil"); err == nil {
		f := filepath.Join(t.TempDir(), "x.plist")
		os.WriteFile(f, plist, 0o644)
		if out, err := exec.Command(plutil, "-lint", f).CombinedOutput(); err != nil {
			t.Fatalf("plist không hợp lệ: %s", out)
		}
	}

	// Linux: systemd user service.
	os.WriteFile(filepath.Join(r.runners, "agenthub-linux-arm64"), []byte("BINARY-DARWIN"), 0o755)
	sum, _ := os.ReadFile(filepath.Join(r.runners, "agenthub-darwin-arm64.sha256"))
	os.WriteFile(filepath.Join(r.runners, "agenthub-linux-arm64.sha256"), []byte(strings.Replace(string(sum), "darwin", "linux", 1)), 0o644)
	home = install("Linux", "u-a")
	svc, err := os.ReadFile(filepath.Join(home, ".config", "systemd", "user", "agenthub-runner.service"))
	if err != nil {
		t.Fatalf("không tạo service: %v", err)
	}
	s := string(svc)
	for _, want := range []string{
		`--agent "claude=claude -p {prompt}"`,
		`--agent "antigravity=agy -p {prompt}"`,
		`Environment="PATH=` + fake,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("service thiếu %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "--agent claude ") || strings.Contains(s, "--agent agy") {
		t.Fatalf("còn cờ --agent thiếu lệnh:\n%s", s)
	}
}
