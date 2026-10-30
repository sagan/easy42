package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MaxConfigBackups is the hardcoded limit on how many config backups to preserve.
const MaxConfigBackups = 100

type backupFileItem struct {
	path string
	ts   int64
}

// BackupConfig creates a timestamped backup of the current easy42 config file
// (config.json.bak.<timestamp> and config.json.bak) and prunes older backups
// to ensure at most MaxConfigBackups are preserved.
func (m *Manager) BackupConfig() (string, error) {
	now := time.Now()
	dataDir := m.store.DataDir()
	configPath := m.store.ConfigFile()

	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create data directory: %w", err)
	}

	var content []byte
	data, err := os.ReadFile(configPath)
	if err == nil {
		content = data
	} else {
		m.mu.RLock()
		cfg := m.store.Get()
		m.mu.RUnlock()
		if cfg == nil {
			var loadErr error
			cfg, loadErr = m.store.Load()
			if loadErr != nil {
				return "", fmt.Errorf("failed to read config: %w", loadErr)
			}
		}
		var marshalErr error
		content, marshalErr = json.MarshalIndent(cfg, "", "  ")
		if marshalErr != nil {
			return "", fmt.Errorf("failed to marshal config: %w", marshalErr)
		}
	}

	backupFilename := fmt.Sprintf("config.json.bak.%d", now.Unix())
	backupFilePath := filepath.Join(dataDir, backupFilename)

	if err := os.WriteFile(backupFilePath, content, 0600); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	// Also write the latest config.json.bak convenience copy
	_ = os.WriteFile(filepath.Join(dataDir, "config.json.bak"), content, 0600)

	// Prune older backups
	_ = m.PruneConfigBackups(MaxConfigBackups)

	return backupFilePath, nil
}

// PruneConfigBackups limits the timestamped backup files (config.json.bak.<timestamp>)
// in the data directory to at most maxKeep, deleting the oldest ones first.
func (m *Manager) PruneConfigBackups(maxKeep int) error {
	if maxKeep <= 0 {
		return nil
	}

	dataDir := m.store.DataDir()
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return err
	}

	var backups []backupFileItem
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "config.json.bak.") {
			continue
		}
		suffix := strings.TrimPrefix(name, "config.json.bak.")
		if suffix == "" {
			continue
		}

		var ts int64
		if parsed, err := strconv.ParseInt(suffix, 10, 64); err == nil {
			ts = parsed
		} else {
			if info, err := entry.Info(); err == nil {
				ts = info.ModTime().Unix()
			}
		}

		backups = append(backups, backupFileItem{
			path: filepath.Join(dataDir, name),
			ts:   ts,
		})
	}

	if len(backups) <= maxKeep {
		return nil
	}

	// Sort oldest first
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].ts == backups[j].ts {
			return backups[i].path < backups[j].path
		}
		return backups[i].ts < backups[j].ts
	})

	toDelete := len(backups) - maxKeep
	for i := 0; i < toDelete; i++ {
		_ = os.Remove(backups[i].path)
	}

	return nil
}
