package engine

import (
	"fmt"
	"net"
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
		ASN:       4224420001,
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
		ASN:       4224420002,
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
		ASN:       4224420003,
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
		ASN:       4224420004,
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
		ASN:       4224420001,
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "192.168.1.2",
		IP:        "192.168.100.2",
		Interface: "lo",
		ASN:       4224420002,
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

func TestLinkUseIpAndResolvedEndpoint(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-engine-useip-*")
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

	origLookup := compiler.DefaultLookupIP
	defer func() { compiler.DefaultLookupIP = origLookup }()
	compiler.DefaultLookupIP = func(host string) ([]net.IP, error) {
		if host == "peer.testdomain.com" {
			return []net.IP{net.ParseIP("198.51.100.99")}, nil
		}
		return nil, fmt.Errorf("lookup error")
	}

	nodeA := config.Node{
		Name:      "node-a",
		Host:      "192.168.1.1",
		Interface: "lo",
		ASN:       4224420001,
		IP:        "192.168.100.1",
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "192.168.1.2",
		Interface: "lo",
		ASN:       4224420002,
		IP:        "192.168.100.2",
		Entrypoints: []config.Entrypoint{
			{IP: "peer.testdomain.com"},
		},
	}
	if err := mgr.AddNode(nodeA); err != nil {
		t.Fatalf("AddNode nodeA failed: %v", err)
	}
	if err := mgr.AddNode(nodeB); err != nil {
		t.Fatalf("AddNode nodeB failed: %v", err)
	}

	// Add link with from.UseIp = false
	link, err := mgr.AddLinkAdvanced("node-a", "node-b", &config.LinkEnd{UseIp: false}, &config.LinkEnd{}, nil)
	if err != nil {
		t.Fatalf("AddLinkAdvanced failed: %v", err)
	}
	if link.From.UseIp {
		t.Errorf("Expected link.From.UseIp to be false")
	}

	links := mgr.GetLinks()
	if len(links) != 1 {
		t.Fatalf("Expected 1 link, got %d", len(links))
	}
	if links[0].From.UseIp {
		t.Errorf("Expected From.UseIp to be false")
	}
	if !strings.Contains(links[0].From.ResolvedEndpoint, "peer.testdomain.com") {
		t.Errorf("Expected domain in From.ResolvedEndpoint when UseIp=false, got %s", links[0].From.ResolvedEndpoint)
	}

	// Update link with from.UseIp = true
	updated, err := mgr.UpdateLinkAdvanced("node-a", "node-b", &config.LinkEnd{UseIp: true}, &config.LinkEnd{}, nil)
	if err != nil {
		t.Fatalf("UpdateLinkAdvanced failed: %v", err)
	}
	if !updated.From.UseIp {
		t.Errorf("Expected updated From.UseIp to be true")
	}
	if !strings.Contains(updated.From.ResolvedEndpoint, "198.51.100.99") {
		t.Errorf("Expected resolved IP in From.ResolvedEndpoint when UseIp=true, got %s", updated.From.ResolvedEndpoint)
	}

	// Verify GetLinks also returns resolved IP
	links = mgr.GetLinks()
	if !links[0].From.UseIp {
		t.Errorf("Expected GetLinks From.UseIp to be true")
	}
	if !strings.Contains(links[0].From.ResolvedEndpoint, "198.51.100.99") {
		t.Errorf("Expected GetLinks From.ResolvedEndpoint to contain 198.51.100.99, got %s", links[0].From.ResolvedEndpoint)
	}

	// Verify generated WireGuard config for nodeA uses the resolved IP
	wgConf, err := compiler.GenerateWgConfigContent(&nodeA, &nodeB, &links[0].From, &links[0].To, mgr.Vault())
	if err != nil {
		t.Fatalf("GenerateWgConfigContent failed: %v", err)
	}
	if !strings.Contains(wgConf, "Endpoint = 198.51.100.99:") {
		t.Errorf("Expected wg config to contain Endpoint = 198.51.100.99:, got:\n%s", wgConf)
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
		ASN:       4224420001,
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "127.0.0.1",
		IP:        "192.168.100.2",
		Interface: "lo",
		ASN:       4224420002,
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
		ASN:       4224420010,
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
		ASN:       4224420001,
	}
	nodeB := config.Node{
		Name:      "node-b",
		Host:      "2.2.2.2",
		IP:        "192.168.100.2",
		Interface: "lo",
		ASN:       4224420002,
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
		{Name: "node-1", Host: "10.0.0.1", IP: "192.168.100.1", Interface: "lo", ASN: 4224420001, Tags: []string{"core"}},
		{Name: "node-2", Host: "10.0.0.2", IP: "192.168.100.2", Interface: "lo", ASN: 4224420002, Tags: []string{"core"}},
		{Name: "node-3", Host: "10.0.0.3", IP: "192.168.100.3", Interface: "lo", ASN: 4224420003, Tags: []string{"edge"}},
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
		ASN:       4224420001,
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
		ASN:       4224420002,
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
		"define SELF_AS = 4224420001;",
		"define TABLE = 254;",
		"protocol kernel kernel_101 {",
		"kernel table 101;",
		"route 192.168.10.0/24 reject;",
		"protocol bgp 'easy42_peer_node-b' from easy42_peer {",
		"as 4224420002;",
	}
	for _, exp := range expected {
		if !strings.Contains(birdConf, exp) {
			t.Fatalf("Expected %q in birdConf:\n%s", exp, birdConf)
		}
	}
}

func TestExternalNodeAndPeering(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	rawPass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := NewManager(store)
	if err := mgr.Unlock(rawPass); err != nil {
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	// 1. Test NetworkSettings
	netSettings := config.NetworkSettings{
		PublicASN: 4242421234,
		Prefixes:  []string{"172.20.1.0/24", "172.20.0.0/14{21,29}"},
	}
	if err := mgr.UpdateNetworkSettings(netSettings); err != nil {
		t.Fatalf("UpdateNetworkSettings failed: %v", err)
	}
	loadedSettings := mgr.GetNetworkSettings()
	if loadedSettings.PublicASN != 4242421234 {
		t.Fatalf("Expected PublicASN 4242421234, got %d", loadedSettings.PublicASN)
	}

	// 2. Add Managed Node
	managed := config.Node{
		Name:      "gw1",
		Host:      "10.0.0.1",
		IP:        "192.168.100.1",
		Interface: "lo",
		ASN:       4224420001,
	}
	if err := mgr.AddNode(managed); err != nil {
		t.Fatalf("AddNode managed failed: %v", err)
	}

	// 3. Add External Node (no Host, no IP)
	external := config.Node{
		Name:        "peer-dn42",
		IsExternal:  true,
		ASN:         4242429876,
		Description: "External DN42 peer",
	}
	if err := mgr.AddNode(external); err != nil {
		t.Fatalf("AddNode external failed: %v", err)
	}

	// External node should have no "none" entrypoint forced
	savedExt := mgr.GetNode("peer-dn42")
	if savedExt == nil || !savedExt.IsExternal {
		t.Fatalf("Expected saved node to be external")
	}

	// 4. Test CreateFullMesh ignores external nodes
	addedLinks, err := mgr.CreateFullMesh(nil)
	if err == nil && len(addedLinks) > 0 {
		t.Fatalf("Expected CreateFullMesh to not create links with only 1 managed node")
	}

	// 5. Add external link with custom properties
	fromEnd := config.LinkEnd{
		Name:       "gw1",
		ListenPort: 51820,
		Address:    "fe80::1001/64",
	}
	toEnd := config.LinkEnd{
		Name:      "peer-dn42",
		Endpoint:  "remote.dn42.org:51820",
		Address:   "fe80::9876/64",
		PublicKey: "dGhpcy1pcy1hLXRlc3QtcHVibGljLWtleS0xMjM0NQ==",
	}
	extLink, err := mgr.AddLinkAdvanced("gw1", "peer-dn42", &fromEnd, &toEnd, []string{"dn42"})
	if err != nil {
		t.Fatalf("AddLinkAdvanced failed: %v", err)
	}

	// Verify only gw1 has private key and interface uses wg42- prefix
	if extLink.From.Name == "gw1" {
		if extLink.From.PrivateKey == "" {
			t.Errorf("Expected gw1 to have private key")
		}
		if extLink.From.Interface != "wg42-peer-dn42" {
			t.Errorf("Expected external link interface wg42-peer-dn42, got %s", extLink.From.Interface)
		}
		if extLink.To.PrivateKey != "" {
			t.Errorf("Expected external peer to have no private key")
		}
		if extLink.To.PublicKey != toEnd.PublicKey {
			t.Errorf("Expected external peer public key preserved")
		}
		if extLink.From.ResolvedEndpoint != "remote.dn42.org:51820" {
			t.Errorf("Expected gw1 ResolvedEndpoint remote.dn42.org:51820, got %s", extLink.From.ResolvedEndpoint)
		}
		if extLink.To.ResolvedEndpoint != "10.0.0.1:51820" {
			t.Errorf("Expected peer-dn42 ResolvedEndpoint 10.0.0.1:51820, got %s", extLink.To.ResolvedEndpoint)
		}
	} else {
		if extLink.To.PrivateKey == "" {
			t.Errorf("Expected gw1 to have private key")
		}
		if extLink.To.Interface != "wg42-peer-dn42" {
			t.Errorf("Expected external link interface wg42-peer-dn42, got %s", extLink.To.Interface)
		}
		if extLink.From.PrivateKey != "" {
			t.Errorf("Expected external peer to have no private key")
		}
	}

	// Test external node name max length validation (max 10 chars)
	tooLongExt := config.Node{
		Name:       "12345678901", // 11 chars
		IsExternal: true,
		ASN:        4242421111,
	}
	if err := mgr.AddNode(tooLongExt); err == nil {
		t.Errorf("Expected error adding external peer with 11 chars, got nil")
	}

	exactExt := config.Node{
		Name:       "1234567890", // 10 chars
		IsExternal: true,
		ASN:        4242421111,
	}
	if err := mgr.AddNode(exactExt); err != nil {
		t.Errorf("Expected success adding external peer with 10 chars, got: %v", err)
	}

	// 6. Test RefreshNodeStatus for external node returns connected/synthetic without SSH
	status, err := mgr.RefreshNodeStatus("peer-dn42")
	if err != nil || !status.Connected {
		t.Fatalf("Expected external node status connected, err: %v", err)
	}

	// 7. Verify BIRD config includes confederation and external peering template
	birdConf, err := mgr.GenerateBirdConfig("gw1")
	if err != nil {
		t.Fatalf("GenerateBirdConfig gw1 failed: %v", err)
	}
	if !strings.Contains(birdConf, "define CONFED_AS = 4242421234;") {
		t.Errorf("Expected define CONFED_AS in bird config:\n%s", birdConf)
	}
	if !strings.Contains(birdConf, "confederation CONFED_AS;") {
		t.Errorf("Expected confederation CONFED_AS; in bird config:\n%s", birdConf)
	}
	if !strings.Contains(birdConf, "template bgp external_peer") {
		t.Errorf("Expected template bgp external_peer in bird config:\n%s", birdConf)
	}

	// 8. Test UpdateState excludes external nodes from SSH connection attempts
	_, warnings, err := mgr.UpdateState()
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}
	for _, w := range warnings {
		if strings.Contains(w, "peer-dn42") || strings.Contains(w, "1234567890") {
			t.Errorf("UpdateState should not attempt to connect to external nodes, got warning: %s", w)
		}
	}
	statuses := mgr.GetNodeStatuses()
	if st, ok := statuses["peer-dn42"]; !ok || !st.Connected {
		t.Errorf("Expected synthetic status for external node peer-dn42, got %+v", st)
	}
}

func TestUpdateStatePartialNodeFilter(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-update-state-partial-*")
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

	node1 := config.Node{
		Name:      "node-a",
		Host:      "none",
		IP:        "172.20.1.1",
		Interface: "eth0",
		ASN:       4224420001,
		IsExternal: true,
	}
	node2 := config.Node{
		Name:      "node-b",
		Host:      "none",
		IP:        "172.20.1.2",
		Interface: "eth0",
		ASN:       4224420002,
		IsExternal: true,
	}

	if err := mgr.AddNode(node1); err != nil {
		t.Fatalf("AddNode node-a failed: %v", err)
	}
	if err := mgr.AddNode(node2); err != nil {
		t.Fatalf("AddNode node-b failed: %v", err)
	}

	// First run full update
	st1, _, err := mgr.UpdateState()
	if err != nil {
		t.Fatalf("Full UpdateState failed: %v", err)
	}
	if len(st1.Nodes) < 2 {
		// External nodes are handled synthetically
	}

	// Manually inject a recorded state node for node-a and node-b
	currentState := mgr.GetNetworkState()
	if currentState.Nodes == nil {
		currentState.Nodes = make(map[string]config.StateNode)
	}
	currentState.Nodes["node-a"] = config.StateNode{
		Name: "node-a",
		Host: "none",
		Interfaces: map[string]config.StateInterface{
			"wg42nodeb": {Name: "wg42nodeb", Status: "active"},
		},
	}
	currentState.Nodes["node-b"] = config.StateNode{
		Name: "node-b",
		Host: "none",
		Interfaces: map[string]config.StateInterface{
			"wg42nodea": {Name: "wg42nodea", Status: "active"},
		},
	}
	_ = mgr.stateStore.Save(currentState)

	// Now run partial UpdateState only targeting node-a
	stPartial, _, err := mgr.UpdateState("node-a")
	if err != nil {
		t.Fatalf("Partial UpdateState failed: %v", err)
	}

	// Ensure node-b was NOT deleted from recorded state during partial update
	if _, exists := stPartial.Nodes["node-b"]; !exists {
		t.Errorf("Partial update for node-a should have preserved node-b in state store")
	}
}

func TestNodeIP6(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-engine-ip6-*")
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

	node1 := config.Node{
		Name:      "node-1",
		Host:      "192.168.1.1",
		IP:        "192.168.100.1",
		IP6:       "fd42:a159:f9f0::1",
		Interface: "lo",
		ASN:       4224420001,
	}
	if err := mgr.AddNode(node1); err != nil {
		t.Fatalf("Failed to add node1: %v", err)
	}

	saved := mgr.GetNode("node-1")
	if saved == nil || saved.IP6 != "fd42:a159:f9f0::1" {
		t.Fatalf("Expected saved node1 to have IP6 fd42:a159:f9f0::1, got %+v", saved)
	}

	// Adding another node with duplicate IP6 should fail
	node2 := config.Node{
		Name:      "node-2",
		Host:      "192.168.1.2",
		IP:        "192.168.100.2",
		IP6:       "fd42:a159:f9f0::1",
		Interface: "lo",
		ASN:       4224420002,
	}
	if err := mgr.AddNode(node2); err == nil {
		t.Fatalf("Expected error when adding duplicate IP6, got nil")
	}

	// Adding node with unique IP6 should succeed
	node2.IP6 = "fd42:a159:f9f0::2"
	if err := mgr.AddNode(node2); err != nil {
		t.Fatalf("Failed to add node2 with unique IP6: %v", err)
	}

	// Updating node2 to collide with node1's IP6 should fail
	node2Update := *mgr.GetNode("node-2")
	node2Update.IP6 = "fd42:a159:f9f0::1"
	if err := mgr.UpdateNode("node-2", node2Update); err == nil {
		t.Fatalf("Expected error when updating to duplicate IP6, got nil")
	}

	// Updating node2 with non-colliding IP6 should succeed
	node2Update.IP6 = "fd42:a159:f9f0::22"
	if err := mgr.UpdateNode("node-2", node2Update); err != nil {
		t.Fatalf("Failed to update node2 IP6: %v", err)
	}
	if mgr.GetNode("node-2").IP6 != "fd42:a159:f9f0::22" {
		t.Fatalf("Expected updated IP6 to be fd42:a159:f9f0::22, got %s", mgr.GetNode("node-2").IP6)
	}
}

func TestNATNodePeerEndpointAndKeepalive(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	rawPass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := NewManager(store)
	if err := mgr.Unlock(rawPass); err != nil {
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	publicNode := config.Node{
		Name:      "public-node",
		Host:      "pub.sagan.me",
		IP:        "192.168.110.210",
		Interface: "wg0",
		ASN:       4224420210,
		Entrypoints: []config.Entrypoint{
			{
				IP:   "pub.sagan.me",
				Tags: []string{"default"},
				MTU:  1420,
			},
			{
				Tags: []string{"nat"},
			},
		},
	}
	if err := mgr.AddNode(publicNode); err != nil {
		t.Fatalf("AddNode publicNode failed: %v", err)
	}

	natNode := config.Node{
		Name:      "nat-node",
		Host:      "nat.sagan.me",
		IP:        "192.168.110.202",
		Interface: "wg0",
		ASN:       4224420202,
		Entrypoints: []config.Entrypoint{
			{
				Tags: []string{"nat"},
			},
		},
	}
	if err := mgr.AddNode(natNode); err != nil {
		t.Fatalf("AddNode natNode failed: %v", err)
	}

	link, err := mgr.AddLink("public-node", "nat-node", 21182, 28753, nil)
	if err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}

	var natEnd, pubEnd *config.LinkEnd
	if link.From.Name == "nat-node" {
		natEnd = &link.From
		pubEnd = &link.To
	} else {
		natEnd = &link.To
		pubEnd = &link.From
	}

	// public-node connects to nat-node: since nat-node is behind NAT, endpoint must be empty
	if pubEnd.Endpoint != "" {
		t.Errorf("Expected public-node peer endpoint to be empty for NAT peer, got %s", pubEnd.Endpoint)
	}
	if pubEnd.ResolvedEndpoint != "" {
		t.Errorf("Expected public-node resolved endpoint to be empty, got %s", pubEnd.ResolvedEndpoint)
	}
	if pubEnd.PersistentKeepalive != 0 {
		t.Errorf("Expected public-node keepalive to be 0 (no endpoint), got %d", pubEnd.PersistentKeepalive)
	}

	// nat-node connects to public-node: endpoint must be public-node's entrypoint, with keepalive 25
	if natEnd.Endpoint != "pub.sagan.me:21182" {
		t.Errorf("Expected nat-node peer endpoint to be pub.sagan.me:21182, got %s", natEnd.Endpoint)
	}
	if natEnd.ResolvedEndpoint != "pub.sagan.me:21182" {
		t.Errorf("Expected nat-node resolved endpoint to be pub.sagan.me:21182, got %s", natEnd.ResolvedEndpoint)
	}
	if natEnd.PersistentKeepalive != 25 {
		t.Errorf("Expected nat-node keepalive to be 25, got %d", natEnd.PersistentKeepalive)
	}
}

func TestSharedTagEmptyEntrypointLink(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	rawPass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := NewManager(store)
	if err := mgr.Unlock(rawPass); err != nil {
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	ggyix := config.Node{
		Name:      "ggyix",
		Host:      "ggyix.s.sagan.me",
		IP:        "192.168.110.202",
		Interface: "wg0",
		ASN:       4224420202,
		Entrypoints: []config.Entrypoint{
			{
				IP:   "ggyix.s.sagan.me",
				Tags: []string{"ix"},
				MTU:  1500,
			},
			{
				Tags: []string{"direct"},
			},
		},
	}
	if err := mgr.AddNode(ggyix); err != nil {
		t.Fatalf("AddNode ggyix failed: %v", err)
	}

	linode := config.Node{
		Name:      "linode",
		Host:      "linode.s.sagan.me",
		IP:        "192.168.110.1",
		Interface: "wg0",
		ASN:       4224420001,
		Entrypoints: []config.Entrypoint{
			{
				IP:   "linode.s.sagan.me",
				Tags: []string{"direct"},
				MTU:  1520,
			},
			{
				Tags: []string{"nat"},
			},
		},
	}
	if err := mgr.AddNode(linode); err != nil {
		t.Fatalf("AddNode linode failed: %v", err)
	}

	link, err := mgr.AddLink("ggyix", "linode", 26945, 21182, nil)
	if err != nil {
		t.Fatalf("AddLink failed: %v", err)
	}

	var ggyixEnd, linodeEnd *config.LinkEnd
	if link.From.Name == "ggyix" {
		ggyixEnd = &link.From
		linodeEnd = &link.To
	} else {
		ggyixEnd = &link.To
		linodeEnd = &link.From
	}

	// linode connects to ggyix: since they share "direct" tag and ggyix has no IP for "direct",
	// linode must NOT have an endpoint (must NOT fall back to ggyix's "ix" endpoint!)
	if linodeEnd.Endpoint != "" {
		t.Errorf("Expected linode peer endpoint to be empty, got %s", linodeEnd.Endpoint)
	}
	if linodeEnd.ResolvedEndpoint != "" {
		t.Errorf("Expected linode resolved endpoint to be empty, got %s", linodeEnd.ResolvedEndpoint)
	}
	if linodeEnd.PersistentKeepalive != 0 {
		t.Errorf("Expected linode keepalive to be 0, got %d", linodeEnd.PersistentKeepalive)
	}

	// ggyix connects to linode: linode has IP on "direct" tag
	if ggyixEnd.Endpoint != "linode.s.sagan.me:21182" {
		t.Errorf("Expected ggyix peer endpoint to be linode.s.sagan.me:21182, got %s", ggyixEnd.Endpoint)
	}
	if ggyixEnd.ResolvedEndpoint != "linode.s.sagan.me:21182" {
		t.Errorf("Expected ggyix resolved endpoint to be linode.s.sagan.me:21182, got %s", ggyixEnd.ResolvedEndpoint)
	}
	if ggyixEnd.PersistentKeepalive != 25 {
		t.Errorf("Expected ggyix keepalive to be 25, got %d", ggyixEnd.PersistentKeepalive)
	}
}

