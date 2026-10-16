package server

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type integrationsResponse struct {
	PrometheusEnabled   bool   `json:"prometheus_enabled"`
	PrometheusAuthToken string `json:"prometheus_auth_token"`
	Endpoint            string `json:"endpoint"`
}

// handlePrometheusMetrics serves Prometheus format metrics with bearer token authorization
func (s *Server) handlePrometheusMetrics(w http.ResponseWriter, r *http.Request) {
	settings := s.mgr.GetIntegrationsSettings()
	if !settings.PrometheusEnabled {
		http.Error(w, "Prometheus metrics export is disabled", http.StatusForbidden)
		return
	}

	expectedToken := strings.TrimSpace(settings.PrometheusAuthToken)
	if expectedToken == "" {
		http.Error(w, "Prometheus auth token not configured", http.StatusInternalServerError)
		return
	}

	// Support both Authorization header (Bearer <token>) and query param ?token=<token>
	var token string
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token = strings.TrimSpace(parts[1])
		} else {
			token = strings.TrimSpace(authHeader)
		}
	}
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}

	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
		w.Header().Set("WWW-Authenticate", `Bearer realm="easy42 metrics"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	data, err := s.mgr.RenderPrometheusMetrics()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to generate metrics: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	_, _ = w.Write(data)
}

// handleGetIntegrationsSettings returns current integrations configuration
func (s *Server) handleGetIntegrationsSettings(w http.ResponseWriter, r *http.Request) {
	settings := s.mgr.GetIntegrationsSettings()
	resp := integrationsResponse{
		PrometheusEnabled:   settings.PrometheusEnabled,
		PrometheusAuthToken: settings.PrometheusAuthToken,
		Endpoint:            "/metrics",
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleUpdateIntegrationsSettings updates integrations configuration
func (s *Server) handleUpdateIntegrationsSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PrometheusEnabled   *bool  `json:"prometheus_enabled"`
		PrometheusAuthToken string `json:"prometheus_auth_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	current := s.mgr.GetIntegrationsSettings()
	if req.PrometheusEnabled != nil {
		current.PrometheusEnabled = *req.PrometheusEnabled
	}
	if strings.TrimSpace(req.PrometheusAuthToken) != "" {
		current.PrometheusAuthToken = strings.TrimSpace(req.PrometheusAuthToken)
	}

	if err := s.mgr.UpdateIntegrationsSettings(current); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := integrationsResponse{
		PrometheusEnabled:   current.PrometheusEnabled,
		PrometheusAuthToken: current.PrometheusAuthToken,
		Endpoint:            "/metrics",
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleResetPrometheusToken generates a new random token and returns updated settings
func (s *Server) handleResetPrometheusToken(w http.ResponseWriter, r *http.Request) {
	updated, err := s.mgr.ResetPrometheusToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := integrationsResponse{
		PrometheusEnabled:   updated.PrometheusEnabled,
		PrometheusAuthToken: updated.PrometheusAuthToken,
		Endpoint:            "/metrics",
	}
	writeJSON(w, http.StatusOK, resp)
}
