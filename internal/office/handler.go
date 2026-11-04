package office

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// Options chứa cấu hình cho HTTP handler của văn phòng ảo.
type Options struct {
	PingEvery       time.Duration
	MaxStreams      int
	MaxPerUser      int
	User            func(*http.Request) (userID string, ok bool)
	Presence        *Presence
	TelegramEnabled bool
}

type apiResponse struct {
	Success bool      `json:"success"`
	Data    any       `json:"data"`
	Error   *apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, resp apiResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func writeSuccess(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, apiResponse{
		Success: true,
		Data:    data,
		Error:   nil,
	})
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiResponse{
		Success: false,
		Data:    nil,
		Error:   &apiError{Code: code, Message: msg},
	})
}

type officeServer struct {
	db          *sql.DB
	bus         *Bus
	snapshot    func() (any, error)
	authz       func(*http.Request) (string, bool)
	opt         Options
	mu          sync.Mutex
	active      int
	userStreams map[string]int
	closers     map[int]chan struct{}
	nextID      int
}

// Handler trả về http.Handler phục vụ các endpoint cài đặt và SSE của văn phòng ảo.
func Handler(
	db *sql.DB,
	b *Bus,
	snapshot func() (any, error),
	authz func(*http.Request) (string, bool),
	opt Options,
) http.Handler {
	if opt.PingEvery <= 0 {
		opt.PingEvery = 20 * time.Second
	}
	if opt.MaxStreams <= 0 {
		opt.MaxStreams = 30
	}
	if opt.MaxPerUser <= 0 {
		opt.MaxPerUser = 3
	}

	s := &officeServer{
		db:          db,
		bus:         b,
		snapshot:    snapshot,
		authz:       authz,
		opt:         opt,
		closers:     make(map[int]chan struct{}),
		userStreams: make(map[string]int),
	}

	r := chi.NewRouter()
	r.Get("/api/admin/settings", s.getSettings)
	r.Patch("/api/admin/settings", s.patchSettings)
	r.Get("/api/admin/office/stream", s.redirectStream)
	r.Get("/api/office/stream", s.stream)
	r.Get("/api/office/status", s.getStatus)

	return r
}

func (s *officeServer) checkAdmin(r *http.Request) (int, string, string) {
	if s.authz == nil {
		return http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"
	}
	role, ok := s.authz(r)
	if !ok {
		return http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized"
	}
	if role != "ADMIN" {
		return http.StatusForbidden, "FORBIDDEN", "forbidden"
	}
	return 0, "", ""
}

func (s *officeServer) getSettings(w http.ResponseWriter, r *http.Request) {
	if status, code, msg := s.checkAdmin(r); status != 0 {
		writeError(w, status, code, msg)
		return
	}

	var val string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key='office_enabled'`).Scan(&val)
	enabled := (err == nil && val == "true")

	writeSuccess(w, map[string]any{
		"office_enabled":   enabled,
		"telegram_enabled": s.opt.TelegramEnabled,
	})
}

func (s *officeServer) getStatus(w http.ResponseWriter, r *http.Request) {
	if s.authz == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	_, ok := s.authz(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}

	writeSuccess(w, map[string]any{
		"enabled": s.isOfficeEnabled(),
	})
}

func (s *officeServer) redirectStream(w http.ResponseWriter, r *http.Request) {
	target := "/api/office/stream"
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
}

func (s *officeServer) patchSettings(w http.ResponseWriter, r *http.Request) {
	if status, code, msg := s.checkAdmin(r); status != 0 {
		writeError(w, status, code, msg)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "cannot read body")
		return
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid json")
		return
	}

	if len(raw) != 1 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "body must contain exactly office_enabled")
		return
	}

	valRaw, ok := raw["office_enabled"]
	if !ok {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "missing office_enabled")
		return
	}

	trimmed := string(bytes.TrimSpace(valRaw))
	if trimmed != "true" && trimmed != "false" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "office_enabled must be a boolean")
		return
	}

	enabled := (trimmed == "true")
	strVal := "false"
	if enabled {
		strVal = "true"
	}

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO settings (key, value, updated_at)
		VALUES ('office_enabled', ?, CURRENT_TIMESTAMP)
	`, strVal)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	if !enabled {
		s.closeAllStreams()
	}

	writeSuccess(w, map[string]any{
		"office_enabled": enabled,
	})
}

func (s *officeServer) closeAllStreams() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, ch := range s.closers {
		close(ch)
		delete(s.closers, id)
	}
}

func (s *officeServer) isOfficeEnabled() bool {
	var val string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key='office_enabled'`).Scan(&val)
	return err == nil && val == "true"
}

func writeEventFrame(w io.Writer, ev Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: office\ndata: %s\n\n", ev.ID, b)
	return err
}

func (s *officeServer) stream(w http.ResponseWriter, r *http.Request) {
	if s.authz == nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}
	_, ok := s.authz(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}

	if !s.isOfficeEnabled() {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "office is disabled")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	var userID string
	var hasUser bool
	if s.opt.User != nil {
		userID, hasUser = s.opt.User(r)
	}

	s.mu.Lock()
	if s.active >= s.opt.MaxStreams {
		s.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "TOO_MANY_CONNECTIONS", "too many connections")
		return
	}
	if hasUser && userID != "" && s.opt.MaxPerUser > 0 {
		if s.userStreams[userID] >= s.opt.MaxPerUser {
			s.mu.Unlock()
			writeError(w, http.StatusTooManyRequests, "TOO_MANY_CONNECTIONS", "too many connections")
			return
		}
	}

	s.active++
	if hasUser && userID != "" {
		s.userStreams[userID]++
	}
	closeCh := make(chan struct{})
	streamID := s.nextID
	s.nextID++
	s.closers[streamID] = closeCh
	s.mu.Unlock()

	if s.opt.Presence != nil && hasUser && userID != "" {
		s.opt.Presence.StreamOpened(userID)
	}

	defer func() {
		s.mu.Lock()
		delete(s.closers, streamID)
		s.active--
		if hasUser && userID != "" {
			if s.userStreams[userID] > 0 {
				s.userStreams[userID]--
			}
		}
		s.mu.Unlock()

		if s.opt.Presence != nil && hasUser && userID != "" {
			s.opt.Presence.StreamClosed(userID)
		}
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Đăng ký nhận trước rồi mới gửi
	subCh, unsub := s.bus.Subscribe()
	defer unsub()

	lastIDStr := r.Header.Get("Last-Event-ID")
	var lastSentID int64
	sentCatchup := false

	if lastIDStr != "" {
		if lastID, err := strconv.ParseInt(lastIDStr, 10, 64); err == nil {
			missed, ok := s.bus.Since(lastID)
			if ok {
				sentCatchup = true
				lastSentID = lastID
				for _, ev := range missed {
					if err := writeEventFrame(w, ev); err != nil {
						return
					}
					flusher.Flush()
					lastSentID = ev.ID
				}
			}
		}
	}

	if !sentCatchup {
		// Không gọi snapshot() khi giữ khoá của Bus
		snapData, err := s.snapshot()
		if err != nil {
			return
		}

		allEvs, _ := s.bus.Since(0)
		if allEvs == nil {
			allEvs = []Event{}
		}
		for _, ev := range allEvs {
			if ev.ID > lastSentID {
				lastSentID = ev.ID
			}
		}

		mergedSnap := make(map[string]any)
		if snapData != nil {
			if b, err := json.Marshal(snapData); err == nil {
				_ = json.Unmarshal(b, &mergedSnap)
			}
		}
		mergedSnap["recent"] = allEvs
		snapBytes, err := json.Marshal(mergedSnap)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", snapBytes); err != nil {
			return
		}
		flusher.Flush()
	}

	ticker := time.NewTicker(s.opt.PingEvery)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-closeCh:
			return
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case ev, ok := <-subCh:
			if !ok {
				return
			}
			if ev.ID <= lastSentID {
				continue
			}
			lastSentID = ev.ID
			if err := writeEventFrame(w, ev); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
