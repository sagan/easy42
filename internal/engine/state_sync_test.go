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
		ASN:       4224420001,
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "127.0.0.1",
		IP:        "192.168.100.2",
		Interface: "lo",
		ASN:       4224420002,
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

	// 1. Initially without state: both WG and BIRD actions must be pending / needs_apply = true
	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}
	if len(actions) != 4 {
		t.Fatalf("Expected 4 actions (2 WG + 2 BIRD), got %d", len(actions))
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

	// Re-plan: node-a's WG action should now be synced, while node-b's WG action is still pending
	actions2, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync 2 failed: %v", err)
	}
	if len(actions2) != 4 {
		t.Fatalf("Expected 4 actions, got %d", len(actions2))
	}

	var nodeAWgAction, nodeBWgAction *config.SyncAction
	for i := range actions2 {
		if actions2[i].Type == config.ActionSyncConfig {
			if actions2[i].NodeName == "node-a" {
				nodeAWgAction = &actions2[i]
			} else if actions2[i].NodeName == "node-b" {
				nodeBWgAction = &actions2[i]
			}
		}
	}

	if nodeAWgAction == nil || nodeBWgAction == nil {
		t.Fatalf("Missing node wireguard action in planned list")
	}

	if nodeAWgAction.NeedsApply {
		t.Errorf("Expected node-a WG action to NOT need apply (already synced)")
	}
	if nodeAWgAction.Status != "synced" {
		t.Errorf("Expected node-a WG status 'synced', got %s", nodeAWgAction.Status)
	}

	if !nodeBWgAction.NeedsApply {
		t.Errorf("Expected node-b WG action to need apply (pending)")
	}
	if nodeBWgAction.Status != "pending" {
		t.Errorf("Expected node-b WG status 'pending', got %s", nodeBWgAction.Status)
	}

	// 3. Mark node-b WG interface also synced, as well as both BIRD configs
	toConf, _ := compiler.GenerateWgConfigContent(&nodeB, &nodeA, &link.To, &link.From, mgr.Vault())
	toHash := config.HashConfig(compiler.NormalizeConfig(toConf))
	_ = mgr.StateStore().UpdateInterface("node-b", "127.0.0.1", config.StateInterface{
		Name:       link.To.Interface,
		TargetFile: "/etc/wireguard/" + link.To.Interface + ".conf",
		ConfigHash: toHash,
		Status:     "active",
		AppliedAt:  time.Now(),
	})

	birdA, _ := compiler.GenerateBirdConfig(&nodeA, []config.Node{nodeA, nodeB}, []config.Link{*link})
	_ = mgr.StateStore().UpdateBirdState("node-a", "127.0.0.1", config.HashConfig(compiler.NormalizeConfig(birdA)), time.Now())

	birdB, _ := compiler.GenerateBirdConfig(&nodeB, []config.Node{nodeA, nodeB}, []config.Link{*link})
	_ = mgr.StateStore().UpdateBirdState("node-b", "127.0.0.1", config.HashConfig(compiler.NormalizeConfig(birdB)), time.Now())

	actions3, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync 3 failed: %v", err)
	}
	for _, act := range actions3 {
		if act.NeedsApply {
			t.Errorf("Expected all actions to be synced, but %s (%s) needs apply", act.NodeName, act.Interface)
		}
	}

	// 4. Update the link (e.g. change MTU or ports), diff should detect update on WG actions
	_, err = mgr.UpdateLink("node-a", "node-b", 51830, 51831, nil, 1380, 1380)
	if err != nil {
		t.Fatalf("UpdateLink failed: %v", err)
	}

	actions4, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync 4 failed: %v", err)
	}
	for _, act := range actions4 {
		if act.Type == config.ActionSyncConfig {
			if !act.NeedsApply {
				t.Errorf("Expected updated link actions to need apply, but %s is marked not needed", act.NodeName)
			}
			if act.DiffStatus != "update" {
				t.Errorf("Expected DiffStatus 'update', got '%s'", act.DiffStatus)
			}
		}
	}
}

func TestPlanSyncBirdConfigDiffing(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-bird-diff-test-*")
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

	node := config.Node{
		Name:      "bird-node",
		Host:      "127.0.0.1",
		IP:        "192.168.100.5",
		Interface: "lo",
		ASN:       4224420005,
	}
	if err := mgr.AddNode(node); err != nil {
		t.Fatalf("AddNode failed: %v", err)
	}

	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}

	var birdAction *config.SyncAction
	for i := range actions {
		if actions[i].Type == config.ActionSyncBirdConfig && actions[i].NodeName == "bird-node" {
			birdAction = &actions[i]
			break
		}
	}
	if birdAction == nil {
		t.Fatalf("Expected BIRD sync action for bird-node")
	}
	if birdAction.TargetFile != "/etc/bird_easy42.conf" {
		t.Errorf("Expected target file /etc/bird_easy42.conf, got %s", birdAction.TargetFile)
	}
	if birdAction.Command != "birdc configure" {
		t.Errorf("Expected command 'birdc configure', got %s", birdAction.Command)
	}
	if !birdAction.NeedsApply {
		t.Errorf("Expected NeedsApply initially")
	}

	// Record synced BIRD state
	hash := config.HashConfig(compiler.NormalizeConfig(birdAction.FileContent))
	_ = mgr.StateStore().UpdateBirdState("bird-node", "10.0.0.1", hash, time.Now())

	actionsAfterSync, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync after state update failed: %v", err)
	}
	var birdActionSynced *config.SyncAction
	for i := range actionsAfterSync {
		if actionsAfterSync[i].Type == config.ActionSyncBirdConfig && actionsAfterSync[i].NodeName == "bird-node" {
			birdActionSynced = &actionsAfterSync[i]
			break
		}
	}
	if birdActionSynced.NeedsApply {
		t.Errorf("Expected BIRD action to be synced (needsApply = false)")
	}
	if birdActionSynced.DiffStatus != "synced" {
		t.Errorf("Expected DiffStatus 'synced', got %s", birdActionSynced.DiffStatus)
	}

	// Update node ASN, BIRD config diff should be triggered
	node.ASN = 4224420099
	_ = mgr.UpdateNode(node.Name, node)

	actionsAfterNodeUpdate, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync after node update failed: %v", err)
	}
	var birdActionUpdated *config.SyncAction
	for i := range actionsAfterNodeUpdate {
		if actionsAfterNodeUpdate[i].Type == config.ActionSyncBirdConfig && actionsAfterNodeUpdate[i].NodeName == "bird-node" {
			birdActionUpdated = &actionsAfterNodeUpdate[i]
			break
		}
	}
	if !birdActionUpdated.NeedsApply {
		t.Errorf("Expected BIRD action to need apply after ASN change")
	}
	if birdActionUpdated.DiffStatus != "update" {
		t.Errorf("Expected DiffStatus 'update', got %s", birdActionUpdated.DiffStatus)
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
