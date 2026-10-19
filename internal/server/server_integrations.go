package server

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type integrationsResponse struct {
	PrometheusEnabled     bool   `json:"prometheus_enabled"`
	PrometheusAuthToken   string `json:"prometheus_auth_token"`
	Endpoint              string `json:"endpoint"`
	TelegramEnabled       bool   `json:"telegram_enabled"`
	TelegramBotToken      string `json:"telegram_bot_token"`
	TelegramAdminID       int64  `json:"telegram_admin_id"`
	TelegramSecretToken   string `json:"telegram_secret_token,omitempty"`
	ServerURL             string `json:"server_url"`
	TelegramBotUsername   string `json:"telegram_bot_username,omitempty"`
	TelegramWebhookStatus string `json:"telegram_webhook_status,omitempty"`
	TelegramWebhookError  string `json:"telegram_webhook_error,omitempty"`
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
	var botUser, whStatus, whErr string
	if bot := s.mgr.TelegramBot(); bot != nil {
		info := bot.GetBotInfo()
		botUser = info.Username
		whStatus = info.WebhookStatus
		whErr = info.WebhookError
	}
	resp := integrationsResponse{
		PrometheusEnabled:     settings.PrometheusEnabled,
		PrometheusAuthToken:   settings.PrometheusAuthToken,
		Endpoint:              "/metrics",
		TelegramEnabled:       settings.TelegramEnabled,
		TelegramBotToken:      settings.TelegramBotToken,
		TelegramAdminID:       settings.TelegramAdminID,
		TelegramSecretToken:   settings.TelegramSecretToken,
		ServerURL:             settings.ServerURL,
		TelegramBotUsername:   botUser,
		TelegramWebhookStatus: whStatus,
		TelegramWebhookError:  whErr,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleUpdateIntegrationsSettings updates integrations configuration
func (s *Server) handleUpdateIntegrationsSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PrometheusEnabled   *bool        `json:"prometheus_enabled"`
		PrometheusAuthToken *string      `json:"prometheus_auth_token"`
		TelegramEnabled     *bool        `json:"telegram_enabled"`
		TelegramBotToken    *string      `json:"telegram_bot_token"`
		TelegramAdminID     *json.Number `json:"telegram_admin_id"`
		ServerURL           *string      `json:"server_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	current := s.mgr.GetIntegrationsSettings()
	if req.PrometheusEnabled != nil {
		current.PrometheusEnabled = *req.PrometheusEnabled
	}
	if req.PrometheusAuthToken != nil && strings.TrimSpace(*req.PrometheusAuthToken) != "" {
		current.PrometheusAuthToken = strings.TrimSpace(*req.PrometheusAuthToken)
	}
	if req.TelegramEnabled != nil {
		current.TelegramEnabled = *req.TelegramEnabled
	}
	if req.TelegramBotToken != nil {
		current.TelegramBotToken = strings.TrimSpace(*req.TelegramBotToken)
	}
	if req.TelegramAdminID != nil {
		if id, err := req.TelegramAdminID.Int64(); err == nil {
			current.TelegramAdminID = id
		}
	}
	if req.ServerURL != nil {
		current.ServerURL = strings.TrimSpace(*req.ServerURL)
	}

	if err := s.mgr.UpdateIntegrationsSettings(current); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	current = s.mgr.GetIntegrationsSettings()

	var botUser, whStatus, whErr string
	if bot := s.mgr.TelegramBot(); bot != nil {
		info := bot.GetBotInfo()
		botUser = info.Username
		whStatus = info.WebhookStatus
		whErr = info.WebhookError
	}

	resp := integrationsResponse{
		PrometheusEnabled:     current.PrometheusEnabled,
		PrometheusAuthToken:   current.PrometheusAuthToken,
		Endpoint:              "/metrics",
		TelegramEnabled:       current.TelegramEnabled,
		TelegramBotToken:      current.TelegramBotToken,
		TelegramAdminID:       current.TelegramAdminID,
		TelegramSecretToken:   current.TelegramSecretToken,
		ServerURL:             current.ServerURL,
		TelegramBotUsername:   botUser,
		TelegramWebhookStatus: whStatus,
		TelegramWebhookError:  whErr,
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

	var botUser, whStatus, whErr string
	if bot := s.mgr.TelegramBot(); bot != nil {
		info := bot.GetBotInfo()
		botUser = info.Username
		whStatus = info.WebhookStatus
		whErr = info.WebhookError
	}

	resp := integrationsResponse{
		PrometheusEnabled:     updated.PrometheusEnabled,
		PrometheusAuthToken:   updated.PrometheusAuthToken,
		Endpoint:              "/metrics",
		TelegramEnabled:       updated.TelegramEnabled,
		TelegramBotToken:      updated.TelegramBotToken,
		TelegramAdminID:       updated.TelegramAdminID,
		TelegramSecretToken:   updated.TelegramSecretToken,
		ServerURL:             updated.ServerURL,
		TelegramBotUsername:   botUser,
		TelegramWebhookStatus: whStatus,
		TelegramWebhookError:  whErr,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleTestTelegramIntegration tests the Telegram bot token and admin user ID connection
func (s *Server) handleTestTelegramIntegration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TelegramBotToken string       `json:"telegram_bot_token"`
		TelegramAdminID  *json.Number `json:"telegram_admin_id"`
		ServerURL        string       `json:"server_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	current := s.mgr.GetIntegrationsSettings()
	token := strings.TrimSpace(req.TelegramBotToken)
	if token == "" {
		token = current.TelegramBotToken
	}

	adminID := int64(0)
	if req.TelegramAdminID != nil {
		if id, err := req.TelegramAdminID.Int64(); err == nil {
			adminID = id
		}
	}
	if adminID == 0 {
		adminID = current.TelegramAdminID
	}

	serverURL := strings.TrimSpace(req.ServerURL)
	if serverURL == "" {
		serverURL = current.ServerURL
	}

	bot := s.mgr.TelegramBot()
	if bot == nil {
		writeError(w, http.StatusInternalServerError, "Telegram bot service unavailable")
		return
	}

	botUser, err := bot.TestConnection(token, adminID, serverURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"bot_username": botUser,
		"message":      fmt.Sprintf("Successfully connected to @%s and sent test notification to Telegram user %d", botUser, adminID),
	})
}

// handleTelegramWebhook processes incoming webhook requests from Telegram servers
func (s *Server) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	settings := s.mgr.GetIntegrationsSettings()
	if !settings.TelegramEnabled {
		http.Error(w, "Telegram integration disabled", http.StatusForbidden)
		return
	}

	expectedSecret := strings.TrimSpace(settings.TelegramSecretToken)
	if expectedSecret == "" {
		http.Error(w, "Telegram secret token not configured", http.StatusUnauthorized)
		return
	}

	receivedSecret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if receivedSecret == "" || subtle.ConstantTimeCompare([]byte(receivedSecret), []byte(expectedSecret)) != 1 {
		http.Error(w, "Unauthorized: invalid secret token", http.StatusUnauthorized)
		return
	}

	bot := s.mgr.TelegramBot()
	if bot == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "note": "telegram service not running"})
		return
	}

	var update tgbotapi.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "invalid json payload"})
		return
	}

	_ = bot.HandleWebhookUpdate(&update)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
