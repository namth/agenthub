package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/namth/agenthub/internal/api"
	"github.com/namth/agenthub/internal/auth"
	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
	"github.com/namth/agenthub/internal/runner"
	"github.com/namth/agenthub/internal/server"
	"github.com/namth/agenthub/internal/tasks"
	"github.com/namth/agenthub/web"
)

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// notifierFunc cho phép tạo dispatcher trước khi có Hub (hai bên phụ thuộc vòng).
type notifierFunc func(dispatcher.Event)

func (f notifierFunc) Notify(e dispatcher.Event) { f(e) }

func envDuration(key string, unit time.Duration, def time.Duration) time.Duration {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return time.Duration(v) * unit
	}
	return def
}

// configFromEnv đọc các ngưỡng FEAT-01 (docs/10 mục LEAD ROTATION & FAILOVER).
func configFromEnv() dispatcher.Config {
	c := dispatcher.DefaultConfig()
	c.LeaseTTL = envDuration("LEAD_LEASE_TTL_SEC", time.Second, c.LeaseTTL)
	c.OfflineAfter = envDuration("HEARTBEAT_OFFLINE_SEC", time.Second, c.OfflineAfter)
	c.ReclaimAfter = envDuration("RECLAIM_AFTER_SEC", time.Second, c.ReclaimAfter)
	c.LogIdleWarn = envDuration("LOG_IDLE_WARN_MIN", time.Minute, c.LogIdleWarn)
	c.DefaultCooldown = envDuration("DEFAULT_COOLDOWN_MIN", time.Minute, c.DefaultCooldown)
	if v, err := strconv.Atoi(os.Getenv("MAX_ATTEMPTS")); err == nil && v > 0 {
		c.MaxAttempts = v
	}
	return c
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("crypto/rand: %v", err)
	}
	return hex.EncodeToString(b)
}

// ensureProject đảm bảo có bản ghi projects cho repo đang cấu hình, trả về id.
func ensureProject(db *sql.DB, owner, repo string) (string, error) {
	full := owner + "/" + repo
	if owner == "" || repo == "" {
		full = "local/agenthub"
	}
	var id string
	err := db.QueryRow(`SELECT id FROM projects WHERE github_repo = ?`, full).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id = randomHex(16)
		_, err = db.Exec(`INSERT INTO projects (id, name, github_repo) VALUES (?, ?, ?)`, id, repo, full)
	}
	return id, err
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "runner" {
		os.Exit(runner.Main(os.Args[2:]))
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		dbPath = "./data/agenthub.db"
	}

	db, err := database.Open(dbPath)
	if err != nil {
		log.Fatalf("failed to open database at %q: %v", dbPath, err)
	}
	defer db.Close()

	var hub *gateway.Hub
	disp := dispatcher.New(db, realClock{}, configFromEnv(), notifierFunc(func(e dispatcher.Event) {
		if hub != nil {
			hub.Notify(e)
		}
	}))
	hub = gateway.New(disp, gateway.DBAuthenticator(db))
	defer hub.Close()

	owner, repo := os.Getenv("GITHUB_REPO_OWNER"), os.Getenv("GITHUB_REPO_NAME")
	projectID, err := ensureProject(db, owner, repo)
	if err != nil {
		log.Fatalf("project: %v", err)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		log.Println("JWT_SECRET ngắn hơn 32 ký tự: dùng khoá ngẫu nhiên tạm, phiên mất khi khởi động lại")
		jwtSecret = randomHex(32)
	}
	authSvc := auth.New(db, auth.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		BaseURL:      os.Getenv("BASE_URL"),
		JWTSecret:    jwtSecret,
		RepoOwner:    owner,
		RepoName:     repo,
		ServerToken:  os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN"),
	})
	roleOf := func(r *http.Request) (string, bool) {
		u, ok := auth.UserFrom(r.Context())
		return u.Role, ok
	}
	userOf := func(r *http.Request) (string, string, bool) {
		u, ok := auth.UserFrom(r.Context())
		return u.ID, u.Role, ok
	}
	gh := tasks.NewGitHubClient("", owner, repo, os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN"), nil)
	fleetAPI := api.FleetHandler(db, disp, roleOf)
	tasksAPI := tasks.Handler(db, gh, disp, projectID, userOf)

	mounts := []server.Mount{
		{Pattern: "/ws/agent", Handler: hub},
		{Pattern: "/api/auth/*", Handler: authSvc.Routes()},
		{Pattern: "/api/fleet", Handler: fleetAPI},
		{Pattern: "/api/admin/lead/*", Handler: fleetAPI},
		{Pattern: "/api/admin/agents/*", Handler: fleetAPI},
		{Pattern: "/api/tasks", Handler: tasksAPI},
		{Pattern: "/api/tasks/*", Handler: tasksAPI},
	}
	if secret := os.Getenv("GITHUB_WEBHOOK_SECRET"); secret != "" {
		mounts = append(mounts, server.Mount{Pattern: "/api/webhooks/github", Handler: tasks.WebhookHandler(db, secret, projectID)})
	} else {
		log.Println("GITHUB_WEBHOOK_SECRET trống: tắt webhook GitHub")
	}
	if key := os.Getenv("RUNNER_REGISTRATION_KEY"); key != "" {
		mounts = append(mounts, server.Mount{Pattern: "/api/fleet/register", Handler: gateway.RegisterHandler(db, key)})
	} else {
		log.Println("RUNNER_REGISTRATION_KEY trống: tắt đăng ký máy mới")
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: authSvc.Middleware(server.New(db, web.Static(), mounts...)),
	}

	ctx, stopTick := context.WithCancel(context.Background())
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := disp.Tick(); err != nil {
					log.Printf("dispatcher tick: %v", err)
				}
			}
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("AgentHub server ready and listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down AgentHub server gracefully...")
	stopTick()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown warning: %v", err)
	}
	log.Println("Server stopped")
}
