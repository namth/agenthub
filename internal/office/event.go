package office

import (
	"regexp"
	"time"
)

// Event đại diện cho một sự kiện hiển thị trên sơ đồ văn phòng ảo (FEAT-02, BR-19).
type Event struct {
	ID   int64     `json:"id"`
	TS   time.Time `json:"ts"`
	Who  string    `json:"who"`
	Type string    `json:"type"`
	To   string    `json:"to,omitempty"`
	Task string    `json:"task,omitempty"`
	Text string    `json:"text,omitempty"`
}

var (
	reSecretPrefix = regexp.MustCompile(`(ghp_|gho_|sk-)\S*`)
	reLongSecret   = regexp.MustCompile(`[A-Za-z0-9]{32,}`)
)

// cleanText che các bí mật (token, key >= 32 ký tự) và cắt tối đa 80 ký tự rune (BR-20).
func cleanText(s string) string {
	if s == "" {
		return ""
	}
	s = reSecretPrefix.ReplaceAllString(s, "***")
	s = reLongSecret.ReplaceAllString(s, "***")
	runes := []rune(s)
	if len(runes) > 80 {
		runes = runes[:80]
	}
	return string(runes)
}
