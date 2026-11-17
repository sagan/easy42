package engine

import (
	"os"
	"strings"
	"testing"

	"easy42/internal/compiler"
	"easy42/internal/config"
)

func TestVirtualLink_BackendBehavior(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "easy42-virtual-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	store := config.NewStore(tempDir)
	_, err = store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	// 1. Initialize manager WITHOUT unlocking the vault
	mgr := NewManager(store)
	if mgr.vault.IsUnlocked() {
		t.Fatalf("expected vault to be locked initially")
	}

	nodeA := config.Node{
		Name:      "router1",
		Host:      "10.0.0.1",
		IP:        "10.0.0.1",
		Interface: "lo",
		ASN:       4242420001,
	}
	nodeB := config.Node{
		Name:      "lan_device",
		Host:      "10.0.0.2",
		IP:        "10.0.0.2",
		Interface: "lo",
		ASN:       4242420002,
	}

	if err := mgr.AddNode(nodeA); err != nil {
		t.Fatalf("failed to add nodeA: %v", err)
	}
	if err := mgr.AddNode(nodeB); err != nil {
		t.Fatalf("failed to add nodeB: %v", err)
	}

	// 2. Add Virtual Link without unlocking vault
	fromEnd := &config.LinkEnd{
		Name:      "router1",
		Type:      config.LinkTypeVirtual,
		Interface: "eth1",
		Address:   "192.168.1.1/24",
		Note:      "Router direct LAN connection",
	}
	toEnd := &config.LinkEnd{
		Name:      "lan_device",
		Type:      config.LinkTypeVirtual,
		Interface: "lan0",
		Address:   "192.168.1.2/24",
		Note:      "Device LAN",
	}

	createdLink, err := mgr.AddLinkWithAllOptionsAndNote(
		"router1", "lan_device",
		fromEnd, toEnd,
		nil,
		false,
		false,
		"Physical direct ethernet cable",
	)
	if err != nil {
		t.Fatalf("failed to add virtual link with locked vault: %v", err)
	}

	// Verify link properties (Easy42 stores links canonicalized: from.Name < to.Name)
	if !createdLink.IsVirtual() {
		t.Fatalf("expected created link to be virtual, got type=%s", createdLink.Type)
	}
	if !createdLink.From.IsVirtual() || !createdLink.To.IsVirtual() {
		t.Fatalf("expected link ends to be virtual")
	}

	getEnd := func(l *config.Link, name string) *config.LinkEnd {
		if l.From.Name == name {
			return &l.From
		}
		if l.To.Name == name {
			return &l.To
		}
		return nil
	}

	endRouter := getEnd(createdLink, "router1")
	endLan := getEnd(createdLink, "lan_device")
	if endRouter == nil || endLan == nil {
		t.Fatalf("missing link ends on created link")
	}
	if endRouter.Interface != "eth1" || endLan.Interface != "lan0" {
		t.Fatalf("unexpected interfaces: router=%s lan=%s", endRouter.Interface, endLan.Interface)
	}
	if createdLink.From.PublicKey != "" || createdLink.To.PublicKey != "" {
		t.Fatalf("expected no WireGuard public keys for virtual link")
	}
	if createdLink.Psk != "" {
		t.Fatalf("expected no PSK for virtual link")
	}

	// 3. Compiler verification: BIRD must skip virtual link BGP session
	nodes := mgr.GetNodes()
	links := mgr.GetLinks()

	birdCfg, err := compiler.GenerateBirdConfig(&nodeA, nodes, links)
	if err != nil {
		t.Fatalf("failed to generate bird config: %v", err)
	}
	// BIRD config should not have protocol bgp for the virtual link
	if len(birdCfg) > 0 && string(birdCfg) != "" {
		if strings.Contains(string(birdCfg), "protocol bgp") {
			t.Fatalf("expected no BGP protocol configured for virtual link, got: %s", string(birdCfg))
		}
	}

	// 4. Update Virtual Link reference options without unlocking vault
	updatedFrom := &config.LinkEnd{
		Name:      "router1",
		Type:      config.LinkTypeVirtual,
		Interface: "eth1.100",
		Address:   "192.168.100.1/24",
	}
	updatedTo := &config.LinkEnd{
		Name:      "lan_device",
		Type:      config.LinkTypeVirtual,
		Interface: "lan0.100",
		Address:   "192.168.100.2/24",
	}
	updatedNote := "VLAN 100 on direct link"

	updatedLink, err := mgr.UpdateLinkWithAllOptions(
		"router1", "lan_device",
		updatedFrom, updatedTo,
		nil, nil, nil,
		&updatedNote,
	)
	if err != nil {
		t.Fatalf("failed to update virtual link: %v", err)
	}
	if !updatedLink.IsVirtual() {
		t.Fatalf("expected updated link to remain virtual")
	}
	updatedRouter := getEnd(updatedLink, "router1")
	updatedLan := getEnd(updatedLink, "lan_device")
	if updatedRouter == nil || updatedLan == nil {
		t.Fatalf("missing link ends on updated link")
	}
	if updatedRouter.Interface != "eth1.100" || updatedLan.Address != "192.168.100.2/24" {
		t.Fatalf("unexpected updated fields: routerIface=%s lanAddr=%s", updatedRouter.Interface, updatedLan.Address)
	}
	if updatedLink.Note != updatedNote {
		t.Fatalf("expected updated note %q, got %q", updatedNote, updatedLink.Note)
	}
}
