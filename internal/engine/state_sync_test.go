package engine

import (
	"os"
	"testing"
	"time"

	"easy42/internal/compiler"
	"easy42/internal/config"
)

func TestPlanSyncDiffingWithState(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-engine-diff-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := NewManager(store)
	if err := mgr.Unlock(pass); err != nil {
		t.Fatalf("Failed to unlock: %v", err)
	}

	nodeA := config.Node{
		Name:      "node-a",
		Host:      "127.0.0.1",
		IP:        "192.168.100.1",
		Interface: "lo",
		ASN:       4299420001,
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "127.0.0.1",
		IP:        "192.168.100.2",
		Interface: "lo",
		ASN:       4299420002,
	}

	if err := mgr.AddNode(nodeA); err != nil {
		t.Fatalf("AddNode A failed: %v", err)
	}
	if err := mgr.AddNode(nodeB); err != nil {
		t.Fatalf("AddNode B failed: %v", err)
	}

	link, err := mgr.AddLink("node-a", "node-b", 51820, 51821, nil)
	if err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}

	// 1. Initially without state: both actions must be pending / needs_apply = true
	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("Expected 2 actions, got %d", len(actions))
	}
	for _, act := range actions {
		if !act.NeedsApply {
			t.Errorf("Expected action on %s (%s) to need apply initially", act.NodeName, act.Interface)
		}
		if act.Status != "pending" {
			t.Errorf("Expected status 'pending', got %s", act.Status)
		}
	}

	// 2. Simulate successful application of node-a's interface by updating stateStore
	fromConf, _ := compiler.GenerateWgConfigContent(&nodeA, &nodeB, &link.From, &link.To, mgr.Vault())
	fromHash := config.HashConfig(compiler.NormalizeConfig(fromConf))

	err = mgr.StateStore().UpdateInterface("node-a", "127.0.0.1", config.StateInterface{
		Name:       link.From.Interface,
		TargetFile: "/etc/wireguard/" + link.From.Interface + ".conf",
		ConfigHash: fromHash,
		Status:     "active",
		AppliedAt:  time.Now(),
	})
	if err != nil {
		t.Fatalf("UpdateInterface failed: %v", err)
	}

	// Re-plan: node-a should now be synced (needs_apply = false), while node-b is still pending
	actions2, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync 2 failed: %v", err)
	}
	if len(actions2) != 2 {
		t.Fatalf("Expected 2 actions, got %d", len(actions2))
	}

	var nodeAAction, nodeBAction *config.SyncAction
	for i := range actions2 {
		if actions2[i].NodeName == "node-a" {
			nodeAAction = &actions2[i]
		} else if actions2[i].NodeName == "node-b" {
			nodeBAction = &actions2[i]
		}
	}

	if nodeAAction == nil || nodeBAction == nil {
		t.Fatalf("Missing node action in planned list")
	}

	if nodeAAction.NeedsApply {
		t.Errorf("Expected node-a action to NOT need apply (already synced)")
	}
	if nodeAAction.Status != "synced" {
		t.Errorf("Expected node-a status 'synced', got %s", nodeAAction.Status)
	}

	if !nodeBAction.NeedsApply {
		t.Errorf("Expected node-b action to need apply (pending)")
	}
	if nodeBAction.Status != "pending" {
		t.Errorf("Expected node-b status 'pending', got %s", nodeBAction.Status)
	}

	// 3. Mark node-b also synced
	toConf, _ := compiler.GenerateWgConfigContent(&nodeB, &nodeA, &link.To, &link.From, mgr.Vault())
	toHash := config.HashConfig(compiler.NormalizeConfig(toConf))
	_ = mgr.StateStore().UpdateInterface("node-b", "127.0.0.1", config.StateInterface{
		Name:       link.To.Interface,
		TargetFile: "/etc/wireguard/" + link.To.Interface + ".conf",
		ConfigHash: toHash,
		Status:     "active",
		AppliedAt:  time.Now(),
	})

	actions3, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync 3 failed: %v", err)
	}
	for _, act := range actions3 {
		if act.NeedsApply {
			t.Errorf("Expected all actions to be synced, but %s needs apply", act.NodeName)
		}
	}

	// 4. Update the link (e.g. change MTU or ports), diff should detect update
	_, err = mgr.UpdateLink("node-a", "node-b", 51830, 51831, nil, 1380, 1380)
	if err != nil {
		t.Fatalf("UpdateLink failed: %v", err)
	}

	actions4, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync 4 failed: %v", err)
	}
	for _, act := range actions4 {
		if !act.NeedsApply {
			t.Errorf("Expected updated link actions to need apply, but %s is marked not needed", act.NodeName)
		}
		if act.DiffStatus != "update" {
			t.Errorf("Expected DiffStatus 'update', got '%s'", act.DiffStatus)
		}
	}
}

func TestInterfaceWorkingStateDerivation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-working-state-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	stateStore := config.NewStateStore(tempDir)
	_, _ = stateStore.Load()

	now := time.Now()
	recentHandshake := now.Add(-30 * time.Second)
	oldHandshake := now.Add(-10 * time.Minute)

	// Case 1: Handshake within 3 minutes -> working
	iface1 := config.StateInterface{
		Name:            "wg42nodeb",
		Status:          "active",
		LatestHandshake: &recentHandshake,
		WorkingState:    config.WorkingStateWorking,
	}
	if iface1.WorkingState != config.WorkingStateWorking {
		t.Errorf("Expected working state")
	}

	// Case 2: Handshake > 3 minutes with keepalive -> not_working
	iface2 := config.StateInterface{
		Name:            "wg42nodec",
		Status:          "active",
		LatestHandshake: &oldHandshake,
		WorkingState:    config.WorkingStateNotWorking,
	}
	if iface2.WorkingState != config.WorkingStateNotWorking {
		t.Errorf("Expected not_working state")
	}

	// Case 3: Handshake > 3 minutes without keepalive -> unknown
	iface3 := config.StateInterface{
		Name:            "wg42noded",
		Status:          "active",
		LatestHandshake: &oldHandshake,
		WorkingState:    config.WorkingStateUnknown,
	}
	if iface3.WorkingState != config.WorkingStateUnknown {
		t.Errorf("Expected unknown state")
	}

	_ = stateStore.UpdateInterface("node-a", "127.0.0.1", iface1)
	_ = stateStore.UpdateInterface("node-a", "127.0.0.1", iface2)
	_ = stateStore.UpdateInterface("node-a", "127.0.0.1", iface3)

	st := stateStore.Get()
	if st.Nodes["node-a"].Interfaces["wg42nodeb"].WorkingState != config.WorkingStateWorking {
		t.Errorf("Persisted state mismatch for wg42nodeb")
	}
	if st.Nodes["node-a"].Interfaces["wg42nodec"].WorkingState != config.WorkingStateNotWorking {
		t.Errorf("Persisted state mismatch for wg42nodec")
	}
	if st.Nodes["node-a"].Interfaces["wg42noded"].WorkingState != config.WorkingStateUnknown {
		t.Errorf("Persisted state mismatch for wg42noded")
	}
}
