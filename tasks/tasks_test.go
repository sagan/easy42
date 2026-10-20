package tasks

import (
	"os"
	"path/filepath"
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

func TestCustomTasks(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Task with run.sh only (auto title/category/weight)
	birdDir := filepath.Join(tmpDir, "install_bird")
	if err := os.MkdirAll(birdDir, 0755); err != nil {
		t.Fatalf("failed to create birdDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(birdDir, "run.sh"), []byte("#!/bin/sh\necho 'hello'\n"), 0755); err != nil {
		t.Fatalf("failed to write run.sh: %v", err)
	}

	// 2. Task with task.json, run.sh, and status.sh
	backupDir := filepath.Join(tmpDir, "custom_backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("failed to create backupDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "run.sh"), []byte("#!/bin/sh\necho 'backup'\n"), 0755); err != nil {
		t.Fatalf("failed to write run.sh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "status.sh"), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatalf("failed to write status.sh: %v", err)
	}
	metaJSON := `{
		"id": "custom_backup",
		"title": "Node Backup Routine",
		"description": "Performs system backup",
		"category": "Maintenance",
		"weight": 99
	}`
	if err := os.WriteFile(filepath.Join(backupDir, "task.json"), []byte(metaJSON), 0644); err != nil {
		t.Fatalf("failed to write task.json: %v", err)
	}

	// 3. Ignored directory without run.sh
	ignoredDir := filepath.Join(tmpDir, "not_a_task")
	if err := os.MkdirAll(ignoredDir, 0755); err != nil {
		t.Fatalf("failed to create ignoredDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ignoredDir, "notes.txt"), []byte("just a note"), 0644); err != nil {
		t.Fatalf("failed to write notes.txt: %v", err)
	}

	// Test GetTasks with custom dir
	taskList, err := GetTasks(tmpDir)
	if err != nil {
		t.Fatalf("GetTasks with customDir failed: %v", err)
	}

	var foundBird, foundBackup, foundIgnored bool
	for _, task := range taskList {
		if task.ID == "install_bird" {
			foundBird = true
			if !task.Custom {
				t.Errorf("expected install_bird to have Custom=true")
			}
			if task.Title != "Install Bird" {
				t.Errorf("expected title 'Install Bird', got '%s'", task.Title)
			}
			if task.Category != "Custom" {
				t.Errorf("expected category 'Custom', got '%s'", task.Category)
			}
		}
		if task.ID == "custom_backup" {
			foundBackup = true
			if !task.Custom {
				t.Errorf("expected custom_backup to have Custom=true")
			}
			if task.Title != "Node Backup Routine" {
				t.Errorf("expected title 'Node Backup Routine', got '%s'", task.Title)
			}
			if task.Category != "Maintenance" {
				t.Errorf("expected category 'Maintenance', got '%s'", task.Category)
			}
			if task.Weight != 99 {
				t.Errorf("expected weight 99, got %d", task.Weight)
			}
		}
		if task.ID == "not_a_task" {
			foundIgnored = true
		}
	}

	if !foundBird {
		t.Errorf("install_bird task was not found")
	}
	if !foundBackup {
		t.Errorf("custom_backup task was not found")
	}
	if foundIgnored {
		t.Errorf("not_a_task directory without run.sh should have been ignored")
	}

	// Test GetTaskMeta
	meta, err := GetTaskMeta("custom_backup", tmpDir)
	if err != nil {
		t.Fatalf("GetTaskMeta failed: %v", err)
	}
	if meta.Title != "Node Backup Routine" {
		t.Errorf("expected title 'Node Backup Routine', got '%s'", meta.Title)
	}

	// Test GetTaskSources for custom task
	sources, err := GetTaskSources("custom_backup", tmpDir)
	if err != nil {
		t.Fatalf("GetTaskSources failed: %v", err)
	}
	srcMap := make(map[string]TaskSourceFile)
	for _, s := range sources {
		srcMap[s.Name] = s
	}
	if _, ok := srcMap["run.sh"]; !ok {
		t.Errorf("missing run.sh in custom task sources")
	}
	if _, ok := srcMap["status.sh"]; !ok {
		t.Errorf("missing status.sh in custom task sources")
	}
	if _, ok := srcMap["task.json"]; !ok {
		t.Errorf("missing task.json in custom task sources")
	}
	if common, ok := srcMap["common.sh"]; !ok || !common.Shared {
		t.Errorf("missing shared common.sh in custom task sources")
	}

	// Test CheckStatus on task without status.sh
	statusRes := CheckStatus(nil, nil, "install_bird", "node1", tmpDir)
	if statusRes.Status != "ready" {
		t.Errorf("expected status 'ready' for task without status.sh, got '%s'", statusRes.Status)
	}
	if statusRes.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", statusRes.ExitCode)
	}
}

