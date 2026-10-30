package engine

import (
	"testing"
	"time"

	"easy42/internal/config"
)

func setupTestManagerWithNodes(t *testing.T) *Manager {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	cfg := &config.Config{
		Nodes: []config.Node{
			{Name: "alice", IP: "192.168.100.1", IsExternal: false},
			{Name: "bob", IP: "192.168.100.2", IsExternal: false},
			{Name: "carol", IP: "192.168.100.3", IsExternal: false},
			{Name: "external1", IP: "1.1.1.1", IsExternal: true},
		},
		Links: []config.Link{
			{
				From: config.LinkEnd{Name: "alice", Interface: "wg42bob", Address: "fe80::c0a8:6401/64"},
				To:   config.LinkEnd{Name: "bob", Interface: "wg42alice", Address: "fe80::c0a8:6402/64"},
			},
		},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("failed to save store: %v", err)
	}

	stateStore := config.NewStateStore(tmpDir)
	if _, err := stateStore.Load(); err != nil {
		t.Fatalf("failed to load stateStore: %v", err)
	}

	m := NewManager(store)
	return m
}

func TestPingTable_E2E_NoAgent(t *testing.T) {
	m := setupTestManagerWithNodes(t)

	task, err := m.RunPingTable("e2e")
	if err != nil {
		t.Fatalf("RunPingTable failed: %v", err)
	}

	if task.ID == "" {
		t.Fatalf("expected non-empty task ID")
	}
	if len(task.Nodes) != 3 {
		t.Fatalf("expected 3 internal nodes (alice, bob, carol), got %d: %v", len(task.Nodes), task.Nodes)
	}
	if task.TotalPairs != 6 {
		t.Fatalf("expected 6 pairs (3*2 directed), got %d", task.TotalPairs)
	}

	// Wait briefly for background execution to complete (no agent -> immediate failure per pair)
	time.Sleep(200 * time.Millisecond)

	current, err := m.GetPingTableTask(task.ID)
	if err != nil {
		t.Fatalf("GetPingTableTask failed: %v", err)
	}
	if current == nil {
		t.Fatalf("expected task in db, got nil")
	}

	if current.Status != "completed" {
		t.Fatalf("expected task status completed, got %s", current.Status)
	}

	// Check diagonal
	for _, n := range current.Nodes {
		diag := current.Results[n][n]
		if diag == nil || diag.Status != "self" {
			t.Fatalf("expected self status on diagonal for %s, got: %+v", n, diag)
		}
	}

	// Check symmetry
	ab := current.Results["alice"]["bob"]
	ba := current.Results["bob"]["alice"]
	if ab == nil || ba == nil {
		t.Fatalf("expected non-nil results for alice-bob")
	}
	if ab.Status != "no_agent" || ba.Status != "no_agent" {
		t.Fatalf("expected no_agent status, got ab=%s, ba=%s", ab.Status, ba.Status)
	}
}

func TestPingTable_Link_NoLinkDetected(t *testing.T) {
	m := setupTestManagerWithNodes(t)

	task, err := m.RunPingTable("link")
	if err != nil {
		t.Fatalf("RunPingTable link failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	current, err := m.GetPingTableTask(task.ID)
	if err != nil {
		t.Fatalf("GetPingTableTask failed: %v", err)
	}

	// alice and carol have no direct link
	ac := current.Results["alice"]["carol"]
	ca := current.Results["carol"]["alice"]
	if ac == nil || ca == nil {
		t.Fatalf("expected non-nil results for alice-carol")
	}
	if ac.Status != "no_link" || ca.Status != "no_link" {
		t.Fatalf("expected no_link status for alice-carol, got ac=%s, ca=%s", ac.Status, ca.Status)
	}

	// alice and bob DO have a link, but no agent connected
	ab := current.Results["alice"]["bob"]
	if ab == nil || ab.Status != "no_agent" {
		t.Fatalf("expected no_agent status for alice-bob, got: %+v", ab)
	}
}

func TestPingTable_Cancel(t *testing.T) {
	m := setupTestManagerWithNodes(t)

	task, err := m.RunPingTable("e2e")
	if err != nil || task == nil {
		t.Fatalf("RunPingTable failed: %v", err)
	}

	if err := m.CancelPingTable(); err != nil {
		t.Fatalf("CancelPingTable failed: %v", err)
	}

	active := m.GetActivePingTable()
	if active != nil && active.Status != "cancelled" && active.Status != "completed" {
		t.Fatalf("expected cancelled or completed, got: %s", active.Status)
	}
}

func TestPingTable_HistoryAndSummary(t *testing.T) {
	m := setupTestManagerWithNodes(t)

	task, err := m.RunPingTable("e2e")
	if err != nil {
		t.Fatalf("RunPingTable failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	history, err := m.GetPingTableHistory(10)
	if err != nil {
		t.Fatalf("GetPingTableHistory failed: %v", err)
	}
	if len(history) == 0 {
		t.Fatalf("expected at least 1 history record")
	}
	if history[0].ID != task.ID {
		t.Fatalf("expected history ID %s, got %s", task.ID, history[0].ID)
	}
	if history[0].NodeCount != 3 {
		t.Fatalf("expected 3 nodes, got %d", history[0].NodeCount)
	}

	// Test Delete
	if err := m.DeletePingTableTask(task.ID); err != nil {
		t.Fatalf("DeletePingTableTask failed: %v", err)
	}
	deleted, err := m.GetPingTableTask(task.ID)
	if err != nil {
		t.Fatalf("expected no error fetching deleted task, got %v", err)
	}
	if deleted != nil {
		t.Fatalf("expected nil after delete, got %+v", deleted)
	}
}
