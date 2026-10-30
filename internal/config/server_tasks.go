package config

import (
	"fmt"
	"strings"
)

const (
	ServerTaskUpdateROA    = "update_roa"
	ServerTaskSync         = "sync"
	ServerTaskSyncDNS      = "sync_dns"
	ServerTaskBackupConfig = "backup_config"
)

// ServerTaskMeta defines immutable metadata for a predefined easy42 server task
type ServerTaskMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PredefinedServerTasks lists the fixed set of server background tasks
var PredefinedServerTasks = []ServerTaskMeta{
	{
		ID:          ServerTaskUpdateROA,
		Name:        "Update ROA",
		Description: "Download and update network policy defined Route Origination Authorization (ROA) URLs",
	},
	{
		ID:          ServerTaskSync,
		Name:        "Sync",
		Description: "Synchronize network topology, WireGuard, BIRD, and nftables configurations to all nodes",
	},
	{
		ID:          ServerTaskSyncDNS,
		Name:        "Sync DNS",
		Description: "Synchronize node records to Cloudflare DNS",
	},
	{
		ID:          ServerTaskBackupConfig,
		Name:        "Backup Config",
		Description: "Create a timestamped backup of the current easy42 configuration file (config.json)",
	},
}

// DefaultCronForTask returns the default cron expression for a given task ID.
// update_roa runs first at 00:00, followed by sync at 00:30 to push updated ROA tables to nodes,
// sync_dns at 01:00, and backup_config at 02:00.
func DefaultCronForTask(id string) string {
	switch id {
	case ServerTaskUpdateROA:
		return "0 0 * * *" // 00:00 daily
	case ServerTaskSync:
		return "30 0 * * *" // 00:30 daily (after ROA update)
	case ServerTaskSyncDNS:
		return "0 1 * * *" // 01:00 daily
	case ServerTaskBackupConfig:
		return "0 2 * * *" // 02:00 daily
	default:
		return "0 0 * * *"
	}
}

// DefaultServerTasks returns the default configuration for all predefined server tasks
func DefaultServerTasks() []ServerTaskConfig {
	tasks := make([]ServerTaskConfig, len(PredefinedServerTasks))
	for i, meta := range PredefinedServerTasks {
		tasks[i] = ServerTaskConfig{
			ID:       meta.ID,
			CronExpr: DefaultCronForTask(meta.ID),
			Enabled:  true,
		}
	}
	return tasks
}

// NormalizeServerTasks ensures all predefined tasks are present with valid defaults and preserved custom settings
func NormalizeServerTasks(existing []ServerTaskConfig) []ServerTaskConfig {
	existingMap := make(map[string]ServerTaskConfig)
	for _, t := range existing {
		t.ID = strings.TrimSpace(t.ID)
		if t.ID != "" {
			existingMap[t.ID] = t
		}
	}

	result := make([]ServerTaskConfig, 0, len(PredefinedServerTasks))
	for _, meta := range PredefinedServerTasks {
		defaultCron := DefaultCronForTask(meta.ID)
		if cur, ok := existingMap[meta.ID]; ok {
			cronExpr := strings.TrimSpace(cur.CronExpr)
			if cronExpr == "" || ValidateCronExpr(cronExpr) != nil {
				cronExpr = defaultCron
			}
			result = append(result, ServerTaskConfig{
				ID:       meta.ID,
				CronExpr: cronExpr,
				Enabled:  cur.Enabled,
			})
		} else {
			result = append(result, ServerTaskConfig{
				ID:       meta.ID,
				CronExpr: defaultCron,
				Enabled:  true,
			})
		}
	}
	return result
}

// IsPredefinedServerTask checks if a task ID is one of the immutable predefined tasks
func IsPredefinedServerTask(id string) bool {
	for _, meta := range PredefinedServerTasks {
		if meta.ID == id {
			return true
		}
	}
	return false
}

// GetServerTaskMeta returns metadata for a predefined task
func GetServerTaskMeta(id string) (*ServerTaskMeta, error) {
	for _, meta := range PredefinedServerTasks {
		if meta.ID == id {
			return &meta, nil
		}
	}
	return nil, fmt.Errorf("task '%s' is not a recognized server task", id)
}
