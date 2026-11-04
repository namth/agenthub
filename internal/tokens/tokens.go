package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrLimit        = errors.New("limit exceeded")
	ErrValidation   = errors.New("validation error")
)

type Token struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Last4      string     `json:"last4"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Store struct {
	db  *sql.DB
	now func() time.Time
	mu  sync.Mutex
}

func NewStore(db *sql.DB, now func() time.Time) *Store {
	if now == nil {
		now = time.Now
	}
	return &Store{
		db:  db,
		now: now,
	}
}

const timeFormat = "2006-01-02 15:04:05"

func parseTime(s string) (time.Time, error) {
	for _, layout := range []string{timeFormat, time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05.999999999-07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("cannot parse time: " + s)
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // RFC 4122 version 4
	b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (s *Store) Create(userID, name string, days int) (string, Token, error) {
	if days != 30 && days != 90 && days != 365 {
		return "", Token{}, ErrValidation
	}
	nameTrim := strings.TrimSpace(name)
	runeLen := utf8.RuneCountInString(nameTrim)
	if runeLen < 1 || runeLen > 40 {
		return "", Token{}, ErrValidation
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now().UTC()
	nowStr := now.Format(timeFormat)

	var activeCount int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM personal_tokens 
		WHERE user_id = ? AND revoked_at IS NULL AND expires_at > ?
	`, userID, nowStr).Scan(&activeCount)
	if err != nil {
		return "", Token{}, err
	}
	if activeCount >= 10 {
		return "", Token{}, ErrLimit
	}

	var duplicateCount int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM personal_tokens 
		WHERE user_id = ? AND name = ? AND revoked_at IS NULL AND expires_at > ?
	`, userID, nameTrim, nowStr).Scan(&duplicateCount)
	if err != nil {
		return "", Token{}, err
	}
	if duplicateCount > 0 {
		return "", Token{}, ErrConflict
	}

	_, _ = s.db.Exec(`
		DELETE FROM personal_tokens 
		WHERE user_id = ? AND name = ? AND (revoked_at IS NOT NULL OR expires_at <= ?)
	`, userID, nameTrim, nowStr)

	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", Token{}, err
	}
	plain := "ahp_" + base64.RawURLEncoding.EncodeToString(rawBytes)
	last4 := plain[len(plain)-4:]
	sum := sha256.Sum256([]byte(plain))
	tokenHash := hex.EncodeToString(sum[:])

	id := newUUID()
	expiresAt := now.Add(time.Duration(days) * 24 * time.Hour)
	expiresAtStr := expiresAt.Format(timeFormat)

	_, err = s.db.Exec(`
		INSERT INTO personal_tokens (id, user_id, name, token_hash, last4, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, id, userID, nameTrim, tokenHash, last4, expiresAtStr, nowStr)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return "", Token{}, ErrConflict
		}
		return "", Token{}, err
	}

	tok := Token{
		ID:        id,
		Name:      nameTrim,
		Last4:     last4,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}

	return plain, tok, nil
}

func (s *Store) List(userID string) ([]Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query(`
		SELECT id, name, last4, expires_at, last_used_at, created_at
		FROM personal_tokens
		WHERE user_id = ? AND revoked_at IS NULL
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]Token, 0)
	for rows.Next() {
		var (
			id, name, last4, expStr, createStr string
			lastUsedStr                        sql.NullString
		)
		if err := rows.Scan(&id, &name, &last4, &expStr, &lastUsedStr, &createStr); err != nil {
			return nil, err
		}
		expAt, err := parseTime(expStr)
		if err != nil {
			return nil, err
		}
		createAt, err := parseTime(createStr)
		if err != nil {
			return nil, err
		}
		var lastUsedAt *time.Time
		if lastUsedStr.Valid && lastUsedStr.String != "" {
			if t, err := parseTime(lastUsedStr.String); err == nil {
				lastUsedAt = &t
			}
		}
		list = append(list, Token{
			ID:         id,
			Name:       name,
			Last4:      last4,
			ExpiresAt:  expAt,
			LastUsedAt: lastUsedAt,
			CreatedAt:  createAt,
		})
	}
	return list, rows.Err()
}

func (s *Store) Revoke(userID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var ownerID string
	err := s.db.QueryRow(`SELECT user_id FROM personal_tokens WHERE id = ?`, id).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if ownerID != userID {
		return ErrForbidden
	}

	nowStr := s.now().UTC().Format(timeFormat)
	_, err = s.db.Exec(`UPDATE personal_tokens SET revoked_at = ? WHERE id = ?`, nowStr, id)
	return err
}

func (s *Store) Authenticate(plain string) (string, error) {
	if !strings.HasPrefix(plain, "ahp_") || len(plain) < 40 {
		return "", ErrUnauthorized
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sum := sha256.Sum256([]byte(plain))
	tokenHash := hex.EncodeToString(sum[:])

	var (
		id, userID, expStr      string
		revokedStr, lastUsedStr sql.NullString
	)
	err := s.db.QueryRow(`
		SELECT id, user_id, expires_at, revoked_at, last_used_at
		FROM personal_tokens
		WHERE token_hash = ?
	`, tokenHash).Scan(&id, &userID, &expStr, &revokedStr, &lastUsedStr)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrUnauthorized
	}
	if err != nil {
		return "", err
	}

	if revokedStr.Valid && revokedStr.String != "" {
		return "", ErrUnauthorized
	}

	expAt, err := parseTime(expStr)
	if err != nil {
		return "", ErrUnauthorized
	}

	now := s.now().UTC()
	if !now.Before(expAt) {
		return "", ErrUnauthorized
	}

	var needUpdate bool
	if !lastUsedStr.Valid || lastUsedStr.String == "" {
		needUpdate = true
	} else if prev, err := parseTime(lastUsedStr.String); err == nil {
		if now.Sub(prev) >= time.Minute {
			needUpdate = true
		}
	} else {
		needUpdate = true
	}

	if needUpdate {
		nowStr := now.Format(timeFormat)
		_, _ = s.db.Exec(`UPDATE personal_tokens SET last_used_at = ? WHERE id = ?`, nowStr, id)
	}

	return userID, nil
}

type apiResponse struct {
	Success bool    `json:"success"`
	Data    any     `json:"data"`
	Error   *apiErr `json:"error"`
}

type apiErr struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
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
		Error: &apiErr{
			Code:    code,
			Message: msg,
		},
	})
}

func Handler(s *Store, authz func(*http.Request) (userID, role string, ok bool)) http.Handler {
	r := chi.NewRouter()

	listTokens := func(w http.ResponseWriter, req *http.Request) {
		userID, role, ok := authz(req)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
			return
		}
		if role != "ADMIN" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "chỉ admin mới có quyền quản lý token")
			return
		}
		list, err := s.List(userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, list)
	}

	createToken := func(w http.ResponseWriter, req *http.Request) {
		userID, role, ok := authz(req)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
			return
		}
		if role != "ADMIN" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "chỉ admin mới có quyền quản lý token")
			return
		}
		var body struct {
			Name        string `json:"name"`
			ExpiresDays int    `json:"expires_days"`
			Days        int    `json:"days"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "dữ liệu không hợp lệ")
			return
		}
		days := body.ExpiresDays
		if days == 0 {
			days = body.Days
		}
		plain, tok, err := s.Create(userID, body.Name, days)
		if err != nil {
			switch {
			case errors.Is(err, ErrValidation):
				writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "tên hoặc số ngày hết hạn không hợp lệ")
			case errors.Is(err, ErrConflict):
				writeError(w, http.StatusConflict, "CONFLICT", "tên token đã tồn tại")
			case errors.Is(err, ErrLimit):
				writeError(w, http.StatusBadRequest, "LIMIT_EXCEEDED", "tối đa 10 token còn hiệu lực")
			default:
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			}
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         tok.ID,
			"name":       tok.Name,
			"token":      plain,
			"last4":      tok.Last4,
			"expires_at": tok.ExpiresAt,
			"created_at": tok.CreatedAt,
		})
	}

	revokeToken := func(w http.ResponseWriter, req *http.Request) {
		userID, role, ok := authz(req)
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "chưa đăng nhập")
			return
		}
		if role != "ADMIN" {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "chỉ admin mới có quyền quản lý token")
			return
		}
		id := chi.URLParam(req, "id")
		if id == "" {
			parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
			if len(parts) > 0 {
				id = parts[len(parts)-1]
			}
		}
		err := s.Revoke(userID, id)
		if err != nil {
			switch {
			case errors.Is(err, ErrNotFound):
				writeError(w, http.StatusNotFound, "NOT_FOUND", "không tìm thấy token")
			case errors.Is(err, ErrForbidden):
				writeError(w, http.StatusForbidden, "FORBIDDEN", "không có quyền thu hồi token này")
			default:
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "revoked": true})
	}

	r.Get("/", listTokens)
	r.Get("/api/me/tokens", listTokens)
	r.Get("/api/me/tokens/", listTokens)

	r.Post("/", createToken)
	r.Post("/api/me/tokens", createToken)
	r.Post("/api/me/tokens/", createToken)

	r.Delete("/{id}", revokeToken)
	r.Delete("/api/me/tokens/{id}", revokeToken)

	return r
}
