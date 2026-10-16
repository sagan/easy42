package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// IntegrationsSettings holds configurations for third-party integrations such as Prometheus
type IntegrationsSettings struct {
	PrometheusEnabled   bool   `json:"prometheus_enabled"`
	PrometheusAuthToken string `json:"prometheus_auth_token"`
}

// GeneratePrometheusToken creates a cryptographically secure random auth token for Prometheus scraping
func GeneratePrometheusToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("e42_prom_%d", time.Now().UnixNano())
	}
	return "e42_prom_" + hex.EncodeToString(b)
}
