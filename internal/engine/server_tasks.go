package engine

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"easy42/internal/config"
)

// ServerTask represents a complete server background task view with configuration and execution state
type ServerTask struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	CronExpr       string     `json:"cron_expr"`
	Enabled        bool       `json:"enabled"`
	NextRun        *time.Time `json:"next_run,omitempty"`
	LastRun        *time.Time `json:"last_run,omitempty"`
	LastStatus     string     `json:"last_status"` // "never_run", "running", "success", "error"
	LastDurationMs int64      `json:"last_duration_ms"`
	LastMessage    string     `json:"last_message"`
}

// GetServerTasks returns all predefined server background tasks with current settings and state
func (m *Manager) GetServerTasks() ([]ServerTask, error) {
	m.mu.RLock()
	cfg := m.store.Get()
	m.mu.RUnlock()

	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return nil, err
		}
	}

	normTasks := config.NormalizeServerTasks(cfg.ServerTasks)
	cfgMap := make(map[string]config.ServerTaskConfig, len(normTasks))
	for _, t := range normTasks {
		cfgMap[t.ID] = t
	}

	states, err := m.stateStore.GetServerTaskStates()
	if err != nil {
		states = make(map[string]config.ServerTaskState)
	}

	m.serverTasksMu.Lock()
	running := make(map[string]bool, len(m.runningTasks))
	for k, v := range m.runningTasks {
		running[k] = v
	}
	m.serverTasksMu.Unlock()

	now := time.Now()
	tasks := make([]ServerTask, 0, len(config.PredefinedServerTasks))

	for _, meta := range config.PredefinedServerTasks {
		cfgTask, ok := cfgMap[meta.ID]
		if !ok {
			cfgTask = config.ServerTaskConfig{
				ID:       meta.ID,
				CronExpr: config.DefaultCronForTask(meta.ID),
				Enabled:  true,
			}
		}

		st, hasState := states[meta.ID]
		lastStatus := "never_run"
		var lastRun *time.Time
		var lastDurationMs int64
		var lastMessage string

		if hasState && !st.LastRun.IsZero() {
			t := st.LastRun
			lastRun = &t
			lastStatus = st.LastStatus
			lastDurationMs = st.LastDurationMs
			lastMessage = st.LastMessage
		}

		if running[meta.ID] {
			lastStatus = "running"
		}

		var nextRun *time.Time
		if cfgTask.Enabled {
			if sched, err := ParseCronSchedule(cfgTask.CronExpr); err == nil {
				next := sched.Next(now)
				if !next.IsZero() {
					nextRun = &next
				}
			}
		}

		tasks = append(tasks, ServerTask{
			ID:             meta.ID,
			Name:           meta.Name,
			Description:    meta.Description,
			CronExpr:       cfgTask.CronExpr,
			Enabled:        cfgTask.Enabled,
			NextRun:        nextRun,
			LastRun:        lastRun,
			LastStatus:     lastStatus,
			LastDurationMs: lastDurationMs,
			LastMessage:    lastMessage,
		})
	}

	return tasks, nil
}

// UpdateServerTask updates the cron schedule and enabled status for a predefined task
func (m *Manager) UpdateServerTask(id string, cronExpr string, enabled bool) (*ServerTask, error) {
	if !config.IsPredefinedServerTask(id) {
		return nil, fmt.Errorf("task '%s' is not an editable server task", id)
	}

	cronExpr = strings.TrimSpace(cronExpr)
	if err := config.ValidateCronExpr(cronExpr); err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	m.mu.Lock()
	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
	}

	normTasks := config.NormalizeServerTasks(cfg.ServerTasks)
	for i := range normTasks {
		if normTasks[i].ID == id {
			normTasks[i].CronExpr = cronExpr
			normTasks[i].Enabled = enabled
			break
		}
	}
	cfg.ServerTasks = normTasks

	if err := m.store.Save(cfg); err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to save server tasks configuration: %w", err)
	}
	m.mu.Unlock()

	allTasks, err := m.GetServerTasks()
	if err != nil {
		return nil, err
	}

	for _, t := range allTasks {
		if t.ID == id {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("task '%s' not found after save", id)
}

// ExecuteServerTask immediately executes a server background task and returns the updated task view
func (m *Manager) ExecuteServerTask(ctx context.Context, id string) (*ServerTask, error) {
	if !config.IsPredefinedServerTask(id) {
		return nil, fmt.Errorf("task '%s' is not a recognized server task", id)
	}

	m.serverTasksMu.Lock()
	if m.runningTasks == nil {
		m.runningTasks = make(map[string]bool)
	}
	if m.runningTasks[id] {
		m.serverTasksMu.Unlock()
		return nil, fmt.Errorf("task '%s' is already running", id)
	}
	m.runningTasks[id] = true
	m.serverTasksMu.Unlock()

	defer func() {
		m.serverTasksMu.Lock()
		delete(m.runningTasks, id)
		m.serverTasksMu.Unlock()
	}()

	start := time.Now()
	var (
		execErr error
		msg     string
	)

	switch id {
	case config.ServerTaskUpdateROA:
		execErr = m.RefreshAllROA(ctx, true)
		if execErr != nil {
			msg = fmt.Sprintf("Failed to refresh ROA caches: %v", execErr)
		} else {
			m.mu.RLock()
			cfg := m.store.Get()
			m.mu.RUnlock()
			count := 0
			if cfg != nil {
				for _, p := range cfg.GetAllPolicies() {
					if strings.TrimSpace(p.ROA4) != "" || strings.TrimSpace(p.ROA6) != "" {
						count++
					}
				}
			}
			msg = fmt.Sprintf("Successfully refreshed ROA caches for %d policy configuration(s)", count)
		}

	case config.ServerTaskSync:
		results, err := m.ExecuteSync(false)
		if err != nil {
			execErr = err
			msg = fmt.Sprintf("Sync failed: %v", err)
		} else {
			uniqueNodes := make(map[string]bool)
			failedActions := 0
			for _, r := range results {
				if r.NodeName != "" {
					uniqueNodes[r.NodeName] = true
				}
				if !r.Success {
					failedActions++
				}
			}
			if failedActions > 0 {
				execErr = fmt.Errorf("sync completed with %d failed action(s)", failedActions)
				msg = fmt.Sprintf("Sync completed with %d failed action(s) across %d node(s)", failedActions, len(uniqueNodes))
			} else {
				msg = fmt.Sprintf("Successfully synced %d node(s) (%d actions executed)", len(uniqueNodes), len(results))
			}
		}

	case config.ServerTaskSyncDNS:
		res, err := m.SyncDNS(ctx, false)
		if err != nil {
			execErr = err
			msg = fmt.Sprintf("DNS sync failed: %v", err)
		} else {
			if len(res.Errors) > 0 {
				execErr = fmt.Errorf("DNS sync encountered %d error(s)", len(res.Errors))
				msg = fmt.Sprintf("DNS sync finished with %d error(s) (created %d, updated %d, deleted %d)",
					len(res.Errors), res.Created, res.Updated, res.Deleted)
			} else {
				msg = fmt.Sprintf("DNS synced successfully: %d created, %d updated, %d deleted, %d ignored",
					res.Created, res.Updated, res.Deleted, res.Ignored)
			}
		}

	case config.ServerTaskBackupConfig:
		backupPath, err := m.BackupConfig()
		if err != nil {
			execErr = err
			msg = fmt.Sprintf("Backup failed: %v", err)
		} else {
			msg = fmt.Sprintf("Successfully created config backup: %s", filepath.Base(backupPath))
		}

	default:
		return nil, fmt.Errorf("unsupported task: %s", id)
	}

	durationMs := time.Since(start).Milliseconds()
	status := "success"
	if execErr != nil {
		status = "error"
	}

	record := config.ServerTaskState{
		TaskID:         id,
		LastRun:        start,
		LastStatus:     status,
		LastDurationMs: durationMs,
		LastMessage:    msg,
	}

	_ = m.stateStore.RecordServerTaskState(record)

	m.serverTasksMu.Lock()
	delete(m.runningTasks, id)
	m.serverTasksMu.Unlock()

	allTasks, err := m.GetServerTasks()
	if err != nil {
		return nil, err
	}

	for _, t := range allTasks {
		if t.ID == id {
			return &t, execErr
		}
	}

	return nil, execErr
}

// StartServerTasks starts the background scheduler for easy42 server tasks
func (m *Manager) StartServerTasks(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		lastFiredSlot := make(map[string]int64)

		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			case now := <-ticker.C:
				m.mu.RLock()
				cfg := m.store.Get()
				m.mu.RUnlock()

				if cfg == nil {
					continue
				}

				tasks := config.NormalizeServerTasks(cfg.ServerTasks)
				for _, t := range tasks {
					if !t.Enabled {
						continue
					}

					sched, err := ParseCronSchedule(t.CronExpr)
					if err != nil {
						continue
					}

					slot := now.Unix() / 60
					if sched.hasSeconds {
						slot = now.Unix()
					}

					if sched.Matches(now) {
						if lastFiredSlot[t.ID] != slot {
							lastFiredSlot[t.ID] = slot
							taskID := t.ID
							go func(id string) {
								log.Printf("[ServerTasks] Scheduled execution of task '%s' started", id)
								taskCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
								defer cancel()
								res, err := m.ExecuteServerTask(taskCtx, id)
								if err != nil {
									log.Printf("[ServerTasks] Scheduled task '%s' failed: %v", id, err)
								} else {
									log.Printf("[ServerTasks] Scheduled task '%s' succeeded: %s (%dms)", id, res.LastMessage, res.LastDurationMs)
								}
							}(taskID)
						}
					}
				}
			}
		}
	}()
}
