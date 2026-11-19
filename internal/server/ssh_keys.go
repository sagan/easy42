package server

import (
	"encoding/json"
	"net/http"
	"time"

	"easy42/internal/config"
	"easy42/internal/engine"

	"github.com/go-chi/chi/v5"
)

// sshKeysResponse is the payload of GET /api/ssh-keys.
type sshKeysResponse struct {
	Nodes   []engine.SSHKeyNodeState `json:"nodes"`
	Rules   []config.SSHKeyRule      `json:"rules"`
	Summary engine.SSHKeysSummary    `json:"summary"`
}

// handleGetSSHKeys returns the collected node public keys, the configured rules
// and aggregate counters for the SSH tab.
func (s *Server) handleGetSSHKeys(w http.ResponseWriter, r *http.Request) {
	nodes := s.mgr.GetSSHKeysOverview()
	if nodes == nil {
		nodes = []engine.SSHKeyNodeState{}
	}
	rules := s.mgr.GetSSHKeyRules()
	if rules == nil {
		rules = []config.SSHKeyRule{}
	}
	writeJSON(w, http.StatusOK, sshKeysResponse{
		Nodes:   nodes,
		Rules:   rules,
		Summary: s.mgr.GetSSHKeysSummary(),
	})
}

// handleGetSSHKeyRules returns all SSH key rules.
func (s *Server) handleGetSSHKeyRules(w http.ResponseWriter, r *http.Request) {
	rules := s.mgr.GetSSHKeyRules()
	if rules == nil {
		rules = []config.SSHKeyRule{}
	}
	writeJSON(w, http.StatusOK, rules)
}

// handleGetSSHKeyRule returns a single SSH key rule.
func (s *Server) handleGetSSHKeyRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := s.mgr.GetSSHKeyRule(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// handleCreateSSHKeyRule creates a new SSH key rule.
func (s *Server) handleCreateSSHKeyRule(w http.ResponseWriter, r *http.Request) {
	var rule config.SSHKeyRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}
	// IDs are server assigned.
	rule.ID = ""
	rule.CreatedAt = time.Time{}
	rule.UpdatedAt = time.Time{}

	saved, err := s.mgr.SaveSSHKeyRule(rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, saved)
}

// handleUpdateSSHKeyRule updates an existing SSH key rule.
func (s *Server) handleUpdateSSHKeyRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Rule ID is required")
		return
	}
	if _, err := s.mgr.GetSSHKeyRule(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	var rule config.SSHKeyRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}
	rule.ID = id
	rule.CreatedAt = time.Time{}
	rule.UpdatedAt = time.Time{}

	saved, err := s.mgr.SaveSSHKeyRule(rule)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

// handleDeleteSSHKeyRule deletes an SSH key rule.
func (s *Server) handleDeleteSSHKeyRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.mgr.DeleteSSHKeyRule(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// handleToggleSSHKeyRule enables or disables an SSH key rule.
func (s *Server) handleToggleSSHKeyRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rule, err := s.mgr.ToggleSSHKeyRule(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// handleRefreshSSHKeys fetches the collected keys of one node (or every
// connected supporting agent) on demand, e.g. right after new keys were added.
func (s *Server) handleRefreshSSHKeys(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Node string `json:"node,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	refreshed := []string{}
	if req.Node != "" {
		if _, _, err := s.mgr.FetchNodeSSHKeys(req.Node); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		refreshed = append(refreshed, req.Node)
	} else {
		for _, node := range s.mgr.GetSSHKeysOverview() {
			if !node.Supported || !node.Connected {
				continue
			}
			if _, _, err := s.mgr.FetchNodeSSHKeys(node.Name); err != nil {
				continue
			}
			refreshed = append(refreshed, node.Name)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":   "SSH public keys refreshed",
		"refreshed": refreshed,
	})
}
