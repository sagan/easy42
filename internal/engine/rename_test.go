package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"easy42/internal/config"
)

func TestManager_RenameNode(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-rename-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to initialize store: %v", err)
	}

	mgr := NewManager(store)
	if err := mgr.Unlock(pass); err != nil {
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	nodeA := config.Node{
		Name:      "node-a",
		Host:      "192.168.1.10",
		IP:        "192.168.100.10",
		Interface: "lo",
		ASN:       4224420001,
		Entrypoints: []config.Entrypoint{
			{IP: "1.1.1.1", MTU: 1420},
		},
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "192.168.1.20",
		IP:        "192.168.100.20",
		Interface: "lo",
		ASN:       4224420002,
		Entrypoints: []config.Entrypoint{
			{IP: "2.2.2.2", MTU: 1420},
		},
	}
	peerExt := config.Node{
		Name:       "dn42ext",
		IsExternal: true,
		ASN:        4242421234,
		Entrypoints: []config.Entrypoint{
			{IP: "3.3.3.3"},
		},
	}

	if err := mgr.AddNode(nodeA); err != nil {
		t.Fatalf("AddNode A failed: %v", err)
	}
	if err := mgr.AddNode(nodeB); err != nil {
		t.Fatalf("AddNode B failed: %v", err)
	}
	if err := mgr.AddNode(peerExt); err != nil {
		t.Fatalf("AddNode peerExt failed: %v", err)
	}

	// Add link between nodeA and nodeB
	_, err = mgr.AddLink("node-a", "node-b", 51820, 51821, nil)
	if err != nil {
		t.Fatalf("AddLink A-B failed: %v", err)
	}

	// Add link between nodeA and peerExt
	_, err = mgr.AddLink("node-a", "dn42ext", 51822, 51823, nil)
	if err != nil {
		t.Fatalf("AddLink A-dn42ext failed: %v", err)
	}

	// Verify initial links
	links := mgr.GetLinks()
	if len(links) != 2 {
		t.Fatalf("Expected 2 links, got %d", len(links))
	}

	// 1. Check duplicate name error
	_, err = mgr.RenameNode("node-a", "node-b")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Expected error when renaming to existing name, got: %v", err)
	}

	// 2. Check invalid name lengths
	_, err = mgr.RenameNode("node-a", "toolongnodename")
	if err == nil || !strings.Contains(err.Error(), "between 1 and 11") {
		t.Fatalf("Expected length error for internal node > 11 chars, got: %v", err)
	}

	_, err = mgr.RenameNode("dn42ext", "toolongpeername")
	if err == nil || !strings.Contains(err.Error(), "between 1 and 10") {
		t.Fatalf("Expected length error for external peer > 10 chars, got: %v", err)
	}

	// 3. Check invalid characters
	_, err = mgr.RenameNode("node-a", "bad name!")
	if err == nil {
		t.Fatalf("Expected error for invalid characters")
	}

	// 4. Check nonexistent node
	_, err = mgr.RenameNode("nonexistent", "newname")
	if err != ErrNodeNotFound {
		t.Fatalf("Expected ErrNodeNotFound, got: %v", err)
	}

	// 5. Successful rename of managed node: "node-a" -> "node-alpha"
	updated, err := mgr.RenameNode("node-a", "node-alpha")
	if err != nil {
		t.Fatalf("RenameNode node-a -> node-alpha failed: %v", err)
	}
	if updated.Name != "node-alpha" {
		t.Fatalf("Expected updated name 'node-alpha', got %q", updated.Name)
	}

	// Verify in memory
	if mgr.GetNode("node-a") != nil {
		t.Fatalf("Old node-a should not exist in manager")
	}
	nodeAlpha := mgr.GetNode("node-alpha")
	if nodeAlpha == nil {
		t.Fatalf("node-alpha should exist in manager")
	}

	// Verify links in memory
	links = mgr.GetLinks()
	foundAB := false
	foundAExt := false
	for _, l := range links {
		if l.From.Name == "node-alpha" && l.To.Name == "node-b" {
			foundAB = true
			if l.From.Interface != "wg42node-b" {
				t.Errorf("Expected From.Interface wg42node-b, got %s", l.From.Interface)
			}
			if l.To.Interface != "wg42node-alpha" {
				t.Errorf("Expected To.Interface wg42node-alpha, got %s", l.To.Interface)
			}
		}
		isAlphaExt := (l.From.Name == "node-alpha" && l.To.Name == "dn42ext") || (l.From.Name == "dn42ext" && l.To.Name == "node-alpha")
		if isAlphaExt {
			foundAExt = true
			if l.From.Name == "node-alpha" {
				if l.From.Interface != "wg42-dn42ext" {
					t.Errorf("Expected From.Interface wg42-dn42ext, got %s", l.From.Interface)
				}
				if l.To.Interface != "wg42node-alpha" {
					t.Errorf("Expected To.Interface wg42node-alpha, got %s", l.To.Interface)
				}
			} else {
				if l.To.Interface != "wg42-dn42ext" {
					t.Errorf("Expected To.Interface wg42-dn42ext, got %s", l.To.Interface)
				}
				if l.From.Interface != "wg42node-alpha" {
					t.Errorf("Expected From.Interface wg42node-alpha, got %s", l.From.Interface)
				}
			}
		}
	}
	if !foundAB || !foundAExt {
		t.Fatalf("Did not find updated links after rename: ab=%v aext=%v", foundAB, foundAExt)
	}

	// 6. Verify config.json file content on disk directly
	configFile := filepath.Join(tempDir, "config.json")
	diskData, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Failed to read config.json: %v", err)
	}

	var diskCfg config.Config
	if err := json.Unmarshal(diskData, &diskCfg); err != nil {
		t.Fatalf("Failed to unmarshal saved config.json: %v", err)
	}

	// Ensure old name "node-a" does not appear as any Node Name or Link Name
	for _, n := range diskCfg.Nodes {
		if n.Name == "node-a" {
			t.Errorf("Found old node-a in disk config.json")
		}
	}
	for _, l := range diskCfg.Links {
		if l.From.Name == "node-a" || l.To.Name == "node-a" {
			t.Errorf("Found old node-a in disk config.json links")
		}
		if l.From.Interface == "wg42node-a" || l.To.Interface == "wg42node-a" {
			t.Errorf("Found old wg42node-a interface in disk config.json")
		}
	}

	// 7. Successful rename of external peer: "dn42ext" -> "dn42-peer"
	extUpdated, err := mgr.RenameNode("dn42ext", "dn42-peer")
	if err != nil {
		t.Fatalf("RenameNode dn42ext -> dn42-peer failed: %v", err)
	}
	if extUpdated.Name != "dn42-peer" {
		t.Fatalf("Expected ext name 'dn42-peer', got %q", extUpdated.Name)
	}

	// Check interface in link is wg42-dn42-peer
	links = mgr.GetLinks()
	foundNewExt := false
	for _, l := range links {
		isAlphaPeer := (l.From.Name == "node-alpha" && l.To.Name == "dn42-peer") || (l.From.Name == "dn42-peer" && l.To.Name == "node-alpha")
		if isAlphaPeer {
			foundNewExt = true
			if l.From.Name == "node-alpha" {
				if l.From.Interface != "wg42-dn42-peer" {
					t.Errorf("Expected From.Interface wg42-dn42-peer, got %s", l.From.Interface)
				}
			} else {
				if l.To.Interface != "wg42-dn42-peer" {
					t.Errorf("Expected To.Interface wg42-dn42-peer, got %s", l.To.Interface)
				}
			}
		}
	}
	if !foundNewExt {
		t.Fatalf("Expected link between node-alpha and dn42-peer not found")
	}
}
