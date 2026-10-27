package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type mcpSettingsResponse struct {
	Enabled          bool   `json:"enabled"`
	ReadOnlyToken    string `json:"read_only_token"`
	FullControlToken string `json:"full_control_token"`
	ServerURL        string `json:"server_url"`
	SSEURL           string `json:"sse_url"`
	Endpoint         string `json:"endpoint"`
}

// buildMCPServerURLs derives external URLs for MCP access
func (s *Server) buildMCPServerURLs(r *http.Request) (string, string) {
	intSettings := s.mgr.GetIntegrationsSettings()
	base := strings.TrimRight(strings.TrimSpace(intSettings.ServerURL), "/")
	if base == "" {
		scheme := "http"
		if r != nil && (r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")) {
			scheme = "https"
		}
		host := "127.0.0.1:4242"
		if r != nil && r.Host != "" {
			host = r.Host
		}
		base = fmt.Sprintf("%s://%s", scheme, host)
	}

	streamableURL := fmt.Sprintf("%s/mcp", base)
	sseURL := fmt.Sprintf("%s/mcp/sse", base)
	return streamableURL, sseURL
}

// handleGetMCPSettings returns the current MCP configuration and access tokens
func (s *Server) handleGetMCPSettings(w http.ResponseWriter, r *http.Request) {
	settings := s.mgr.GetMCPSettings()
	streamableURL, sseURL := s.buildMCPServerURLs(r)

	resp := mcpSettingsResponse{
		Enabled:          settings.Enabled,
		ReadOnlyToken:    settings.ReadOnlyToken,
		FullControlToken: settings.FullControlToken,
		ServerURL:        streamableURL,
		SSEURL:           sseURL,
		Endpoint:         "/mcp",
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleUpdateMCPSettings updates MCP server configuration in config.json
func (s *Server) handleUpdateMCPSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled          *bool   `json:"enabled"`
		ReadOnlyToken    *string `json:"read_only_token"`
		FullControlToken *string `json:"full_control_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	current := s.mgr.GetMCPSettings()
	if req.Enabled != nil {
		current.Enabled = *req.Enabled
	}
	if req.ReadOnlyToken != nil && strings.TrimSpace(*req.ReadOnlyToken) != "" {
		current.ReadOnlyToken = strings.TrimSpace(*req.ReadOnlyToken)
	}
	if req.FullControlToken != nil && strings.TrimSpace(*req.FullControlToken) != "" {
		current.FullControlToken = strings.TrimSpace(*req.FullControlToken)
	}

	if err := s.mgr.UpdateMCPSettings(current); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated := s.mgr.GetMCPSettings()
	streamableURL, sseURL := s.buildMCPServerURLs(r)

	writeJSON(w, http.StatusOK, mcpSettingsResponse{
		Enabled:          updated.Enabled,
		ReadOnlyToken:    updated.ReadOnlyToken,
		FullControlToken: updated.FullControlToken,
		ServerURL:        streamableURL,
		SSEURL:           sseURL,
		Endpoint:         "/mcp",
	})
}

// handleResetMCPToken rotates read_only, full_control, or both MCP tokens
func (s *Server) handleResetMCPToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TokenType string `json:"token_type"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	updated, err := s.mgr.ResetMCPToken(req.TokenType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	streamableURL, sseURL := s.buildMCPServerURLs(r)
	writeJSON(w, http.StatusOK, mcpSettingsResponse{
		Enabled:          updated.Enabled,
		ReadOnlyToken:    updated.ReadOnlyToken,
		FullControlToken: updated.FullControlToken,
		ServerURL:        streamableURL,
		SSEURL:           sseURL,
		Endpoint:         "/mcp",
	})
}
