package gateway

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/namth/agenthub/internal/dispatcher"
)

type clientConn struct {
	hub        *Hub
	workerID   string
	ws         *websocket.Conn
	sendCh     chan []byte
	closeOnce  sync.Once
	closed     chan struct{}
	replaced   bool
	handshaken bool
}

func newClientConn(hub *Hub, workerID string, ws *websocket.Conn) *clientConn {
	return &clientConn{
		hub:      hub,
		workerID: workerID,
		ws:       ws,
		sendCh:   make(chan []byte, 256),
		closed:   make(chan struct{}),
	}
}

func (c *clientConn) close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.ws.Close()
	})
}

func (c *clientConn) enqueue(data []byte) {
	select {
	case <-c.closed:
		return
	default:
	}

	select {
	case c.sendCh <- data:
	case <-c.closed:
		return
	default:
		c.close()
	}
}

func (c *clientConn) send(data []byte) {
	select {
	case <-c.closed:
		return
	case c.sendCh <- data:
	default:
		c.close()
	}
}

func (c *clientConn) writePump() {
	for {
		select {
		case <-c.closed:
			return
		case msg := <-c.sendCh:
			c.ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
				c.close()
				return
			}
		}
	}
}

type incomingEnvelope struct {
	Event        string          `json:"event"`
	Token        string          `json:"token,omitempty"`
	Data         json.RawMessage `json:"data,omitempty"`
	Timestamp    any             `json:"timestamp,omitempty"`
	AgentID      string          `json:"agent_id,omitempty"`
	AssignmentID *string         `json:"assignment_id,omitempty"`
	Epoch        *int64          `json:"epoch,omitempty"`
	Action       string          `json:"action,omitempty"`
	ResetsAt     *string         `json:"resets_at,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	HandoffNote  *string         `json:"handoff_note,omitempty"`
	WipCommit    *string         `json:"wip_commit,omitempty"`
	Chunk        string          `json:"chunk,omitempty"`
	PrUrl        string          `json:"pr_url,omitempty"`
	Branch       string          `json:"branch,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
}

type errorEnvelope struct {
	Event string         `json:"event"`
	Code  string         `json:"code"`
	Ref   map[string]any `json:"ref,omitempty"`
}

func (c *clientConn) sendError(code string, ref map[string]any) {
	env := errorEnvelope{
		Event: "ERROR",
		Code:  code,
	}
	if len(ref) > 0 {
		env.Ref = ref
	}
	b, _ := json.Marshal(env)
	c.send(b)
}

func (c *clientConn) readPump() {
	defer func() {
		c.close()
		c.hub.removeClient(c)
	}()

	c.ws.SetReadLimit(1024 * 1024)

	for {
		_, raw, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
		c.handleMessage(raw)
	}
}

func (c *clientConn) handleMessage(raw []byte) {
	var env incomingEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.Event == "" {
		c.sendError("BAD_REQUEST", nil)
		return
	}

	if !c.handshaken {
		if env.Event != "HANDSHAKE" {
			c.sendError("NOT_HANDSHAKEN", nil)
			return
		}
		c.handleHandshake(env)
		return
	}

	switch env.Event {
	case "PING":
		c.handlePing(env)
	case "REQUEST_TASK":
		c.handleRequestTask(env)
	case "LOG_CHUNK":
		c.handleLogChunk(env)
	case "TASK_SUBMITTED":
		c.handleTaskSubmitted(env)
	case "AGENT_EXHAUSTED":
		c.handleAgentExhausted(env)
	case "LEASE_RENEW":
		c.handleLeaseRenew(env)
	case "LEAD_RELEASE":
		c.handleLeadRelease(env)
	case "LEAD_ACTION":
		c.handleLeadAction(env)
	default:
		c.sendError("BAD_REQUEST", nil)
	}
}

type handshakeData struct {
	OS     string `json:"os"`
	Agents []struct {
		Kind string `json:"kind"`
	} `json:"agents"`
}

func (c *clientConn) handleHandshake(env incomingEnvelope) {
	var d handshakeData
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &d)
	}

	kinds := make([]string, 0, len(d.Agents))
	for _, a := range d.Agents {
		if a.Kind != "" {
			kinds = append(kinds, a.Kind)
		}
	}

	agents, err := c.hub.api.Connect(c.workerID, kinds)
	if err != nil {
		c.handleDomainError(err, env)
		return
	}

	c.handshaken = true

	type agentItem struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	respAgents := make([]agentItem, 0, len(agents))
	for _, a := range agents {
		respAgents = append(respAgents, agentItem{
			ID:     a.ID,
			Status: a.Status,
		})
	}

	resp := map[string]any{
		"event":  "HANDSHAKE_OK",
		"agents": respAgents,
	}
	b, _ := json.Marshal(resp)
	c.send(b)
}

func (c *clientConn) handlePing(env incomingEnvelope) {
	if err := c.hub.api.Heartbeat(c.workerID); err != nil {
		c.handleDomainError(err, env)
		return
	}

	resp := map[string]any{
		"event":     "PONG",
		"timestamp": env.Timestamp,
	}
	b, _ := json.Marshal(resp)
	c.send(b)
}

func (c *clientConn) handleRequestTask(env incomingEnvelope) {
	asg, err := c.hub.api.RequestJob(c.workerID, env.AgentID)
	if err != nil {
		c.handleDomainError(err, env)
		return
	}

	if asg == nil {
		resp := map[string]any{
			"event":    "NO_TASK",
			"agent_id": env.AgentID,
		}
		b, _ := json.Marshal(resp)
		c.send(b)
		return
	}

	payload := map[string]any{
		"task_id":      asg.TaskID,
		"branch_name":  asg.BranchName,
		"handoff_note": asg.HandoffNote,
	}
	if c.hub.promptFn != nil {
		payload["prompt"] = c.hub.promptFn(*asg)
	}

	resp := map[string]any{
		"event":         "ASSIGN_TASK",
		"assignment_id": asg.ID,
		"agent_id":      asg.AgentID,
		"job_kind":      asg.JobKind,
		"attempt":       asg.Attempt,
		"payload":       payload,
	}
	b, _ := json.Marshal(resp)
	c.send(b)
}

func (c *clientConn) handleLogChunk(env incomingEnvelope) {
	assignmentID := ""
	if env.AssignmentID != nil {
		assignmentID = *env.AssignmentID
	}
	if err := c.hub.api.Log(c.workerID, assignmentID); err != nil {
		c.handleDomainError(err, env)
		return
	}
	if c.hub.logSink != nil {
		c.hub.logSink(c.workerID, assignmentID, env.Chunk)
	}
}

func (c *clientConn) handleTaskSubmitted(env incomingEnvelope) {
	assignmentID := ""
	if env.AssignmentID != nil {
		assignmentID = *env.AssignmentID
	}
	if err := c.hub.api.Submit(c.workerID, assignmentID); err != nil {
		c.handleDomainError(err, env)
		return
	}
	if c.hub.submitHook != nil {
		c.hub.submitHook(c.workerID, assignmentID, env.PrUrl, env.Result)
	}
}

func (c *clientConn) handleAgentExhausted(env incomingEnvelope) {
	assignmentID := ""
	if env.AssignmentID != nil {
		assignmentID = *env.AssignmentID
	}

	handoffNote := ""
	if env.HandoffNote != nil {
		handoffNote = *env.HandoffNote
	}

	wipCommit := ""
	if env.WipCommit != nil {
		wipCommit = *env.WipCommit
	}

	var resetsAt *time.Time
	if env.ResetsAt != nil && *env.ResetsAt != "" {
		if t, err := time.Parse(time.RFC3339, *env.ResetsAt); err == nil {
			resetsAt = &t
		}
	}

	if err := c.hub.api.Exhausted(c.workerID, env.AgentID, assignmentID, resetsAt, env.Reason, handoffNote, wipCommit); err != nil {
		c.handleDomainError(err, env)
		return
	}
}

func (c *clientConn) handleLeaseRenew(env incomingEnvelope) {
	var epoch int64
	if env.Epoch != nil {
		epoch = *env.Epoch
	}

	lease, err := c.hub.api.Renew(c.workerID, env.AgentID, epoch)
	if err != nil {
		c.handleDomainError(err, env)
		return
	}

	resp := map[string]any{
		"event":    "LEASE_GRANTED",
		"agent_id": lease.AgentID,
		"epoch":    lease.Epoch,
	}
	if lease.ExpiresAt != nil {
		resp["expires_at"] = lease.ExpiresAt.UTC().Format(time.RFC3339)
	}
	b, _ := json.Marshal(resp)
	c.send(b)
}

func (c *clientConn) handleLeadRelease(env incomingEnvelope) {
	var epoch int64
	if env.Epoch != nil {
		epoch = *env.Epoch
	}

	if err := c.hub.api.Release(c.workerID, env.AgentID, epoch); err != nil {
		c.handleDomainError(err, env)
		return
	}
}

type leadActionData struct {
	TaskID string `json:"task_id"`
	Note   string `json:"note"`
}

func (c *clientConn) handleLeadAction(env incomingEnvelope) {
	if env.Action != "NOTE" {
		c.sendError("UNSUPPORTED", nil)
		return
	}

	var d leadActionData
	if len(env.Data) > 0 {
		_ = json.Unmarshal(env.Data, &d)
	}

	var epoch int64
	if env.Epoch != nil {
		epoch = *env.Epoch
	}

	if err := c.hub.api.LeadNote(c.workerID, env.AgentID, epoch, d.TaskID, d.Note); err != nil {
		c.handleDomainError(err, env)
		return
	}
}

func (c *clientConn) handleDomainError(err error, env incomingEnvelope) {
	var code string
	switch {
	case errors.Is(err, dispatcher.ErrStaleAssignment):
		code = "STALE_ASSIGNMENT"
	case errors.Is(err, dispatcher.ErrStaleEpoch):
		code = "STALE_EPOCH"
	case errors.Is(err, dispatcher.ErrAgentNotOwned):
		code = "AGENT_NOT_OWNED"
	case errors.Is(err, dispatcher.ErrNotLead):
		code = "NOT_LEAD"
	case errors.Is(err, dispatcher.ErrAgentNotEligible):
		code = "AGENT_NOT_ELIGIBLE"
	case errors.Is(err, dispatcher.ErrNoLead):
		code = "NO_LEAD"
	case errors.Is(err, dispatcher.ErrNotFound):
		code = "NOT_FOUND"
	default:
		code = "INTERNAL"
	}

	ref := make(map[string]any)
	if env.AssignmentID != nil && *env.AssignmentID != "" {
		ref["assignment_id"] = *env.AssignmentID
	}
	if env.Epoch != nil {
		ref["epoch"] = *env.Epoch
	}
	if env.AgentID != "" {
		ref["agent_id"] = env.AgentID
	}

	c.sendError(code, ref)
}
