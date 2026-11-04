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
	"github.com/namth/agenthub/internal/contrib"
	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
	"github.com/namth/agenthub/internal/logs"
	"github.com/namth/agenthub/internal/machines"
	"github.com/namth/agenthub/internal/office"
	"github.com/namth/agenthub/internal/pipeline"
	"github.com/namth/agenthub/internal/projects"
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
	ghAPI, ghToken := os.Getenv("GITHUB_API_URL"), os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	// Chế độ đa dự án (FEAT-04) bật khi biết quản trị hệ thống (GITHUB_OWNER, mặc định = GITHUB_REPO_OWNER).
	sysOwner := envOr("GITHUB_OWNER", owner)
	multi := sysOwner != ""
	projectID, err := ensureProject(db, owner, repo)
	if err != nil {
		log.Fatalf("project: %v", err)
	}
	gh := tasks.NewGitHubClient(ghAPI, owner, repo, ghToken, nil)
	clients := newRepoClients(db, ghAPI, ghToken)
	isOwnerID := ownerCheck(db, sysOwner)
	roleIn := memberRole(db)

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
	plOpts := []pipeline.Option{pipeline.WithCIGate(os.Getenv("CI_GATE") == "true")}
	if multi {
		plOpts = append(plOpts, pipeline.WithGitHubFor(clients.GitHubOrNil))
	}
	pl = pipeline.New(db, disp, gh, projectID, plOpts...)
	logStore := logs.NewStore(db, time.Now, 5000)
	hubOpts := []gateway.Option{
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
		}),
		gateway.WithServerVersion(runner.Version),
		gateway.WithVersionSink(func(workerID, version string) {
			if _, err := db.Exec(`UPDATE workers SET version = ? WHERE id = ?`, version, workerID); err != nil {
				log.Printf("version %s: %v", workerID, err)
			}
		}),
	}
	if multi {
		hubOpts = append(hubOpts, gateway.WithRepo(pl.RepoFor))
	}
	hub = gateway.New(disp, gateway.DBAuthenticator(db), hubOpts...)
	defer hub.Close()

	baseURL := baseURLOf(port)
	projSvc := projects.New(db, projects.NewGitHub(ghAPI, ghToken, nil), projects.Config{
		Owner:         sysOwner,
		BaseURL:       baseURL(),
		WebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
	})

	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		log.Println("JWT_SECRET ngắn hơn 32 ký tự: dùng khoá ngẫu nhiên tạm, phiên mất khi khởi động lại")
		jwtSecret = randomHex(32)
	}
	authCfg := auth.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		BaseURL:      os.Getenv("BASE_URL"),
		JWTSecret:    jwtSecret,
		RepoOwner:    owner,
		RepoName:     repo,
		ServerToken:  ghToken,
		GitHubAPIURL: ghAPI,
	}
	if multi {
		authCfg.Owner = sysOwner
		authCfg.OnLogin = projSvc.SyncUser
	}
	authSvc := auth.New(db, authCfg)
	roleOf := func(r *http.Request) (string, bool) {
		u, ok := auth.UserFrom(r.Context())
		return u.Role, ok
	}
	userOf := func(r *http.Request) (string, string, bool) {
		u, ok := auth.UserFrom(r.Context())
		return u.ID, u.Role, ok
	}
	fleetAPI := api.FleetHandler(db, disp, roleOf)
	presence := office.NewPresence(db, time.Now, bus, isOwnerID)
	taskOpts := []tasks.Option{
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
		}),
		tasks.WithOnApproved(func(taskID, userID string) {
			var username string
			err := db.QueryRow(`SELECT username FROM users WHERE id = ?`, userID).Scan(&username)
			if err != nil || username == "" {
				username = userID
			}
			bus.Emit(office.Event{
				Who:  "user:" + username,
				Type: "approve",
				Task: taskID,
			})
		}),
	}
	var tasksSvc *tasks.Service
	if multi {
		tasksSvc = tasks.NewMultiService(db, clients.GitHub, disp, append(taskOpts,
			tasks.WithMergerFor(clients.Merger),
			tasks.WithOwner(isOwnerID),
			tasks.WithRoleIn(roleIn))...)
	} else {
		tasksSvc = tasks.NewService(db, gh, disp, projectID, append(taskOpts,
			tasks.WithMerger(tasks.NewGitHubMerger(ghAPI, owner, repo, ghToken, nil)))...)
	}
	tasksAPI := tasks.HandlerWithService(tasksSvc, userOf)
	officeOpt := office.Options{
		User: func(r *http.Request) (string, bool) {
			u, ok := auth.UserFrom(r.Context())
			return u.ID, ok
		},
		Presence:        presence,
		TelegramEnabled: tgSender != nil,
	}
	officeAPI := office.Handler(db, bus, func() (any, error) {
		lease, err := disp.Lead()
		if err != nil {
			return nil, err
		}
		return office.BuildSnapshot(db, lease, presence)
	}, roleOf, officeOpt)
	projectsAPI := projects.Handler(projSvc, func(r *http.Request) (string, string, bool) {
		u, ok := auth.UserFrom(r.Context())
		return u.ID, u.Username, ok
	})
	machinesAPI := machines.Handler(db, machines.Config{
		BaseURL:    baseURL,
		RunnersDir: envOr("RUNNERS_DIR", "./runners"),
		IsOwner:    isOwnerID,
		IsMember: func(userID, pid string) bool {
			if isOwnerID(userID) {
				return true
			}
			_, ok := roleIn(userID, pid)
			return ok
		},
		OnRemoved: func(workerID string) {
			if !hub.CloseWorker(workerID) {
				_ = disp.Disconnect(workerID)
			}
		},
		Recheck: hub.SendAuthRecheck,
	}, func(r *http.Request) (string, bool) {
		u, ok := auth.UserFrom(r.Context())
		return u.ID, ok
	})
	contribAPI := contrib.Handler(db, func(r *http.Request) (string, bool, bool) {
		u, ok := auth.UserFrom(r.Context())
		return u.ID, ok && isOwnerID(u.ID), ok
	})

	mounts := []server.Mount{
		{Pattern: "/ws/agent", Handler: hub},
		{Pattern: "/api/auth/*", Handler: authSvc.Routes()},
		{Pattern: "/api/fleet", Handler: fleetAPI},
		{Pattern: "/api/fleet/join-codes", Handler: machinesAPI},
		{Pattern: "/api/fleet/join", Handler: machinesAPI},
		{Pattern: "/api/my/machines", Handler: machinesAPI},
		{Pattern: "/api/my/machines/*", Handler: machinesAPI},
		{Pattern: "/install.sh", Handler: machinesAPI},
		{Pattern: "/download/*", Handler: machinesAPI},
		{Pattern: "/api/admin/lead/*", Handler: fleetAPI},
		{Pattern: "/api/admin/agents/*", Handler: fleetAPI},
		{Pattern: "/api/projects", Handler: projectsAPI},
		{Pattern: "/api/admin/projects", Handler: projectsAPI},
		{Pattern: "/api/admin/projects/*", Handler: projectsAPI},
		{Pattern: "/api/admin/github/repos", Handler: projectsAPI},
		{Pattern: "/api/contributions", Handler: contribAPI},
		{Pattern: "/api/contributions/*", Handler: contribAPI},
		{Pattern: "/api/tasks", Handler: tasksAPI},
		{Pattern: "/api/tasks/*", Handler: tasksAPI},
		{Pattern: "/api/admin/settings", Handler: officeAPI},
		{Pattern: "/api/admin/office/*", Handler: officeAPI},
		{Pattern: "/api/office/*", Handler: officeAPI},
		{Pattern: "/api/admin/workers/*", Handler: logs.Handler(db, logStore, roleOf)},
	}
	if secret := os.Getenv("TELEGRAM_WEBHOOK_SECRET"); secret != "" && tgSender != nil {
		mounts = append(mounts, server.Mount{Pattern: "/api/webhooks/telegram", Handler: telegram.WebhookHandler(tgSender, tasksSvc, tgChat, secret)})
	}
	if secret := os.Getenv("GITHUB_WEBHOOK_SECRET"); secret != "" {
		whOpts := []tasks.WebhookOption{
			tasks.OnWorkflowRun(func(branch, conclusion, url string) {
				if err := pl.OnCI(context.Background(), branch, conclusion, url); err != nil {
					log.Printf("pipeline ci: %v", err)
				}
			}),
			tasks.OnPullRequestMerged(func(prURL string) {
				if err := pl.OnMerged(context.Background(), prURL); err != nil {
					log.Printf("pipeline merged: %v", err)
				}
			}),
		}
		var wh http.Handler
		if multi {
			wh = tasks.WebhookHandlerMulti(db, secret, projectOfRepo(db), whOpts...)
		} else {
			wh = tasks.WebhookHandler(db, secret, projectID, whOpts...)
		}
		mounts = append(mounts, server.Mount{Pattern: "/api/webhooks/github", Handler: wh})
	} else {
		log.Println("GITHUB_WEBHOOK_SECRET trống: tắt webhook GitHub")
	}
	if key := os.Getenv("RUNNER_REGISTRATION_KEY"); key != "" {
		mounts = append(mounts, server.Mount{Pattern: "/api/fleet/register", Handler: gateway.RegisterHandler(db, key)})
	} else {
		log.Println("RUNNER_REGISTRATION_KEY trống: chỉ thêm máy bằng mã tham gia")
	}
	log.Printf("AgentHub %s · đa dự án=%v (quản trị: %q)", runner.Version, multi, sysOwner)

	presenceMW := presence.Middleware(func(r *http.Request) (string, bool) {
		u, ok := auth.UserFrom(r.Context())
		return u.ID, ok
	})

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: authSvc.Middleware(presenceMW(server.New(db, web.Static(), mounts...))),
	}

	ctx, stopTick := context.WithCancel(context.Background())
	go func() {
		t := time.NewTicker(60 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				presence.Tick()
			}
		}
	}()
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
