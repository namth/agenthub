package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/gateway"
	"github.com/namth/agenthub/internal/server"
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

func main() {
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

	mounts := []server.Mount{{Pattern: "/ws/agent", Handler: hub}}
	if key := os.Getenv("RUNNER_REGISTRATION_KEY"); key != "" {
		mounts = append(mounts, server.Mount{Pattern: "/api/fleet/register", Handler: gateway.RegisterHandler(db, key)})
	} else {
		log.Println("RUNNER_REGISTRATION_KEY trống: tắt đăng ký máy mới")
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: server.New(db, web.Static(), mounts...),
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
