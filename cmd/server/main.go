package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
	"github.com/namth/agenthub/internal/mcp"
	"github.com/namth/agenthub/internal/office"
	"github.com/namth/agenthub/internal/pipeline"
	"github.com/namth/agenthub/internal/projects"
	"github.com/namth/agenthub/internal/runner"
	"github.com/namth/agenthub/internal/server"
	"github.com/namth/agenthub/internal/smoke"
	"github.com/namth/agenthub/internal/tasks"
	"github.com/namth/agenthub/internal/telegram"
	"github.com/namth/agenthub/internal/tokens"
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
	ghForTask := func(projectID string) tasks.GitHub {
		if clients != nil {
			if c := clients.GitHubOrNil(projectID); c != nil {
				return c
			}
		}
		return gh
	}
	plOpts := []pipeline.Option{
		pipeline.WithCIGate(os.Getenv("CI_GATE") == "true"),
		pipeline.WithOnNeedInfo(func(taskID string, round int, qs []pipeline.ClarifyQuestion) {
			var projectID, title string
			var issueNum sql.NullInt64
			err := db.QueryRow(`SELECT project_id, title, github_issue_id FROM tasks WHERE id = ?`, taskID).Scan(&projectID, &title, &issueNum)
			if err != nil {
				return
			}
			if issueNum.Valid && issueNum.Int64 > 0 {
				client := ghForTask(projectID)
				if commenter, ok := client.(tasks.Commenter); ok {
					body := formatClarifyComment(round, qs)
					_ = commenter.CommentIssue(context.Background(), int(issueNum.Int64), body)
				}
			}
			if tg != nil {
				tqs := make([]telegram.ClarifyQ, len(qs))
				for i, q := range qs {
					tqs[i] = telegram.ClarifyQ{
						ID:          q.ID,
						Question:    q.Question,
						Options:     q.Options,
						Recommended: q.Recommended,
					}
				}
				tg.NotifyClarifyQuestions(taskID, title, round, tqs)
			}
		}),
		pipeline.WithOnClarifyReminder(func(taskID string, round int, hours int) {
			var projectID, title string
			var issueNum sql.NullInt64
			err := db.QueryRow(`SELECT project_id, title, github_issue_id FROM tasks WHERE id = ?`, taskID).Scan(&projectID, &title, &issueNum)
			if err != nil {
				return
			}
			if tg != nil {
				tg.NotifyClarifyReminder(taskID, title, hours)
			}
			if issueNum.Valid && issueNum.Int64 > 0 {
				client := ghForTask(projectID)
				if commenter, ok := client.(tasks.Commenter); ok {
					body := fmt.Sprintf("⏰ **Nhắc làm rõ (%dh)**: Vui lòng trả lời câu hỏi vòng %d bằng cú pháp `/answer <id>: <giá trị>`.", hours, round)
					_ = commenter.CommentIssue(context.Background(), int(issueNum.Int64), body)
				}
			}
		}),
		pipeline.WithOnClarifyCapped(func(taskID string) {
			var title string
			err := db.QueryRow(`SELECT title FROM tasks WHERE id = ?`, taskID).Scan(&title)
			if err != nil {
				return
			}
			if tg != nil {
				tg.NotifyClarifyCapped(taskID, title)
			}
		}),
		pipeline.WithOnProposal(func(taskID, proposalID, reason string) {
			if tg != nil {
				var title string
				_ = db.QueryRow(`SELECT title FROM tasks WHERE id = ?`, taskID).Scan(&title)
				tg.NotifyProposal(taskID, title, proposalID, reason)
			}
		}),
		pipeline.WithOnReplanInvalid(func(taskID string) {
			if tg != nil {
				var title string
				_ = db.QueryRow(`SELECT title FROM tasks WHERE id = ?`, taskID).Scan(&title)
				tg.NotifyReplanInvalid(taskID, title)
			}
		}),
	}
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
		gateway.WithPayloadExtras(pl.PayloadFor),
		gateway.WithSubmitHook(func(workerID, assignmentID, prURL string, result json.RawMessage) {
			if err := pl.OnSubmitted(context.Background(), workerID, assignmentID, prURL, result); err != nil {
				log.Printf("pipeline: %s: %v", assignmentID, err)
			}
			if tg != nil {
				var taskID, title, pr, status string
				row := db.QueryRow(`
					SELECT t.id, t.title, COALESCE(t.pr_url, ''), t.status
					FROM assignments a
					JOIN jobs j ON a.job_id = j.id
					JOIN tasks t ON j.task_id = t.id
					WHERE a.id = ?
				`, assignmentID)
				if err := row.Scan(&taskID, &title, &pr, &status); err == nil && status == "QA_PASSED" {
					if pr == "" {
						pr = prURL
					}
					tg.NotifyQAPassed(taskID, title, pr)
				}
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
		tasks.WithOnClarified(func(taskID string, round int, byLogin, via string) {
			bus.Emit(office.Event{
				Who:  "user:" + byLogin,
				Type: "work",
				Task: taskID,
				Text: fmt.Sprintf("Làm rõ vòng %d (%s)", round, via),
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
	tokensStore := tokens.NewStore(db, time.Now)
	tokensAPI := tokens.Handler(tokensStore, userOf)
	mcpAPI := mcp.Handler(tokensStore, isOwnerID, newMCPBackend(db, tasksSvc, disp), time.Now)
	smokeOps := &smokeTaskOps{
		db:      db,
		tasks:   tasksSvc,
		clients: clients,
		gh:      gh,
		multi:   multi,
	}
	smokeSvc := smoke.New(db, smokeOps, time.Now)
	smokeAPI := smoke.Handler(smokeSvc, userOf)

	mounts := []server.Mount{
		{Pattern: "/mcp", Handler: mcpAPI},
		{Pattern: "/api/me/tokens", Handler: tokensAPI},
		{Pattern: "/api/me/tokens/*", Handler: tokensAPI},
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
		{Pattern: "/api/admin/projects/{id}/smoke-run", Handler: smokeAPI},
		{Pattern: "/api/admin/projects/{id}/smoke-runs", Handler: smokeAPI},
		{Pattern: "/api/admin/projects/{id}/smoke-runs/*", Handler: smokeAPI},
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
		commander := NewTelegramCommander(db, tasksSvc, sysOwner)
		mounts = append(mounts, server.Mount{Pattern: "/api/webhooks/telegram", Handler: telegram.WebhookHandlerWith(tgSender, tasksSvc, commander, tgChat, secret)})
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
			tasks.WithOnIssueComment(func(projectID string, issue int, login, body string) {
				answers, ok := tasks.ParseAnswerComment(body)
				if !ok {
					return
				}
				var taskID string
				err := db.QueryRow(`SELECT id FROM tasks WHERE project_id = ? AND github_issue_id = ?`, projectID, issue).Scan(&taskID)
				if err != nil {
					return
				}
				_ = tasksSvc.AnswerByLogin(context.Background(), login, taskID, answers)
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
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := pl.RemindClarifications(ctx, time.Now()); err != nil {
					log.Printf("pipeline remind clarifications: %v", err)
				} else if n > 0 {
					log.Printf("pipeline remind clarifications: nhắc %d vòng", n)
				}
			}
		}
	}()
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := smokeSvc.Track(ctx); err != nil {
					log.Printf("smoke track: %v", err)
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

func formatClarifyComment(round int, qs []pipeline.ClarifyQuestion) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### ❓ Câu hỏi làm rõ yêu cầu (vòng %d)\n\n", round))
	sb.WriteString("Trưởng phòng cần làm rõ các câu hỏi sau trước khi tiếp tục lập kế hoạch:\n\n")
	for i, q := range qs {
		sb.WriteString(fmt.Sprintf("%d. **%s** (`%s`)\n", i+1, q.Question, q.ID))
		if len(q.Options) > 0 {
			sb.WriteString("   - Các lựa chọn: ")
			var opts []string
			for _, opt := range q.Options {
				if opt == q.Recommended {
					opts = append(opts, fmt.Sprintf("`%s` (Đề xuất)", opt))
				} else {
					opts = append(opts, fmt.Sprintf("`%s`", opt))
				}
			}
			sb.WriteString(strings.Join(opts, ", ") + "\n")
		}
		if q.Reason != "" {
			sb.WriteString(fmt.Sprintf("   - Lý do: %s\n", q.Reason))
		}
	}
	sb.WriteString("\n---\n**Cách trả lời:** Bình luận tại issue này bằng cú pháp:\n```\n")
	for _, q := range qs {
		val := "<giá trị>"
		if q.Recommended != "" {
			val = q.Recommended
		} else if len(q.Options) > 0 {
			val = q.Options[0]
		}
		sb.WriteString(fmt.Sprintf("/answer %s: %s\n", q.ID, val))
	}
	sb.WriteString("/answer _notes: ghi chú thêm nếu có\n```\n")
	return sb.String()
}

type smokeTaskOps struct {
	db      *sql.DB
	tasks   *tasks.Service
	clients *repoClients
	gh      tasks.GitHub
	multi   bool
}

func (o *smokeTaskOps) CreateIn(ctx context.Context, userID, projectID, title, description string) (string, error) {
	t, err := o.tasks.CreateIn(ctx, userID, projectID, title, description)
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

func (o *smokeTaskOps) Approve(ctx context.Context, taskID string) error {
	return o.tasks.Approve(ctx, taskID)
}

func (o *smokeTaskOps) Cleanup(ctx context.Context, taskID string) error {
	var projectID string
	var issueID sql.NullInt64
	var prURL sql.NullString
	err := o.db.QueryRowContext(ctx, `SELECT project_id, github_issue_id, pr_url FROM tasks WHERE id = ?`, taskID).Scan(&projectID, &issueID, &prURL)
	if err != nil {
		return err
	}

	var ghClient tasks.GitHub
	if o.multi && o.clients != nil {
		ghClient = o.clients.GitHubOrNil(projectID)
	} else {
		ghClient = o.gh
	}

	if ghClient == nil {
		return nil
	}

	if issueID.Valid && issueID.Int64 > 0 {
		_ = ghClient.CloseIssue(ctx, int(issueID.Int64), "Smoke test kết thúc — đóng issue mẫu")
	}

	closePR := func(rawURL string) {
		trimmed := strings.TrimRight(rawURL, "/")
		parts := strings.Split(trimmed, "/")
		if len(parts) > 0 {
			if num, err := strconv.Atoi(parts[len(parts)-1]); err == nil && num > 0 {
				_ = ghClient.CloseIssue(ctx, num, "Smoke test kết thúc — đóng PR mẫu")
			}
		}
	}

	if prURL.Valid && prURL.String != "" {
		closePR(prURL.String)
	}

	subRows, err := o.db.QueryContext(ctx, `SELECT github_issue_id, pr_url FROM tasks WHERE parent_id = ?`, taskID)
	if err == nil {
		defer subRows.Close()
		for subRows.Next() {
			var subIssue sql.NullInt64
			var subPR sql.NullString
			if err := subRows.Scan(&subIssue, &subPR); err == nil {
				if subIssue.Valid && subIssue.Int64 > 0 {
					_ = ghClient.CloseIssue(ctx, int(subIssue.Int64), "Smoke test kết thúc — đóng sub-task issue mẫu")
				}
				if subPR.Valid && subPR.String != "" {
					closePR(subPR.String)
				}
			}
		}
	}

	return nil
}
