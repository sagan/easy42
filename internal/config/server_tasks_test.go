package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeServerTasks_NewTaskAdded(t *testing.T) {
	// Simulate an older config.json that only has 1 or 2 tasks
	existing := []ServerTaskConfig{
		{
			ID:       ServerTaskUpdateROA,
			CronExpr: "15 2 * * *", // Custom user cron
			Enabled:  false,        // Custom user state
		},
	}

	normalized := NormalizeServerTasks(existing)

	if len(normalized) != len(PredefinedServerTasks) {
		t.Fatalf("expected %d tasks, got %d", len(PredefinedServerTasks), len(normalized))
	}

	// Existing task should preserve customized cron and enabled state
	var roaTask *ServerTaskConfig
	var syncTask *ServerTaskConfig
	var dnsTask *ServerTaskConfig

	for i := range normalized {
		task := &normalized[i]
		switch task.ID {
		case ServerTaskUpdateROA:
			roaTask = task
		case ServerTaskSync:
			syncTask = task
		case ServerTaskSyncDNS:
			dnsTask = task
		}
	}

	if roaTask == nil || roaTask.CronExpr != "15 2 * * *" || roaTask.Enabled != false {
		t.Fatalf("expected roaTask to preserve custom settings, got %+v", roaTask)
	}

	// Missing tasks should be automatically added with their defaults
	if syncTask == nil || syncTask.CronExpr != DefaultCronForTask(ServerTaskSync) || syncTask.Enabled != true {
		t.Fatalf("expected syncTask to be added with defaults, got %+v", syncTask)
	}
	if dnsTask == nil || dnsTask.CronExpr != DefaultCronForTask(ServerTaskSyncDNS) || dnsTask.Enabled != true {
		t.Fatalf("expected dnsTask to be added with defaults, got %+v", dnsTask)
	}
}

func TestStoreLoad_AutoMigratesNewTasks(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore(tmpDir)

	// Initialize new store
	_, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	// Manually write config.json with an older tasks list (e.g. missing sync_dns)
	cfgPath := filepath.Join(tmpDir, "config.json")
	oldJSON := `{
  "network_settings": {
    "public_asn": 4242420001,
    "prefixes": ["172.20.0.0/14{21,29}"]
  },
  "nodes": [],
  "links": [],
  "server_tasks": [
    { "id": "update_roa", "cron_expr": "5 0 * * *", "enabled": true }
  ]
}`
	if err := os.WriteFile(cfgPath, []byte(oldJSON), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Reload config
	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("failed to reload store: %v", err)
	}

	if len(reloaded.ServerTasks) != len(PredefinedServerTasks) {
		t.Fatalf("expected %d tasks in loaded config, got %d", len(PredefinedServerTasks), len(reloaded.ServerTasks))
	}

	// Check that the file on disk was also updated with the missing tasks
	store2 := NewStore(tmpDir)
	reloaded2, err := store2.Load()
	if err != nil {
		t.Fatalf("failed to reload store2: %v", err)
	}
	if len(reloaded2.ServerTasks) != len(PredefinedServerTasks) {
		t.Fatalf("expected %d tasks on disk after auto-migration, got %d", len(PredefinedServerTasks), len(reloaded2.ServerTasks))
	}
}
