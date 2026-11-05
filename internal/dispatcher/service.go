package dispatcher

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
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

var (
	reSecretPrefix = regexp.MustCompile(`(ghp_|gho_|sk-)\S*`)
	reLongSecret   = regexp.MustCompile(`\S{32,}`)
	reAgentName    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
)

func sanitizeDetail(s string) string {
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

// Service cài đặt interface dispatcher.API và dispatcher.AuthAPI.
var _ API = (*Service)(nil)
var _ AuthAPI = (*Service)(nil)

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

// Now trả giờ theo Clock của dispatcher (để API tính cửa sổ thời gian nhất quán với dữ liệu).
func (s *Service) Now() time.Time { return s.clk.Now() }

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
	hellos := make([]AgentHello, len(kinds))
	for i, k := range kinds {
		hellos[i] = AgentHello{Kind: k}
	}
	return s.ConnectAuth(workerID, "", hellos)
}

func (s *Service) ConnectAuth(workerID, ghAuth string, agents []AgentHello) ([]Agent, error) {
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

	// 2. Chuyển worker sang ONLINE, cập nhật gh_auth nếu được chỉ định
	if ghAuth != "" {
		_, err = tx.Exec(
			"UPDATE workers SET conn_status = 'ONLINE', offline_since = NULL, last_heartbeat = ?, gh_auth = ? WHERE id = ?",
			nowStr, ghAuth, workerID,
		)
	} else {
		_, err = tx.Exec(
			"UPDATE workers SET conn_status = 'ONLINE', offline_since = NULL, last_heartbeat = ? WHERE id = ?",
			nowStr, workerID,
		)
	}
	if err != nil {
		return nil, err
	}

	// 3. Đăng ký/cập nhật các agent
	for _, h := range agents {
		if h.Kind == "" {
			continue
		}
		name := h.Name
		if name == "" {
			name = strings.ToLower(h.Kind)
		}
		if !reAgentName.MatchString(name) {
			continue
		}
		agentID := workerID + "/" + name

		// Kiểm tra tombstone & ever_ok hiện tại trong DB
		var removedAt any
		var existingEverOK int
		rowErr := tx.QueryRow("SELECT removed_at, ever_ok FROM agents WHERE id = ?", agentID).Scan(&removedAt, &existingEverOK)
		if rowErr == nil && removedAt != nil {
			// Đã xoá -> bỏ qua (không hồi sinh, không xoá tombstone)
			continue
		}

		everOK := 1
		if rowErr != nil && errors.Is(rowErr, sql.ErrNoRows) {
			// Hàng mới
			if h.EverOK != nil {
				if *h.EverOK || h.AuthStatus == AuthOK {
					everOK = 1
				} else {
					everOK = 0
				}
			} else {
				if h.AuthStatus == AuthOK {
					everOK = 1
				} else {
					everOK = 1
				}
			}
		} else {
			// Hàng đã có: không bao giờ hạ 1 -> 0
			if existingEverOK == 1 {
				everOK = 1
			} else {
				if (h.EverOK != nil && *h.EverOK) || h.AuthStatus == AuthOK {
					everOK = 1
				} else {
					everOK = 0
				}
			}
		}

		cleanDetail := sanitizeDetail(h.AuthDetail)
		var detailVal any = cleanDetail
		if cleanDetail == "" {
			detailVal = nil
		}

		leadEligible := 1
		if strings.EqualFold(h.Kind, KindAntigravity) {
			leadEligible = 0
		}

		if h.AuthStatus == "" {
			_, err = tx.Exec(
				`INSERT INTO agents (id, worker_id, kind, name, status, idle_since, created_at, lead_eligible, ever_ok)
				 VALUES (?, ?, ?, ?, 'IDLE', ?, ?, ?, ?)
				 ON CONFLICT(id) DO UPDATE SET
					idle_since = COALESCE(agents.idle_since, excluded.idle_since),
					ever_ok = CASE WHEN agents.ever_ok = 1 THEN 1 ELSE excluded.ever_ok END`,
				agentID, workerID, h.Kind, name, nowStr, nowStr, leadEligible, everOK,
			)
		} else {
			_, err = tx.Exec(
				`INSERT INTO agents (id, worker_id, kind, name, status, idle_since, created_at, auth_status, auth_detail, auth_checked_at, lead_eligible, ever_ok)
				 VALUES (?, ?, ?, ?, 'IDLE', ?, ?, ?, ?, ?, ?, ?)
				 ON CONFLICT(id) DO UPDATE SET
					idle_since = COALESCE(agents.idle_since, excluded.idle_since),
					auth_status = excluded.auth_status,
					auth_detail = excluded.auth_detail,
					auth_checked_at = excluded.auth_checked_at,
					ever_ok = CASE WHEN agents.ever_ok = 1 THEN 1 ELSE excluded.ever_ok END`,
				agentID, workerID, h.Kind, name, nowStr, nowStr, h.AuthStatus, detailVal, nowStr, leadEligible, everOK,
			)
		}
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
		"SELECT id, name, worker_id, kind, status, cooldown_until, auth_status FROM agents WHERE worker_id = ? AND ever_ok = 1 AND removed_at IS NULL ORDER BY id",
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
		var nameVal sql.NullString
		if err := rows.Scan(&a.ID, &nameVal, &a.WorkerID, &a.Kind, &a.Status, &cdVal, &a.AuthStatus); err != nil {
			return nil, err
		}
		if nameVal.Valid {
			a.Name = nameVal.String
		}
		a.CooldownUntil = parseDBTime(cdVal)
		result = append(result, a)
	}
	return result, rows.Err()
}

func (s *Service) SetAuth(workerID, agentID, status, detail string) error {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	// 1. Kiểm tra agent tồn tại và quyền sở hữu
	var agWorker, prevAuth string
	err := s.db.QueryRow("SELECT worker_id, auth_status FROM agents WHERE id = ?", agentID).Scan(&agWorker, &prevAuth)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if agWorker != workerID {
		return ErrAgentNotOwned
	}

	// 2. Validate status
	if status != AuthOK && status != AuthNeeds && status != AuthUnknown {
		return ErrInvalidAuth
	}

	cleanDetail := sanitizeDetail(detail)
	var detailVal any = cleanDetail
	if cleanDetail == "" {
		detailVal = nil
	}

	_, err = s.db.Exec(
		"UPDATE agents SET auth_status = ?, auth_detail = ?, auth_checked_at = ? WHERE id = ?",
		status, detailVal, nowStr, agentID,
	)
	if err != nil {
		return err
	}

	if status != prevAuth {
		events = append(events, Event{
			Type:           EvAgentAuth,
			At:             now,
			WorkerID:       workerID,
			AgentID:        agentID,
			AuthStatus:     status,
			PrevAuthStatus: prevAuth,
			Detail:         cleanDetail,
		})
	}

	if status == AuthNeeds {
		var leaseState string
		var leaseAgent sql.NullString
		_ = s.db.QueryRow("SELECT state, agent_id FROM lead_lease WHERE id = 1").Scan(&leaseState, &leaseAgent)
		if leaseState == "HELD" && leaseAgent.Valid && leaseAgent.String == agentID {
			evs, err := s.revokeLeadLocked(RevokeAuth, false)
			if err != nil {
				return err
			}
			events = append(events, evs...)
		}
	} else if status == AuthOK || status == AuthUnknown {
		var leaseState string
		_ = s.db.QueryRow("SELECT state FROM lead_lease WHERE id = 1").Scan(&leaseState)
		if leaseState == "VACANT" {
			evs, err := s.tryAssignLeaseLocked("")
			if err != nil {
				return err
			}
			events = append(events, evs...)
		}
	}

	return nil
}

func (s *Service) SetGhAuth(workerID, status, detail string) error {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()

	// 1. Kiểm tra worker tồn tại
	var prevGh string
	err := s.db.QueryRow("SELECT gh_auth FROM workers WHERE id = ?", workerID).Scan(&prevGh)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	// 2. Validate status
	if status != AuthOK && status != AuthNeeds && status != AuthUnknown {
		return ErrInvalidAuth
	}

	cleanDetail := sanitizeDetail(detail)
	_, err = s.db.Exec("UPDATE workers SET gh_auth = ? WHERE id = ?", status, workerID)
	if err != nil {
		return err
	}

	if status != prevGh {
		events = append(events, Event{
			Type:           EvWorkerAuth,
			At:             now,
			WorkerID:       workerID,
			AuthStatus:     status,
			PrevAuthStatus: prevGh,
			Detail:         cleanDetail,
		})
	}

	if status == AuthNeeds {
		var leaseState string
		var leaseAgent sql.NullString
		_ = s.db.QueryRow("SELECT state, agent_id FROM lead_lease WHERE id = 1").Scan(&leaseState, &leaseAgent)
		if leaseState == "HELD" && leaseAgent.Valid && leaseAgent.String != "" {
			var leadWorker string
			_ = s.db.QueryRow("SELECT worker_id FROM agents WHERE id = ?", leaseAgent.String).Scan(&leadWorker)
			if leadWorker == workerID {
				evs, err := s.revokeLeadLocked(RevokeAuth, false)
				if err != nil {
					return err
				}
				events = append(events, evs...)
			}
		}
	} else if status == AuthOK || status == AuthUnknown {
		var leaseState string
		_ = s.db.QueryRow("SELECT state FROM lead_lease WHERE id = 1").Scan(&leaseState)
		if leaseState == "VACANT" {
			evs, err := s.tryAssignLeaseLocked("")
			if err != nil {
				return err
			}
			events = append(events, evs...)
		}
	}

	return nil
}

func (s *Service) AuthRequired(workerID, agentID, assignmentID, detail, handoffNote, wipCommit string) error {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	// 1. Kiểm tra agent tồn tại và quyền sở hữu
	var agWorker, prevAuth string
	err := s.db.QueryRow("SELECT worker_id, auth_status FROM agents WHERE id = ?", agentID).Scan(&agWorker, &prevAuth)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if agWorker != workerID {
		return ErrAgentNotOwned
	}

	// 2. Kiểm tra assignment
	var asgStatus, jobID string
	var asgAttempt int
	var asgAgentID, asgWorker string
	var currentAsgID sql.NullString
	var taskID string
	err = s.db.QueryRow(`
		SELECT a.status, a.job_id, a.attempt, a.agent_id, ag.worker_id, j.current_assignment_id, j.task_id
		FROM assignments a
		JOIN agents ag ON a.agent_id = ag.id
		JOIN jobs j ON a.job_id = j.id
		WHERE a.id = ?`, assignmentID).Scan(&asgStatus, &jobID, &asgAttempt, &asgAgentID, &asgWorker, &currentAsgID, &taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if asgWorker != workerID || asgAgentID != agentID {
		return ErrAgentNotOwned
	}
	if asgStatus != "ACTIVE" || !currentAsgID.Valid || currentAsgID.String != assignmentID {
		return ErrStaleAssignment
	}

	cleanDetail := sanitizeDetail(detail)
	var detailVal any = cleanDetail
	if cleanDetail == "" {
		detailVal = nil
	}

	var note *string
	if handoffNote != "" {
		note = &handoffNote
	}
	var wip *string
	if wipCommit != "" {
		wip = &wipCommit
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"UPDATE agents SET auth_status = 'NEEDS_AUTH', auth_detail = ?, auth_checked_at = ?, status = 'IDLE', idle_since = ? WHERE id = ?",
		detailVal, nowStr, nowStr, agentID,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		"UPDATE assignments SET status = 'RECLAIMED', ended_at = ?, handoff_note = ?, wip_commit = ? WHERE id = ?",
		nowStr, note, wip, assignmentID,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		"UPDATE jobs SET status = 'QUEUED', attempt = attempt - 1, current_assignment_id = NULL, updated_at = ? WHERE id = ?",
		nowStr, jobID,
	)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	if prevAuth != AuthNeeds {
		events = append(events, Event{
			Type:           EvAgentAuth,
			At:             now,
			WorkerID:       workerID,
			AgentID:        agentID,
			AuthStatus:     AuthNeeds,
			PrevAuthStatus: prevAuth,
			Detail:         cleanDetail,
		})
	}

	var leaseState string
	var leaseAgent sql.NullString
	_ = s.db.QueryRow("SELECT state, agent_id FROM lead_lease WHERE id = 1").Scan(&leaseState, &leaseAgent)
	if leaseState == "HELD" && leaseAgent.Valid && leaseAgent.String == agentID {
		evs, err := s.revokeLeadLocked(RevokeAuth, false)
		if err != nil {
			return err
		}
		events = append(events, evs...)
	}

	events = append(events, Event{
		Type:   EvJobAvailable,
		At:     now,
		JobID:  jobID,
		TaskID: taskID,
	})

	return nil
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

	// 2b. Assignment ACTIVE trên máy partner mà chủ máy mất tư cách thành viên dự án (DEC-37, BR-58)
	rows, err = s.db.Query(
		`SELECT a.id, a.job_id, a.agent_id, a.attempt, j.task_id, j.project_id
		 FROM assignments a
		 JOIN agents ag ON a.agent_id = ag.id
		 JOIN workers w ON ag.worker_id = w.id
		 JOIN users u ON w.owner_user_id = u.id
		 JOIN jobs j ON a.job_id = j.id
		 WHERE a.status = 'ACTIVE'
		   AND w.owner_user_id IS NOT NULL
		   AND u.role <> 'ADMIN'
		   AND NOT EXISTS (
		       SELECT 1 FROM project_members pm
		       WHERE pm.project_id = j.project_id AND pm.user_id = w.owner_user_id
		   )`,
	)
	if err != nil {
		return err
	}
	var toReclaimMembership []reclaimInfo
	for rows.Next() {
		var ri reclaimInfo
		if err := rows.Scan(&ri.asgID, &ri.jobID, &ri.agentID, &ri.attempt, &ri.taskID, &ri.projectID); err != nil {
			rows.Close()
			return err
		}
		toReclaimMembership = append(toReclaimMembership, ri)
	}
	rows.Close()

	for _, ri := range toReclaimMembership {
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

		_, err = tx.Exec("UPDATE jobs SET status = 'QUEUED', current_assignment_id = NULL, updated_at = ? WHERE id = ?", nowStr, ri.jobID)
		if err != nil {
			tx.Rollback()
			return err
		}

		if err := tx.Commit(); err != nil {
			return err
		}

		events = append(events, Event{
			Type:         EvJobReclaimed,
			At:           now,
			JobID:        ri.jobID,
			AssignmentID: ri.asgID,
		})
		events = append(events, Event{
			Type:         EvAbortTask,
			At:           now,
			AgentID:      ri.agentID,
			AssignmentID: ri.asgID,
			Reason:       "membership_lost",
		})
		events = append(events, Event{
			Type:   EvJobAvailable,
			At:     now,
			JobID:  ri.jobID,
			TaskID: ri.taskID,
		})
	}

	// 3. Máy của lead tạm dừng -> thu hồi RELEASED (BR-32)
	var leaseState string
	var leaseAgent sql.NullString
	_ = s.db.QueryRow("SELECT state, agent_id FROM lead_lease WHERE id = 1").Scan(&leaseState, &leaseAgent)
	if leaseState == "HELD" && leaseAgent.Valid && leaseAgent.String != "" {
		var isPaused int
		err := s.db.QueryRow(`
			SELECT w.paused
			FROM agents a
			JOIN workers w ON a.worker_id = w.id
			WHERE a.id = ?`, leaseAgent.String).Scan(&isPaused)
		if err == nil && isPaused == 1 {
			evs, err := s.revokeLeadLocked(RevokeReleased, false)
			if err != nil {
				return err
			}
			events = append(events, evs...)
		}
	}

	// 4. Lease hết hạn -> thu hồi EXPIRED
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
	var agWorker, kind, status, authStatus, connStatus, ghAuth string
	var paused, everOK int
	var removedAt any
	var ownerUserID, userRole sql.NullString
	err := s.db.QueryRow(`
		SELECT a.worker_id, a.kind, a.status, w.paused, a.auth_status, w.conn_status, w.gh_auth,
		       w.owner_user_id, u.role, a.ever_ok, a.removed_at
		FROM agents a
		JOIN workers w ON a.worker_id = w.id
		LEFT JOIN users u ON w.owner_user_id = u.id
		WHERE a.id = ?`, agentID).Scan(&agWorker, &kind, &status, &paused, &authStatus, &connStatus, &ghAuth, &ownerUserID, &userRole, &everOK, &removedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if removedAt != nil || everOK == 0 {
		return nil, nil
	}

	// 2. Kiểm tra quyền sở hữu
	if agWorker != workerID {
		return nil, ErrAgentNotOwned
	}

	// 2b. Máy tạm dừng không nhận việc (BR-32)
	if paused == 1 {
		return nil, nil
	}

	// BR-37: Agent chỉ được nhận job khi status = 'IDLE', auth_status <> 'NEEDS_AUTH',
	// máy ONLINE, paused = 0, gh_auth <> 'NEEDS_AUTH'. UNKNOWN vẫn được nhận việc.
	if connStatus != "ONLINE" || status != "IDLE" || authStatus == AuthNeeds || ghAuth == AuthNeeds {
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

	isPartner := ownerUserID.Valid && ownerUserID.String != "" && userRole.Valid && userRole.String != "ADMIN"
	partnerOwner := ""
	if isPartner {
		partnerOwner = ownerUserID.String
	}

	// 5. Kiểm tra lead
	var leaseState string
	var leaseAgent sql.NullString
	_ = s.db.QueryRow("SELECT state, agent_id FROM lead_lease WHERE id = 1").Scan(&leaseState, &leaseAgent)
	isLead := (leaseState == "HELD" && leaseAgent.Valid && leaseAgent.String == agentID)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 6. Tìm job QUEUED (DEC-16, BR-25, BR-29)
	var jobID, taskID, jobKind string
	var prevAttempt int
	var branch sql.NullString
	found := false

	// 1) Agent là lead -> job lead_only trước (mọi dự án, bất kể required_agent, DEC-29)
	if isLead {
		leadQuery := `
			SELECT j.id, j.task_id, j.kind, j.attempt, j.branch_name
			FROM jobs j
			WHERE j.status = 'QUEUED' AND j.lead_only = 1
			  AND NOT (
			      j.kind = 'QA_REVIEW'
			      AND EXISTS (
			          SELECT 1
			          FROM assignments a_code
			          JOIN jobs j_code ON a_code.job_id = j_code.id
			          WHERE j_code.task_id = j.task_id
			            AND j_code.kind = 'CODE'
			            AND a_code.agent_id = ?
			            AND a_code.status = 'SUBMITTED'
			      )
			  )`
		leadArgs := []any{agentID}
		if isPartner {
			leadQuery += ` AND j.project_id IN (SELECT project_id FROM project_members WHERE user_id = ?)`
			leadArgs = append(leadArgs, partnerOwner)
		}
		leadQuery += `
			ORDER BY j.priority ASC, j.created_at ASC, j.id ASC
			LIMIT 1`
		err = tx.QueryRow(leadQuery, leadArgs...).Scan(&jobID, &taskID, &jobKind, &prevAttempt, &branch)
		if err == nil {
			found = true
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	// 2) Còn lại: job QUEUED đúng required_agent, lead_only = 0, thuộc dự án status = 'ACTIVE',
	// chưa đạt max_parallel, và nằm trong agent_projects của agent (agent không có dòng nào = mọi dự án).
	if !found {
		normQuery := `
			SELECT j.id, j.task_id, j.kind, j.attempt, j.branch_name
			FROM jobs j
			JOIN projects p ON j.project_id = p.id
			WHERE j.status = 'QUEUED'
			  AND j.required_agent = ?
			  AND j.lead_only = 0
			  AND p.status = 'ACTIVE'
			  AND (p.max_parallel IS NULL OR (
			      SELECT COUNT(*) FROM jobs r WHERE r.project_id = j.project_id AND r.status = 'RUNNING'
			  ) < p.max_parallel)
			  AND (
			      NOT EXISTS (SELECT 1 FROM agent_projects ap WHERE ap.agent_id = ?)
			      OR j.project_id IN (SELECT ap2.project_id FROM agent_projects ap2 WHERE ap2.agent_id = ?)
			  )`
		normArgs := []any{kind, agentID, agentID}
		if isPartner {
			normQuery += ` AND j.project_id IN (SELECT project_id FROM project_members WHERE user_id = ?)`
			normArgs = append(normArgs, partnerOwner)
		}
		normQuery += `
			  AND NOT (
			      j.kind = 'QA_REVIEW'
			      AND EXISTS (
			          SELECT 1
			          FROM assignments a_code
			          JOIN jobs j_code ON a_code.job_id = j_code.id
			          WHERE j_code.task_id = j.task_id
			            AND j_code.kind = 'CODE'
			            AND a_code.agent_id = ?
			            AND a_code.status = 'SUBMITTED'
			      )
			  )
			ORDER BY
			  j.priority ASC,
			  (CAST((SELECT COUNT(*) FROM jobs r WHERE r.project_id = j.project_id AND r.status = 'RUNNING') AS REAL) / p.weight) ASC,
			  j.created_at ASC,
			  j.id ASC
			LIMIT 1`
		normArgs = append(normArgs, agentID)

		err = tx.QueryRow(normQuery, normArgs...).Scan(&jobID, &taskID, &jobKind, &prevAttempt, &branch)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
	}

	// Ghi chú bàn giao gần nhất của job
	var handoffNote sql.NullString
	_ = tx.QueryRow(
		"SELECT handoff_note FROM assignments WHERE job_id = ? AND handoff_note IS NOT NULL AND handoff_note != '' ORDER BY started_at DESC LIMIT 1",
		jobID,
	).Scan(&handoffNote)

	newAttempt := prevAttempt + 1
	asgID := newID("asg")

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
	var kind, status, workerID, authStatus string
	var leadEligible, everOK int
	var removedAt any
	err := s.db.QueryRow("SELECT kind, status, worker_id, lead_eligible, auth_status, ever_ok, removed_at FROM agents WHERE id = ?", agentID).Scan(&kind, &status, &workerID, &leadEligible, &authStatus, &everOK, &removedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Lease{}, ErrNotFound
	}
	if err != nil {
		return Lease{}, err
	}

	// 2. Kiểm tra worker ONLINE & không tạm dừng & gh_auth
	var connStatus, ghAuth string
	var paused int
	err = s.db.QueryRow("SELECT conn_status, paused, gh_auth FROM workers WHERE id = ?", workerID).Scan(&connStatus, &paused, &ghAuth)
	if err != nil {
		return Lease{}, err
	}

	// 3. Đủ điều kiện (BR-06, BR-32, BR-33, BR-37, DEC-29: bỏ điều kiện kind = CLAUDE)
	if status != "IDLE" || connStatus != "ONLINE" || paused != 0 || leadEligible != 1 || authStatus == AuthNeeds || ghAuth == AuthNeeds || everOK != 1 || removedAt != nil {
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
			"SELECT MIN(cooldown_until) FROM agents WHERE lead_eligible = 1 AND status = 'COOLDOWN' AND cooldown_until IS NOT NULL AND ever_ok = 1 AND removed_at IS NULL",
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
		 WHERE a.status = 'IDLE' AND w.conn_status = 'ONLINE'
		   AND w.paused = 0 AND a.lead_eligible = 1
		   AND a.auth_status <> 'NEEDS_AUTH' AND w.gh_auth <> 'NEEDS_AUTH'
		   AND a.ever_ok = 1 AND a.removed_at IS NULL`,
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

func (s *Service) RemoveAgent(agentID string) error {
	var events []Event
	s.mu.Lock()
	defer func() {
		s.mu.Unlock()
		s.notify(events)
	}()

	now := s.clk.Now()
	nowStr := s.formatTime(now)

	var workerID string
	var removedAt any
	err := s.db.QueryRow("SELECT worker_id, removed_at FROM agents WHERE id = ?", agentID).Scan(&workerID, &removedAt)
	if errors.Is(err, sql.ErrNoRows) || removedAt != nil {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	var activeCount int
	err = s.db.QueryRow("SELECT COUNT(*) FROM assignments WHERE agent_id = ? AND status = 'ACTIVE'", agentID).Scan(&activeCount)
	if err != nil {
		return err
	}
	if activeCount > 0 {
		return ErrConflict
	}

	var leaseState string
	var leaseAgent sql.NullString
	_ = s.db.QueryRow("SELECT state, agent_id FROM lead_lease WHERE id = 1").Scan(&leaseState, &leaseAgent)
	if leaseState == "HELD" && leaseAgent.Valid && leaseAgent.String == agentID {
		evs, err := s.revokeLeadLocked(RevokeAdmin, false)
		if err != nil {
			return err
		}
		events = append(events, evs...)
	}

	_, err = s.db.Exec("UPDATE agents SET removed_at = ?, status = 'IDLE' WHERE id = ?", nowStr, agentID)
	if err != nil {
		return err
	}

	events = append(events, Event{
		Type:     EvAgentRemoved,
		At:       now,
		WorkerID: workerID,
		AgentID:  agentID,
	})

	return nil
}

func (s *Service) Tombstoned(workerID string, ids []string) []string {
	var out []string
	for _, id := range ids {
		var dummy int
		err := s.db.QueryRow("SELECT 1 FROM agents WHERE id = ? AND worker_id = ? AND removed_at IS NOT NULL", id, workerID).Scan(&dummy)
		if err == nil {
			out = append(out, id)
		}
	}
	return out
}
