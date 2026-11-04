package machines_test

import (
	"regexp"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/machines"
)

func TestExtraJoinCodeGenerator(t *testing.T) {
	re := regexp.MustCompile(`^[A-HJ-NP-Z2-9]{4}-[A-HJ-NP-Z2-9]{4}$`)
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		code, err := machines.GenerateJoinCodeForTest()
		if err != nil {
			t.Fatalf("GenerateJoinCodeForTest lỗi: %v", err)
		}
		if !re.MatchString(code) {
			t.Fatalf("mã %q không khớp định dạng", code)
		}
		if seen[code] {
			t.Fatalf("trùng mã: %s", code)
		}
		seen[code] = true
	}
}

func TestExtraDownloadRegex(t *testing.T) {
	valid := []string{
		"agenthub-darwin-arm64",
		"agenthub-darwin-arm64.sha256",
		"agenthub-darwin-amd64",
		"agenthub-darwin-amd64.sha256",
		"agenthub-linux-arm64",
		"agenthub-linux-arm64.sha256",
		"agenthub-linux-amd64",
		"agenthub-linux-amd64.sha256",
	}
	for _, v := range valid {
		if !machines.DownloadRegexForTest.MatchString(v) {
			t.Errorf("muốn hợp lệ: %s", v)
		}
	}

	invalid := []string{
		"agenthub-windows-amd64",
		"agenthub-plan9-mips",
		"agenthub-darwin-arm64.tar.gz",
		"../agenthub-darwin-arm64",
		"agenthub-darwin-arm64/test",
	}
	for _, inv := range invalid {
		if machines.DownloadRegexForTest.MatchString(inv) {
			t.Errorf("muốn không hợp lệ: %s", inv)
		}
	}
}

func TestExtraRateLimiterSlidingWindow(t *testing.T) {
	rl := machines.NewRateLimiterForTest()
	now := time.Now()
	ip := "192.168.1.100"

	for i := 0; i < 4; i++ {
		if rl.IsBlocked(ip, now) {
			t.Fatalf("lần %d không được block", i+1)
		}
		rl.RecordFailure(ip, now)
	}

	if rl.IsBlocked(ip, now) {
		t.Fatal("4 lần lỗi chưa bị block")
	}
	rl.RecordFailure(ip, now)

	// Lần 5 lỗi đã ghi nhận -> isBlocked = true
	if !rl.IsBlocked(ip, now) {
		t.Fatal("5 lần lỗi phải bị block")
	}

	// Sau 61 giây, cửa sổ trượt hết hạn -> không còn bị block
	future := now.Add(61 * time.Second)
	if rl.IsBlocked(ip, future) {
		t.Fatal("sau 1 phút phải hết block")
	}
}
