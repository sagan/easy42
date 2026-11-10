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
