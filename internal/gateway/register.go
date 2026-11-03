package gateway

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

var nameRegex = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

type registerRequest struct {
	Name   string   `json:"name"`
	OS     string   `json:"os"`
	Agents []string `json:"agents"`
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

func RegisterHandler(db *sql.DB, registrationKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		key := r.Header.Get("X-Registration-Key")
		if key == "" || registrationKey == "" || subtle.ConstantTimeCompare([]byte(key), []byte(registrationKey)) != 1 {
			writeJSON(w, http.StatusUnauthorized, apiResponse{
				Success: false,
				Data:    nil,
				Error: &apiError{
					Code:    "UNAUTHORIZED",
					Message: "invalid or missing registration key",
				},
			})
			return
		}

		var req registerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{
				Success: false,
				Data:    nil,
				Error: &apiError{
					Code:    "VALIDATION_ERROR",
					Message: "invalid JSON body",
				},
			})
			return
		}

		if !nameRegex.MatchString(req.Name) || strings.TrimSpace(req.OS) == "" || len(req.Agents) == 0 {
			writeJSON(w, http.StatusBadRequest, apiResponse{
				Success: false,
				Data:    nil,
				Error: &apiError{
					Code:    "VALIDATION_ERROR",
					Message: "validation failed",
				},
			})
			return
		}

		for _, ag := range req.Agents {
			if ag != "CLAUDE" && ag != "ANTIGRAVITY" {
				writeJSON(w, http.StatusBadRequest, apiResponse{
					Success: false,
					Data:    nil,
					Error: &apiError{
						Code:    "VALIDATION_ERROR",
						Message: "unsupported agent kind",
					},
				})
				return
			}
		}

		var existingID string
		err := db.QueryRow(`SELECT id FROM workers WHERE id = ?`, req.Name).Scan(&existingID)
		if err == nil {
			writeJSON(w, http.StatusConflict, apiResponse{
				Success: false,
				Data:    nil,
				Error: &apiError{
					Code:    "CONFLICT",
					Message: "worker already exists",
				},
			})
			return
		}

		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{
				Success: false,
				Data:    nil,
				Error: &apiError{
					Code:    "INTERNAL",
					Message: "failed to generate token",
				},
			})
			return
		}
		rawToken := hex.EncodeToString(tokenBytes)
		tokenHash := HashToken(rawToken)

		_, err = db.Exec(`INSERT INTO workers (id, name, os, token_hash) VALUES (?, ?, ?, ?)`,
			req.Name, req.Name, req.OS, tokenHash)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
				writeJSON(w, http.StatusConflict, apiResponse{
					Success: false,
					Data:    nil,
					Error: &apiError{
						Code:    "CONFLICT",
						Message: "worker already exists",
					},
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, apiResponse{
				Success: false,
				Data:    nil,
				Error: &apiError{
					Code:    "INTERNAL",
					Message: "database error",
				},
			})
			return
		}

		type regData struct {
			WorkerID    string `json:"worker_id"`
			RunnerToken string `json:"runner_token"`
		}

		writeJSON(w, http.StatusOK, apiResponse{
			Success: true,
			Data: regData{
				WorkerID:    req.Name,
				RunnerToken: rawToken,
			},
			Error: nil,
		})
	})
}
