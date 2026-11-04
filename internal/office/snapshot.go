package office

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/namth/agenthub/internal/dispatcher"
)

// AgentJob chứa thông tin công việc đang làm của agent.
type AgentJob struct {
	Task any    `json:"task"`
	Kind string `json:"kind"`
}

// AgentSnapshot chứa ảnh chụp trạng thái của một agent.
type AgentSnapshot struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
	AuthStatus    string    `json:"auth_status"`
	CooldownUntil *string   `json:"cooldown_until"`
	Job           *AgentJob `json:"job"`
}

// WorkerSnapshot chứa ảnh chụp trạng thái của một worker.
type WorkerSnapshot struct {
	ID         string          `json:"id"`
	ConnStatus string          `json:"conn_status"`
	GhAuth     string          `json:"gh_auth"`
	Owner      *string         `json:"owner"`
	Agents     []AgentSnapshot `json:"agents"`
}

// BuildSnapshot dựng ảnh chụp trạng thái văn phòng cho client SSE (FEAT-02 bản 2).
func BuildSnapshot(db *sql.DB, lead dispatcher.Lease, p *Presence) (map[string]any, error) {
	var agentID *string
	if lead.State != "VACANT" && lead.AgentID != "" {
		agentID = &lead.AgentID
	}
	leadData := map[string]any{
		"state":    lead.State,
		"agent_id": agentID,
		"epoch":    lead.Epoch,
	}

	rows, err := db.Query(`
		SELECT w.id, COALESCE(w.conn_status, 'OFFLINE'), COALESCE(w.gh_auth, 'UNKNOWN'), u.username
		FROM workers w
		LEFT JOIN users u ON w.owner_user_id = u.id
		ORDER BY w.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	workerList := make([]*WorkerSnapshot, 0)
	workerMap := make(map[string]*WorkerSnapshot)
	for rows.Next() {
		var id, connStatus, ghAuth string
		var ownerUsername sql.NullString
		if err := rows.Scan(&id, &connStatus, &ghAuth, &ownerUsername); err != nil {
			return nil, err
		}
		if ghAuth == "" {
			ghAuth = "UNKNOWN"
		}
		var ownerPtr *string
		if ownerUsername.Valid && ownerUsername.String != "" {
			s := ownerUsername.String
			ownerPtr = &s
		}
		ws := &WorkerSnapshot{
			ID:         id,
			ConnStatus: connStatus,
			GhAuth:     ghAuth,
			Owner:      ownerPtr,
			Agents:     make([]AgentSnapshot, 0),
		}
		workerList = append(workerList, ws)
		workerMap[id] = ws
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	activeJobs := make(map[string]*AgentJob)
	asgRows, err := db.Query(`
		SELECT a.agent_id, j.task_id, j.kind
		FROM assignments a
		JOIN jobs j ON a.job_id = j.id
		WHERE a.status = 'ACTIVE'
		ORDER BY a.started_at DESC
	`)
	if err == nil {
		defer asgRows.Close()
		for asgRows.Next() {
			var agentID, taskID, kind string
			if err := asgRows.Scan(&agentID, &taskID, &kind); err == nil {
				if _, ok := activeJobs[agentID]; !ok {
					var taskVal any = taskID
					if n, err := strconv.Atoi(taskID); err == nil {
						taskVal = n
					}
					activeJobs[agentID] = &AgentJob{
						Task: taskVal,
						Kind: kind,
					}
				}
			}
		}
	}

	agentRows, err := db.Query(`
		SELECT id, worker_id, kind, status, COALESCE(auth_status, 'UNKNOWN'), cooldown_until
		FROM agents
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer agentRows.Close()

	for agentRows.Next() {
		var id, workerID, kind, status, authStatus string
		var cu sql.NullString
		if err := agentRows.Scan(&id, &workerID, &kind, &status, &authStatus, &cu); err != nil {
			return nil, err
		}
		if authStatus == "" {
			authStatus = "UNKNOWN"
		}
		var cooldownUntil *string
		if cu.Valid && cu.String != "" {
			if t, ok := parseDBTime(cu.String); ok {
				v := t.UTC().Format(time.RFC3339)
				cooldownUntil = &v
			}
		}
		as := AgentSnapshot{
			ID:            id,
			Kind:          kind,
			Status:        status,
			AuthStatus:    authStatus,
			CooldownUntil: cooldownUntil,
			Job:           activeJobs[id],
		}
		if ws, ok := workerMap[workerID]; ok {
			ws.Agents = append(ws.Agents, as)
		}
	}
	if err := agentRows.Err(); err != nil {
		return nil, err
	}

	users := make([]UserView, 0)
	if p != nil {
		u, err := p.Users()
		if err != nil {
			return nil, err
		}
		if u != nil {
			users = u
		}
	}

	workersResult := make([]WorkerSnapshot, len(workerList))
	for i, w := range workerList {
		workersResult[i] = *w
	}

	return map[string]any{
		"lead":    leadData,
		"workers": workersResult,
		"users":   users,
	}, nil
}
