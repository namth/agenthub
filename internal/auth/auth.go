package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type User struct {
	ID        string `json:"id"`
	GitHubID  string `json:"github_id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	Role      string `json:"role"`
}

type Config struct {
	ClientID     string
	ClientSecret string
	BaseURL      string
	JWTSecret    string
	RepoOwner    string
	RepoName     string
	ServerToken  string
	GitHubWebURL string
	GitHubAPIURL string
	SessionTTL   time.Duration
	HTTPClient   *http.Client
	Now          func() time.Time
}

type Service struct {
	db  *sql.DB
	cfg Config
}

func New(db *sql.DB, cfg Config) *Service {
	if cfg.GitHubWebURL == "" {
		cfg.GitHubWebURL = "https://github.com"
	}
	cfg.GitHubWebURL = strings.TrimRight(cfg.GitHubWebURL, "/")

	if cfg.GitHubAPIURL == "" {
		cfg.GitHubAPIURL = "https://api.github.com"
	}
	cfg.GitHubAPIURL = strings.TrimRight(cfg.GitHubAPIURL, "/")

	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 7 * 24 * time.Hour
	}

	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}

	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	return &Service{
		db:  db,
		cfg: cfg,
	}
}

type userCtxKey struct{}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userCtxKey{}, u)
}

func UserFrom(ctx context.Context) (User, bool) {
	if ctx == nil {
		return User{}, false
	}
	u, ok := ctx.Value(userCtxKey{}).(User)
	return u, ok
}

func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := UserFrom(r.Context())
			if !ok {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
				return
			}
			allowed := false
			for _, role := range roles {
				if u.Role == role {
					allowed = true
					break
				}
			}
			if !allowed {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
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

func writeSuccess(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Success: true,
		Data:    data,
		Error:   nil,
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiResponse{
		Success: false,
		Data:    nil,
		Error: &apiError{
			Code:    code,
			Message: message,
		},
	})
}

type Claims struct {
	Role     string `json:"role"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func (s *Service) isSecure() bool {
	return strings.HasPrefix(s.cfg.BaseURL, "https://")
}

func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFrom(r.Context()); ok {
			next.ServeHTTP(w, r)
			return
		}

		ck, err := r.Cookie("agenthub_session")
		if err != nil || ck.Value == "" {
			next.ServeHTTP(w, r)
			return
		}

		var claims Claims
		token, err := jwt.ParseWithClaims(ck.Value, &claims, func(t *jwt.Token) (any, error) {
			if t.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(s.cfg.JWTSecret), nil
		}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(s.cfg.Now))

		if err != nil || !token.Valid || claims.Subject == "" {
			next.ServeHTTP(w, r)
			return
		}

		var u User
		err = s.db.QueryRowContext(r.Context(), `
			SELECT id, github_id, username, coalesce(avatar_url, ''), role
			FROM users
			WHERE id = ?
		`, claims.Subject).Scan(&u.ID, &u.GitHubID, &u.Username, &u.AvatarURL, &u.Role)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		ctx := WithUser(r.Context(), u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Service) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(s.Middleware)

	for _, prefix := range []string{"", "/api/auth"} {
		p := prefix
		r.Get(p+"/github", s.handleGitHub)
		r.Get(p+"/callback", s.handleCallback)
		r.Get(p+"/me", s.handleMe)
		r.Post(p+"/logout", s.handleLogout)
	}
	return r
}

func (s *Service) handleGitHub(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate oauth state")
		return
	}
	state := hex.EncodeToString(b)

	http.SetCookie(w, &http.Cookie{
		Name:     "agenthub_oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   10 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isSecure(),
	})

	redirectURI := strings.TrimRight(s.cfg.BaseURL, "/") + "/api/auth/callback"
	q := url.Values{}
	q.Set("client_id", s.cfg.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("scope", "read:user")
	q.Set("state", state)

	target := s.cfg.GitHubWebURL + "/login/oauth/authorize?" + q.Encode()
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Service) handleCallback(w http.ResponseWriter, r *http.Request) {
	stateQuery := r.URL.Query().Get("state")
	stateCookie, err := r.Cookie("agenthub_oauth_state")
	if stateQuery == "" || err != nil || stateCookie.Value == "" || subtle.ConstantTimeCompare([]byte(stateQuery), []byte(stateCookie.Value)) != 1 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid or missing oauth state")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing code")
		return
	}

	tokenURL := s.cfg.GitHubWebURL + "/login/oauth/access_token"
	form := url.Values{}
	form.Set("client_id", s.cfg.ClientID)
	form.Set("client_secret", s.cfg.ClientSecret)
	form.Set("code", code)

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "failed to create token request")
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.cfg.HTTPClient.Do(req)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "failed to exchange code for token")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "failed to exchange code for token")
		return
	}

	var tokenResult struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResult); err != nil || tokenResult.Error != "" || tokenResult.AccessToken == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "failed to exchange code for token")
		return
	}

	userReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, s.cfg.GitHubAPIURL+"/user", nil)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "failed to create user request")
		return
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenResult.AccessToken)
	userReq.Header.Set("Accept", "application/json")

	userResp, err := s.cfg.HTTPClient.Do(userReq)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "failed to fetch user profile")
		return
	}
	defer userResp.Body.Close()

	if userResp.StatusCode != http.StatusOK {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "failed to fetch user profile")
		return
	}

	var ghUser struct {
		ID        any    `json:"id"`
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&ghUser); err != nil || ghUser.Login == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "failed to parse user profile")
		return
	}

	var ghIDStr string
	switch v := ghUser.ID.(type) {
	case float64:
		ghIDStr = strconv.FormatInt(int64(v), 10)
	case int:
		ghIDStr = strconv.Itoa(v)
	case int64:
		ghIDStr = strconv.FormatInt(v, 10)
	case string:
		ghIDStr = v
	default:
		ghIDStr = fmt.Sprintf("%v", v)
	}
	if ghIDStr == "" || ghIDStr == "<nil>" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid github user id")
		return
	}

	permURL := fmt.Sprintf("%s/repos/%s/%s/collaborators/%s/permission", s.cfg.GitHubAPIURL, s.cfg.RepoOwner, s.cfg.RepoName, url.PathEscape(ghUser.Login))
	permReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, permURL, nil)
	if err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "failed to check permission")
		return
	}
	permReq.Header.Set("Authorization", "Bearer "+s.cfg.ServerToken)
	permReq.Header.Set("Accept", "application/json")

	permResp, err := s.cfg.HTTPClient.Do(permReq)
	if err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "failed to check permission")
		return
	}
	defer permResp.Body.Close()

	if permResp.StatusCode == http.StatusNotFound {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "user is not a collaborator")
		return
	}
	if permResp.StatusCode != http.StatusOK {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "collaborator check failed")
		return
	}

	var permResult struct {
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(permResp.Body).Decode(&permResult); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "failed to parse permission")
		return
	}

	role := "PARTNER"
	switch permResult.Permission {
	case "admin", "maintain":
		role = "ADMIN"
	}

	var userID string
	err = s.db.QueryRowContext(r.Context(), `SELECT id FROM users WHERE github_id = ?`, ghIDStr).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		userID = uuid.NewString()
		_, err = s.db.ExecContext(r.Context(), `
			INSERT INTO users (id, github_id, username, avatar_url, role)
			VALUES (?, ?, ?, ?, ?)
		`, userID, ghIDStr, ghUser.Login, ghUser.AvatarURL, role)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to create user")
			return
		}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "database error")
		return
	} else {
		_, err = s.db.ExecContext(r.Context(), `
			UPDATE users
			SET username = ?, avatar_url = ?, role = ?
			WHERE id = ?
		`, ghUser.Login, ghUser.AvatarURL, role, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to update user")
			return
		}
	}

	now := s.cfg.Now()
	claims := Claims{
		Role:     role,
		Username: ghUser.Login,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.cfg.SessionTTL)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to sign session token")
		return
	}

	isSecure := s.isSecure()

	http.SetCookie(w, &http.Cookie{
		Name:     "agenthub_session",
		Value:    signedToken,
		Path:     "/",
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "agenthub_oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecure,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Service) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
		return
	}

	var fresh User
	err := s.db.QueryRowContext(r.Context(), `
		SELECT id, github_id, username, coalesce(avatar_url, ''), role
		FROM users
		WHERE id = ?
	`, u.ID).Scan(&fresh.ID, &fresh.GitHubID, &fresh.Username, &fresh.AvatarURL, &fresh.Role)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not found")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{
		"id":         fresh.ID,
		"username":   fresh.Username,
		"avatar_url": fresh.AvatarURL,
		"role":       fresh.Role,
	})
}

func (s *Service) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "agenthub_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.isSecure(),
	})
	writeSuccess(w, http.StatusOK, nil)
}
