package server

import (
	"database/sql"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// New creates and returns the HTTP handler for AgentHub.
func New(db *sql.DB, static fs.FS) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)

	// API Health check
	r.Get("/api/health", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":true,"data":{"status":"ok"},"error":null}`))
	})

	apiNotFound := func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"success":false,"data":null,"error":{"code":"NOT_FOUND","message":"endpoint not found"}}`))
	}

	r.HandleFunc("/api", apiNotFound)
	r.HandleFunc("/api/*", apiNotFound)

	serveStaticOrSPA := func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api" || strings.HasPrefix(req.URL.Path, "/api/") {
			apiNotFound(w, req)
			return
		}

		trimmed := strings.TrimPrefix(req.URL.Path, "/")
		if trimmed != "" {
			if f, err := static.Open(trimmed); err == nil {
				defer f.Close()
				if fi, err := f.Stat(); err == nil && !fi.IsDir() {
					http.FileServer(http.FS(static)).ServeHTTP(w, req)
					return
				}
			}
		}

		data, err := fs.ReadFile(static, "index.html")
		if err != nil {
			http.NotFound(w, req)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	}

	r.NotFound(serveStaticOrSPA)
	r.MethodNotAllowed(serveStaticOrSPA)
	r.Get("/*", serveStaticOrSPA)

	return r
}
