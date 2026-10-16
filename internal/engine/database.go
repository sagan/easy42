package engine

import (
	"fmt"
	"time"

	"easy42/internal/config"
)

// DatabaseInfo combines database storage statistics and current retention configuration
type DatabaseInfo struct {
	config.DatabaseStats
	RetentionDays    int `json:"retention_days"`
	RawRetentionDays int `json:"raw_retention_days"`
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
		DatabaseStats:    stats,
		RetentionDays:    m.GetRetentionDays(),
		RawRetentionDays: m.GetRawRetentionDays(),
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

// GetRawRetentionDays returns the configured raw data retention in days (defaults to 2)
func (m *Manager) GetRawRetentionDays() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	return cfg.GetRawRetentionDays()
}

// GetRawRetentionDuration returns the raw data retention duration
func (m *Manager) GetRawRetentionDuration() time.Duration {
	days := m.GetRawRetentionDays()
	if days <= 0 {
		days = config.DefaultRawRetentionDays
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
	if settings.RawRetentionDays <= 0 {
		settings.RawRetentionDays = config.DefaultRawRetentionDays
	}
	if settings.RawRetentionDays > 30 {
		return DatabaseInfo{}, fmt.Errorf("raw retention days cannot exceed 30 days")
	}
	if settings.RawRetentionDays > settings.RetentionDays {
		settings.RawRetentionDays = settings.RetentionDays
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

	// Rollup and clean older data with new retention periods
	if m.stateStore != nil {
		rawRetention := time.Duration(settings.RawRetentionDays) * 24 * time.Hour
		retention := time.Duration(settings.RetentionDays) * 24 * time.Hour
		_, _ = m.stateStore.RollupAndCleanMetrics(rawRetention, retention)
		_, _ = m.stateStore.RollupAndCleanCronResults(rawRetention, retention)
	}

	return m.GetDatabaseInfo()
}

// VacuumDatabase executes SQLite VACUUM to reclaim space and returns updated database stats.
// It performs a rollup and clean pass first so any data past retention thresholds is pruned.
func (m *Manager) VacuumDatabase() (DatabaseInfo, error) {
	if m.stateStore == nil {
		return DatabaseInfo{}, fmt.Errorf("state store not initialized")
	}

	rawRetention := m.GetRawRetentionDuration()
	retention := m.GetRetentionDuration()
	_, _ = m.stateStore.RollupAndCleanMetrics(rawRetention, retention)
	_, _ = m.stateStore.RollupAndCleanCronResults(rawRetention, retention)

	if err := m.stateStore.Vacuum(); err != nil {
		return DatabaseInfo{}, fmt.Errorf("vacuum failed: %w", err)
	}
	return m.GetDatabaseInfo()
}
