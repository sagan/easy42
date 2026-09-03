package engine

import (
	"os"
	"strings"
	"testing"
	"time"

	"easy42/internal/compiler"
	"easy42/internal/config"
)

func TestAddLinkMTU(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-engine-test-*")
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
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	// 1. Add two nodes with default entrypoint MTU = 1500
	nodeA := config.Node{
		Name:      "node-a",
		Host:      "192.168.1.1",
		IP:        "192.168.100.1",
		Interface: "lo",
		ASN:       4299420001,
		Entrypoints: []config.Entrypoint{
			{
				IP:   "1.1.1.1",
				Tags: []string{"default"},
				MTU:  1500,
			},
		},
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "192.168.1.2",
		IP:        "192.168.100.2",
		Interface: "lo",
		ASN:       4299420002,
		Entrypoints: []config.Entrypoint{
			{
				IP:   "2.2.2.2",
				Tags: []string{"default"},
				MTU:  1500,
			},
		},
	}

	if err := mgr.AddNode(nodeA); err != nil {
		t.Fatalf("Failed to add nodeA: %v", err)
	}
	if err := mgr.AddNode(nodeB); err != nil {
		t.Fatalf("Failed to add nodeB: %v", err)
	}

	// Add link
	link, err := mgr.AddLink("node-a", "node-b", 0, 0, nil)
	if err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}

	// LinkEnd mtu should be 1500 - 80 = 1420
	if link.From.MTU != 1420 {
		t.Errorf("Expected link.From.MTU to be 1420, got %d", link.From.MTU)
	}
	if link.To.MTU != 1420 {
		t.Errorf("Expected link.To.MTU to be 1420, got %d", link.To.MTU)
	}

	// Link ports and endpoints
	if link.From.ListenPort != 28389 {
		t.Errorf("Expected link.From.ListenPort = 28389, got %d", link.From.ListenPort)
	}
	if link.From.Endpoint != "2.2.2.2:25532" {
		t.Errorf("Expected link.From.Endpoint = '2.2.2.2:25532', got '%s'", link.From.Endpoint)
	}
	if link.To.ListenPort != 25532 {
		t.Errorf("Expected link.To.ListenPort = 25532, got %d", link.To.ListenPort)
	}
	if link.To.Endpoint != "1.1.1.1:28389" {
		t.Errorf("Expected link.To.Endpoint = '1.1.1.1:28389', got '%s'", link.To.Endpoint)
	}

	// Test wg config content contains MTU = 1420
	cfgFrom, err := compiler.GenerateWgConfigContent(&nodeA, &nodeB, &link.From, &link.To, mgr.Vault())
	if err != nil {
		t.Fatalf("GenerateWgConfigContent failed: %v", err)
	}
	if !strings.Contains(cfgFrom, "MTU = 1420") {
		t.Errorf("Expected generated WireGuard config to contain 'MTU = 1420', got:\n%s", cfgFrom)
	}

	// 2. Add two nodes with custom jumbo MTU = 9000
	nodeC := config.Node{
		Name:      "node-c",
		Host:      "192.168.1.3",
		IP:        "192.168.100.3",
		Interface: "lo",
		ASN:       4299420003,
		Entrypoints: []config.Entrypoint{
			{
				IP:   "3.3.3.3",
				Tags: []string{"lan"},
				MTU:  9000,
			},
		},
	}
	nodeD := config.Node{
		Name:      "node-d",
		Host:      "192.168.1.4",
		IP:        "192.168.100.4",
		Interface: "lo",
		ASN:       4299420004,
		Entrypoints: []config.Entrypoint{
			{
				IP:   "4.4.4.4",
				Tags: []string{"lan"},
				MTU:  9000,
			},
		},
	}

	if err := mgr.AddNode(nodeC); err != nil {
		t.Fatalf("Failed to add nodeC: %v", err)
	}
	if err := mgr.AddNode(nodeD); err != nil {
		t.Fatalf("Failed to add nodeD: %v", err)
	}

	linkCD, err := mgr.AddLink("node-c", "node-d", 0, 0, nil)
	if err != nil {
		t.Fatalf("AddLink node-c node-d failed: %v", err)
	}

	// LinkEnd mtu should be 9000 - 80 = 8920
	if linkCD.From.MTU != 8920 {
		t.Errorf("Expected linkCD.From.MTU to be 8920, got %d", linkCD.From.MTU)
	}
	if linkCD.To.MTU != 8920 {
		t.Errorf("Expected linkCD.To.MTU to be 8920, got %d", linkCD.To.MTU)
	}

	cfgCD, err := compiler.GenerateWgConfigContent(&nodeC, &nodeD, &linkCD.From, &linkCD.To, mgr.Vault())
	if err != nil {
		t.Fatalf("GenerateWgConfigContent failed: %v", err)
	}
	if !strings.Contains(cfgCD, "MTU = 8920") {
		t.Errorf("Expected generated WireGuard config to contain 'MTU = 8920', got:\n%s", cfgCD)
	}
}

func TestUpdateLink(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-engine-update-link-*")
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
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	nodeA := config.Node{
		Name:      "node-a",
		Host:      "192.168.1.1",
		IP:        "192.168.100.1",
		Interface: "lo",
		ASN:       4299420001,
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "192.168.1.2",
		IP:        "192.168.100.2",
		Interface: "lo",
		ASN:       4299420002,
	}

	_ = mgr.AddNode(nodeA)
	_ = mgr.AddNode(nodeB)

	link, err := mgr.AddLink("node-a", "node-b", 51820, 51821, []string{"fast"})
	if err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}
	origPubKeyA := link.From.PublicKey

	// Update the link
	updated, err := mgr.UpdateLink("node-a", "node-b", 52000, 52001, []string{"updated"}, 1380, 1380)
	if err != nil {
		t.Fatalf("UpdateLink failed: %v", err)
	}

	if updated.From.ListenPort != 52000 {
		t.Errorf("Expected From.ListenPort 52000, got %d", updated.From.ListenPort)
	}
	if updated.To.ListenPort != 52001 {
		t.Errorf("Expected To.ListenPort 52001, got %d", updated.To.ListenPort)
	}
	if updated.From.MTU != 1380 || updated.To.MTU != 1380 {
		t.Errorf("Expected MTU 1380, got %d and %d", updated.From.MTU, updated.To.MTU)
	}
	// Keypairs must be preserved
	if updated.From.PublicKey != origPubKeyA {
		t.Errorf("Expected public key to remain identical")
	}
}

func TestPlanSyncCleanDeletedLinks(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-engine-plansync-*")
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
		t.Fatalf("Failed to unlock manager: %v", err)
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

	_ = mgr.AddNode(nodeA)
	_ = mgr.AddNode(nodeB)

	_, err = mgr.AddLink("node-a", "node-b", 51820, 51821, nil)
	if err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}

	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}

	// Should have 2 configure actions (node-a and node-b)
	configActionCount := 0
	for _, act := range actions {
		if act.Type == config.ActionSyncConfig {
			configActionCount++
		}
	}
	if configActionCount != 2 {
		t.Fatalf("Expected 2 sync_config actions, got %d", configActionCount)
	}

	// Delete the link
	if err := mgr.DeleteLink("node-a", "node-b"); err != nil {
		t.Fatalf("DeleteLink failed: %v", err)
	}

	actionsAfterDelete, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync after delete failed: %v", err)
	}
	// Config actions should now be 0 since link was removed
	for _, act := range actionsAfterDelete {
		if act.Type == config.ActionSyncConfig {
			t.Fatalf("Expected no sync_config actions after link deletion, got %v", act)
		}
	}
}

func TestUpdateNodePosition(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-pos-test-*")
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
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	node := config.Node{
		Name:      "test-node",
		Host:      "192.168.1.10",
		IP:        "192.168.100.10",
		Interface: "lo",
		ASN:       4299420010,
	}
	if err := mgr.AddNode(node); err != nil {
		t.Fatalf("Failed to add node: %v", err)
	}

	// Verify initially coordinates are nil
	n := mgr.FindNode("test-node")
	if n == nil {
		t.Fatalf("Node not found")
	}
	if n.X != nil || n.Y != nil {
		t.Fatalf("Expected X and Y to be nil initially, got X=%v, Y=%v", n.X, n.Y)
	}

	// Update coordinates
	if err := mgr.UpdateNodePosition("test-node", 450.5, 320.0); err != nil {
		t.Fatalf("UpdateNodePosition failed: %v", err)
	}

	n = mgr.FindNode("test-node")
	if n == nil || n.X == nil || n.Y == nil {
		t.Fatalf("Expected non-nil coordinates")
	}
	if *n.X != 450.5 || *n.Y != 320.0 {
		t.Fatalf("Expected X=450.5, Y=320.0, got X=%f, Y=%f", *n.X, *n.Y)
	}

	// Update general node details without passing X/Y, ensure X/Y are preserved
	updatedNode := *n
	updatedNode.Host = "192.168.1.11"
	updatedNode.X = nil
	updatedNode.Y = nil
	if err := mgr.UpdateNode("test-node", updatedNode); err != nil {
		t.Fatalf("UpdateNode failed: %v", err)
	}

	n = mgr.FindNode("test-node")
	if n.Host != "192.168.1.11" {
		t.Fatalf("Expected host updated to 192.168.1.11")
	}
	if n.X == nil || *n.X != 450.5 || n.Y == nil || *n.Y != 320.0 {
		t.Fatalf("Expected coordinates preserved after UpdateNode: X=%v, Y=%v", n.X, n.Y)
	}
}

func TestNodeAndLinkModifiedAt(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-engine-test-*")
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
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	nodeA := config.Node{
		Name:      "node-a",
		Host:      "1.1.1.1",
		IP:        "192.168.100.1",
		Interface: "lo",
		ASN:       4299420001,
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "2.2.2.2",
		IP:        "192.168.100.2",
		Interface: "lo",
		ASN:       4299420002,
	}

	if err := mgr.AddNode(nodeA); err != nil {
		t.Fatalf("AddNode nodeA failed: %v", err)
	}
	nA := mgr.FindNode("node-a")
	if nA == nil || nA.ModifiedAt.IsZero() {
		t.Fatalf("Expected node-a to have non-zero ModifiedAt after AddNode")
	}
	tA1 := nA.ModifiedAt

	time.Sleep(10 * time.Millisecond)

	if err := mgr.AddNode(nodeB); err != nil {
		t.Fatalf("AddNode nodeB failed: %v", err)
	}
	nB := mgr.FindNode("node-b")
	if nB == nil || nB.ModifiedAt.IsZero() {
		t.Fatalf("Expected node-b to have non-zero ModifiedAt after AddNode")
	}
	if !nB.ModifiedAt.After(tA1) {
		t.Fatalf("Expected node-b ModifiedAt (%v) to be after node-a (%v)", nB.ModifiedAt, tA1)
	}

	// Add link
	link, err := mgr.AddLink("node-a", "node-b", 0, 0, nil)
	if err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}
	if link.ModifiedAt.IsZero() {
		t.Fatalf("Expected link to have non-zero ModifiedAt after AddLink")
	}
	tLink1 := link.ModifiedAt

	time.Sleep(10 * time.Millisecond)

	// Update node position
	if err := mgr.UpdateNodePosition("node-a", 100, 200); err != nil {
		t.Fatalf("UpdateNodePosition failed: %v", err)
	}
	nA = mgr.FindNode("node-a")
	if !nA.ModifiedAt.After(tA1) {
		t.Fatalf("Expected node-a ModifiedAt updated after UpdateNodePosition")
	}
	tA2 := nA.ModifiedAt

	time.Sleep(10 * time.Millisecond)

	// Update node
	updatedA := *nA
	updatedA.Host = "1.1.1.2"
	if err := mgr.UpdateNode("node-a", updatedA); err != nil {
		t.Fatalf("UpdateNode failed: %v", err)
	}
	nA = mgr.FindNode("node-a")
	if !nA.ModifiedAt.After(tA2) {
		t.Fatalf("Expected node-a ModifiedAt updated after UpdateNode")
	}

	time.Sleep(10 * time.Millisecond)

	// Update link
	updatedLink, err := mgr.UpdateLink("node-a", "node-b", 51821, 51822, []string{"fast"})
	if err != nil {
		t.Fatalf("UpdateLink failed: %v", err)
	}
	if !updatedLink.ModifiedAt.After(tLink1) {
		t.Fatalf("Expected link ModifiedAt updated after UpdateLink")
	}
}

func TestCreateFullMesh(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-mesh-test-*")
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
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	nodes := []config.Node{
		{Name: "node-1", Host: "10.0.0.1", IP: "192.168.100.1", Interface: "lo", ASN: 4299420001, Tags: []string{"core"}},
		{Name: "node-2", Host: "10.0.0.2", IP: "192.168.100.2", Interface: "lo", ASN: 4299420002, Tags: []string{"core"}},
		{Name: "node-3", Host: "10.0.0.3", IP: "192.168.100.3", Interface: "lo", ASN: 4299420003, Tags: []string{"edge"}},
	}
	for _, n := range nodes {
		if err := mgr.AddNode(n); err != nil {
			t.Fatalf("AddNode %s failed: %v", n.Name, err)
		}
	}

	// 1. Create mesh for only the "core" tagged nodes: node-1 and node-2
	added, err := mgr.CreateFullMesh([]string{"node-1", "node-2"})
	if err != nil {
		t.Fatalf("CreateFullMesh failed: %v", err)
	}
	if len(added) != 1 {
		t.Fatalf("Expected 1 link added between 2 nodes, got %d", len(added))
	}

	// Calling again should add 0 links
	addedAgain, err := mgr.CreateFullMesh([]string{"node-1", "node-2"})
	if err != nil {
		t.Fatalf("CreateFullMesh repeat failed: %v", err)
	}
	if len(addedAgain) != 0 {
		t.Fatalf("Expected 0 links added when mesh already exists, got %d", len(addedAgain))
	}

	// 2. Create mesh across all 3 nodes (should add missing links for node-3 with node-1 and node-2 = 2 new links)
	addedAll, err := mgr.CreateFullMesh(nil)
	if err != nil {
		t.Fatalf("CreateFullMesh all failed: %v", err)
	}
	if len(addedAll) != 2 {
		t.Fatalf("Expected 2 links added for full mesh of 3 nodes, got %d", len(addedAll))
	}

	// Total links in config should be 3
	cfg := store.Get()
	if len(cfg.Links) != 3 {
		t.Fatalf("Expected 3 total links, got %d", len(cfg.Links))
	}
}

func TestGenerateBirdConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-bird-test-*")
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
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	nodeA := config.Node{
		Name:      "node-a",
		Host:      "10.0.0.1",
		IP:        "192.168.10.1",
		Interface: "lo",
		ASN:       4299420001,
		StaticRoutes: []string{
			"192.168.10.0/24",
		},
		Routes: []config.KernelRouteRule{
			{
				Table:    101,
				Prefixes: []string{"10.10.0.0/16+"},
			},
		},
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "10.0.0.2",
		IP:        "192.168.10.2",
		Interface: "lo",
		ASN:       4299420002,
	}

	if err := mgr.AddNode(nodeA); err != nil {
		t.Fatalf("AddNode A failed: %v", err)
	}
	if err := mgr.AddNode(nodeB); err != nil {
		t.Fatalf("AddNode B failed: %v", err)
	}

	// Verify default table was set to 254
	savedNodeA := mgr.GetNode("node-a")
	if savedNodeA.Table != 254 {
		t.Fatalf("Expected table 254, got %d", savedNodeA.Table)
	}

	// Add link
	if _, err := mgr.AddLink("node-a", "node-b", 0, 0, []string{"mesh"}); err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}

	// Generate BIRD config for node-a
	birdConf, err := mgr.GenerateBirdConfig("node-a")
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}

	expected := []string{
		"define SELF_IP = 192.168.10.1;",
		"define SELF_AS = 4299420001;",
		"define TABLE = 254;",
		"protocol kernel kernel_101 {",
		"kernel table 101;",
		"route 192.168.10.0/24 reject;",
		"protocol bgp 'easy42_peer_node-b' from easy42_peer {",
		"as 4299420002;",
	}
	for _, exp := range expected {
		if !strings.Contains(birdConf, exp) {
			t.Fatalf("Expected %q in birdConf:\n%s", exp, birdConf)
		}
	}
}


