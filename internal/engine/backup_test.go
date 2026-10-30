package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"easy42/internal/config"
)

func TestBackupConfig_Basic(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	_, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	mgr := NewManager(store)
	defer mgr.Close()

	backupPath, err := mgr.BackupConfig()
	if err != nil {
		t.Fatalf("BackupConfig failed: %v", err)
	}

	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("timestamped backup file does not exist: %v", err)
	}

	filename := filepath.Base(backupPath)
	if !strings.HasPrefix(filename, "config.json.bak.") {
		t.Errorf("expected backup filename to start with 'config.json.bak.', got %s", filename)
	}

	bakConvenience := filepath.Join(tmpDir, "config.json.bak")
	if _, err := os.Stat(bakConvenience); err != nil {
		t.Fatalf("config.json.bak convenience file does not exist: %v", err)
	}

	content, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("failed to read backup file: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("backup file is not valid JSON: %v", err)
	}

	convenienceContent, err := os.ReadFile(bakConvenience)
	if err != nil {
		t.Fatalf("failed to read config.json.bak: %v", err)
	}
	if string(content) != string(convenienceContent) {
		t.Errorf("expected timestamped backup and config.json.bak to have identical content")
	}
}

func TestBackupConfig_PruneLimit100(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	_, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	mgr := NewManager(store)
	defer mgr.Close()

	// Pre-create 105 mock backup files with sequential timestamps
	baseTime := int64(1700000000)
	for i := 0; i < 105; i++ {
		filename := fmt.Sprintf("config.json.bak.%d", baseTime+int64(i))
		filePath := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(filePath, []byte(fmt.Sprintf(`{"index": %d}`, i)), 0600); err != nil {
			t.Fatalf("failed to write mock backup %d: %v", i, err)
		}
	}

	// Verify all 105 are present before pruning
	entries, _ := os.ReadDir(tmpDir)
	count := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config.json.bak.") {
			count++
		}
	}
	if count != 105 {
		t.Fatalf("expected 105 initial backups, got %d", count)
	}

	// Perform backup config which also triggers pruning with MaxConfigBackups (100)
	time.Sleep(10 * time.Millisecond)
	_, err = mgr.BackupConfig()
	if err != nil {
		t.Fatalf("BackupConfig failed: %v", err)
	}

	// Check remaining timestamped backup files
	entries, _ = os.ReadDir(tmpDir)
	var remaining []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config.json.bak.") {
			remaining = append(remaining, e.Name())
		}
	}

	if len(remaining) != MaxConfigBackups {
		t.Fatalf("expected exactly %d backups remaining after pruning, got %d", MaxConfigBackups, len(remaining))
	}

	// Verify the oldest ones were deleted (e.g. index 0, 1, 2, 3, 4, 5)
	for i := 0; i < 6; i++ {
		oldFile := filepath.Join(tmpDir, fmt.Sprintf("config.json.bak.%d", baseTime+int64(i)))
		if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
			t.Errorf("expected old backup %s to be deleted, but it still exists", oldFile)
		}
	}

	// Verify newer ones still exist
	for i := 6; i < 105; i++ {
		keepFile := filepath.Join(tmpDir, fmt.Sprintf("config.json.bak.%d", baseTime+int64(i)))
		if _, err := os.Stat(keepFile); err != nil {
			t.Errorf("expected backup %s to be preserved, but got error: %v", keepFile, err)
		}
	}

	// Verify config.json and config.json.bak are preserved
	if _, err := os.Stat(filepath.Join(tmpDir, "config.json")); err != nil {
		t.Errorf("config.json should not be deleted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "config.json.bak")); err != nil {
		t.Errorf("config.json.bak should not be deleted: %v", err)
	}
}
