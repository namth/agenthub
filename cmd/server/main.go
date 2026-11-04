package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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
	"github.com/namth/agenthub/internal/logs"
	"github.com/namth/agenthub/internal/office"
	"github.com/namth/agenthub/internal/pipeline"
	"github.com/namth/agenthub/internal/runner"
	"github.com/namth/agenthub/internal/server"
	"github.com/namth/agenthub/internal/tasks"
	"github.com/namth/agenthub/internal/telegram"
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

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// fleetSnapshot dựng ảnh chụp cụm máy cho văn phòng ảo (cùng dạng lead/workers với GET /api/fleet, rút gọn).
func fleetSnapshot(db *sql.DB, disp dispatcher.API) (any, error) {
	lease, err := disp.Lead()
	if err != nil {
		return nil, err
	}
	var leadID any
	if lease.AgentID != "" {
		leadID = lease.AgentID
	}
	type agent struct {
		ID            string  `json:"id"`
		Kind          string  `json:"kind"`
		Status        string  `json:"status"`
		CooldownUntil *string `json:"cooldown_until"`
	}
	type worker struct {
		ID         string  `json:"id"`
		ConnStatus string  `json:"conn_status"`
		Agents     []agent `json:"agents"`
	}
	workers := []*worker{}
	byID := map[string]*worker{}
	rows, err := db.Query(`SELECT id, conn_status FROM workers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		w := &worker{Agents: []agent{}}
		if err := rows.Scan(&w.ID, &w.ConnStatus); err != nil {
			rows.Close()
			return nil, err
		}
		workers = append(workers, w)
		byID[w.ID] = w
	}
	rows.Close()
	rows, err = db.Query(`SELECT id, worker_id, kind, status, cooldown_until FROM agents ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a agent
		var wid string
		var cu sql.NullString
		if err := rows.Scan(&a.ID, &wid, &a.Kind, &a.Status, &cu); err != nil {
			return nil, err
		}
		if cu.Valid {
			if t, err := time.Parse("2006-01-02 15:04:05", cu.String); err == nil {
				v := t.UTC().Format(time.RFC3339)
				a.CooldownUntil = &v
			}
		}
		if w := byID[wid]; w != nil {
			w.Agents = append(w.Agents, a)
		}
	}
	return map[string]any{
		"lead":    map[string]any{"state": lease.State, "agent_id": leadID, "epoch": lease.Epoch},
		"workers": workers,
	}, rows.Err()
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

	loc, err := time.LoadLocation(envOr("TZ_DISPLAY", "Asia/Ho_Chi_Minh"))
	if err != nil {
		loc = time.UTC
	}
	bus := office.NewBus(400, time.Now, loc)

	var tg *telegram.Notifier
	tgToken, tgChat := os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_ADMIN_CHAT_ID")
	var tgSender telegram.Sender
	if tgToken != "" && tgChat != "" {
		tgSender = telegram.NewBotAPI(tgToken, "", nil)
		tg = telegram.NewNotifier(tgSender, tgChat, loc)
		defer tg.Close()
	} else {
		log.Println("TELEGRAM_BOT_TOKEN/TELEGRAM_ADMIN_CHAT_ID trống: tắt thông báo Telegram")
	}

	owner, repo := os.Getenv("GITHUB_REPO_OWNER"), os.Getenv("GITHUB_REPO_NAME")
	projectID, err := ensureProject(db, owner, repo)
	if err != nil {
		log.Fatalf("project: %v", err)
	}
	gh := tasks.NewGitHubClient(os.Getenv("GITHUB_API_URL"), owner, repo, os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN"), nil)

	var hub *gateway.Hub
	var pl *pipeline.Pipeline
	disp := dispatcher.New(db, realClock{}, configFromEnv(), notifierFunc(func(e dispatcher.Event) {
		if hub != nil {
			hub.Notify(e)
		}
		if pl != nil {
			pl.Notify(e)
		}
		bus.Notify(e)
		if tg != nil {
			tg.Notify(e)
		}
	}))
	pl = pipeline.New(db, disp, gh, projectID, pipeline.WithCIGate(os.Getenv("CI_GATE") == "true"))
	logStore := logs.NewStore(db, time.Now, 5000)
	hub = gateway.New(disp, gateway.DBAuthenticator(db),
		gateway.WithLogSink(func(workerID, assignmentID, chunk string) {
			if err := logStore.Append(workerID, assignmentID, chunk); err != nil {
				log.Printf("logs: %v", err)
			}
		}),
		gateway.WithPrompt(pl.PromptFor),
		gateway.WithSubmitHook(func(workerID, assignmentID, prURL string, result json.RawMessage) {
			if err := pl.OnSubmitted(context.Background(), workerID, assignmentID, prURL, result); err != nil {
				log.Printf("pipeline: %s: %v", assignmentID, err)
			}
		}))
	defer hub.Close()

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
		GitHubAPIURL: os.Getenv("GITHUB_API_URL"),
	})
	roleOf := func(r *http.Request) (string, bool) {
		u, ok := auth.UserFrom(r.Context())
		return u.Role, ok
	}
	userOf := func(r *http.Request) (string, string, bool) {
		u, ok := auth.UserFrom(r.Context())
		return u.ID, u.Role, ok
	}
	fleetAPI := api.FleetHandler(db, disp, roleOf)
	tasksSvc := tasks.NewService(db, gh, disp, projectID,
		tasks.WithMerger(tasks.NewGitHubMerger(os.Getenv("GITHUB_API_URL"), owner, repo, os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN"), nil)),
		tasks.WithOnMerged(func(prURL string) {
			if err := pl.OnMerged(context.Background(), prURL); err != nil {
				log.Printf("pipeline merged: %v", err)
			}
		}),
		tasks.WithOnCreated(func(c tasks.Created) {
			bus.Emit(office.Event{Who: "admin", Type: "work", Task: c.TaskID, Text: "Yêu cầu mới: " + c.Title})
			if tg != nil {
				tg.NotifyNewTask(c.TaskID, c.Issue, c.Title, c.Requester, c.Description)
			}
		}))
	tasksAPI := tasks.HandlerWithService(tasksSvc, userOf)
	officeAPI := office.Handler(db, bus, func() (any, error) { return fleetSnapshot(db, disp) }, roleOf, office.Options{})

	mounts := []server.Mount{
		{Pattern: "/ws/agent", Handler: hub},
		{Pattern: "/api/auth/*", Handler: authSvc.Routes()},
		{Pattern: "/api/fleet", Handler: fleetAPI},
		{Pattern: "/api/admin/lead/*", Handler: fleetAPI},
		{Pattern: "/api/admin/agents/*", Handler: fleetAPI},
		{Pattern: "/api/tasks", Handler: tasksAPI},
		{Pattern: "/api/tasks/*", Handler: tasksAPI},
		{Pattern: "/api/admin/settings", Handler: officeAPI},
		{Pattern: "/api/admin/office/*", Handler: officeAPI},
		{Pattern: "/api/admin/workers/*", Handler: logs.Handler(db, logStore, roleOf)},
	}
	if secret := os.Getenv("TELEGRAM_WEBHOOK_SECRET"); secret != "" && tgSender != nil {
		mounts = append(mounts, server.Mount{Pattern: "/api/webhooks/telegram", Handler: telegram.WebhookHandler(tgSender, tasksSvc, tgChat, secret)})
	}
	if secret := os.Getenv("GITHUB_WEBHOOK_SECRET"); secret != "" {
		mounts = append(mounts, server.Mount{Pattern: "/api/webhooks/github", Handler: tasks.WebhookHandler(db, secret, projectID,
			tasks.OnWorkflowRun(func(branch, conclusion, url string) {
				if err := pl.OnCI(context.Background(), branch, conclusion, url); err != nil {
					log.Printf("pipeline ci: %v", err)
				}
			}),
			tasks.OnPullRequestMerged(func(prURL string) {
				if err := pl.OnMerged(context.Background(), prURL); err != nil {
					log.Printf("pipeline merged: %v", err)
				}
			}))})
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
		n := 0
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := disp.Tick(); err != nil {
					log.Printf("dispatcher tick: %v", err)
				}
				if n++; n%6 == 0 {
					if made, err := pl.Reconcile(ctx); err != nil {
						log.Printf("pipeline reconcile: %v", err)
					} else if made > 0 {
						log.Printf("pipeline reconcile: bù %d job", made)
					}
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
