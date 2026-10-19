package engine

import (
	"strings"

	"easy42/internal/config"
)

// GetIntegrationsSettings returns the current external integrations configuration
func (m *Manager) GetIntegrationsSettings() config.IntegrationsSettings {
	m.mu.RLock()
	cfg := m.store.Get()
	m.mu.RUnlock()

	if cfg == nil {
		return config.IntegrationsSettings{
			PrometheusEnabled:   true,
			PrometheusAuthToken: config.GeneratePrometheusToken(),
		}
	}

	settings := cfg.Integrations
	if strings.TrimSpace(settings.PrometheusAuthToken) == "" || (settings.TelegramEnabled && strings.TrimSpace(settings.TelegramSecretToken) == "") {
		m.mu.Lock()
		defer m.mu.Unlock()
		cfg = m.store.Get()
		if cfg != nil {
			modified := false
			if strings.TrimSpace(cfg.Integrations.PrometheusAuthToken) == "" {
				cfg.Integrations.PrometheusAuthToken = config.GeneratePrometheusToken()
				cfg.Integrations.PrometheusEnabled = true
				modified = true
			}
			if cfg.Integrations.TelegramEnabled && strings.TrimSpace(cfg.Integrations.TelegramSecretToken) == "" {
				cfg.Integrations.TelegramSecretToken = config.GenerateTelegramSecretToken()
				modified = true
			}
			if modified {
				_ = m.store.Save(cfg)
				settings = cfg.Integrations
			}
		}
	}

	return settings
}

// UpdateIntegrationsSettings updates integrations configuration in config.json
func (m *Manager) UpdateIntegrationsSettings(settings config.IntegrationsSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return config.ErrConfigNotFound
	}

	if strings.TrimSpace(settings.PrometheusAuthToken) == "" {
		settings.PrometheusAuthToken = config.GeneratePrometheusToken()
	}

	if strings.TrimSpace(settings.TelegramSecretToken) == "" {
		settings.TelegramSecretToken = config.GenerateTelegramSecretToken()
	}

	cfg.Integrations = settings
	if err := m.store.Save(cfg); err != nil {
		return err
	}

	if m.prometheusExporter != nil {
		m.prometheusExporter.Invalidate()
	}
	if m.telegramBot != nil {
		_ = m.telegramBot.Reload(settings)
	}
	return nil
}

// ResetPrometheusToken generates a new random Prometheus auth token and persists it
func (m *Manager) ResetPrometheusToken() (config.IntegrationsSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return config.IntegrationsSettings{}, config.ErrConfigNotFound
	}

	newToken := config.GeneratePrometheusToken()
	cfg.Integrations.PrometheusAuthToken = newToken
	cfg.Integrations.PrometheusEnabled = true

	if err := m.store.Save(cfg); err != nil {
		return config.IntegrationsSettings{}, err
	}

	if m.prometheusExporter != nil {
		m.prometheusExporter.Invalidate()
	}

	return cfg.Integrations, nil
}

// RenderPrometheusMetrics produces Prometheus text exposition output from the in-memory cache
func (m *Manager) RenderPrometheusMetrics() ([]byte, error) {
	if m.prometheusExporter == nil {
		m.prometheusExporter = NewPrometheusExporter(m)
	}
	return m.prometheusExporter.Render()
}
