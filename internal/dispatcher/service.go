package dispatcher

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const timeFormat = "2006-01-02 15:04:05"

func parseDBTime(val any) *time.Time {
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
		if t, err := time.Parse(timeFormat, v); err == nil {
			t = t.UTC()
			return &t
		}
		if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
			t = t.UTC()
			return &t
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}

func newID(prefix string) string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, b[0])
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b[:]))
}

// Service cài đặt interface dispatcher.API.
type Service struct {
	db  *sql.DB
	clk Clock
	cfg Config
	n   Notifier

	mu               sync.Mutex
	adminBannedUntil map[string]time.Time
	isVacant         bool
}

// New khởi tạo một Service mới. n có thể nil.
func New(db *sql.DB, clk Clock, cfg Config, n Notifier) *Service {
	s := &Service{
		db:               db,
		clk:              clk,
		cfg:              cfg,
		n:                n,
		adminBannedUntil: make(map[string]time.Time),
	}
	var state string
	var vacantSince any
	_ = db.QueryRow("SELECT state, vacant_since FROM lead_lease WHERE id = 1").Scan(&state, &vacantSince)
	if state == "VACANT" && parseDBTime(vacantSince) != nil {
		s.isVacant = true
	}
	return s
}

func (s *Service) formatTime(t time.Time) string {
	return t.UTC().Format(timeFormat)
}

func (s *Service) notify(events []Event) {
	if s.n == nil || len(events) == 0 {
		return
	}
	for _, e := range events {
		s.n.Notify(e)
	}
}

// ---------- Fleet ----------

func (s *Service) Connect(workerID string, kinds []string) ([]Agent, error) {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	// 1. Kiểm tra máy tồn tại
	var dummy string
	err := s.db.QueryRow("SELECT id FROM workers WHERE id = ?", workerID).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 2. Chuyển worker sang ONLINE
	_, err = tx.Exec(
		"UPDATE workers SET conn_status = 'ONLINE', offline_since = NULL, last_heartbeat = ? WHERE id = ?",
		nowStr, workerID,
	)
	if err != nil {
		return nil, err
	}

	// 3. Đăng ký/cập nhật các agent
	for _, kind := range kinds {
		agentID := workerID + "/" + strings.ToLower(kind)
		_, err = tx.Exec(
			`INSERT INTO agents (id, worker_id, kind, status, idle_since, created_at)
			 VALUES (?, ?, ?, 'IDLE', ?, ?)
			 ON CONFLICT(worker_id, kind) DO UPDATE SET idle_since = COALESCE(agents.idle_since, excluded.idle_since)`,
			agentID, workerID, kind, nowStr, nowStr,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// 4. Thử trao lease (BR-06)
	evs, err := s.tryAssignLeaseLocked("")
	if err != nil {
		return nil, err
	}
	events = append(events, evs...)

	// 5. Trả danh sách agent của worker
	rows, err := s.db.Query(
		"SELECT id, worker_id, kind, status, cooldown_until FROM agents WHERE worker_id = ? ORDER BY id",
		workerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Agent
	for rows.Next() {
		var a Agent
		var cdVal any
		if err := rows.Scan(&a.ID, &a.WorkerID, &a.Kind, &a.Status, &cdVal); err != nil {
			return nil, err
		}
		a.CooldownUntil = parseDBTime(cdVal)
		result = append(result, a)
	}
	return result, rows.Err()
}

func (s *Service) Heartbeat(workerID string) error {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	var connStatus string
	err := s.db.QueryRow("SELECT conn_status FROM workers WHERE id = ?", workerID).Scan(&connStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if connStatus == "OFFLINE" {
		_, err = s.db.Exec(
			"UPDATE workers SET conn_status = 'ONLINE', offline_since = NULL, last_heartbeat = ? WHERE id = ?",
			nowStr, workerID,
		)
		if err != nil {
			return err
		}
		events = append(events, Event{Type: EvWorkerOnline, At: now, WorkerID: workerID})
	} else {
		_, err = s.db.Exec("UPDATE workers SET last_heartbeat = ? WHERE id = ?", nowStr, workerID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Disconnect(workerID string) error {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	var connStatus string
	err := s.db.QueryRow("SELECT conn_status FROM workers WHERE id = ?", workerID).Scan(&connStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	_, err = s.db.Exec("UPDATE workers SET conn_status = 'OFFLINE', offline_since = ? WHERE id = ?", nowStr, workerID)
	if err != nil {
		return err
	}
	events = append(events, Event{Type: EvWorkerOffline, At: now, WorkerID: workerID})

	// Kiểm tra nếu lead thuộc worker này
	var leadAgentID sql.NullString
	err = s.db.QueryRow("SELECT agent_id FROM lead_lease WHERE id = 1 AND state = 'HELD'").Scan(&leadAgentID)
	if err == nil && leadAgentID.Valid && leadAgentID.String != "" {
		var agentWorker string
		err = s.db.QueryRow("SELECT worker_id FROM agents WHERE id = ?", leadAgentID.String).Scan(&agentWorker)
		if err == nil && agentWorker == workerID {
			evs, err := s.revokeLeadLocked(RevokeOffline, false)
			if err != nil {
				return err
			}
			events = append(events, evs...)
		}
	}

	return nil
}

func (s *Service) Tick() error {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	// 1. Máy quá OfflineAfter -> OFFLINE (+ thu hồi lease nếu lead)
	offlineDeadline := s.formatTime(now.Add(-s.cfg.OfflineAfter))
	rows, err := s.db.Query(
		"SELECT id FROM workers WHERE conn_status = 'ONLINE' AND (last_heartbeat IS NULL OR last_heartbeat < ?)",
		offlineDeadline,
	)
	if err != nil {
		return err
	}
	var offlineWorkers []string
	for rows.Next() {
		var wID string
		if err := rows.Scan(&wID); err != nil {
			rows.Close()
			return err
		}
		offlineWorkers = append(offlineWorkers, wID)
	}
	rows.Close()

	for _, wID := range offlineWorkers {
		_, err := s.db.Exec("UPDATE workers SET conn_status = 'OFFLINE', offline_since = ? WHERE id = ?", nowStr, wID)
		if err != nil {
			return err
		}
		events = append(events, Event{Type: EvWorkerOffline, At: now, WorkerID: wID})

		var leaseState string
		var leaseAgent sql.NullString
		_ = s.db.QueryRow("SELECT state, agent_id FROM lead_lease WHERE id = 1").Scan(&leaseState, &leaseAgent)
		if leaseState == "HELD" && leaseAgent.Valid && leaseAgent.String != "" {
			var agWorker string
			_ = s.db.QueryRow("SELECT worker_id FROM agents WHERE id = ?", leaseAgent.String).Scan(&agWorker)
			if agWorker == wID {
				evs, err := s.revokeLeadLocked(RevokeOffline, false)
				if err != nil {
					return err
				}
				events = append(events, evs...)
			}
		}
	}

	// 2. Máy OFFLINE >= ReclaimAfter -> assignment RECLAIMED, job QUEUED
	reclaimDeadline := s.formatTime(now.Add(-s.cfg.ReclaimAfter))
	rows, err = s.db.Query(
		`SELECT a.id, a.job_id, a.agent_id, a.attempt, j.task_id, j.project_id
		 FROM assignments a
		 JOIN agents ag ON a.agent_id = ag.id
		 JOIN workers w ON ag.worker_id = w.id
		 JOIN jobs j ON a.job_id = j.id
		 WHERE a.status = 'ACTIVE' AND w.conn_status = 'OFFLINE' AND w.offline_since IS NOT NULL AND w.offline_since <= ?`,
		reclaimDeadline,
	)
	if err != nil {
		return err
	}
	type reclaimInfo struct {
		asgID     string
		jobID     string
		agentID   string
		attempt   int
		taskID    string
		projectID string
	}
	var toReclaim []reclaimInfo
	for rows.Next() {
		var ri reclaimInfo
		if err := rows.Scan(&ri.asgID, &ri.jobID, &ri.agentID, &ri.attempt, &ri.taskID, &ri.projectID); err != nil {
			rows.Close()
			return err
		}
		toReclaim = append(toReclaim, ri)
	}
	rows.Close()

	for _, ri := range toReclaim {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}

		_, err = tx.Exec("UPDATE assignments SET status = 'RECLAIMED', ended_at = ? WHERE id = ?", nowStr, ri.asgID)
		if err != nil {
			tx.Rollback()
			return err
		}

		_, err = tx.Exec("UPDATE agents SET status = 'IDLE', idle_since = ? WHERE id = ?", nowStr, ri.agentID)
		if err != nil {
			tx.Rollback()
			return err
		}

		events = append(events, Event{
			Type:         EvJobReclaimed,
			At:           now,
			JobID:        ri.jobID,
			AssignmentID: ri.asgID,
		})

		if ri.attempt >= s.cfg.MaxAttempts {
			_, err = tx.Exec("UPDATE jobs SET status = 'FAILED', current_assignment_id = NULL, updated_at = ? WHERE id = ?", nowStr, ri.jobID)
			if err != nil {
				tx.Rollback()
				return err
			}
			_, err = tx.Exec("UPDATE tasks SET status = 'PLANNING', updated_at = ? WHERE id = ?", nowStr, ri.taskID)
			if err != nil {
				tx.Rollback()
				return err
			}
			replanID := newID("job")
			_, err = tx.Exec(
				`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, lead_only, status, priority, attempt, created_at, updated_at)
				 VALUES (?, ?, ?, 'REPLAN', 'CLAUDE', 1, 'QUEUED', 10, 0, ?, ?)`,
				replanID, ri.projectID, ri.taskID, nowStr, nowStr,
			)
			if err != nil {
				tx.Rollback()
				return err
			}
			events = append(events, Event{
				Type:   EvJobFailed,
				At:     now,
				JobID:  ri.jobID,
				TaskID: ri.taskID,
			})
			events = append(events, Event{
				Type:   EvJobAvailable,
				At:     now,
				JobID:  replanID,
				TaskID: ri.taskID,
			})
		} else {
			_, err = tx.Exec("UPDATE jobs SET status = 'QUEUED', current_assignment_id = NULL, updated_at = ? WHERE id = ?", nowStr, ri.jobID)
			if err != nil {
				tx.Rollback()
				return err
			}
			events = append(events, Event{
				Type:   EvJobAvailable,
				At:     now,
				JobID:  ri.jobID,
				TaskID: ri.taskID,
			})
		}

		if err := tx.Commit(); err != nil {
			return err
		}
	}

	// 3. Lease hết hạn -> thu hồi EXPIRED
	var leaseState string
	var expVal any
	_ = s.db.QueryRow("SELECT state, expires_at FROM lead_lease WHERE id = 1").Scan(&leaseState, &expVal)
	expTime := parseDBTime(expVal)
	if leaseState == "HELD" && expTime != nil && !expTime.After(now) {
		evs, err := s.revokeLeadLocked(RevokeExpired, false)
		if err != nil {
			return err
		}
		events = append(events, evs...)
	}

	// 4. Agent hết cooldown -> IDLE
	rows, err = s.db.Query(
		"SELECT id FROM agents WHERE status = 'COOLDOWN' AND cooldown_until IS NOT NULL AND cooldown_until <= ?",
		nowStr,
	)
	if err != nil {
		return err
	}
	var expiredAgents []string
	for rows.Next() {
		var aID string
		if err := rows.Scan(&aID); err != nil {
			rows.Close()
			return err
		}
		expiredAgents = append(expiredAgents, aID)
	}
	rows.Close()

	for _, aID := range expiredAgents {
		_, err := s.db.Exec(
			"UPDATE agents SET status = 'IDLE', cooldown_until = NULL, cooldown_reason = NULL, idle_since = ? WHERE id = ?",
			nowStr, aID,
		)
		if err != nil {
			return err
		}
		events = append(events, Event{
			Type:    EvAgentIdle,
			At:      now,
			AgentID: aID,
		})
	}

	// 5. Assignment ACTIVE không log >= LogIdleWarn -> tạo job UNBLOCK (lead_only)
	warnDeadline := s.formatTime(now.Add(-s.cfg.LogIdleWarn))
	rows, err = s.db.Query(
		`SELECT a.id, a.job_id, j.task_id, j.project_id
		 FROM assignments a
		 JOIN jobs j ON a.job_id = j.id
		 WHERE a.status = 'ACTIVE' AND COALESCE(a.last_log_at, a.started_at) <= ?`,
		warnDeadline,
	)
	if err != nil {
		return err
	}
	type idleWarnInfo struct {
		asgID     string
		jobID     string
		taskID    string
		projectID string
	}
	var warnAssignments []idleWarnInfo
	for rows.Next() {
		var wi idleWarnInfo
		if err := rows.Scan(&wi.asgID, &wi.jobID, &wi.taskID, &wi.projectID); err != nil {
			rows.Close()
			return err
		}
		warnAssignments = append(warnAssignments, wi)
	}
	rows.Close()

	for _, wi := range warnAssignments {
		var unblockCount int
		err := s.db.QueryRow(
			"SELECT COUNT(*) FROM jobs WHERE task_id = ? AND kind = 'UNBLOCK' AND status IN ('QUEUED', 'RUNNING')",
			wi.taskID,
		).Scan(&unblockCount)
		if err == nil && unblockCount == 0 {
			unblockJobID := newID("job")
			_, _ = s.db.Exec(
				`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, lead_only, status, priority, attempt, created_at, updated_at)
				 VALUES (?, ?, ?, 'UNBLOCK', 'CLAUDE', 1, 'QUEUED', 100, 0, ?, ?)`,
				unblockJobID, wi.projectID, wi.taskID, nowStr, nowStr,
			)
			events = append(events, Event{
				Type:   EvJobAvailable,
				At:     now,
				JobID:  unblockJobID,
				TaskID: wi.taskID,
			})
		}
	}

	// 6. VACANT -> thử trao
	_ = s.db.QueryRow("SELECT state FROM lead_lease WHERE id = 1").Scan(&leaseState)
	if leaseState == "VACANT" {
		evs, err := s.tryAssignLeaseLocked("")
		if err != nil {
			return err
		}
		events = append(events, evs...)
	}

	return nil
}

// ---------- Jobs ----------

func (s *Service) EnqueueJob(j NewJob) (string, error) {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	jobID := newID("job")
	priority := j.Priority
	if priority == 0 {
		priority = 100
	}
	leadOnly := 0
	if j.LeadOnly {
		leadOnly = 1
	}

	var branchName *string
	if j.BranchName != "" {
		branchName = &j.BranchName
	}

	_, err := s.db.Exec(
		`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, lead_only, status, priority, attempt, branch_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, 'QUEUED', ?, 0, ?, ?, ?)`,
		jobID, j.ProjectID, j.TaskID, j.Kind, j.RequiredAgent, leadOnly, priority, branchName, nowStr, nowStr,
	)
	if err != nil {
		return "", err
	}

	events = append(events, Event{
		Type:   EvJobAvailable,
		At:     now,
		JobID:  jobID,
		TaskID: j.TaskID,
	})

	return jobID, nil
}

func (s *Service) RequestJob(workerID, agentID string) (*Assignment, error) {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	// 1. Kiểm tra agent tồn tại
	var agWorker, kind, status string
	err := s.db.QueryRow("SELECT worker_id, kind, status FROM agents WHERE id = ?", agentID).Scan(&agWorker, &kind, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// 2. Kiểm tra quyền sở hữu
	if agWorker != workerID {
		return nil, ErrAgentNotOwned
	}

	// 3. Phải IDLE
	if status != "IDLE" {
		return nil, nil
	}

	// 4. Không có assignment ACTIVE nào khác
	var activeCount int
	err = s.db.QueryRow("SELECT COUNT(*) FROM assignments WHERE agent_id = ? AND status = 'ACTIVE'", agentID).Scan(&activeCount)
	if err != nil {
		return nil, err
	}
	if activeCount > 0 {
		return nil, nil
	}

	// 5. Kiểm tra lead
	var leaseState string
	var leaseAgent sql.NullString
	_ = s.db.QueryRow("SELECT state, agent_id FROM lead_lease WHERE id = 1").Scan(&leaseState, &leaseAgent)
	isLead := (leaseState == "HELD" && leaseAgent.Valid && leaseAgent.String == agentID)

	// 6. Tìm job QUEUED
	var query string
	if isLead {
		query = `SELECT id, task_id, kind, attempt, branch_name
		         FROM jobs
		         WHERE status = 'QUEUED' AND required_agent = ?
		         ORDER BY lead_only DESC, priority ASC, created_at ASC
		         LIMIT 1`
	} else {
		query = `SELECT id, task_id, kind, attempt, branch_name
		         FROM jobs
		         WHERE status = 'QUEUED' AND required_agent = ? AND lead_only = 0
		         ORDER BY priority ASC, created_at ASC
		         LIMIT 1`
	}

	var jobID, taskID, jobKind string
	var prevAttempt int
	var branch sql.NullString
	err = s.db.QueryRow(query, kind).Scan(&jobID, &taskID, &jobKind, &prevAttempt, &branch)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Ghi chú bàn giao gần nhất của job
	var handoffNote sql.NullString
	_ = s.db.QueryRow(
		"SELECT handoff_note FROM assignments WHERE job_id = ? AND handoff_note IS NOT NULL AND handoff_note != '' ORDER BY started_at DESC LIMIT 1",
		jobID,
	).Scan(&handoffNote)

	newAttempt := prevAttempt + 1
	asgID := newID("asg")

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"UPDATE jobs SET status = 'RUNNING', attempt = ?, current_assignment_id = ?, updated_at = ? WHERE id = ?",
		newAttempt, asgID, nowStr, jobID,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec("UPDATE agents SET status = 'BUSY' WHERE id = ?", agentID)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(
		"INSERT INTO assignments (id, job_id, agent_id, attempt, status, handoff_note, started_at) VALUES (?, ?, ?, ?, 'ACTIVE', NULL, ?)",
		asgID, jobID, agentID, newAttempt, nowStr,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	events = append(events, Event{Type: EvJobAssigned, At: now, AgentID: agentID, AssignmentID: asgID, JobID: jobID, TaskID: taskID, Reason: jobKind})

	return &Assignment{
		ID:          asgID,
		JobID:       jobID,
		TaskID:      taskID,
		AgentID:     agentID,
		JobKind:     jobKind,
		Attempt:     newAttempt,
		BranchName:  branch.String,
		HandoffNote: handoffNote.String,
	}, nil
}

func (s *Service) Log(workerID, assignmentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	var asgStatus string
	var agWorker string
	var currentAsgID sql.NullString
	err := s.db.QueryRow(
		`SELECT a.status, ag.worker_id, j.current_assignment_id
		 FROM assignments a
		 JOIN agents ag ON a.agent_id = ag.id
		 JOIN jobs j ON a.job_id = j.id
		 WHERE a.id = ?`,
		assignmentID,
	).Scan(&asgStatus, &agWorker, &currentAsgID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	if agWorker != workerID {
		return ErrAgentNotOwned
	}
	if asgStatus != "ACTIVE" || !currentAsgID.Valid || currentAsgID.String != assignmentID {
		return ErrStaleAssignment
	}

	_, err = s.db.Exec("UPDATE assignments SET last_log_at = ? WHERE id = ?", nowStr, assignmentID)
	return err
}

func (s *Service) Submit(workerID, assignmentID string) error {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	// 1. Kiểm tra assignment
	var asgStatus string
	var jobID, agentID, agWorker string
	var currentAsgID sql.NullString
	var softMax sql.NullInt64
	var softWindow int
	err := s.db.QueryRow(
		`SELECT a.status, a.job_id, a.agent_id, ag.worker_id, ag.soft_max_jobs, ag.soft_window_hours, j.current_assignment_id
		 FROM assignments a
		 JOIN agents ag ON a.agent_id = ag.id
		 JOIN jobs j ON a.job_id = j.id
		 WHERE a.id = ?`,
		assignmentID,
	).Scan(&asgStatus, &jobID, &agentID, &agWorker, &softMax, &softWindow, &currentAsgID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// 2. Quyền sở hữu
	if agWorker != workerID {
		return ErrAgentNotOwned
	}

	// 3. Fencing / Stale
	if asgStatus != "ACTIVE" || !currentAsgID.Valid || currentAsgID.String != assignmentID {
		return ErrStaleAssignment
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE assignments SET status = 'SUBMITTED', ended_at = ? WHERE id = ?", nowStr, assignmentID)
	if err != nil {
		return err
	}
	events = append(events, Event{Type: EvJobSubmitted, At: now, AgentID: agentID, AssignmentID: assignmentID, JobID: jobID})

	_, err = tx.Exec("UPDATE jobs SET status = 'DONE', updated_at = ? WHERE id = ?", nowStr, jobID)
	if err != nil {
		return err
	}

	// Kiểm tra hạn mềm (BR-09)
	hitSoftLimit := false
	var cooldownUntil time.Time
	if softMax.Valid {
		windowStart := now.Add(-time.Duration(softWindow) * time.Hour)
		var count int
		var oldestVal any
		err = tx.QueryRow(
			`SELECT COUNT(*), MIN(started_at)
			 FROM assignments
			 WHERE agent_id = ? AND started_at >= ?`,
			agentID, s.formatTime(windowStart),
		).Scan(&count, &oldestVal)
		if err != nil {
			return err
		}
		if count >= int(softMax.Int64) {
			hitSoftLimit = true
			oldestTime := parseDBTime(oldestVal)
			if oldestTime != nil {
				cooldownUntil = oldestTime.Add(time.Duration(softWindow) * time.Hour)
			} else {
				cooldownUntil = now.Add(time.Duration(softWindow) * time.Hour)
			}
		}
	}

	if hitSoftLimit {
		_, err = tx.Exec(
			"UPDATE agents SET status = 'COOLDOWN', cooldown_until = ?, cooldown_reason = 'SOFT_LIMIT' WHERE id = ?",
			s.formatTime(cooldownUntil), agentID,
		)
		if err != nil {
			return err
		}
		events = append(events, Event{
			Type:    EvAgentCooldown,
			At:      now,
			AgentID: agentID,
			Until:   &cooldownUntil,
			Reason:  "SOFT_LIMIT",
		})
	} else {
		_, err = tx.Exec(
			"UPDATE agents SET status = 'IDLE', idle_since = ? WHERE id = ?",
			nowStr, agentID,
		)
		if err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Nếu lead chạm hạn mềm, thu hồi lease ngay
	if hitSoftLimit {
		var leaseState string
		var leaseAgent sql.NullString
		_ = s.db.QueryRow("SELECT state, agent_id FROM lead_lease WHERE id = 1").Scan(&leaseState, &leaseAgent)
		if leaseState == "HELD" && leaseAgent.Valid && leaseAgent.String == agentID {
			evs, err := s.revokeLeadLocked(RevokeReleased, false)
			if err != nil {
				return err
			}
			events = append(events, evs...)
		}
	}

	return nil
}

func (s *Service) Exhausted(workerID, agentID, assignmentID string, resetsAt *time.Time, reason, handoffNote, wipCommit string) error {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	// 1. Kiểm tra agent tồn tại
	var agWorker string
	err := s.db.QueryRow("SELECT worker_id FROM agents WHERE id = ?", agentID).Scan(&agWorker)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// 2. Quyền sở hữu
	if agWorker != workerID {
		return ErrAgentNotOwned
	}

	// 3. Tính thời điểm cooldown
	var cdUntil time.Time
	if resetsAt != nil {
		cdUntil = *resetsAt
	} else {
		cdUntil = now.Add(s.cfg.DefaultCooldown)
	}
	cdUntilStr := s.formatTime(cdUntil)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"UPDATE agents SET status = 'COOLDOWN', cooldown_until = ?, cooldown_reason = 'EXHAUSTED' WHERE id = ?",
		cdUntilStr, agentID,
	)
	if err != nil {
		return err
	}

	events = append(events, Event{
		Type:    EvAgentCooldown,
		At:      now,
		AgentID: agentID,
		Until:   &cdUntil,
		Reason:  "EXHAUSTED",
	})

	// 4. Xử lý assignment nếu có
	if assignmentID != "" {
		var asgStatus string
		var jobID string
		var asgAttempt int
		err = tx.QueryRow("SELECT status, job_id, attempt FROM assignments WHERE id = ?", assignmentID).Scan(&asgStatus, &jobID, &asgAttempt)
		if err == nil && asgStatus == "ACTIVE" {
			var wip *string
			if wipCommit != "" {
				wip = &wipCommit
			}
			var note *string
			if handoffNote != "" {
				note = &handoffNote
			}

			_, err = tx.Exec(
				"UPDATE assignments SET status = 'EXHAUSTED', ended_at = ?, handoff_note = ?, wip_commit = ? WHERE id = ?",
				nowStr, note, wip, assignmentID,
			)
			if err != nil {
				return err
			}

			var taskID, projectID string
			err = tx.QueryRow("SELECT task_id, project_id FROM jobs WHERE id = ?", jobID).Scan(&taskID, &projectID)
			if err != nil {
				return err
			}

			if asgAttempt >= s.cfg.MaxAttempts {
				_, err = tx.Exec(
					"UPDATE jobs SET status = 'FAILED', current_assignment_id = NULL, updated_at = ? WHERE id = ?",
					nowStr, jobID,
				)
				if err != nil {
					return err
				}

				_, err = tx.Exec("UPDATE tasks SET status = 'PLANNING', updated_at = ? WHERE id = ?", nowStr, taskID)
				if err != nil {
					return err
				}

				replanID := newID("job")
				_, err = tx.Exec(
					`INSERT INTO jobs (id, project_id, task_id, kind, required_agent, lead_only, status, priority, attempt, created_at, updated_at)
					 VALUES (?, ?, ?, 'REPLAN', 'CLAUDE', 1, 'QUEUED', 10, 0, ?, ?)`,
					replanID, projectID, taskID, nowStr, nowStr,
				)
				if err != nil {
					return err
				}

				events = append(events, Event{
					Type:   EvJobFailed,
					At:     now,
					JobID:  jobID,
					TaskID: taskID,
				})
				events = append(events, Event{
					Type:   EvJobAvailable,
					At:     now,
					JobID:  replanID,
					TaskID: taskID,
				})
			} else {
				_, err = tx.Exec(
					"UPDATE jobs SET status = 'QUEUED', current_assignment_id = NULL, updated_at = ? WHERE id = ?",
					nowStr, jobID,
				)
				if err != nil {
					return err
				}
				events = append(events, Event{
					Type:   EvJobAvailable,
					At:     now,
					JobID:  jobID,
					TaskID: taskID,
				})
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Kiểm tra nếu agent là lead
	var leaseState string
	var leaseAgent sql.NullString
	_ = s.db.QueryRow("SELECT state, agent_id FROM lead_lease WHERE id = 1").Scan(&leaseState, &leaseAgent)
	if leaseState == "HELD" && leaseAgent.Valid && leaseAgent.String == agentID {
		evs, err := s.revokeLeadLocked(RevokeExhausted, false)
		if err != nil {
			return err
		}
		events = append(events, evs...)
	}

	return nil
}

// ---------- Lead ----------

func (s *Service) Lead() (Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leadLocked()
}

func (s *Service) leadLocked() (Lease, error) {
	var state string
	var agentID sql.NullString
	var epoch int64
	var expVal any
	err := s.db.QueryRow("SELECT state, agent_id, epoch, expires_at FROM lead_lease WHERE id = 1").Scan(&state, &agentID, &epoch, &expVal)
	if err != nil {
		return Lease{}, err
	}

	l := Lease{
		State: state,
		Epoch: epoch,
	}
	if agentID.Valid {
		l.AgentID = agentID.String
	}
	l.ExpiresAt = parseDBTime(expVal)
	return l, nil
}

func (s *Service) Renew(workerID, agentID string, epoch int64) (Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clk.Now()

	// 1. Agent tồn tại
	var agWorker string
	err := s.db.QueryRow("SELECT worker_id FROM agents WHERE id = ?", agentID).Scan(&agWorker)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, ErrNotFound
	}
	if err != nil {
		return Lease{}, err
	}

	// 2. Quyền sở hữu
	if agWorker != workerID {
		return Lease{}, ErrAgentNotOwned
	}

	// 3. Stale epoch / holder
	var state string
	var currentAgent sql.NullString
	var currentEpoch int64
	err = s.db.QueryRow("SELECT state, agent_id, epoch FROM lead_lease WHERE id = 1").Scan(&state, &currentAgent, &currentEpoch)
	if err != nil {
		return Lease{}, err
	}

	if state != "HELD" || !currentAgent.Valid || currentAgent.String != agentID || currentEpoch != epoch {
		return Lease{}, ErrStaleEpoch
	}

	expiresAt := now.Add(s.cfg.LeaseTTL)
	expiresAtStr := s.formatTime(expiresAt)

	_, err = s.db.Exec("UPDATE lead_lease SET expires_at = ? WHERE id = 1", expiresAtStr)
	if err != nil {
		return Lease{}, err
	}

	return Lease{
		State:     "HELD",
		AgentID:   agentID,
		Epoch:     epoch,
		ExpiresAt: &expiresAt,
	}, nil
}

func (s *Service) Release(workerID, agentID string, epoch int64) error {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	// 1. Agent tồn tại
	var agWorker string
	err := s.db.QueryRow("SELECT worker_id FROM agents WHERE id = ?", agentID).Scan(&agWorker)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// 2. Quyền sở hữu
	if agWorker != workerID {
		return ErrAgentNotOwned
	}

	// 3. Stale epoch / holder
	var state string
	var currentAgent sql.NullString
	var currentEpoch int64
	err = s.db.QueryRow("SELECT state, agent_id, epoch FROM lead_lease WHERE id = 1").Scan(&state, &currentAgent, &currentEpoch)
	if err != nil {
		return err
	}

	if state != "HELD" || !currentAgent.Valid || currentAgent.String != agentID || currentEpoch != epoch {
		return ErrStaleEpoch
	}

	evs, err := s.revokeLeadLocked(RevokeReleased, false)
	if err != nil {
		return err
	}
	events = append(events, evs...)
	return nil
}

func (s *Service) LeadNote(workerID, agentID string, epoch int64, taskID, entry string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	// 1. Agent tồn tại
	var agWorker string
	err := s.db.QueryRow("SELECT worker_id FROM agents WHERE id = ?", agentID).Scan(&agWorker)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// 2. Quyền sở hữu
	if agWorker != workerID {
		return ErrAgentNotOwned
	}

	// 3. Stale epoch / holder
	var state string
	var currentAgent sql.NullString
	var currentEpoch int64
	err = s.db.QueryRow("SELECT state, agent_id, epoch FROM lead_lease WHERE id = 1").Scan(&state, &currentAgent, &currentEpoch)
	if err != nil {
		return err
	}

	if state != "HELD" || !currentAgent.Valid || currentAgent.String != agentID || currentEpoch != epoch {
		return ErrStaleEpoch
	}

	_, err = s.db.Exec(
		"INSERT INTO lead_journal (epoch, agent_id, task_id, kind, entry, created_at) VALUES (?, ?, ?, 'NOTE', ?, ?)",
		epoch, agentID, taskID, entry, nowStr,
	)
	return err
}

func (s *Service) AdminAssign(agentID string) (Lease, error) {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	// 1. Kiểm tra agent tồn tại
	var kind, status, workerID string
	err := s.db.QueryRow("SELECT kind, status, worker_id FROM agents WHERE id = ?", agentID).Scan(&kind, &status, &workerID)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, ErrNotFound
	}
	if err != nil {
		return Lease{}, err
	}

	// 2. Kiểm tra worker ONLINE
	var connStatus string
	err = s.db.QueryRow("SELECT conn_status FROM workers WHERE id = ?", workerID).Scan(&connStatus)
	if err != nil {
		return Lease{}, err
	}

	// 3. Đủ điều kiện (BR-06)
	if kind != KindClaude || status != "IDLE" || connStatus != "ONLINE" {
		return Lease{}, ErrAgentNotEligible
	}

	// 4. Thu hồi lead cũ nếu có
	var currentState string
	var currentAgent sql.NullString
	var currentEpoch int64
	err = s.db.QueryRow("SELECT state, agent_id, epoch FROM lead_lease WHERE id = 1").Scan(&currentState, &currentAgent, &currentEpoch)
	if err != nil {
		return Lease{}, err
	}

	if currentState == "HELD" && currentAgent.Valid && currentAgent.String != "" {
		events = append(events, Event{
			Type:    EvLeaseRevoked,
			At:      now,
			AgentID: currentAgent.String,
			Epoch:   currentEpoch,
			Reason:  RevokeAdmin,
		})
		_, _ = s.db.Exec("UPDATE agents SET idle_since = ? WHERE id = ?", nowStr, currentAgent.String)
	}

	newEpoch := currentEpoch + 1
	expiresAt := now.Add(s.cfg.LeaseTTL)
	expiresAtStr := s.formatTime(expiresAt)

	tx, err := s.db.Begin()
	if err != nil {
		return Lease{}, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"UPDATE lead_lease SET state = 'HELD', agent_id = ?, epoch = ?, expires_at = ?, vacant_since = NULL WHERE id = 1",
		agentID, newEpoch, expiresAtStr,
	)
	if err != nil {
		return Lease{}, err
	}

	_, err = tx.Exec(
		"INSERT INTO lead_journal (epoch, agent_id, kind, entry, created_at) VALUES (?, ?, 'HANDOVER', 'Admin assigned lead', ?)",
		newEpoch, agentID, nowStr,
	)
	if err != nil {
		return Lease{}, err
	}

	if err := tx.Commit(); err != nil {
		return Lease{}, err
	}

	if s.isVacant {
		s.isVacant = false
		events = append(events, Event{Type: EvLeadRestored, At: now})
	}

	events = append(events, Event{
		Type:    EvLeaseGranted,
		At:      now,
		AgentID: agentID,
		Epoch:   newEpoch,
		Until:   &expiresAt,
	})

	return Lease{
		State:     "HELD",
		AgentID:   agentID,
		Epoch:     newEpoch,
		ExpiresAt: &expiresAt,
	}, nil
}

func (s *Service) AdminRevoke() (Lease, error) {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	var state string
	err := s.db.QueryRow("SELECT state FROM lead_lease WHERE id = 1").Scan(&state)
	if err != nil {
		return Lease{}, err
	}
	if state == "VACANT" {
		return Lease{}, ErrNoLead
	}

	evs, err := s.revokeLeadLocked(RevokeAdmin, true)
	if err != nil {
		return Lease{}, err
	}
	events = append(events, evs...)

	return s.leadLocked()
}

// ---------- Helpers ----------

func (s *Service) revokeLeadLocked(reason string, banAgent bool) ([]Event, error) {
	now := s.clk.Now()
	nowStr := s.formatTime(now)

	var events []Event

	var state string
	var currentAgent sql.NullString
	var currentEpoch int64
	err := s.db.QueryRow("SELECT state, agent_id, epoch FROM lead_lease WHERE id = 1").Scan(&state, &currentAgent, &currentEpoch)
	if err != nil {
		return nil, err
	}

	if state != "HELD" || !currentAgent.Valid || currentAgent.String == "" {
		return nil, nil
	}

	oldAgent := currentAgent.String
	oldEpoch := currentEpoch

	if banAgent {
		s.adminBannedUntil[oldAgent] = now.Add(s.cfg.AdminRevokeBan)
	}

	events = append(events, Event{
		Type:    EvLeaseRevoked,
		At:      now,
		AgentID: oldAgent,
		Epoch:   oldEpoch,
		Reason:  reason,
	})

	// Cập nhật idle_since của agent cũ thành now
	_, _ = s.db.Exec("UPDATE agents SET idle_since = ? WHERE id = ?", nowStr, oldAgent)

	_, err = s.db.Exec(
		"UPDATE lead_lease SET state = 'VACANT', agent_id = NULL, expires_at = NULL, vacant_since = ? WHERE id = 1",
		nowStr,
	)
	if err != nil {
		return nil, err
	}

	evs, err := s.tryAssignLeaseLocked(oldAgent)
	if err != nil {
		return nil, err
	}
	events = append(events, evs...)

	var newState string
	_ = s.db.QueryRow("SELECT state FROM lead_lease WHERE id = 1").Scan(&newState)
	if newState == "VACANT" {
		s.isVacant = true
		var cdVal any
		_ = s.db.QueryRow(
			"SELECT MIN(cooldown_until) FROM agents WHERE kind = 'CLAUDE' AND status = 'COOLDOWN' AND cooldown_until IS NOT NULL",
		).Scan(&cdVal)
		until := parseDBTime(cdVal)
		events = append(events, Event{
			Type:  EvLeadVacant,
			At:    now,
			Until: until,
		})
	}

	return events, nil
}

func (s *Service) tryAssignLeaseLocked(excludeAgent string) ([]Event, error) {
	now := s.clk.Now()
	nowStr := s.formatTime(now)

	var events []Event

	var state string
	var currentEpoch int64
	err := s.db.QueryRow("SELECT state, epoch FROM lead_lease WHERE id = 1").Scan(&state, &currentEpoch)
	if err != nil {
		return nil, err
	}

	if state == "HELD" {
		return nil, nil
	}

	type candidate struct {
		id              string
		leadPriority    *int
		softMaxJobs     *int
		softWindowHours int
		idleSince       *time.Time
		remaining       int64
	}

	rows, err := s.db.Query(
		`SELECT a.id, a.lead_priority, a.soft_max_jobs, a.soft_window_hours, a.idle_since
		 FROM agents a
		 JOIN workers w ON a.worker_id = w.id
		 WHERE a.kind = 'CLAUDE' AND a.status = 'IDLE' AND w.conn_status = 'ONLINE'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []candidate
	for rows.Next() {
		var c candidate
		var lp sql.NullInt64
		var sm sql.NullInt64
		var idleVal any
		if err := rows.Scan(&c.id, &lp, &sm, &c.softWindowHours, &idleVal); err != nil {
			return nil, err
		}

		if excludeAgent != "" && c.id == excludeAgent {
			continue
		}

		if bannedUntil, ok := s.adminBannedUntil[c.id]; ok && now.Before(bannedUntil) {
			continue
		}

		if lp.Valid {
			v := int(lp.Int64)
			c.leadPriority = &v
		}
		if sm.Valid {
			v := int(sm.Int64)
			c.softMaxJobs = &v
		}
		c.idleSince = parseDBTime(idleVal)

		if c.softMaxJobs == nil {
			c.remaining = 1<<62 - 1
		} else {
			windowStart := now.Add(-time.Duration(c.softWindowHours) * time.Hour)
			var count int
			err := s.db.QueryRow(
				"SELECT COUNT(*) FROM assignments WHERE agent_id = ? AND started_at >= ?",
				c.id, s.formatTime(windowStart),
			).Scan(&count)
			if err != nil {
				return nil, err
			}
			c.remaining = int64(*c.softMaxJobs - count)
		}

		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, nil
	}

	// BR-06: Sắp xếp ứng viên
	sort.SliceStable(candidates, func(i, j int) bool {
		c1, c2 := candidates[i], candidates[j]
		if (c1.leadPriority != nil) != (c2.leadPriority != nil) {
			return c1.leadPriority != nil
		}
		if c1.leadPriority != nil && c2.leadPriority != nil && *c1.leadPriority != *c2.leadPriority {
			return *c1.leadPriority < *c2.leadPriority
		}
		if c1.remaining != c2.remaining {
			return c1.remaining > c2.remaining
		}
		if (c1.idleSince != nil) != (c2.idleSince != nil) {
			return c1.idleSince != nil
		}
		if c1.idleSince != nil && c2.idleSince != nil && !c1.idleSince.Equal(*c2.idleSince) {
			return c1.idleSince.Before(*c2.idleSince)
		}
		return c1.id < c2.id
	})

	winner := candidates[0]
	newEpoch := currentEpoch + 1
	expiresAt := now.Add(s.cfg.LeaseTTL)
	expiresAtStr := s.formatTime(expiresAt)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"UPDATE lead_lease SET state = 'HELD', agent_id = ?, epoch = ?, expires_at = ?, vacant_since = NULL WHERE id = 1",
		winner.id, newEpoch, expiresAtStr,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(
		"INSERT INTO lead_journal (epoch, agent_id, kind, entry, created_at) VALUES (?, ?, 'HANDOVER', 'Lease granted', ?)",
		newEpoch, winner.id, nowStr,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if s.isVacant {
		s.isVacant = false
		events = append(events, Event{
			Type: EvLeadRestored,
			At:   now,
		})
	}
	events = append(events, Event{
		Type:    EvLeaseGranted,
		At:      now,
		AgentID: winner.id,
		Epoch:   newEpoch,
		Until:   &expiresAt,
	})

	return events, nil
}
