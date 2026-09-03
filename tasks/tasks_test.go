package tasks

import (
	"testing"
)

func TestGetTasks(t *testing.T) {
	taskList, err := GetTasks()
	if err != nil {
		t.Fatalf("GetTasks failed: %v", err)
	}

	if len(taskList) < 5 {
		t.Fatalf("expected at least 5 tasks, got %d", len(taskList))
	}

	expectedIDs := []string{
		"install_wireguard",
		"install_bird",
		"config_bird",
		"sysctl_params",
		"autostart_interfaces",
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
