package logs

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

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

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, apiResponse{
		Success: false,
		Data:    nil,
		Error:   &apiError{Code: code, Message: msg},
	})
}

func Handler(db *sql.DB, s *Store, authz func(*http.Request) (string, bool)) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/admin/workers/{id}/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		if authz == nil {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}
		role, ok := authz(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}
		if role != "ADMIN" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "forbidden")
			return
		}

		workerID := chi.URLParam(r, "id")
		var exists int
		err := db.QueryRow(`SELECT 1 FROM workers WHERE id = ?`, workerID).Scan(&exists)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "worker not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		subCh, unsub := s.Subscribe(workerID)
		defer unsub()

		recent, err := s.Recent(workerID, 500)
		if err != nil {
			return
		}
		if recent == nil {
			recent = []Line{}
		}

		var lastID int64
		for _, l := range recent {
			if l.ID > lastID {
				lastID = l.ID
			}
		}

		histBytes, err := json.Marshal(recent)
		if err != nil {
			return
		}

		if _, err := fmt.Fprintf(w, "event: history\ndata: %s\n\n", histBytes); err != nil {
			return
		}
		flusher.Flush()

		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				if _, err := fmt.Fprintf(w, ": ping\n\n"); err != nil {
					return
				}
				flusher.Flush()
			case line, ok := <-subCh:
				if !ok {
					return
				}
				if line.ID <= lastID {
					continue
				}
				lastID = line.ID
				lineBytes, err := json.Marshal(line)
				if err != nil {
					return
				}
				if _, err := fmt.Fprintf(w, "event: log\ndata: %s\n\n", lineBytes); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	})
	return r
}
