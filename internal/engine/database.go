package engine

import (
	"fmt"
	"time"

	"easy42/internal/config"
)

// DatabaseInfo combines database storage statistics and current retention configuration
type DatabaseInfo struct {
	config.DatabaseStats
	RetentionDays int `json:"retention_days"`
}

// GetDatabaseInfo returns sqlite database state and configured retention days
func (m *Manager) GetDatabaseInfo() (DatabaseInfo, error) {
	if m.stateStore == nil {
		return DatabaseInfo{}, fmt.Errorf("state store not initialized")
	}
	stats, err := m.stateStore.GetDatabaseStats()
	if err != nil {
		return DatabaseInfo{}, err
	}
	return DatabaseInfo{
		DatabaseStats: stats,
		RetentionDays: m.GetRetentionDays(),
	}, nil
}

// GetRetentionDays returns the configured data retention in days (defaults to 47)
func (m *Manager) GetRetentionDays() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	return cfg.GetRetentionDays()
}

// GetRetentionDuration returns the retention duration
func (m *Manager) GetRetentionDuration() time.Duration {
	days := m.GetRetentionDays()
	if days <= 0 {
		days = config.DefaultRetentionDays
	}
	return time.Duration(days) * 24 * time.Hour
}

// UpdateDatabaseSettings updates retention configuration and immediately cleans older data
func (m *Manager) UpdateDatabaseSettings(settings config.DatabaseSettings) (DatabaseInfo, error) {
	if settings.RetentionDays <= 0 {
		settings.RetentionDays = config.DefaultRetentionDays
	}
	if settings.RetentionDays > 3650 {
		return DatabaseInfo{}, fmt.Errorf("retention days cannot exceed 3650 days (10 years)")
	}

	m.mu.Lock()
	cfg := m.store.Get()
	if cfg == nil {
		m.mu.Unlock()
		return DatabaseInfo{}, config.ErrConfigNotFound
	}
	cfg.Database = settings
	if err := m.store.Save(cfg); err != nil {
		m.mu.Unlock()
		return DatabaseInfo{}, err
	}
	m.mu.Unlock()

	// Clean older data with new retention period
	if m.stateStore != nil {
		retention := time.Duration(settings.RetentionDays) * 24 * time.Hour
		_, _ = m.stateStore.CleanMetricsOlderThan(retention)
		_, _ = m.stateStore.CleanCronResultsOlderThan(retention)
	}

	return m.GetDatabaseInfo()
}

// VacuumDatabase executes SQLite VACUUM to reclaim space and returns updated database stats
func (m *Manager) VacuumDatabase() (DatabaseInfo, error) {
	if m.stateStore == nil {
		return DatabaseInfo{}, fmt.Errorf("state store not initialized")
	}
	if err := m.stateStore.Vacuum(); err != nil {
		return DatabaseInfo{}, fmt.Errorf("vacuum failed: %w", err)
	}
	return m.GetDatabaseInfo()
}
