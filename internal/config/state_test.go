package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateStoreLifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-state-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewStateStore(tempDir)
	if store.Exists() {
		t.Errorf("Expected data.db not to exist initially")
	}

	st, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if st == nil || len(st.Nodes) != 0 {
		t.Fatalf("Expected empty state")
	}

	// Update interface
	iface := StateInterface{
		Name:       "wg42nodeb",
		TargetFile: "/etc/wireguard/wg42nodeb.conf",
		ConfigHash: HashConfig("test-config"),
		PeerNode:   "node-b",
		Status:     "active",
		AppliedAt:  time.Now(),
	}

	if err := store.UpdateInterface("node-a", "192.168.1.1", iface); err != nil {
		t.Fatalf("UpdateInterface failed: %v", err)
	}

	if !store.Exists() {
		t.Errorf("Expected data.db to exist after update")
	}

	// Reload from disk in a fresh store instance
	store2 := NewStateStore(tempDir)
	st2, err := store2.Load()
	if err != nil {
		t.Fatalf("Load 2 failed: %v", err)
	}

	nodeA, ok := st2.Nodes["node-a"]
	if !ok {
		t.Fatalf("Expected node-a in loaded state")
	}
	if ifaceLoaded, ok := nodeA.Interfaces["wg42nodeb"]; !ok {
		t.Fatalf("Expected wg42nodeb interface on node-a")
	} else if ifaceLoaded.ConfigHash != iface.ConfigHash {
		t.Errorf("Config hash mismatch: %s vs %s", ifaceLoaded.ConfigHash, iface.ConfigHash)
	}

	// Remove interface
	if err := store2.RemoveInterface("node-a", "wg42nodeb"); err != nil {
		t.Fatalf("RemoveInterface failed: %v", err)
	}
	if len(store2.Get().Nodes["node-a"].Interfaces) != 0 {
		t.Errorf("Expected 0 interfaces after removal")
	}

	// Remove node
	if err := store2.RemoveNode("node-a"); err != nil {
		t.Fatalf("RemoveNode failed: %v", err)
	}
	if _, ok := store2.Get().Nodes["node-a"]; ok {
		t.Errorf("Expected node-a to be removed")
	}

	// Check state db file content exists and is not empty
	content, err := os.ReadFile(store.FilePath())
	if err != nil {
		t.Fatalf("Failed to read data.db: %v", err)
	}
	if len(content) == 0 {
		t.Errorf("data.db is empty")
	}

	_ = store.Close()
	_ = store2.Close()
}

func TestStateStoreBirdState(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-bird-state-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := NewStateStore(tempDir)
	_, _ = store.Load()

	appliedAt := time.Now().Truncate(time.Second)
	testHash := HashConfig("router id 192.168.100.1;")

	if err := store.UpdateBirdState("node-1", "10.0.0.1", testHash, appliedAt); err != nil {
		t.Fatalf("UpdateBirdState failed: %v", err)
	}

	// Verify in-memory state
	node, ok := store.Get().Nodes["node-1"]
	if !ok {
		t.Fatalf("Expected node-1 in state")
	}
	if node.BirdConfigHash != testHash {
		t.Errorf("Expected hash %s, got %s", testHash, node.BirdConfigHash)
	}
	if node.BirdAppliedAt == nil || !node.BirdAppliedAt.Equal(appliedAt) {
		t.Errorf("Expected appliedAt %v, got %v", appliedAt, node.BirdAppliedAt)
	}

	// Reload from disk in a new StateStore instance
	store2 := NewStateStore(tempDir)
	st2, err := store2.Load()
	if err != nil {
		t.Fatalf("Load store2 failed: %v", err)
	}
	nodeLoaded, ok := st2.Nodes["node-1"]
	if !ok {
		t.Fatalf("Expected node-1 in loaded store2")
	}
	if nodeLoaded.BirdConfigHash != testHash {
		t.Errorf("Loaded hash mismatch: %s vs %s", nodeLoaded.BirdConfigHash, testHash)
	}
	if nodeLoaded.BirdAppliedAt == nil || !nodeLoaded.BirdAppliedAt.Equal(appliedAt) {
		t.Errorf("Loaded appliedAt mismatch: %v vs %v", appliedAt, nodeLoaded.BirdAppliedAt)
	}

	_ = store.Close()
	_ = store2.Close()
}

func TestUpdateNftablesState(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStateStore(tempDir)

	appliedAt := time.Now().Truncate(time.Second)
	testHash := HashConfig("table inet easy42 { }")

	if err := store.UpdateNftablesState("node-1", "10.0.0.1", testHash, appliedAt); err != nil {
		t.Fatalf("UpdateNftablesState failed: %v", err)
	}

	// Verify in-memory state
	node, ok := store.Get().Nodes["node-1"]
	if !ok {
		t.Fatalf("Expected node-1 in state")
	}
	if node.NftablesConfigHash != testHash {
		t.Errorf("Expected hash %s, got %s", testHash, node.NftablesConfigHash)
	}
	if node.NftablesAppliedAt == nil || !node.NftablesAppliedAt.Equal(appliedAt) {
		t.Errorf("Expected appliedAt %v, got %v", appliedAt, node.NftablesAppliedAt)
	}

	// Reload from disk in a new StateStore instance
	store2 := NewStateStore(tempDir)
	st2, err := store2.Load()
	if err != nil {
		t.Fatalf("Load store2 failed: %v", err)
	}
	nodeLoaded, ok := st2.Nodes["node-1"]
	if !ok {
		t.Fatalf("Expected node-1 in loaded store2")
	}
	if nodeLoaded.NftablesConfigHash != testHash {
		t.Errorf("Loaded hash mismatch: %s vs %s", nodeLoaded.NftablesConfigHash, testHash)
	}
	if nodeLoaded.NftablesAppliedAt == nil || !nodeLoaded.NftablesAppliedAt.Equal(appliedAt) {
		t.Errorf("Loaded appliedAt mismatch: %v vs %v", appliedAt, nodeLoaded.NftablesAppliedAt)
	}

	_ = store.Close()
	_ = store2.Close()
}

func TestStateStoreMetrics(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStateStore(tempDir)
	defer store.Close()

	baseTime := time.Now().Add(-10 * time.Minute)

	// Record sample 1
	err := store.RecordMetrics("node-test", NodeMetricInput{
		Timestamp:        baseTime,
		CPUPercent:       15.5,
		MemoryUsedBytes:  1024 * 1024 * 512,
		MemoryTotalBytes: 1024 * 1024 * 1024,
		UptimeSeconds:    12345,
		Load1m:           0.5,
		Load5m:           0.3,
		Load15m:          0.1,
		NetRxBytes:       1000,
		NetTxBytes:       2000,
	})
	if err != nil {
		t.Fatalf("RecordMetrics 1 failed: %v", err)
	}

	// Record sample 2 (10 seconds later, 10,000 rx bytes delta -> 1,000 bytes/sec)
	err = store.RecordMetrics("node-test", NodeMetricInput{
		Timestamp:        baseTime.Add(10 * time.Second),
		CPUPercent:       25.0,
		MemoryUsedBytes:  1024 * 1024 * 600,
		MemoryTotalBytes: 1024 * 1024 * 1024,
		UptimeSeconds:    12355,
		Load1m:           0.8,
		Load5m:           0.4,
		Load15m:          0.2,
		NetRxBytes:       11000,
		NetTxBytes:       12000,
	})
	if err != nil {
		t.Fatalf("RecordMetrics 2 failed: %v", err)
	}

	history, err := store.GetMetricsHistory("node-test", baseTime.Add(-time.Minute), 100)
	if err != nil {
		t.Fatalf("GetMetricsHistory failed: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("Expected 2 metric points, got %d", len(history))
	}

	// Check sample 2 rate calculation
	pt2 := history[1]
	if pt2.NetRxRate != 1000.0 {
		t.Errorf("Expected NetRxRate 1000.0, got %f", pt2.NetRxRate)
	}

	latest, err := store.GetLatestMetrics()
	if err != nil {
		t.Fatalf("GetLatestMetrics failed: %v", err)
	}
	if latest["node-test"].CPUPercent != 25.0 {
		t.Errorf("Expected latest CPU 25.0, got %f", latest["node-test"].CPUPercent)
	}
}

func TestStateStoreLegacyMigration(t *testing.T) {
	tempDir := t.TempDir()

	legacyState := NetworkState{
		Version:   1,
		UpdatedAt: time.Now(),
		Nodes: map[string]StateNode{
			"legacy-node": {
				Name:           "legacy-node",
				Host:           "1.2.3.4",
				BirdConfigHash: "test-bird-hash",
				Interfaces: map[string]StateInterface{
					"wg42leg": {
						Name:       "wg42leg",
						ConfigHash: "iface-hash",
					},
				},
			},
		},
	}

	data, err := json.Marshal(legacyState)
	if err != nil {
		t.Fatalf("Marshal legacy state failed: %v", err)
	}

	legacyFile := filepath.Join(tempDir, "state.json")
	if err := os.WriteFile(legacyFile, data, 0600); err != nil {
		t.Fatalf("Write legacy file failed: %v", err)
	}

	// Now initialize StateStore with SQLite
	store := NewStateStore(tempDir)
	defer store.Close()

	st, err := store.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if _, ok := st.Nodes["legacy-node"]; !ok {
		t.Fatalf("Expected legacy-node to be migrated into SQLite")
	}

	if st.Nodes["legacy-node"].BirdConfigHash != "test-bird-hash" {
		t.Errorf("BirdConfigHash mismatch: got %s", st.Nodes["legacy-node"].BirdConfigHash)
	}

	// Verify legacy state.json was renamed to state.json.migrated
	if _, err := os.Stat(legacyFile); !os.IsNotExist(err) {
		t.Errorf("Expected original state.json to be moved")
	}

	if _, err := os.Stat(filepath.Join(tempDir, "state.json.migrated")); err != nil {
		t.Errorf("Expected state.json.migrated to exist")
	}
}
