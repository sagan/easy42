package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// IntegrationsSettings holds configurations for third-party integrations such as Prometheus and Telegram Bot
type IntegrationsSettings struct {
	PrometheusEnabled   bool   `json:"prometheus_enabled"`
	PrometheusAuthToken string `json:"prometheus_auth_token"`
	TelegramEnabled     bool   `json:"telegram_enabled"`
	TelegramBotToken    string `json:"telegram_bot_token"`
	TelegramAdminID     int64  `json:"telegram_admin_id"`
	TelegramSecretToken string `json:"telegram_secret_token"`
	ServerURL           string `json:"server_url"`
}

// GeneratePrometheusToken creates a cryptographically secure random auth token for Prometheus scraping
func GeneratePrometheusToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("e42_prom_%d", time.Now().UnixNano())
	}
	return "e42_prom_" + hex.EncodeToString(b)
}

// GenerateTelegramSecretToken creates a cryptographically secure random token for Telegram webhook verification
func GenerateTelegramSecretToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("e42_tg_%d", time.Now().UnixNano())
	}
	return "e42_tg_" + hex.EncodeToString(b)
}
