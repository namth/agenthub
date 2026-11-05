package machines

import "time"

var DownloadRegexForTest = downloadRegex

func GenerateJoinCodeForTest() (string, error) {
	return generateJoinCode()
}

type TestRateLimiter struct {
	rl *rateLimiter
}

func NewRateLimiterForTest() *TestRateLimiter {
	return &TestRateLimiter{rl: newRateLimiter()}
}

func (t *TestRateLimiter) IsBlocked(ip string, now time.Time) bool {
	return t.rl.isBlocked(ip, now)
}

func (t *TestRateLimiter) RecordFailure(ip string, now time.Time) {
	t.rl.recordFailure(ip, now)
}
