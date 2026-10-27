package engine

import (
	"strings"

	"easy42/internal/config"
)

// GetMCPSettings returns the current MCP server configuration and tokens
func (m *Manager) GetMCPSettings() config.MCPSettings {
	m.mu.RLock()
	cfg := m.store.Get()
	m.mu.RUnlock()

	if cfg == nil {
		return config.MCPSettings{
			Enabled:          true,
			ReadOnlyToken:    config.GenerateMCPToken("ro"),
			FullControlToken: config.GenerateMCPToken("full"),
		}
	}

	settings := cfg.GetMCPSettings()
	if strings.TrimSpace(settings.ReadOnlyToken) == "" || strings.TrimSpace(settings.FullControlToken) == "" {
		m.mu.Lock()
		defer m.mu.Unlock()
		cfg = m.store.Get()
		if cfg != nil {
			modified := false
			if strings.TrimSpace(cfg.MCP.ReadOnlyToken) == "" {
				cfg.MCP.ReadOnlyToken = config.GenerateMCPToken("ro")
				modified = true
			}
			if strings.TrimSpace(cfg.MCP.FullControlToken) == "" {
				cfg.MCP.FullControlToken = config.GenerateMCPToken("full")
				modified = true
			}
			if modified {
				_ = m.store.Save(cfg)
				settings = cfg.MCP
			}
		}
	}

	return settings
}

// UpdateMCPSettings updates MCP server configuration in config.json
func (m *Manager) UpdateMCPSettings(settings config.MCPSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return config.ErrConfigNotFound
	}

	if err := config.ValidateMCPSettings(&settings); err != nil {
		return err
	}

	if strings.TrimSpace(settings.ReadOnlyToken) == "" {
		settings.ReadOnlyToken = config.GenerateMCPToken("ro")
	}
	if strings.TrimSpace(settings.FullControlToken) == "" {
		settings.FullControlToken = config.GenerateMCPToken("full")
	}

	cfg.MCP = settings
	return m.store.Save(cfg)
}

// ResetMCPToken generates a new random token for read_only, full_control, or both
func (m *Manager) ResetMCPToken(tokenType string) (config.MCPSettings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return config.MCPSettings{}, config.ErrConfigNotFound
	}

	lower := strings.ToLower(tokenType)
	switch {
	case strings.Contains(lower, "ro") || strings.Contains(lower, "read"):
		cfg.MCP.ReadOnlyToken = config.GenerateMCPToken("ro")
	case strings.Contains(lower, "full") || strings.Contains(lower, "admin") || strings.Contains(lower, "rw"):
		cfg.MCP.FullControlToken = config.GenerateMCPToken("full")
	default:
		cfg.MCP.ReadOnlyToken = config.GenerateMCPToken("ro")
		cfg.MCP.FullControlToken = config.GenerateMCPToken("full")
	}

	if err := m.store.Save(cfg); err != nil {
		return config.MCPSettings{}, err
	}

	return cfg.MCP, nil
}
