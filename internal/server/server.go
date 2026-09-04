package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"easy42/internal/crypto"
	"easy42/internal/engine"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Server encapsulates the HTTP REST API server and static frontend handler
type Server struct {
	mgr       *engine.Manager
	router    *chi.Mux
	distFS    fs.FS
	listen    string
	httpSrv   *http.Server
}

// Config provides options for server creation
type Config struct {
	ListenAddr string
	Manager    *engine.Manager
	DistFS     fs.FS
}

// New creates and configures the HTTP server router
func New(cfg Config) *Server {
	s := &Server{
		mgr:    cfg.Manager,
		distFS: cfg.DistFS,
		listen: cfg.ListenAddr,
	}

	r := chi.NewRouter()

	// Standard middlewares
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS for local development with Vite dev server
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:*", "http://127.0.0.1:*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// API Routes
	r.Route("/api", func(r chi.Router) {
		// Auth routes (public)
		r.Post("/auth/login", s.handleLogin)
		r.Get("/auth/status", s.handleAuthStatus)
		r.Post("/auth/logout", s.handleLogout)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)

			r.Post("/auth/unlock", s.handleUnlock)
			r.Post("/auth/lock", s.handleLock)
			r.Post("/auth/change-password", s.handleChangePassword)
			r.Post("/auth/logout-all", s.handleLogoutAll)

			// Nodes
			r.Get("/nodes", s.handleGetNodes)
			r.Post("/nodes", s.handleAddNode)
			r.Put("/nodes/{name}", s.handleUpdateNode)
			r.Put("/nodes/{name}/position", s.handleUpdateNodePosition)
			r.Patch("/nodes/{name}/position", s.handleUpdateNodePosition)
			r.Delete("/nodes/{name}", s.handleDeleteNode)
			r.Post("/nodes/probe", s.handleProbeNode)
			r.Get("/nodes/status", s.handleGetNodeStatuses)
			r.Post("/nodes/{name}/status", s.handleRefreshNodeStatus)
			r.Get("/nodes/{name}/bird", s.handleGetNodeBirdConfig)

			// Links
			r.Get("/links", s.handleGetLinks)
			r.Post("/links", s.handleAddLink)
			r.Post("/links/mesh", s.handleCreateMeshLinks)
			r.Put("/links", s.handleUpdateLink)
			r.Delete("/links", s.handleDeleteLink)

			// Network Settings
			r.Get("/settings/network", s.handleGetNetworkSettings)
			r.Put("/settings/network", s.handleUpdateNetworkSettings)

			// Sync & State
			r.Get("/sync/preview", s.handleSyncPreview)
			r.Post("/sync", s.handleExecuteSync)
			r.Get("/sync/status", s.handleSyncStatus)
			r.Post("/state/update", s.handleUpdateState)
			r.Get("/state", s.handleGetState)

			// Device Helper Tasks
			r.Get("/tasks", s.handleGetTasks)
			r.Post("/tasks/{id}/status", s.handleTaskStatus)
			r.Post("/tasks/{id}/run", s.handleTaskRun)
		})
	})

	// Static frontend routing
	if s.distFS != nil {
		fileServer := http.FileServer(http.FS(s.distFS))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path != "" {
				if _, err := fs.Stat(s.distFS, path); err == nil {
					fileServer.ServeHTTP(w, r)
					return
				}
			}
			// Fallback to index.html for SPA routing
			indexData, err := fs.ReadFile(s.distFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(indexData)
		})
	}

	s.router = r
	return s
}

// Start launches the HTTP server
func (s *Server) Start() error {
	s.httpSrv = &http.Server{
		Addr:         s.listen,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	return s.httpSrv.ListenAndServe()
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

// JSON helpers
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// Auth Middleware
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("easy42_session")
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "Authentication required")
			return
		}

		cfg := s.mgr.Store().Get()
		if cfg == nil {
			var loadErr error
			cfg, loadErr = s.mgr.Store().Load()
			if loadErr != nil {
				writeError(w, http.StatusInternalServerError, "Config store error")
				return
			}
		}

		if !crypto.VerifySessionToken(cookie.Value, cfg.SessionSecret, cfg.PasswordHash) {
			writeError(w, http.StatusUnauthorized, "Invalid or expired session")
			return
		}

		next.ServeHTTP(w, r)
	})
}
