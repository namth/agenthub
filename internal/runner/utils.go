package runner

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

func DefaultExhaustPatterns() []string {
	return []string{
		"individual quota reached",
		"claude usage limit reached",
		"usage limit reached",
		"rate limit exceeded",
		"429 rate limit",
		"resource_exhausted",
		"quota exceeded",
	}
}

var (
	rfc3339Regex  = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})\b`)
	durationRegex = regexp.MustCompile(`(?i)\bresets\s+in\s+([0-9]+[hms](?:[0-9]+[hms])*)`)
	ampmRegex     = regexp.MustCompile(`(?i)\bresets\s+(\d{1,2})(?::(\d{2}))?\s*(am|pm)\b`)
)

func ParseResetsAt(text string, now time.Time) *time.Time {
	if text == "" {
		return nil
	}

	// 1. RFC3339 timestamp
	if m := rfc3339Regex.FindString(text); m != "" {
		if t, err := time.Parse(time.RFC3339, m); err == nil {
			return &t
		}
	}

	// 2. Relative duration: "Resets in 105h19m38s", "resets in 45m"
	if sub := durationRegex.FindStringSubmatch(text); len(sub) > 1 {
		if d, err := time.ParseDuration(sub[1]); err == nil {
			t := now.Add(d)
			return &t
		}
	}

	// 3. Time of day: "resets 9:40pm", "resets 7am"
	if sub := ampmRegex.FindStringSubmatch(text); len(sub) > 3 {
		hour, err := strconv.Atoi(sub[1])
		if err != nil {
			return nil
		}
		min := 0
		if sub[2] != "" {
			if m, err := strconv.Atoi(sub[2]); err == nil {
				min = m
			}
		}
		ampm := strings.ToLower(sub[3])
		if ampm == "am" {
			if hour == 12 {
				hour = 0
			}
		} else if ampm == "pm" {
			if hour < 12 {
				hour += 12
			}
		}
		target := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())
		if !target.After(now) {
			target = target.AddDate(0, 0, 1)
		}
		return &target
	}

	return nil
}

func BuildHandoffNote(diffStat string, logLines []string) string {
	if len(logLines) > 50 {
		logLines = logLines[len(logLines)-50:]
	}

	var b strings.Builder
	if diffStat != "" {
		b.WriteString(diffStat)
		if !strings.HasSuffix(diffStat, "\n") {
			b.WriteString("\n")
		}
	}
	for _, l := range logLines {
		b.WriteString(l)
		if !strings.HasSuffix(l, "\n") {
			b.WriteString("\n")
		}
	}

	res := b.String()
	if len(res) > 8192 {
		res = res[:8192]
	}
	return res
}
