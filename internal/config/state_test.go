package config

import (
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
		t.Errorf("Expected state.json not to exist initially")
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
		t.Errorf("Expected state.json to exist after update")
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

	// Check state file content exists and is valid json
	content, err := os.ReadFile(filepath.Join(tempDir, "state.json"))
	if err != nil {
		t.Fatalf("Failed to read state.json: %v", err)
	}
	if len(content) == 0 {
		t.Errorf("state.json is empty")
	}
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
}
