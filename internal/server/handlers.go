package server

import (
	"encoding/json"
	"net/http"
	"time"

	"easy42/internal/config"
	"easy42/internal/crypto"
	"github.com/go-chi/chi/v5"
)

// Auth Handlers

type loginRequest struct {
	Password string `json:"password"`
}

type authStatusResponse struct {
	Authenticated bool `json:"authenticated"`
	Unlocked      bool `json:"unlocked"`
	HasConfig     bool `json:"has_config"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	cfg := s.mgr.Store().Get()
	if cfg == nil {
		var err error
		cfg, err = s.mgr.Store().Load()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Config not initialized")
			return
		}
	}

	valid, err := crypto.VerifyPassword(req.Password, cfg.PasswordHash)
	if err != nil || !valid {
		writeError(w, http.StatusUnauthorized, "Invalid password")
		return
	}

	// Unlock in-memory key vault
	_ = s.mgr.Unlock(req.Password)

	// Create session token (valid for 30 days)
	token := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 30*24*time.Hour)

	http.SetCookie(w, &http.Cookie{
		Name:     "easy42_session",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, authStatusResponse{
		Authenticated: true,
		Unlocked:      s.mgr.IsUnlocked(),
		HasConfig:     true,
	})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	hasConfig := s.mgr.Store().Exists()
	if !hasConfig {
		writeJSON(w, http.StatusOK, authStatusResponse{
			Authenticated: false,
			Unlocked:      false,
			HasConfig:     false,
		})
		return
	}

	cfg := s.mgr.Store().Get()
	if cfg == nil {
		var err error
		cfg, err = s.mgr.Store().Load()
		if err != nil {
			writeJSON(w, http.StatusOK, authStatusResponse{
				Authenticated: false,
				Unlocked:      false,
				HasConfig:     false,
			})
			return
		}
	}

	cookie, err := r.Cookie("easy42_session")
	authed := false
	if err == nil && cookie.Value != "" {
		authed = crypto.VerifySessionToken(cookie.Value, cfg.SessionSecret, cfg.PasswordHash)
	}

	writeJSON(w, http.StatusOK, authStatusResponse{
		Authenticated: authed,
		Unlocked:      s.mgr.IsUnlocked(),
		HasConfig:     true,
	})
}

func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := s.mgr.Unlock(req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"unlocked": true,
	})
}

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	s.mgr.Lock()
	writeJSON(w, http.StatusOK, map[string]any{
		"unlocked": false,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.mgr.Lock()
	http.SetCookie(w, &http.Cookie{
		Name:     "easy42_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"logged_out": true})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	OldPassword     string `json:"old_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	oldPass := req.CurrentPassword
	if oldPass == "" {
		oldPass = req.OldPassword
	}
	if oldPass == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "Current password and new password are both required")
		return
	}

	if err := s.mgr.ChangePassword(oldPass, req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Update session cookie for current user with new password hash
	cfg := s.mgr.Store().Get()
	token := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 30*24*time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name:     "easy42_session",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "Password changed successfully",
	})
}

func (s *Server) handleLogoutAll(w http.ResponseWriter, r *http.Request) {
	if err := s.mgr.ResetSessionSecret(); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to reset session secret: "+err.Error())
		return
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "easy42_session",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		MaxAge:   -1,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"logged_out": true,
		"message":    "All sessions have been terminated",
	})
}

// Node Handlers

func (s *Server) handleGetNodes(w http.ResponseWriter, r *http.Request) {
	nodes := s.mgr.GetNodes()
	writeJSON(w, http.StatusOK, nodes)
}

func (s *Server) handleAddNode(w http.ResponseWriter, r *http.Request) {
	var node config.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid node payload")
		return
	}

	if err := s.mgr.AddNode(node); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, node)
}

func (s *Server) handleUpdateNode(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var node config.Node
	if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid node payload")
		return
	}

	if err := s.mgr.UpdateNode(name, node); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, node)
}

type updateNodePositionRequest struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func (s *Server) handleUpdateNodePosition(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req updateNodePositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid position payload")
		return
	}

	if err := s.mgr.UpdateNodePosition(name, req.X, req.Y); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"name":    name,
		"x":       req.X,
		"y":       req.Y,
	})
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := s.mgr.DeleteNode(name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

type probeRequest struct {
	Host string `json:"host"`
}

func (s *Server) handleProbeNode(w http.ResponseWriter, r *http.Request) {
	var req probeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Host == "" {
		writeError(w, http.StatusBadRequest, "Host parameter required")
		return
	}

	res, err := s.mgr.ProbeHost(req.Host)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleGetNodeStatuses(w http.ResponseWriter, r *http.Request) {
	statuses := s.mgr.GetNodeStatuses()
	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleRefreshNodeStatus(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	status, err := s.mgr.RefreshNodeStatus(name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// Link Handlers

func (s *Server) handleGetLinks(w http.ResponseWriter, r *http.Request) {
	links := s.mgr.GetLinks()
	writeJSON(w, http.StatusOK, links)
}

type addLinkRequest struct {
	FromNode string   `json:"from_node"`
	ToNode   string   `json:"to_node"`
	FromPort int      `json:"from_port,omitempty"`
	ToPort   int      `json:"to_port,omitempty"`
	FromMTU  int      `json:"from_mtu,omitempty"`
	ToMTU    int      `json:"to_mtu,omitempty"`
	MTU      int      `json:"mtu,omitempty"`
	Tags     []string `json:"tags,omitempty"`
}

func (s *Server) handleAddLink(w http.ResponseWriter, r *http.Request) {
	var req addLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid link payload")
		return
	}

	fromMTU := req.FromMTU
	toMTU := req.ToMTU
	if req.MTU > 0 {
		if fromMTU <= 0 {
			fromMTU = req.MTU
		}
		if toMTU <= 0 {
			toMTU = req.MTU
		}
	}

	link, err := s.mgr.AddLink(req.FromNode, req.ToNode, req.FromPort, req.ToPort, req.Tags, fromMTU, toMTU)
	if err != nil {
		if err == crypto.ErrVaultLocked {
			writeError(w, http.StatusLocked, "Vault is locked. Unlock with password first.")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, link)
}

type updateLinkRequest struct {
	FromNode string          `json:"from_node"`
	ToNode   string          `json:"to_node"`
	FromPort int             `json:"from_port,omitempty"`
	ToPort   int             `json:"to_port,omitempty"`
	FromMTU  int             `json:"from_mtu,omitempty"`
	ToMTU    int             `json:"to_mtu,omitempty"`
	MTU      int             `json:"mtu,omitempty"`
	Tags     []string        `json:"tags,omitempty"`
	From     *config.LinkEnd `json:"from,omitempty"`
	To       *config.LinkEnd `json:"to,omitempty"`
}

func (s *Server) handleUpdateLink(w http.ResponseWriter, r *http.Request) {
	var req updateLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid link payload")
		return
	}

	fromNode := req.FromNode
	toNode := req.ToNode
	if fromNode == "" && req.From != nil {
		fromNode = req.From.Name
	}
	if toNode == "" && req.To != nil {
		toNode = req.To.Name
	}
	if fromNode == "" {
		fromNode = r.URL.Query().Get("from")
	}
	if toNode == "" {
		toNode = r.URL.Query().Get("to")
	}

	if fromNode == "" || toNode == "" {
		writeError(w, http.StatusBadRequest, "Both from and to node names required")
		return
	}

	fromPort := req.FromPort
	toPort := req.ToPort
	if fromPort == 0 && req.From != nil {
		fromPort = req.From.ListenPort
	}
	if toPort == 0 && req.To != nil {
		toPort = req.To.ListenPort
	}

	fromMTU := req.FromMTU
	toMTU := req.ToMTU
	if req.MTU > 0 {
		if fromMTU <= 0 {
			fromMTU = req.MTU
		}
		if toMTU <= 0 {
			toMTU = req.MTU
		}
	}
	if fromMTU <= 0 && req.From != nil && req.From.MTU > 0 {
		fromMTU = req.From.MTU
	}
	if toMTU <= 0 && req.To != nil && req.To.MTU > 0 {
		toMTU = req.To.MTU
	}

	link, err := s.mgr.UpdateLink(fromNode, toNode, fromPort, toPort, req.Tags, fromMTU, toMTU)
	if err != nil {
		if err == crypto.ErrVaultLocked {
			writeError(w, http.StatusLocked, "Vault is locked. Unlock with password first.")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, link)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		writeError(w, http.StatusBadRequest, "Both 'from' and 'to' query parameters required")
		return
	}

	if err := s.mgr.DeleteLink(from, to); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// Sync Handlers

func (s *Server) handleSyncPreview(w http.ResponseWriter, r *http.Request) {
	actions, err := s.mgr.PlanSync()
	if err != nil {
		if err == crypto.ErrVaultLocked {
			writeError(w, http.StatusLocked, "Vault is locked. Unlock with password first.")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if actions == nil {
		actions = []config.SyncAction{}
	}
	writeJSON(w, http.StatusOK, actions)
}

func (s *Server) handleExecuteSync(w http.ResponseWriter, r *http.Request) {
	results, err := s.mgr.ExecuteSync()
	if err != nil {
		if err == crypto.ErrVaultLocked {
			writeError(w, http.StatusLocked, "Vault is locked. Unlock with password first.")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if results == nil {
		results = []config.SyncResult{}
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	lastSync, results := s.mgr.GetLastSyncResults()
	if results == nil {
		results = []config.SyncResult{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"last_sync": lastSync,
		"results":   results,
	})
}
