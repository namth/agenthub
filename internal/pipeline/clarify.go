package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"
)

type ClarifyQuestion struct {
	ID          string   `json:"id"`
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	Recommended string   `json:"recommended"`
	Reason      string   `json:"reason"`
}

func WithOnNeedInfo(fn func(taskID string, round int, questions []ClarifyQuestion)) Option {
	return func(p *Pipeline) {
		p.onNeedInfo = fn
	}
}

func WithOnClarifyCapped(fn func(taskID string)) Option {
	return func(p *Pipeline) {
		p.onClarifyCapped = fn
	}
}

func WithOnClarifyReminder(fn func(taskID string, round int, hours int)) Option {
	return func(p *Pipeline) {
		p.onClarifyReminder = fn
	}
}

var clarifyIDRegex = regexp.MustCompile(`^[a-z0-9_]{2,40}$`)

func validateClarifyQuestions(qs []ClarifyQuestion) bool {
	if len(qs) < 1 || len(qs) > 8 {
		return false
	}
	seenIDs := make(map[string]bool, len(qs))
	for _, q := range qs {
		if !clarifyIDRegex.MatchString(q.ID) {
			return false
		}
		if seenIDs[q.ID] {
			return false
		}
		seenIDs[q.ID] = true

		qLen := utf8.RuneCountInString(q.Question)
		if qLen < 1 || qLen > 300 {
			return false
		}

		if len(q.Options) > 6 {
			return false
		}
		for _, opt := range q.Options {
			if utf8.RuneCountInString(opt) > 300 {
				return false
			}
		}

		if len(q.Options) == 0 {
			if q.Recommended != "" {
				return false
			}
		} else {
			if q.Recommended != "" {
				found := false
				for _, opt := range q.Options {
					if opt == q.Recommended {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
		}

		if utf8.RuneCountInString(q.Reason) > 300 {
			return false
		}
	}
	return true
}

func parseDBTime(s string) (time.Time, error) {
	for _, layout := range []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05.999999999",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Parse("2006-01-02", s)
}

// RemindClarifications nhắc nhở các câu hỏi làm rõ đang ở trạng thái WAITING sau 24h và 72h.
func (p *Pipeline) RemindClarifications(ctx context.Context, now time.Time) (int, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, task_id, round, created_at, reminded_at
		FROM task_clarifications
		WHERE status = 'WAITING'
	`)
	if err != nil {
		return 0, fmt.Errorf("query waiting clarifications: %w", err)
	}
	defer rows.Close()

	type clarifyItem struct {
		id         string
		taskID     string
		round      int
		createdAt  string
		remindedAt sql.NullString
	}
	var items []clarifyItem
	for rows.Next() {
		var it clarifyItem
		if err := rows.Scan(&it.id, &it.taskID, &it.round, &it.createdAt, &it.remindedAt); err != nil {
			return 0, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	remindedCount := 0
	for _, it := range items {
		createdTime, err := parseDBTime(it.createdAt)
		if err != nil {
			continue
		}

		var hasReminded bool
		var remindedTime time.Time
		if it.remindedAt.Valid && it.remindedAt.String != "" {
			if t, err := parseDBTime(it.remindedAt.String); err == nil {
				hasReminded = true
				remindedTime = t
			}
		}

		// Điều kiện 72h: created_at <= now - 72h và (chưa nhắc hoặc reminded_at < created_at + 72h)
		if !createdTime.After(now.Add(-72 * time.Hour)) {
			if !hasReminded || remindedTime.Before(createdTime.Add(72*time.Hour)) {
				remindedCount++
				nowStr := now.UTC().Format("2006-01-02 15:04:05")
				_, _ = p.db.ExecContext(ctx, `UPDATE task_clarifications SET reminded_at = ? WHERE id = ?`, nowStr, it.id)
				if p.onClarifyReminder != nil {
					p.onClarifyReminder(it.taskID, it.round, 72)
				}
				continue
			}
		}

		// Điều kiện 24h: created_at <= now - 24h và chưa nhắc
		if !createdTime.After(now.Add(-24 * time.Hour)) {
			if !hasReminded {
				remindedCount++
				nowStr := now.UTC().Format("2006-01-02 15:04:05")
				_, _ = p.db.ExecContext(ctx, `UPDATE task_clarifications SET reminded_at = ? WHERE id = ?`, nowStr, it.id)
				if p.onClarifyReminder != nil {
					p.onClarifyReminder(it.taskID, it.round, 24)
				}
			}
		}
	}

	return remindedCount, nil
}
