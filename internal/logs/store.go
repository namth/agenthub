package logs

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Line struct {
	ID           int64     `json:"id"`
	WorkerID     string    `json:"worker_id"`
	AssignmentID string    `json:"assignment_id"`
	TaskID       string    `json:"task_id"`
	Chunk        string    `json:"chunk"`
	TS           time.Time `json:"ts"`
}

type Store struct {
	db          *sql.DB
	now         func() time.Time
	keepPerTask int

	mu   sync.RWMutex
	subs map[string]map[chan Line]struct{}
}

func NewStore(db *sql.DB, now func() time.Time, keepPerTask int) *Store {
	if now == nil {
		now = time.Now
	}
	if keepPerTask <= 0 {
		keepPerTask = 5000
	}
	return &Store{
		db:          db,
		now:         now,
		keepPerTask: keepPerTask,
		subs:        make(map[string]map[chan Line]struct{}),
	}
}

func (s *Store) Append(workerID, assignmentID, chunk string) error {
	var taskID string
	err := s.db.QueryRow(`
		SELECT jobs.task_id 
		FROM assignments 
		JOIN jobs ON assignments.job_id = jobs.id 
		WHERE assignments.id = ?
	`, assignmentID).Scan(&taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("assignment %q not found: %w", assignmentID, sql.ErrNoRows)
		}
		return fmt.Errorf("lookup task for assignment %q: %w", assignmentID, err)
	}

	const maxChunk = 64 * 1024
	if len(chunk) > maxChunk {
		chunk = chunk[:maxChunk]
	}

	ts := s.now().UTC()
	formattedTS := ts.Format("2006-01-02 15:04:05")

	res, err := s.db.Exec(`
		INSERT INTO task_logs (task_id, worker_id, assignment_id, log_chunk, timestamp)
		VALUES (?, ?, ?, ?, ?)
	`, taskID, workerID, assignmentID, chunk, formattedTS)
	if err != nil {
		return fmt.Errorf("insert task_logs: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("get last insert id: %w", err)
	}

	_, err = s.db.Exec(`
		DELETE FROM task_logs 
		WHERE task_id = ? AND id NOT IN (
			SELECT id FROM task_logs WHERE task_id = ? ORDER BY id DESC LIMIT ?
		)
	`, taskID, taskID, s.keepPerTask)
	if err != nil {
		return fmt.Errorf("retention cleanup: %w", err)
	}

	line := Line{
		ID:           id,
		WorkerID:     workerID,
		AssignmentID: assignmentID,
		TaskID:       taskID,
		Chunk:        chunk,
		TS:           ts,
	}

	s.broadcast(workerID, line)
	return nil
}

func (s *Store) broadcast(workerID string, line Line) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subs[workerID] {
		select {
		case ch <- line:
		default:
		}
	}
}

func (s *Store) Recent(workerID string, n int) ([]Line, error) {
	if n <= 0 {
		return []Line{}, nil
	}

	rows, err := s.db.Query(`
		SELECT id, worker_id, assignment_id, task_id, log_chunk, timestamp 
		FROM (
			SELECT id, worker_id, assignment_id, task_id, log_chunk, timestamp 
			FROM task_logs 
			WHERE worker_id = ? 
			ORDER BY id DESC 
			LIMIT ?
		) 
		ORDER BY id ASC
	`, workerID, n)
	if err != nil {
		return nil, fmt.Errorf("query recent logs: %w", err)
	}
	defer rows.Close()

	lines := make([]Line, 0, n)
	for rows.Next() {
		var l Line
		var rawTS any
		if err := rows.Scan(&l.ID, &l.WorkerID, &l.AssignmentID, &l.TaskID, &l.Chunk, &rawTS); err != nil {
			return nil, fmt.Errorf("scan log line: %w", err)
		}
		if t := parseTime(rawTS); t != nil {
			l.TS = *t
		}
		lines = append(lines, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return lines, nil
}

func parseTime(val any) *time.Time {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case time.Time:
		t := v.UTC()
		return &t
	case string:
		if v == "" {
			return nil
		}
		for _, layout := range []string{
			"2006-01-02 15:04:05",
			time.RFC3339Nano,
			time.RFC3339,
		} {
			if t, err := time.Parse(layout, v); err == nil {
				t = t.UTC()
				return &t
			}
		}
	case []byte:
		return parseTime(string(v))
	}
	return nil
}

func (s *Store) Subscribe(workerID string) (<-chan Line, func()) {
	ch := make(chan Line, 256)
	s.mu.Lock()
	if s.subs == nil {
		s.subs = make(map[string]map[chan Line]struct{})
	}
	if s.subs[workerID] == nil {
		s.subs[workerID] = make(map[chan Line]struct{})
	}
	s.subs[workerID][ch] = struct{}{}
	s.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if m, ok := s.subs[workerID]; ok {
				delete(m, ch)
				if len(m) == 0 {
					delete(s.subs, workerID)
				}
			}
			close(ch)
		})
	}
	return ch, cancel
}
