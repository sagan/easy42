package tasks

import (
	"testing"
)

func TestGetTasks(t *testing.T) {
	taskList, err := GetTasks()
	if err != nil {
		t.Fatalf("GetTasks failed: %v", err)
	}

	if len(taskList) < 6 {
		t.Fatalf("expected at least 6 tasks, got %d", len(taskList))
	}

	expectedIDs := []string{
		"deploy_agent",
		"install_wireguard",
		"install_bird",
		"config_bird",
		"config_nftables",
		"sysctl_params",
		"autostart_interfaces",
		"configure_network",
	}

	foundMap := make(map[string]bool)
	for _, task := range taskList {
		foundMap[task.ID] = true
		if task.Title == "" {
			t.Errorf("task %s has empty title", task.ID)
		}
		if task.Description == "" {
			t.Errorf("task %s has empty description", task.ID)
		}

		// Check status.sh and run.sh existence
		statusData, err := tasksEmbed.ReadFile(task.ID + "/status.sh")
		if err != nil || len(statusData) == 0 {
			t.Errorf("task %s missing status.sh: %v", task.ID, err)
		}

		runData, err := tasksEmbed.ReadFile(task.ID + "/run.sh")
		if err != nil || len(runData) == 0 {
			t.Errorf("task %s missing run.sh: %v", task.ID, err)
		}
	}

	for _, expectedID := range expectedIDs {
		if !foundMap[expectedID] {
			t.Errorf("expected task %s not found in registry", expectedID)
		}
	}

	// Verify common.sh
	commonData, err := tasksEmbed.ReadFile("_common/common.sh")
	if err != nil || len(commonData) == 0 {
		t.Errorf("missing _common/common.sh: %v", err)
	}
}

func TestGetTaskSources(t *testing.T) {
	sources, err := GetTaskSources("configure_network")
	if err != nil {
		t.Fatalf("GetTaskSources failed: %v", err)
	}

	if len(sources) < 3 {
		t.Fatalf("expected at least 3 source files, got %d", len(sources))
	}

	// Should contain run.sh, status.sh, common.sh
	names := make(map[string]bool)
	hasSharedCommon := false
	for _, s := range sources {
		names[s.Name] = true
		if s.Content == "" {
			t.Errorf("file %s has empty content", s.Name)
		}
		if s.Name == "common.sh" && s.Shared {
			hasSharedCommon = true
		}
	}

	if !names["run.sh"] {
		t.Errorf("missing run.sh in sources")
	}
	if !names["status.sh"] {
		t.Errorf("missing status.sh in sources")
	}
	if !hasSharedCommon {
		t.Errorf("expected common.sh to be present and marked Shared")
	}

	// Verify error for unknown task
	if _, err := GetTaskSources("nonexistent_task_123"); err == nil {
		t.Errorf("expected error for nonexistent task, got nil")
	}
}

