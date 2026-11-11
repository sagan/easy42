package engine

import (
	"strings"
	"testing"
	"time"

	"easy42/internal/config"
)

func TestHostsBlockPlanningAndSync(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	mgr := NewManager(store)
	defer mgr.Close()
	if err := mgr.Unlock(pass); err != nil {
		t.Fatalf("failed to unlock: %v", err)
	}

	cfg := &config.Config{
		Nodes: []config.Node{
			{
				Name:        "foo",
				IP:          "192.168.100.1",
				Mode:        "agent",
				AgentToken:  "token-foo",
				UpdateHosts: true,
			},
			{
				Name:        "bar",
				IP:          "192.168.100.2",
				Mode:        "agent",
				AgentToken:  "token-bar",
				UpdateHosts: false,
			},
			{
				Name:       "peer-dn42",
				IP:         "172.20.1.1",
				IsExternal: true,
			},
		},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// 1. Verify ExpectedHostsBlock
	block, hash := mgr.ExpectedHostsBlock()
	expectedBlock := "### EASY42_START\n192.168.100.2 bar bar.internal\n192.168.100.1 foo foo.internal\n### EASY42_END\n"
	if block != expectedBlock {
		t.Fatalf("expected hosts block:\n%q\ngot:\n%q", expectedBlock, block)
	}
	if hash == "" {
		t.Fatal("expected non-empty hosts block hash")
	}

	// 2. PlanSync: only "foo" should have ActionSyncHosts because UpdateHosts=true and IsAgentMode=true
	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}

	var fooHostAction *config.SyncAction
	for _, act := range actions {
		if act.Type == config.ActionSyncHosts {
			if act.NodeName == "bar" {
				t.Fatalf("bar should not have ActionSyncHosts because UpdateHosts=false")
			}
			if act.NodeName == "foo" {
				copyAct := act
				fooHostAction = &copyAct
			}
		}
	}

	if fooHostAction == nil {
		t.Fatalf("expected ActionSyncHosts for foo, but none found")
	}
	if !fooHostAction.NeedsApply {
		t.Fatalf("expected NeedsApply=true for foo when hash not recorded")
	}
	if fooHostAction.TargetFile != "/etc/hosts" {
		t.Fatalf("expected TargetFile=/etc/hosts, got %s", fooHostAction.TargetFile)
	}
	if !strings.Contains(fooHostAction.FileContent, "192.168.100.2 bar bar.internal") {
		t.Fatalf("expected FileContent to contain bar record, got %s", fooHostAction.FileContent)
	}

	// 3. Mark foo as already synced in stateStore with matching hash
	_ = mgr.stateStore.UpdateHostsState("foo", "foo", hash, time.Now())

	actionsAfterSync, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync after state update failed: %v", err)
	}
	var fooActionAfter *config.SyncAction
	for _, act := range actionsAfterSync {
		if act.Type == config.ActionSyncHosts && act.NodeName == "foo" {
			copyAct := act
			fooActionAfter = &copyAct
			break
		}
	}
	if fooActionAfter == nil {
		t.Fatalf("expected ActionSyncHosts to still be present in actions list")
	}
	if fooActionAfter.NeedsApply {
		t.Fatalf("expected NeedsApply=false when state hash matches desired hash")
	}
	if fooActionAfter.DiffStatus != "synced" {
		t.Fatalf("expected DiffStatus=synced, got %s", fooActionAfter.DiffStatus)
	}
}

func TestBuildClusterHostRecords(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	_, _ = store.Initialize()
	mgr := NewManager(store)
	defer mgr.Close()

	_ = mgr.store.Save(&config.Config{
		Nodes: []config.Node{
			{Name: "zeta", IP: "10.0.0.9"},
			{Name: "alpha", IP: "10.0.0.1"},
			{Name: "ext", IP: "1.2.3.4", IsExternal: true},
			{Name: "no-ip", IP: ""},
		},
	})

	records := mgr.GetClusterHostRecords()
	if len(records) != 2 {
		t.Fatalf("expected 2 valid cluster records, got %d", len(records))
	}

	protoRecords := mgr.buildClusterHostRecords()
	if len(protoRecords) != 2 {
		t.Fatalf("expected 2 proto records, got %d", len(protoRecords))
	}

	if protoRecords[0].Name != "zeta" || protoRecords[0].Ip != "10.0.0.9" {
		t.Fatalf("unexpected record 0: %+v", protoRecords[0])
	}
	if protoRecords[1].Name != "alpha" || protoRecords[1].Ip != "10.0.0.1" {
		t.Fatalf("unexpected record 1: %+v", protoRecords[1])
	}
}

func TestHostsExtraMergingAndPriority(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	pass, _ := store.Initialize()
	mgr := NewManager(store)
	defer mgr.Close()
	_ = mgr.Unlock(pass)

	cfg := &config.Config{
		NetworkSettings: config.NetworkSettings{
			ExtraHosts: "10.0.0.2 bar\n10.0.0.50 global.lan\n",
		},
		Nodes: []config.Node{
			{
				Name:        "nodeA",
				IP:          "192.168.1.1",
				Mode:        "agent",
				AgentToken:  "token-a",
				UpdateHosts: true,
				ExtraHosts:  "172.16.0.1 bar\n172.16.0.10 custom-a.lan\n",
			},
			{
				Name:        "nodeB",
				IP:          "192.168.1.2",
				Mode:        "agent",
				AgentToken:  "token-b",
				UpdateHosts: true,
				ExtraHosts:  "",
			},
			{
				Name: "bar",
				IP:   "192.168.1.99",
			},
		},
	}
	_ = mgr.store.Save(cfg)

	// NodeA should have bar overridden by nodeA ExtraHosts (172.16.0.1)
	nodeARecords := mgr.GetHostRecordsForNode("nodeA")
	lookupA := make(map[string]string)
	for _, r := range nodeARecords {
		lookupA[r.Name] = r.IP
		for _, a := range r.Aliases {
			lookupA[a] = r.IP
		}
	}
	if lookupA["bar"] != "172.16.0.1" {
		t.Fatalf("expected nodeA bar -> 172.16.0.1 (node extra priority), got %s", lookupA["bar"])
	}
	if lookupA["custom-a.lan"] != "172.16.0.10" {
		t.Fatalf("expected custom-a.lan -> 172.16.0.10, got %s", lookupA["custom-a.lan"])
	}
	if lookupA["global.lan"] != "10.0.0.50" {
		t.Fatalf("expected global.lan -> 10.0.0.50, got %s", lookupA["global.lan"])
	}

	// NodeB should NOT have nodeA's extra hosts; bar should be overridden by Global ExtraHosts (10.0.0.2)
	nodeBRecords := mgr.GetHostRecordsForNode("nodeB")
	lookupB := make(map[string]string)
	for _, r := range nodeBRecords {
		lookupB[r.Name] = r.IP
		for _, a := range r.Aliases {
			lookupB[a] = r.IP
		}
	}
	if lookupB["bar"] != "10.0.0.2" {
		t.Fatalf("expected nodeB bar -> 10.0.0.2 (global extra priority), got %s", lookupB["bar"])
	}
	if _, exists := lookupB["custom-a.lan"]; exists {
		t.Fatalf("nodeB should not have custom-a.lan from nodeA")
	}
	if lookupB["global.lan"] != "10.0.0.50" {
		t.Fatalf("expected nodeB global.lan -> 10.0.0.50, got %s", lookupB["global.lan"])
	}

	// Verify buildHostRecordsForNode sends correct proto records to agent
	protoA := mgr.buildHostRecordsForNode("nodeA")
	foundBar := false
	for _, p := range protoA {
		if p.Name == "bar" && p.Ip == "172.16.0.1" {
			foundBar = true
			break
		}
	}
	if !foundBar {
		t.Fatalf("expected protoA to contain bar with 172.16.0.1")
	}
}

func TestPlanSyncHostsPendingChanges(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	pass, _ := store.Initialize()
	mgr := NewManager(store)
	defer mgr.Close()
	_ = mgr.Unlock(pass)

	cfg := &config.Config{
		Nodes: []config.Node{
			{
				Name:        "nodeA",
				Host:        "1.1.1.1",
				IP:          "192.168.1.1",
				Mode:        "agent",
				AgentToken:  "token-a",
				UpdateHosts: true,
			},
			{
				Name:        "nodeB",
				Host:        "2.2.2.2",
				IP:          "192.168.1.2",
				Mode:        "agent",
				AgentToken:  "token-b",
				UpdateHosts: true,
			},
		},
	}
	_ = mgr.store.Save(cfg)

	// Initial PlanSync: both nodes need ActionSyncHosts
	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}
	t.Logf("Initial PlanSync actions count: %d", len(actions))
	for _, act := range actions {
		t.Logf("act: type=%s node=%s needsApply=%v diffStatus=%s", act.Type, act.NodeName, act.NeedsApply, act.DiffStatus)
	}

	// Simulate that nodeA and nodeB were successfully synced:
	_, hashA := mgr.ExpectedHostsBlockForNode("nodeA")
	_, hashB := mgr.ExpectedHostsBlockForNode("nodeB")
	_ = mgr.stateStore.UpdateHostsState("nodeA", "nodeA", hashA, time.Now())
	_ = mgr.stateStore.UpdateHostsState("nodeB", "nodeB", hashB, time.Now())

	// PlanSync immediately after: everything should be synced (NeedsApply=false)
	actionsAfterSync, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync after sync failed: %v", err)
	}
	for _, act := range actionsAfterSync {
		if act.Type == config.ActionSyncHosts && act.NeedsApply {
			t.Fatalf("expected ActionSyncHosts to have NeedsApply=false after sync, got true for %s", act.NodeName)
		}
	}


	// Now: change nodeB's IP
	updatedB := cfg.Nodes[1]
	updatedB.IP = "192.168.1.200"
	if err := mgr.UpdateNode("nodeB", updatedB); err != nil {
		t.Fatalf("UpdateNode failed: %v", err)
	}

	// PlanSync for nodeA: does nodeA have pending ActionSyncHosts?
	actionsNodeA, err := mgr.PlanSync("nodeA")
	if err != nil {
		t.Fatalf("PlanSync(nodeA) failed: %v", err)
	}
	foundNodeAHosts := false
	for _, act := range actionsNodeA {
		t.Logf("actionsNodeA act: type=%s node=%s needsApply=%v diffStatus=%s", act.Type, act.NodeName, act.NeedsApply, act.DiffStatus)
		if act.Type == config.ActionSyncHosts && act.NodeName == "nodeA" {
			foundNodeAHosts = true
			if !act.NeedsApply {
				t.Errorf("expected nodeA ActionSyncHosts to have NeedsApply=true after nodeB IP change, got false")
			}
		}
	}
	if !foundNodeAHosts {
		t.Errorf("expected ActionSyncHosts for nodeA in PlanSync(nodeA), but none found")
	}

	// PlanSync for nodeB: does nodeB have pending ActionSyncHosts?
	actionsNodeB, err := mgr.PlanSync("nodeB")
	if err != nil {
		t.Fatalf("PlanSync(nodeB) failed: %v", err)
	}
	foundNodeBHosts := false
	for _, act := range actionsNodeB {
		t.Logf("actionsNodeB act: type=%s node=%s needsApply=%v diffStatus=%s", act.Type, act.NodeName, act.NeedsApply, act.DiffStatus)
		if act.Type == config.ActionSyncHosts && act.NodeName == "nodeB" {
			foundNodeBHosts = true
			if !act.NeedsApply {
				t.Errorf("expected nodeB ActionSyncHosts to have NeedsApply=true after nodeB IP change, got false")
			}
		}
	}
	if !foundNodeBHosts {
		t.Errorf("expected ActionSyncHosts for nodeB in PlanSync(nodeB), but none found")
	}

	// Sync both nodes again to establish baseline
	_, hashA2 := mgr.ExpectedHostsBlockForNode("nodeA")
	_, hashB2 := mgr.ExpectedHostsBlockForNode("nodeB")
	_ = mgr.stateStore.UpdateHostsState("nodeA", "nodeA", hashA2, time.Now())
	_ = mgr.stateStore.UpdateHostsState("nodeB", "nodeB", hashB2, time.Now())

	// Verify all synced
	actionsSynced2, _ := mgr.PlanSync()
	for _, act := range actionsSynced2 {
		if act.Type == config.ActionSyncHosts && act.NeedsApply {
			t.Fatalf("expected ActionSyncHosts to be synced, got NeedsApply=true for %s", act.NodeName)
		}
	}

	// Change nodeA extra hosts
	cfgAfterIP := mgr.store.Get()
	updatedA := cfgAfterIP.Nodes[0]
	updatedA.ExtraHosts = "10.10.10.1 test.custom\n"
	if err := mgr.UpdateNode("nodeA", updatedA); err != nil {
		t.Fatalf("UpdateNode with ExtraHosts failed: %v", err)
	}

	// nodeA should now have pending ActionSyncHosts, but nodeB should remain synced
	actionsNodeAExtra, _ := mgr.PlanSync("nodeA")
	foundPendingA := false
	for _, act := range actionsNodeAExtra {
		if act.Type == config.ActionSyncHosts && act.NodeName == "nodeA" {
			foundPendingA = act.NeedsApply
		}
	}
	if !foundPendingA {
		t.Errorf("expected nodeA ActionSyncHosts to have NeedsApply=true after nodeA ExtraHosts change")
	}

	actionsNodeBExtra, _ := mgr.PlanSync("nodeB")
	for _, act := range actionsNodeBExtra {
		if act.Type == config.ActionSyncHosts && act.NodeName == "nodeB" && act.NeedsApply {
			t.Errorf("expected nodeB ActionSyncHosts to stay synced when nodeA ExtraHosts changes")
		}
	}

	// Sync nodeA
	_, hashA3 := mgr.ExpectedHostsBlockForNode("nodeA")
	_ = mgr.stateStore.UpdateHostsState("nodeA", "nodeA", hashA3, time.Now())

	// Change Global extra hosts
	if err := mgr.UpdateNetworkSettings(config.NetworkSettings{
		ExtraHosts: "10.20.20.1 global.test\n",
	}); err != nil {
		t.Fatalf("UpdateNetworkSettings failed: %v", err)
	}

	// Both nodeA and nodeB should now have pending ActionSyncHosts
	actionsGlobal, _ := mgr.PlanSync()
	pendingCount := 0
	for _, act := range actionsGlobal {
		if act.Type == config.ActionSyncHosts && act.NeedsApply {
			pendingCount++
		}
	}
	if pendingCount != 2 {
		t.Errorf("expected 2 pending ActionSyncHosts after Global ExtraHosts change, got %d", pendingCount)
	}
}

func TestHostsDriftAndRemediation(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	pass, _ := store.Initialize()
	mgr := NewManager(store)
	defer mgr.Close()
	_ = mgr.Unlock(pass)

	cfg := &config.Config{
		Nodes: []config.Node{
			{
				Name:        "alpha",
				Host:        "1.1.1.1",
				IP:          "10.0.0.1",
				Mode:        "agent",
				AgentToken:  "token-alpha",
				UpdateHosts: true,
			},
		},
	}
	_ = mgr.store.Save(cfg)

	_, expectedHash := mgr.ExpectedHostsBlockForNode("alpha")

	// 1. When stateStore has not recorded applied hash (unapplied/pending):
	// checkAndSyncHostsIfMismatched should NOT push to agent
	mgr.checkAndSyncHostsIfMismatched("alpha", "different-hash")

	// 2. Mark as applied in stateStore
	_ = mgr.stateStore.UpdateHostsState("alpha", "1.1.1.1", expectedHash, time.Now())

	// PlanSync should be synced
	actions, err := mgr.PlanSync("alpha")
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}
	for _, act := range actions {
		if act.Type == config.ActionSyncHosts && act.NeedsApply {
			t.Errorf("expected ActionSyncHosts to be synced when appliedHash == expectedHash, got NeedsApply=true")
		}
	}
}



