package compiler

import (
	"strings"
	"testing"

	"easy42/internal/config"
	"easy42/internal/crypto"
)

func TestDeriveIPv6LinkLocal(t *testing.T) {
	addr, err := DeriveIPv6LinkLocal("192.168.100.10")
	if err != nil {
		t.Fatalf("DeriveIPv6LinkLocal failed: %v", err)
	}

	// 192.168 -> c0a8, 100.10 -> 640a
	if addr != "fe80::c0a8:640a/64" {
		t.Fatalf("Expected fe80::c0a8:640a/64, got %s", addr)
	}

	addrOnly, err := DeriveIPv6LinkLocalAddressOnly("192.168.100.10")
	if err != nil {
		t.Fatalf("DeriveIPv6LinkLocalAddressOnly failed: %v", err)
	}
	if addrOnly != "fe80::c0a8:640a" {
		t.Fatalf("Expected fe80::c0a8:640a, got %s", addrOnly)
	}
}

func TestDerivePortFromIP(t *testing.T) {
	port1 := DerivePortFromIP("192.168.100.1")
	if port1 != 25532 {
		t.Fatalf("Expected port 25532 for 192.168.100.1, got %d", port1)
	}

	port2 := DerivePortFromIP("192.168.100.2")
	if port2 != 28389 {
		t.Fatalf("Expected port 28389 for 192.168.100.2, got %d", port2)
	}

	portEmpty := DerivePortFromIP("")
	if portEmpty != 20000 {
		t.Fatalf("Expected port 20000 for empty IP, got %d", portEmpty)
	}

	if port1 < 20000 || port1 > 29999 || port2 < 20000 || port2 > 29999 {
		t.Fatalf("Port out of range 20000-29999")
	}
}

func TestDerivePortFromASN(t *testing.T) {
	port := DerivePortFromASN(4224420001)
	if port != 20001 {
		t.Fatalf("Expected port 20001, got %d", port)
	}

	port2 := DerivePortFromASN(4224425182)
	if port2 != 25182 {
		t.Fatalf("Expected port 25182, got %d", port2)
	}
}

func TestResolvePeerEndpoint(t *testing.T) {
	nodeA := &config.Node{
		Name: "node-a",
		ASN:  4224420001,
		IP:   "192.168.100.1",
		Entrypoints: []config.Entrypoint{
			{
				IP:   "1.1.1.1",
				Tags: []string{"direct"},
			},
		},
	}

	nodeB := &config.Node{
		Name: "node-b",
		ASN:  4224420002,
		IP:   "192.168.100.2",
		Entrypoints: []config.Entrypoint{
			{
				IP:   "2.2.2.2",
				Tags: []string{"direct"},
			},
		},
	}

	ep, port := ResolvePeerEndpoint(nodeA, nodeB, nil)
	if ep != "2.2.2.2:25532" || port != 25532 {
		t.Fatalf("Expected 2.2.2.2:25532 / 25532, got %s / %d", ep, port)
	}

	// Test with explicit target listen port
	epCustom, portCustom := ResolvePeerEndpoint(nodeA, nodeB, nil, 33333)
	if epCustom != "2.2.2.2:33333" || portCustom != 33333 {
		t.Fatalf("Expected 2.2.2.2:33333 / 33333, got %s / %d", epCustom, portCustom)
	}
}

func TestGenerateWgConfigContent(t *testing.T) {
	nodeA := &config.Node{
		Name: "node-a",
		ASN:  4224420001,
		IP:   "192.168.100.1",
	}

	nodeB := &config.Node{
		Name: "node-b",
		ASN:  4224420002,
		IP:   "192.168.100.2",
		Entrypoints: []config.Entrypoint{
			{
				IP: "2.2.2.2",
			},
		},
	}

	vault := crypto.NewKeyVault()
	dek, _ := crypto.GenerateRandomBytes(32)
	_ = vault.Unlock(dek)

	kpA, _ := crypto.GenerateWgKeyPair()
	kpB, _ := crypto.GenerateWgKeyPair()

	encPrivA, _ := vault.EncryptField(kpA.PrivateKey)

	endA := &config.LinkEnd{
		Name:       "node-a",
		Interface:  "wg42node-b",
		PrivateKey: encPrivA,
		PublicKey:  kpA.PublicKey,
		ListenPort: 20002,
	}

	endB := &config.LinkEnd{
		Name:      "node-b",
		PublicKey: kpB.PublicKey,
	}

	conf, err := GenerateWgConfigContent(nodeA, nodeB, endA, endB, vault)
	if err != nil {
		t.Fatalf("GenerateWgConfigContent failed: %v", err)
	}

	if !strings.Contains(conf, "[Interface]") || !strings.Contains(conf, "[Peer]") {
		t.Fatalf("Missing sections in generated config:\n%s", conf)
	}
	if !strings.Contains(conf, kpA.PrivateKey) {
		t.Fatalf("Private key not in config:\n%s", conf)
	}
	if !strings.Contains(conf, kpB.PublicKey) {
		t.Fatalf("Public key not in config:\n%s", conf)
	}
	if !strings.Contains(conf, "Table = off") {
		t.Fatalf("Table = off not in config:\n%s", conf)
	}
	if !strings.Contains(conf, "MTU = 1420") {
		t.Fatalf("MTU = 1420 not in config:\n%s", conf)
	}
	if !strings.Contains(conf, "2.2.2.2:25532") {
		t.Fatalf("Endpoint not correctly set in config:\n%s", conf)
	}

	// Test custom MTU
	endA.MTU = 1380
	confCustom, err := GenerateWgConfigContent(nodeA, nodeB, endA, endB, vault)
	if err != nil {
		t.Fatalf("GenerateWgConfigContent with custom MTU failed: %v", err)
	}
	if !strings.Contains(confCustom, "MTU = 1380") {
		t.Fatalf("Expected custom MTU = 1380, got:\n%s", confCustom)
	}
}
