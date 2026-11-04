package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/namth/agenthub/internal/dispatcher"
)

type Option func(*Hub)

type Hub struct {
	api      dispatcher.API
	auth     Authenticator
	upgrader websocket.Upgrader

	promptFn      func(dispatcher.Assignment) string
	repoFn        func(dispatcher.Assignment) string
	submitHook    func(workerID, assignmentID, prURL string, result json.RawMessage)
	logSink       func(workerID, assignmentID, chunk string)
	serverVersion string
	versionSink   func(workerID, version string)

	mu      sync.Mutex
	clients map[string]*clientConn
	closed  bool
}

func WithPrompt(fn func(dispatcher.Assignment) string) Option {
	return func(h *Hub) {
		h.promptFn = fn
	}
}

func WithRepo(fn func(dispatcher.Assignment) string) Option {
	return func(h *Hub) {
		h.repoFn = fn
	}
}

func WithSubmitHook(fn func(workerID, assignmentID, prURL string, result json.RawMessage)) Option {
	return func(h *Hub) {
		h.submitHook = fn
	}
}

func WithLogSink(fn func(workerID, assignmentID, chunk string)) Option {
	return func(h *Hub) {
		h.logSink = fn
	}
}

func WithServerVersion(v string) Option {
	return func(h *Hub) {
		h.serverVersion = v
	}
}

func WithVersionSink(fn func(workerID, version string)) Option {
	return func(h *Hub) {
		h.versionSink = fn
	}
}

func New(api dispatcher.API, auth Authenticator, opts ...Option) *Hub {
	h := &Hub{
		api:  api,
		auth: auth,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		clients: make(map[string]*clientConn),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *Hub) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	conns := make([]*clientConn, 0, len(h.clients))
	for _, c := range h.clients {
		conns = append(conns, c)
	}
	h.clients = make(map[string]*clientConn)
	h.mu.Unlock()

	for _, c := range conns {
		c.close()
	}
	return nil
}

func (h *Hub) CloseWorker(workerID string) bool {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return false
	}
	c := h.clients[workerID]
	h.mu.Unlock()

	if c == nil {
		return false
	}
	return c.close()
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("X-Runner-Token")
	if token == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	workerID, err := h.auth(token)
	if err != nil || workerID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	c := newClientConn(h, workerID, ws)
	h.addClient(c)

	go c.writePump()
	c.readPump()
}

func (h *Hub) addClient(c *clientConn) {
	var old *clientConn
	h.mu.Lock()
	if existing, ok := h.clients[c.workerID]; ok {
		existing.replaced = true
		old = existing
	}
	h.clients[c.workerID] = c
	h.mu.Unlock()

	if old != nil {
		old.close()
	}
}

func (h *Hub) removeClient(c *clientConn) {
	var shouldDisconnect bool
	h.mu.Lock()
	if h.clients[c.workerID] == c {
		delete(h.clients, c.workerID)
		if !c.replaced && c.handshaken && !h.closed {
			shouldDisconnect = true
		}
	}
	h.mu.Unlock()

	if shouldDisconnect {
		_ = h.api.Disconnect(c.workerID)
	}
}

func workerFromAgentID(agentID string) string {
	idx := strings.LastIndex(agentID, "/")
	if idx >= 0 {
		return agentID[:idx]
	}
	return agentID
}

func (h *Hub) Notify(ev dispatcher.Event) {
	var payload []byte
	var err error

	switch ev.Type {
	case dispatcher.EvLeaseGranted:
		m := map[string]any{
			"event":    "LEASE_GRANTED",
			"agent_id": ev.AgentID,
			"epoch":    ev.Epoch,
		}
		if ev.Until != nil {
			m["expires_at"] = ev.Until.UTC().Format(time.RFC3339)
		}
		payload, err = json.Marshal(m)

	case dispatcher.EvLeaseRevoked:
		payload, err = json.Marshal(map[string]any{
			"event":    "LEASE_REVOKED",
			"agent_id": ev.AgentID,
			"epoch":    ev.Epoch,
			"reason":   ev.Reason,
		})

	case dispatcher.EvAbortTask:
		payload, err = json.Marshal(map[string]any{
			"event":         "ABORT_TASK",
			"assignment_id": ev.AssignmentID,
			"reason":        ev.Reason,
		})

	case dispatcher.EvJobAvailable:
		payload, err = json.Marshal(map[string]any{
			"event": "JOB_AVAILABLE",
		})

	default:
		return
	}

	if err != nil {
		return
	}

	if ev.Type == dispatcher.EvJobAvailable {
		h.mu.Lock()
		conns := make([]*clientConn, 0, len(h.clients))
		for _, c := range h.clients {
			conns = append(conns, c)
		}
		h.mu.Unlock()

		for _, c := range conns {
			c.enqueue(payload)
		}
		return
	}

	targetWorker := workerFromAgentID(ev.AgentID)
	if targetWorker == "" {
		targetWorker = ev.WorkerID
	}

	var targetConn *clientConn
	h.mu.Lock()
	targetConn = h.clients[targetWorker]
	h.mu.Unlock()

	if targetConn != nil {
		targetConn.enqueue(payload)
	}
}
