package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"easy42/internal/crypto"
	"easy42/internal/engine"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// Server encapsulates the HTTP REST API server and static frontend handler
type Server struct {
	mgr         *engine.Manager
	router      *chi.Mux
	distFS      fs.FS
	listen      string
	listenAddrs []string
	httpSrv     *http.Server
	listeners   []net.Listener
	closed      bool
	mu          sync.Mutex
}

// Config provides options for server creation
type Config struct {
	ListenAddr  string
	ListenAddrs []string
	Manager     *engine.Manager
	DistFS      fs.FS
}

// New creates and configures the HTTP server router
func New(cfg Config) *Server {
	var rawAddrs []string
	if len(cfg.ListenAddrs) > 0 {
		rawAddrs = append(rawAddrs, cfg.ListenAddrs...)
	}
	if cfg.ListenAddr != "" {
		rawAddrs = append(rawAddrs, cfg.ListenAddr)
	}

	var parsedAddrs []string
	for _, raw := range rawAddrs {
		for _, part := range strings.Split(raw, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				parsedAddrs = append(parsedAddrs, trimmed)
			}
		}
	}

	seen := make(map[string]bool)
	var finalAddrs []string
	for _, a := range parsedAddrs {
		_, port, err := net.SplitHostPort(a)
		if err == nil && port != "0" {
			if seen[a] {
				continue
			}
			seen[a] = true
		} else if err != nil {
			if seen[a] {
				continue
			}
			seen[a] = true
		}
		finalAddrs = append(finalAddrs, a)
	}

	if len(finalAddrs) == 0 {
		finalAddrs = []string{"127.0.0.1:4242"}
	}

	s := &Server{
		mgr:         cfg.Manager,
		distFS:      cfg.DistFS,
		listen:      strings.Join(finalAddrs, ","),
		listenAddrs: finalAddrs,
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
		// Version endpoint (public)
		r.Get("/version", s.handleGetVersion)

		// Auth routes (public)
		r.Post("/auth/login", s.handleLogin)
		r.Get("/auth/status", s.handleAuthStatus)
		r.Post("/auth/logout", s.handleLogout)

		// Agent routes (public - authenticated via agent token)
		r.HandleFunc("/agent/ws", s.handleAgentWebSocket)
		r.Get("/agent/install.sh", s.handleAgentInstallScript)
		r.Get("/agent/bin/{arch}", s.handleAgentBinary)

		// Telegram Bot webhook route (public - called by Telegram servers)
		r.Post("/telegram/webhook", s.handleTelegramWebhook)

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
			r.Post("/nodes/{name}/rename", s.handleRenameNode)
			r.Put("/nodes/{name}/rename", s.handleRenameNode)
			r.Put("/nodes/{name}/position", s.handleUpdateNodePosition)
			r.Patch("/nodes/{name}/position", s.handleUpdateNodePosition)
			r.Delete("/nodes/{name}", s.handleDeleteNode)
			r.Post("/nodes/probe", s.handleProbeNode)
			r.Get("/nodes/status", s.handleGetNodeStatuses)
			r.Get("/nodes/live", s.handleGetFleetLiveStatus)
			r.Post("/nodes/{name}/status", s.handleRefreshNodeStatus)
			r.Post("/nodes/{name}/state", s.handleUpdateNodeState)
			r.Get("/nodes/{name}/bird", s.handleGetNodeBirdConfig)
			r.Get("/nodes/{name}/nftables", s.handleGetNodeNftablesConfig)
			r.Post("/nodes/{name}/restart-wg", s.handleRestartNodeWg)
			r.Post("/nodes/{name}/restart-bird", s.handleRestartNodeBird)
			r.Post("/nodes/{name}/interfaces/{iface}/restart", s.handleRestartNodeInterface)
			r.Post("/nodes/{name}/restart-wg/{iface}", s.handleRestartNodeInterface)
			r.Post("/nodes/{name}/agent-token", s.handleGenerateAgentToken)
			r.Get("/nodes/{name}/agent-token", s.handleGetNodeAgent)
			r.Get("/nodes/{name}/agent", s.handleGetNodeAgent)
			r.Post("/nodes/{name}/agent/uninstall", s.handleUninstallAgent)
			r.Delete("/nodes/{name}/agent", s.handleUninstallAgent)
			r.Get("/nodes/{name}/telemetry", s.handleGetNodeTelemetry)
			r.Get("/nodes/{name}/metrics", s.handleGetNodeMetrics)
			r.Get("/nodes/{name}/traffic", s.handleGetNodeTraffic)

			// Links
			r.Get("/links", s.handleGetLinks)
			r.Post("/links", s.handleAddLink)
			r.Post("/links/mesh", s.handleCreateMeshLinks)
			r.Put("/links", s.handleUpdateLink)
			r.Delete("/links", s.handleDeleteLink)
			r.Post("/links/psk/reveal", s.handleRevealLinkPsk)
			r.Post("/links/psk/reset", s.handleResetLinkPsk)
			r.Post("/links/private-key/reveal", s.handleRevealLinkPrivateKey)
			r.Post("/links/private-key/reset", s.handleResetLinkPrivateKey)

			// Blocks
			r.Get("/blocks", s.handleGetBlocks)
			r.Put("/blocks", s.handleUpdateBlocks)

			// Network Settings
			r.Get("/settings/network", s.handleGetNetworkSettings)
			r.Put("/settings/network", s.handleUpdateNetworkSettings)

			// DNS Settings
			r.Get("/settings/dns", s.handleGetDNSConfig)
			r.Put("/settings/dns", s.handleUpdateDNSConfig)
			r.Post("/settings/dns/sync", s.handleSyncDNS)

			// Server Background Tasks
			r.Get("/settings/tasks", s.handleGetServerTasks)
			r.Put("/settings/tasks/{id}", s.handleUpdateServerTask)
			r.Post("/settings/tasks/{id}/run", s.handleRunServerTask)

			// Database & Maintenance Settings
			r.Get("/settings/database", s.handleGetDatabaseInfo)
			r.Put("/settings/database", s.handleUpdateDatabaseSettings)
			r.Post("/settings/database/vacuum", s.handleVacuumDatabase)

			// Integrations & Prometheus / Telegram Settings
			r.Get("/settings/integrations", s.handleGetIntegrationsSettings)
			r.Put("/settings/integrations", s.handleUpdateIntegrationsSettings)
			r.Post("/settings/integrations/reset-token", s.handleResetPrometheusToken)
			r.Post("/settings/integrations/telegram/test", s.handleTestTelegramIntegration)

			// Network Policies
			r.Get("/network-policies", s.handleGetNetworkPolicies)
			r.Post("/network-policies", s.handleCreateNetworkPolicy)
			r.Get("/network-policies/{id}", s.handleGetNetworkPolicy)
			r.Put("/network-policies/{id}", s.handleUpdateNetworkPolicy)
			r.Delete("/network-policies/{id}", s.handleDeleteNetworkPolicy)
			r.Post("/network-policies/{id}/refresh-roa", s.handleRefreshPolicyROA)

			// Config Templates
			r.Get("/templates", s.handleGetTemplates)
			r.Post("/templates", s.handleCreateTemplate)
			r.Get("/templates/{id}", s.handleGetTemplate)
			r.Put("/templates/{id}", s.handleUpdateTemplate)
			r.Delete("/templates/{id}", s.handleDeleteTemplate)

			// ROA Cache
			r.Post("/roa/refresh", s.handleRefreshAllROA)
			r.Post("/roa/clear-cache", s.handleClearROACache)

			// Sync & State
			r.Get("/sync/preview", s.handleSyncPreview)
			r.Post("/sync", s.handleExecuteSync)
			r.Get("/sync/status", s.handleSyncStatus)
			r.Post("/state/update", s.handleUpdateState)
			r.Get("/state", s.handleGetState)

			// Config Changes & Apply
			r.Get("/config/apply/schema", s.handleGetConfigApplySchema)
			r.Post("/config/apply/validate", s.handleValidateConfigChange)
			r.Post("/config/apply/preview", s.handlePreviewConfigApply)
			r.Post("/config/apply", s.handleExecuteConfigApply)
			r.Get("/config/changes", s.handleListConfigChanges)
			r.Post("/config/changes", s.handleSaveConfigChange)
			r.Get("/config/changes/{id}", s.handleGetConfigChange)
			r.Put("/config/changes/{id}", s.handleSaveConfigChange)
			r.Delete("/config/changes/{id}", s.handleDeleteConfigChange)
			r.Post("/config/changes/{id}/apply", s.handleApplySavedConfigChange)

			// Device Helper Tasks
			r.Get("/tasks", s.handleGetTasks)
			r.Get("/tasks/online/sources", s.handleGetOnlineSources)
			r.Get("/tasks/online/manifest", s.handleGetOnlineManifest)
			r.Post("/tasks/online/install", s.handleInstallOnlineTask)
			r.Delete("/tasks/custom/{id}", s.handleDeleteCustomTask)
			r.Get("/tasks/{id}/source", s.handleGetTaskSources)
			r.Get("/tasks/{id}/sources", s.handleGetTaskSources)
			r.Post("/tasks/{id}/status", s.handleTaskStatus)
			r.Post("/tasks/{id}/run", s.handleTaskRun)

			// Looking Glass
			r.Get("/looking-glass/tasks", s.handleGetLookingGlassTasks)
			r.Post("/looking-glass/tasks", s.handleSaveLookingGlassTask)
			r.Put("/looking-glass/tasks/{id}", s.handleSaveLookingGlassTask)
			r.Delete("/looking-glass/tasks/{id}", s.handleDeleteLookingGlassTask)
			r.Post("/looking-glass/run", s.handleRunLookingGlass)

			// Cron Tasks
			r.Get("/cron-tasks", s.handleGetCronTasks)
			r.Post("/cron-tasks", s.handleCreateCronTask)
			r.Put("/cron-tasks/{id}", s.handleUpdateCronTask)
			r.Delete("/cron-tasks/{id}", s.handleDeleteCronTask)
			r.Post("/cron-tasks/{id}/toggle", s.handleToggleCronTask)
			r.Get("/nodes/{name}/cron-tasks/results", s.handleGetNodeCronTasksResults)

			// FlapAlerted Metrics
			r.Get("/flaps/metrics/stream", s.handleFlapAlertedMetricsStream)
			r.Get("/flaps/metrics", s.handleFlapAlertedMetrics)
		})
	})

	// Prometheus compatible metrics endpoint (authorized via Bearer token)
	r.Get("/metrics", s.handlePrometheusMetrics)

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

// ListenAddrs returns a copy of configured listen addresses
func (s *Server) ListenAddrs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.listenAddrs...)
}

// Listeners returns a copy of active net.Listeners if started
func (s *Server) Listeners() []net.Listener {
	s.mu.Lock()
	defer s.mu.Unlock()
	res := make([]net.Listener, len(s.listeners))
	copy(res, s.listeners)
	return res
}

// Start launches the HTTP server on all configured listen addresses
func (s *Server) Start() error {
	s.mu.Lock()
	if len(s.listenAddrs) == 0 {
		s.listenAddrs = []string{"127.0.0.1:4242"}
	}
	addrs := append([]string(nil), s.listenAddrs...)
	s.mu.Unlock()

	listeners := make([]net.Listener, 0, len(addrs))
	for _, addr := range addrs {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			for _, l := range listeners {
				_ = l.Close()
			}
			return fmt.Errorf("failed to listen on %s: %w", addr, err)
		}
		listeners = append(listeners, ln)
	}

	return s.Serve(listeners...)
}

// Serve binds the HTTP server to the provided listeners and starts serving
func (s *Server) Serve(listeners ...net.Listener) error {
	if len(listeners) == 0 {
		return errors.New("no listeners provided")
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		for _, l := range listeners {
			_ = l.Close()
		}
		return http.ErrServerClosed
	}
	s.listeners = listeners
	s.httpSrv = &http.Server{
		Addr:         s.listen,
		Handler:      s.router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
	s.mu.Unlock()

	errChan := make(chan error, len(listeners))
	var wg sync.WaitGroup

	for _, ln := range listeners {
		wg.Add(1)
		go func(l net.Listener) {
			defer wg.Done()
			if err := s.httpSrv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errChan <- err
			}
		}(ln)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	var firstErr error
	for err := range errChan {
		if firstErr == nil && err != nil {
			firstErr = err
			_ = s.httpSrv.Close()
		}
	}
	return firstErr
}

// Shutdown gracefully stops the server and closes all listeners
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true

	if s.mgr != nil {
		s.mgr.Close()
	}

	var err error
	if s.httpSrv != nil {
		err = s.httpSrv.Shutdown(ctx)
	}

	for _, l := range s.listeners {
		_ = l.Close()
	}

	return err
}

// Close immediately stops the server and closes all listeners
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true

	if s.mgr != nil {
		s.mgr.Close()
	}

	var err error
	if s.httpSrv != nil {
		err = s.httpSrv.Close()
	}

	for _, l := range s.listeners {
		_ = l.Close()
	}

	return err
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

func isAllowedOrigin(originStr, reqHost string) bool {
	u, err := url.Parse(originStr)
	if err != nil || u.Host == "" {
		return false
	}
	// Direct match with request Host
	if strings.EqualFold(u.Host, reqHost) {
		return true
	}
	// Strip ports and compare hosts if ports differ (e.g. reverse proxy)
	reqHostname := reqHost
	if h, _, err := net.SplitHostPort(reqHost); err == nil {
		reqHostname = h
	}
	uHostname := u.Hostname()
	if strings.EqualFold(uHostname, reqHostname) {
		return true
	}
	// Local development origins
	if uHostname == "localhost" || uHostname == "127.0.0.1" || uHostname == "::1" {
		return true
	}
	return false
}

// Auth Middleware
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// CSRF defense-in-depth for state-changing requests
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete || r.Method == http.MethodPatch {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if !isAllowedOrigin(origin, r.Host) {
					writeError(w, http.StatusForbidden, "Cross-origin request rejected")
					return
				}
			} else if ref := r.Header.Get("Referer"); ref != "" {
				if !isAllowedOrigin(ref, r.Host) {
					writeError(w, http.StatusForbidden, "Cross-origin request rejected")
					return
				}
			}
		}

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
