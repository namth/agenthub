package machines

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "embed"

	"github.com/go-chi/chi/v5"
	"github.com/namth/agenthub/internal/gateway"
)

//go:embed install.sh
var installScriptTmpl string

type Config struct {
	BaseURL    func() string
	RunnersDir string
	Now        func() time.Time
	IsOwner    func(userID string) bool
	IsMember   func(userID, projectID string) bool
	OnRemoved  func(workerID string)
}

func (c *Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Config) baseURL() string {
	if c.BaseURL != nil {
		return c.BaseURL()
	}
	return ""
}

func (c *Config) isOwner(userID string) bool {
	if c.IsOwner != nil {
		return c.IsOwner(userID)
	}
	return false
}

func (c *Config) isMember(userID, projectID string) bool {
	if c.IsMember != nil {
		return c.IsMember(userID, projectID)
	}
	return false
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

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Success: true,
		Data:    data,
		Error:   nil,
	})
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Success: false,
		Data:    nil,
		Error: &apiError{
			Code:    code,
			Message: msg,
		},
	})
}

var (
	nameRegex     = regexp.MustCompile(`^[a-zA-Z0-9-]{2,40}$`)
	downloadRegex = regexp.MustCompile(`^agenthub-(darwin|linux)-(arm64|amd64)(\.sha256)?$`)
)

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generateJoinCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	res := make([]byte, 9)
	for i := 0; i < 4; i++ {
		res[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	res[4] = '-'
	for i := 4; i < 8; i++ {
		res[i+1] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(res), nil
}

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02T15:04:05", s)
}

func getClientIP(r *http.Request) string {
	// Cloudflare Tunnel ghi IP thật vào CF-Connecting-IP (client không tự đặt được khi đi qua Cloudflare).
	if ip := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); ip != "" {
		return ip
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

type rateLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		failures: make(map[string][]time.Time),
	}
}

func (rl *rateLimiter) isBlocked(ip string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	windowStart := now.Add(-1 * time.Minute)
	times := rl.failures[ip]
	var active []time.Time
	for _, t := range times {
		if t.After(windowStart) {
			active = append(active, t)
		}
	}
	rl.failures[ip] = active
	return len(active) >= 5
}

func (rl *rateLimiter) recordFailure(ip string, now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	windowStart := now.Add(-1 * time.Minute)
	times := rl.failures[ip]
	var active []time.Time
	for _, t := range times {
		if t.After(windowStart) {
			active = append(active, t)
		}
	}
	active = append(active, now)
	rl.failures[ip] = active
}

func Handler(db *sql.DB, cfg Config, authz func(*http.Request) (userID string, ok bool)) http.Handler {
	r := chi.NewRouter()
	rl := newRateLimiter()

	// POST /api/fleet/join-codes
	r.Post("/api/fleet/join-codes", func(w http.ResponseWriter, req *http.Request) {
		userID, ok := authz(req)
		if !ok || userID == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}

		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
			return
		}

		if !nameRegex.MatchString(body.Name) {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid machine name")
			return
		}

		// Kiểm tra tên đã là máy của người khác hay chưa
		var ownerID sql.NullString
		err := db.QueryRow("SELECT owner_user_id FROM workers WHERE id = ?", body.Name).Scan(&ownerID)
		if err == nil {
			if !ownerID.Valid || ownerID.String != userID {
				writeError(w, http.StatusConflict, "CONFLICT", "machine name already taken by another user")
				return
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}

		now := cfg.now()
		nowRFC := now.UTC().Format(time.RFC3339)

		// Kiểm tra trong join_codes nếu có mã chưa dùng còn hạn của người khác
		var jcOwner string
		err = db.QueryRow("SELECT owner_user_id FROM join_codes WHERE worker_name = ? AND used_at IS NULL AND expires_at > ? LIMIT 1", body.Name, nowRFC).Scan(&jcOwner)
		if err == nil {
			if jcOwner != userID {
				writeError(w, http.StatusConflict, "CONFLICT", "machine name reserved by another user")
				return
			}
		}

		code, err := generateJoinCode()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to generate join code")
			return
		}

		codeHash := gateway.HashToken(code)
		expiresAt := now.Add(15 * time.Minute)
		expiresAtStr := expiresAt.UTC().Format(time.RFC3339)

		_, err = db.Exec("INSERT INTO join_codes (code_hash, owner_user_id, worker_name, expires_at) VALUES (?, ?, ?, ?)",
			codeHash, userID, body.Name, expiresAtStr)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to store join code")
			return
		}

		baseURL := cfg.baseURL()
		if baseURL == "" {
			baseURL = "http://" + req.Host
		}

		command := fmt.Sprintf("curl -fsSL %s/install.sh | sh -s -- --join %s --name %s", baseURL, code, body.Name)
		agentPrompt := fmt.Sprintf(`# Nhiệm vụ: nối máy này vào AgentHub (%s) với tên "%s"
Mã tham gia (dùng một lần, hết hạn %s): %s
1. Kiểm công cụ: git, gh, go, node; agent CLI: claude và/hoặc agy. Thiếu → hỏi người dùng rồi cài theo hệ điều hành (macOS: Homebrew).
2. Chạy: %s
3. Đăng nhập (nhờ NGƯỜI DÙNG làm, không tự làm): `+"`gh auth login`"+`, `+"`claude`"+` → /login, `+"`agy`"+` (nếu dùng).
4. Kiểm: `+"`~/.agenthub/agenthub runner --status`"+` báo ONLINE; báo lại danh sách agent đã nhận.
Ràng buộc: không in/gửi token; không dùng chế độ bỏ qua quyền (`+"`--dangerously-skip-permissions`"+`); không sửa cấu hình ngoài ~/.agenthub và dịch vụ runner.`,
			baseURL, body.Name, expiresAt.Format("15:04:05"), code, command)

		writeJSON(w, http.StatusCreated, map[string]any{
			"code":         code,
			"expires_at":   expiresAtStr,
			"command":      command,
			"agent_prompt": agentPrompt,
		})
	})

	// POST /api/fleet/join
	r.Post("/api/fleet/join", func(w http.ResponseWriter, req *http.Request) {
		now := cfg.now()
		ip := getClientIP(req)

		if rl.isBlocked(ip, now) {
			writeError(w, http.StatusTooManyRequests, "TOO_MANY_REQUESTS", "too many failed attempts")
			return
		}

		var body struct {
			Code   string   `json:"code"`
			Name   string   `json:"name"`
			OS     string   `json:"os"`
			Arch   string   `json:"arch"`
			Agents []string `json:"agents"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			rl.recordFailure(ip, now)
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
			return
		}

		if body.Code == "" {
			rl.recordFailure(ip, now)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing join code")
			return
		}

		codeHash := gateway.HashToken(body.Code)
		var ownerUserID, workerName string
		var expiresAtStr string
		var usedAt sql.NullString

		err := db.QueryRow("SELECT owner_user_id, worker_name, expires_at, used_at FROM join_codes WHERE code_hash = ?", codeHash).
			Scan(&ownerUserID, &workerName, &expiresAtStr, &usedAt)
		if err != nil {
			rl.recordFailure(ip, now)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid code")
			return
		}

		if usedAt.Valid {
			rl.recordFailure(ip, now)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "code already used")
			return
		}

		expTime, err := parseTime(expiresAtStr)
		if err != nil || now.After(expTime) {
			rl.recordFailure(ip, now)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "code expired")
			return
		}

		// Đánh dấu mã đã dùng bằng UPDATE ... WHERE used_at IS NULL
		nowRFC := now.UTC().Format(time.RFC3339)
		res, err := db.Exec("UPDATE join_codes SET used_at = ? WHERE code_hash = ? AND used_at IS NULL", nowRFC, codeHash)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}
		rowsAff, _ := res.RowsAffected()
		if rowsAff == 0 {
			rl.recordFailure(ip, now)
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "code already used")
			return
		}

		// Sinh token runner 32 byte hex ngẫu nhiên
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to generate runner token")
			return
		}
		rawToken := hex.EncodeToString(tokenBytes)
		tokenHash := gateway.HashToken(rawToken)

		workerID := workerName
		if body.Name != "" {
			workerID = body.Name
		}
		osName := body.OS
		if osName == "" {
			osName = "darwin"
		}

		// Tạo / ghi đè máy
		_, err = db.Exec(`
			INSERT INTO workers (id, name, os, token_hash, owner_user_id, paused, conn_status)
			VALUES (?, ?, ?, ?, ?, 0, 'OFFLINE')
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				os = excluded.os,
				token_hash = excluded.token_hash,
				owner_user_id = excluded.owner_user_id,
				paused = 0
		`, workerID, workerID, osName, tokenHash, ownerUserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}

		leadEligible := 0
		if cfg.isOwner(ownerUserID) {
			leadEligible = 1
		}

		for _, ag := range body.Agents {
			agTrimmed := strings.TrimSpace(ag)
			if agTrimmed == "" {
				continue
			}
			agUpper := strings.ToUpper(agTrimmed)
			agLower := strings.ToLower(agUpper)
			agentID := workerID + "/" + agLower

			_, err = db.Exec(`
				INSERT INTO agents (id, worker_id, kind, status, lead_eligible)
				VALUES (?, ?, ?, 'IDLE', ?)
				ON CONFLICT(id) DO UPDATE SET
					worker_id = excluded.worker_id,
					kind = excluded.kind,
					lead_eligible = excluded.lead_eligible
			`, agentID, workerID, agUpper, leadEligible)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
				return
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"worker_id":    workerID,
			"runner_token": rawToken,
		})
	})

	// GET /install.sh
	r.Get("/install.sh", func(w http.ResponseWriter, req *http.Request) {
		baseURL := cfg.baseURL()
		if baseURL == "" {
			baseURL = "http://" + req.Host
		}
		script := strings.ReplaceAll(installScriptTmpl, "__BASE_URL__", baseURL)
		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(script))
	})

	// GET /download/{filename}
	handleDownload := func(w http.ResponseWriter, req *http.Request) {
		filename := chi.URLParam(req, "filename")
		if filename == "" {
			filename = chi.URLParam(req, "*")
		}
		if filename == "" {
			filename = strings.TrimPrefix(req.URL.Path, "/download/")
		}

		if !downloadRegex.MatchString(filename) {
			http.NotFound(w, req)
			return
		}

		target := filepath.Join(cfg.RunnersDir, filepath.Clean(filename))
		st, err := os.Stat(target)
		if err != nil || st.IsDir() {
			http.NotFound(w, req)
			return
		}

		http.ServeFile(w, req, target)
	}
	r.Get("/download/{filename}", handleDownload)
	r.Get("/download/*", handleDownload)

	// GET /api/my/machines
	r.Get("/api/my/machines", func(w http.ResponseWriter, req *http.Request) {
		userID, ok := authz(req)
		if !ok || userID == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}

		rows, err := db.Query(`
			SELECT id, name, os, conn_status, version, paused
			FROM workers
			WHERE owner_user_id = ?
			ORDER BY id
		`, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}
		defer rows.Close()

		type currentJobDTO struct {
			JobID   string `json:"job_id"`
			TaskID  string `json:"task_id"`
			Kind    string `json:"kind"`
			Attempt int    `json:"attempt"`
		}

		type agentDTO struct {
			ID         string         `json:"id"`
			Kind       string         `json:"kind"`
			Status     string         `json:"status"`
			Projects   []string       `json:"projects"`
			CurrentJob *currentJobDTO `json:"current_job"`
		}

		type machineDTO struct {
			ID         string     `json:"id"`
			Name       string     `json:"name"`
			OS         string     `json:"os"`
			ConnStatus string     `json:"conn_status"`
			Version    *string    `json:"version"`
			Paused     bool       `json:"paused"`
			Agents     []agentDTO `json:"agents"`
		}

		machines := make([]machineDTO, 0)
		for rows.Next() {
			var m machineDTO
			var pausedInt int
			var version sql.NullString

			if err := rows.Scan(&m.ID, &m.Name, &m.OS, &m.ConnStatus, &version, &pausedInt); err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
				return
			}
			m.Paused = (pausedInt == 1)
			if version.Valid {
				m.Version = &version.String
			}
			m.Agents = make([]agentDTO, 0)

			// Query agents của worker
			agRows, err := db.Query(`
				SELECT id, kind, status
				FROM agents
				WHERE worker_id = ?
				ORDER BY id
			`, m.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
				return
			}

			for agRows.Next() {
				var ag agentDTO
				if err := agRows.Scan(&ag.ID, &ag.Kind, &ag.Status); err != nil {
					agRows.Close()
					writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
					return
				}
				ag.Projects = make([]string, 0)

				// Query projects của agent
				pRows, err := db.Query(`SELECT project_id FROM agent_projects WHERE agent_id = ? ORDER BY project_id`, ag.ID)
				if err == nil {
					for pRows.Next() {
						var pid string
						if err := pRows.Scan(&pid); err == nil {
							ag.Projects = append(ag.Projects, pid)
						}
					}
					pRows.Close()
				}

				// Query active assignment
				var cj currentJobDTO
				err = db.QueryRow(`
					SELECT a.job_id, j.task_id, j.kind, a.attempt
					FROM assignments a
					JOIN jobs j ON a.job_id = j.id
					WHERE a.agent_id = ? AND a.status = 'ACTIVE'
					ORDER BY a.started_at DESC
					LIMIT 1
				`, ag.ID).Scan(&cj.JobID, &cj.TaskID, &cj.Kind, &cj.Attempt)
				if err == nil {
					ag.CurrentJob = &cj
				}

				m.Agents = append(m.Agents, ag)
			}
			agRows.Close()

			machines = append(machines, m)
		}

		writeJSON(w, http.StatusOK, machines)
	})

	// PATCH /api/my/machines/{id}
	r.Patch("/api/my/machines/{id}", func(w http.ResponseWriter, req *http.Request) {
		userID, ok := authz(req)
		if !ok || userID == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}

		id := chi.URLParam(req, "id")
		var ownerID sql.NullString
		err := db.QueryRow("SELECT owner_user_id FROM workers WHERE id = ?", id).Scan(&ownerID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "machine not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}

		isSelf := ownerID.Valid && ownerID.String == userID
		if !isSelf && !cfg.isOwner(userID) {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "forbidden")
			return
		}

		var body struct {
			Paused   *bool           `json:"paused"`
			Projects json.RawMessage `json:"projects"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
			return
		}

		if body.Paused != nil {
			pVal := 0
			if *body.Paused {
				pVal = 1
			}
			if _, err := db.Exec("UPDATE workers SET paused = ? WHERE id = ?", pVal, id); err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to update paused")
				return
			}
		}

		if body.Projects != nil {
			if string(body.Projects) == "null" {
				// null = mọi dự án (xoá hết danh sách)
				_, err := db.Exec("DELETE FROM agent_projects WHERE agent_id IN (SELECT id FROM agents WHERE worker_id = ?)", id)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
					return
				}
			} else {
				var projectIDs []string
				if err := json.Unmarshal(body.Projects, &projectIDs); err != nil {
					writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid projects array")
					return
				}

				// Kiểm tra IsMember cho mọi projectID
				for _, pid := range projectIDs {
					if !cfg.isMember(userID, pid) {
						writeError(w, http.StatusForbidden, "FORBIDDEN", "not member of project "+pid)
						return
					}
				}

				tx, err := db.Begin()
				if err != nil {
					writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
					return
				}

				_, err = tx.Exec("DELETE FROM agent_projects WHERE agent_id IN (SELECT id FROM agents WHERE worker_id = ?)", id)
				if err != nil {
					tx.Rollback()
					writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
					return
				}

				agRows, err := tx.Query("SELECT id FROM agents WHERE worker_id = ?", id)
				if err != nil {
					tx.Rollback()
					writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
					return
				}
				var agentIDs []string
				for agRows.Next() {
					var agID string
					if err := agRows.Scan(&agID); err == nil {
						agentIDs = append(agentIDs, agID)
					}
				}
				agRows.Close()

				for _, agID := range agentIDs {
					for _, pid := range projectIDs {
						_, err = tx.Exec("INSERT INTO agent_projects (agent_id, project_id) VALUES (?, ?)", agID, pid)
						if err != nil {
							tx.Rollback()
							writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
							return
						}
					}
				}

				if err := tx.Commit(); err != nil {
					writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
					return
				}
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{"id": id})
	})

	// DELETE /api/my/machines/{id}
	r.Delete("/api/my/machines/{id}", func(w http.ResponseWriter, req *http.Request) {
		userID, ok := authz(req)
		if !ok || userID == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
			return
		}

		id := chi.URLParam(req, "id")
		var ownerID sql.NullString
		err := db.QueryRow("SELECT owner_user_id FROM workers WHERE id = ?", id).Scan(&ownerID)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "machine not found")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}

		isSelf := ownerID.Valid && ownerID.String == userID
		if !isSelf && !cfg.isOwner(userID) {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "forbidden")
			return
		}

		now := cfg.now()
		nowStr := now.UTC().Format("2006-01-02 15:04:05")

		tx, err := db.Begin()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}

		// 1. Chuyển assignment ACTIVE của máy thành RECLAIMED + job về QUEUED
		asgRows, err := tx.Query(`
			SELECT a.id, a.job_id
			FROM assignments a
			JOIN agents ag ON a.agent_id = ag.id
			WHERE ag.worker_id = ? AND a.status = 'ACTIVE'
		`, id)
		if err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}

		type activeAsg struct {
			id    string
			jobID string
		}
		var asgs []activeAsg
		for asgRows.Next() {
			var aa activeAsg
			if err := asgRows.Scan(&aa.id, &aa.jobID); err == nil {
				asgs = append(asgs, aa)
			}
		}
		asgRows.Close()

		for _, aa := range asgs {
			_, err = tx.Exec("UPDATE assignments SET status = 'RECLAIMED', ended_at = ? WHERE id = ?", nowStr, aa.id)
			if err != nil {
				tx.Rollback()
				writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
				return
			}
			_, err = tx.Exec("UPDATE jobs SET status = 'QUEUED', current_assignment_id = NULL, updated_at = ? WHERE id = ?", nowStr, aa.jobID)
			if err != nil {
				tx.Rollback()
				writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
				return
			}
		}

		// 2. Chuyển agent về IDLE
		_, err = tx.Exec("UPDATE agents SET status = 'IDLE', idle_since = ? WHERE worker_id = ?", nowStr, id)
		if err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}

		// 3. Đổi token_hash sang giá trị ngẫu nhiên không dùng được
		randBytes := make([]byte, 32)
		if _, err := rand.Read(randBytes); err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to generate random hash")
			return
		}
		newHash := hex.EncodeToString(randBytes)
		_, err = tx.Exec("UPDATE workers SET token_hash = ?, conn_status = 'OFFLINE', offline_since = ? WHERE id = ?", newHash, nowStr, id)
		if err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}

		if err := tx.Commit(); err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "database error")
			return
		}

		// 4. Gọi OnRemoved
		if cfg.OnRemoved != nil {
			cfg.OnRemoved(id)
		}

		w.WriteHeader(http.StatusNoContent)
	})

	return r
}
