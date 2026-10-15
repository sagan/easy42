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

func TestStateStoreInterfaceTrafficDerivedStats(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStateStore(tempDir)
	defer store.Close()

	baseTime := time.Now().Add(-1 * time.Hour)

	// Snapshot 1: eth0 at 1000 rx, 2000 tx
	err := store.RecordMetrics("node-alpha", NodeMetricInput{
		Timestamp:  baseTime,
		CPUPercent: 10,
		NetRxBytes: 1000,
		NetTxBytes: 2000,
		Interfaces: []InterfaceMetricInput{
			{Name: "eth0", RxBytes: 1000, TxBytes: 2000, IsPrimary: true},
			{Name: "wg42node2", RxBytes: 500, TxBytes: 800},
		},
	})
	if err != nil {
		t.Fatalf("RecordMetrics 1 failed: %v", err)
	}

	// Snapshot 2: 10s later, eth0 rx increased to 2500 (delta: 1500), tx to 3000 (delta: 1000)
	err = store.RecordMetrics("node-alpha", NodeMetricInput{
		Timestamp:  baseTime.Add(10 * time.Second),
		CPUPercent: 12,
		NetRxBytes: 2500,
		NetTxBytes: 3000,
		Interfaces: []InterfaceMetricInput{
			{Name: "eth0", RxBytes: 2500, TxBytes: 3000, IsPrimary: true},
			{Name: "wg42node2", RxBytes: 700, TxBytes: 900},
		},
	})
	if err != nil {
		t.Fatalf("RecordMetrics 2 failed: %v", err)
	}

	// Snapshot 3: 20s later, interface RESTARTED! Raw counter reset to 300 rx (< 2500), 100 tx (< 3000).
	// Reset event must be detected, and new value added to previous accumulated value.
	// Previous accum rx: 1000 + 1500 = 2500. New accum rx should be 2500 + 300 = 2800!
	// Previous accum tx: 2000 + 1000 = 3000. New accum tx should be 3000 + 100 = 3100!
	err = store.RecordMetrics("node-alpha", NodeMetricInput{
		Timestamp:  baseTime.Add(20 * time.Second),
		CPUPercent: 15,
		NetRxBytes: 300,
		NetTxBytes: 100,
		Interfaces: []InterfaceMetricInput{
			{Name: "eth0", RxBytes: 300, TxBytes: 100, IsPrimary: true},
			{Name: "wg42node2", RxBytes: 900, TxBytes: 1100},
		},
	})
	if err != nil {
		t.Fatalf("RecordMetrics 3 failed: %v", err)
	}

	// Snapshot 4: 30s later, eth0 rx increased to 500 (delta: 200), tx to 400 (delta: 300)
	// New accum rx: 2800 + 200 = 3000.
	// New accum tx: 3100 + 300 = 3400.
	err = store.RecordMetrics("node-alpha", NodeMetricInput{
		Timestamp:  baseTime.Add(30 * time.Second),
		CPUPercent: 14,
		NetRxBytes: 500,
		NetTxBytes: 400,
		Interfaces: []InterfaceMetricInput{
			{Name: "eth0", RxBytes: 500, TxBytes: 400, IsPrimary: true},
			{Name: "wg42node2", RxBytes: 1200, TxBytes: 1300},
		},
	})
	if err != nil {
		t.Fatalf("RecordMetrics 4 failed: %v", err)
	}

	// Verify distinct interfaces
	ifaces, err := store.GetNodeInterfaces("node-alpha")
	if err != nil {
		t.Fatalf("GetNodeInterfaces failed: %v", err)
	}
	if len(ifaces) != 2 || ifaces[0] != "eth0" || ifaces[1] != "wg42node2" {
		t.Fatalf("Unexpected interfaces: %v", ifaces)
	}

	// Query derived traffic summary for eth0
	summary, err := store.GetNodeInterfaceTraffic("node-alpha", "eth0", baseTime.Add(-time.Minute), baseTime.Add(time.Minute), 100)
	if err != nil {
		t.Fatalf("GetNodeInterfaceTraffic failed: %v", err)
	}
	if len(summary.Points) != 4 {
		t.Fatalf("Expected 4 points, got %d", len(summary.Points))
	}

	// Point 0 (start of timespan): Traffic starts from 0
	if summary.Points[0].TrafficRxBytes != 0 || summary.Points[0].TrafficTxBytes != 0 {
		t.Errorf("Point 0 traffic should start from 0, got rx=%d tx=%d", summary.Points[0].TrafficRxBytes, summary.Points[0].TrafficTxBytes)
	}

	// Point 1: 1500 rx, 1000 tx
	if summary.Points[1].TrafficRxBytes != 1500 || summary.Points[1].TrafficTxBytes != 1000 {
		t.Errorf("Point 1 expected rx=1500 tx=1000, got rx=%d tx=%d", summary.Points[1].TrafficRxBytes, summary.Points[1].TrafficTxBytes)
	}

	// Point 2 (after reset): accumulated 1500 + 300 = 1800 rx, 1000 + 100 = 1100 tx
	if !summary.Points[2].ResetDetected {
		t.Errorf("Point 2 should have ResetDetected=true")
	}
	if summary.Points[2].TrafficRxBytes != 1800 || summary.Points[2].TrafficTxBytes != 1100 {
		t.Errorf("Point 2 expected rx=1800 tx=1100, got rx=%d tx=%d", summary.Points[2].TrafficRxBytes, summary.Points[2].TrafficTxBytes)
	}

	// Point 3: accumulated 1800 + 200 = 2000 rx, 1100 + 300 = 1400 tx
	if summary.Points[3].TrafficRxBytes != 2000 || summary.Points[3].TrafficTxBytes != 1400 {
		t.Errorf("Point 3 expected rx=2000 tx=1400, got rx=%d tx=%d", summary.Points[3].TrafficRxBytes, summary.Points[3].TrafficTxBytes)
	}
	if summary.TotalRxBytes != 2000 || summary.TotalTxBytes != 1400 {
		t.Errorf("Total expected rx=2000 tx=1400, got rx=%d tx=%d", summary.TotalRxBytes, summary.TotalTxBytes)
	}

	// Query unified history with interface selected
	history, availIfaces, selectedIface, err := store.GetMetricsHistoryWithInterface("node-alpha", "eth0", baseTime.Add(-time.Minute), 100)
	if err != nil {
		t.Fatalf("GetMetricsHistoryWithInterface failed: %v", err)
	}
	if selectedIface != "eth0" {
		t.Errorf("Expected selectedIface eth0, got %s", selectedIface)
	}
	if len(availIfaces) != 2 {
		t.Errorf("Expected 2 availIfaces, got %d", len(availIfaces))
	}
	if len(history) != 4 {
		t.Fatalf("Expected 4 history points, got %d", len(history))
	}
	if history[0].TrafficRxBytes != 0 {
		t.Errorf("History point 0 traffic rx should be 0, got %d", history[0].TrafficRxBytes)
	}
	if history[3].TrafficRxBytes != 2000 {
		t.Errorf("History point 3 traffic rx should be 2000, got %d", history[3].TrafficRxBytes)
	}

	// Clean metrics older than 10 minutes (should clean our samples which were 1 hour ago)
	cleaned, err := store.CleanMetricsOlderThan(10 * time.Minute)
	if err != nil {
		t.Fatalf("CleanMetricsOlderThan failed: %v", err)
	}
	if cleaned != 4 {
		t.Errorf("Expected 4 cleaned rows, got %d", cleaned)
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

func TestStateStoreUpdateInterfacesTelemetry(t *testing.T) {
	tempDir := t.TempDir()
	store := NewStateStore(tempDir)
	defer store.Close()

	// Initial interface with not_working state
	_ = store.UpdateInterface("node-a", "10.0.0.1", StateInterface{
		Name:         "wg42b",
		TargetFile:   "/etc/wireguard/wg42b.conf",
		ConfigHash:   "hash123",
		Status:       "down",
		WorkingState: WorkingStateNotWorking,
	})

	now := time.Now()
	err := store.UpdateInterfacesTelemetry("node-a", map[string]StateInterface{
		"wg42b": {
			Name:            "wg42b",
			Status:          "active",
			WorkingState:    WorkingStateWorking,
			LatestHandshake: &now,
			TransferRxBytes: 1024,
			TransferTxBytes: 2048,
		},
	})
	if err != nil {
		t.Fatalf("UpdateInterfacesTelemetry failed: %v", err)
	}

	st := store.Get()
	iface := st.Nodes["node-a"].Interfaces["wg42b"]
	if iface.WorkingState != WorkingStateWorking {
		t.Errorf("expected working state %s, got %s", WorkingStateWorking, iface.WorkingState)
	}
	if iface.Status != "active" {
		t.Errorf("expected status active, got %s", iface.Status)
	}
	if iface.ConfigHash != "hash123" {
		t.Errorf("expected ConfigHash to be preserved, got %s", iface.ConfigHash)
	}
	if iface.TargetFile != "/etc/wireguard/wg42b.conf" {
		t.Errorf("expected TargetFile to be preserved, got %s", iface.TargetFile)
	}
	if iface.TransferRxBytes != 1024 || iface.TransferTxBytes != 2048 {
		t.Errorf("expected rx=1024, tx=2048, got rx=%d, tx=%d", iface.TransferRxBytes, iface.TransferTxBytes)
	}
}

