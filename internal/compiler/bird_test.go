package compiler

import (
	"strings"
	"testing"

	"easy42/internal/config"
)

func TestGetDefaultBirdTemplate(t *testing.T) {
	tmpl, err := GetDefaultBirdTemplate()
	if err != nil {
		t.Fatalf("GetDefaultBirdTemplate failed: %v", err)
	}
	if !strings.Contains(tmpl, "protocol bgp 'easy42_peer_{{.remote.name}}'") {
		t.Fatalf("Expected peer template in default bird template, got:\n%s", tmpl)
	}
}

func TestBuildNodeContextAndGenerateBirdConfig(t *testing.T) {
	node1 := config.Node{
		Name:      "router1",
		Host:      "10.0.0.1",
		IP:        "192.168.100.1",
		Interface: "eth0",
		ASN:       4299420001,
		Table:     0, // should default to 254
		StaticRoutes: []string{
			"192.168.100.0/24",
		},
		Routes: []config.KernelRouteRule{
			{
				Table: 100,
				Prefixes: []string{
					"10.0.0.0/8+",
					"192.168.0.0/16+",
				},
			},
		},
	}

	node2 := config.Node{
		Name:      "router2",
		Host:      "10.0.0.2",
		IP:        "192.168.100.2",
		Interface: "eth0",
		ASN:       4299420002,
		Table:     254,
	}

	allNodes := []config.Node{node1, node2}

	links := []config.Link{
		{
			From: config.LinkEnd{
				Name:      "router1",
				Interface: "wg42router2",
				Address:   "fe80::c0a8:6401/64",
			},
			To: config.LinkEnd{
				Name:      "router2",
				Interface: "wg42router1",
				Address:   "fe80::c0a8:6402/64",
			},
			Tags: []string{"lan"},
		},
	}

	// 1. Check Context for Node 1
	ctx1, err := BuildNodeContext(&node1, allNodes, links)
	if err != nil {
		t.Fatalf("BuildNodeContext node1 failed: %v", err)
	}

	if ctx1["table"] != 254 {
		t.Fatalf("Expected default table 254, got %v", ctx1["table"])
	}
	if ctx1["ip"] != "192.168.100.1" {
		t.Fatalf("Expected ip 192.168.100.1, got %v", ctx1["ip"])
	}

	links1, ok := ctx1["links"].([]map[string]any)
	if !ok || len(links1) != 1 {
		t.Fatalf("Expected 1 link in context, got %v", ctx1["links"])
	}

	local1 := links1[0]["local"].(map[string]any)
	remote1 := links1[0]["remote"].(map[string]any)
	remoteNode1 := links1[0]["remote_node"].(map[string]any)

	// Verify CIDR suffix stripped
	if local1["address"] != "fe80::c0a8:6401" {
		t.Fatalf("Expected stripped address fe80::c0a8:6401, got %v", local1["address"])
	}
	if remote1["address"] != "fe80::c0a8:6402" {
		t.Fatalf("Expected stripped remote address fe80::c0a8:6402, got %v", remote1["address"])
	}
	if local1["interface"] != "wg42router2" {
		t.Fatalf("Expected interface wg42router2, got %v", local1["interface"])
	}
	if remote1["name"] != "router2" {
		t.Fatalf("Expected remote name router2, got %v", remote1["name"])
	}
	if remoteNode1["asn"].(uint64) != 4299420002 {
		t.Fatalf("Expected remote ASN 4299420002, got %v", remoteNode1["asn"])
	}

	// 2. Generate BIRD config for Node 1
	conf1, err := GenerateBirdConfig(&node1, allNodes, links)
	if err != nil {
		t.Fatalf("GenerateBirdConfig node1 failed: %v", err)
	}

	expectedSnippets := []string{
		"define SELF_IP = 192.168.100.1;",
		"define SELF_AS = 4299420001;",
		"define TABLE = 254;",
		"router id SELF_IP;",
		"protocol kernel kernel_v4",
		"if source ~ [ RTS_BGP, RTS_STATIC ] then accept;",
		"protocol kernel kernel_100 {",
		"kernel table 100;",
		"if net ~ 10.0.0.0/8+ then accept;",
		"if net ~ 192.168.0.0/16+ then accept;",
		"protocol static static_self {",
		"route SELF_IP/32 reject;",
		"protocol static static_v4 {",
		"route 192.168.100.0/24 reject;",
		"template bgp easy42_peer",
		"protocol bgp 'easy42_peer_router2' from easy42_peer {",
		"local fe80::c0a8:6401 as SELF_AS;",
		"neighbor fe80::c0a8:6402 % 'wg42router2' as 4299420002;",
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(conf1, s) {
			t.Errorf("Missing expected snippet in generated config:\nSnippet: %s\nGenerated:\n%s", s, conf1)
		}
	}

	// 3. Test symmetry: generate BIRD config for Node 2
	conf2, err := GenerateBirdConfig(&node2, allNodes, links)
	if err != nil {
		t.Fatalf("GenerateBirdConfig node2 failed: %v", err)
	}

	expectedSnippetsNode2 := []string{
		"define SELF_IP = 192.168.100.2;",
		"define SELF_AS = 4299420002;",
		"protocol bgp 'easy42_peer_router1' from easy42_peer {",
		"local fe80::c0a8:6402 as SELF_AS;",
		"neighbor fe80::c0a8:6401 % 'wg42router1' as 4299420001;",
	}

	for _, s := range expectedSnippetsNode2 {
		if !strings.Contains(conf2, s) {
			t.Errorf("Missing expected snippet in node2 config:\nSnippet: %s\nGenerated:\n%s", s, conf2)
		}
	}
}

func TestDeriveAddressFallback(t *testing.T) {
	nodeA := config.Node{
		Name: "nodeA",
		IP:   "192.168.10.1",
		ASN:  4299420001,
	}
	nodeB := config.Node{
		Name: "nodeB",
		IP:   "192.168.10.2",
		ASN:  4299420002,
	}

	// Link without explicit addresses or interface names
	links := []config.Link{
		{
			From: config.LinkEnd{
				Name: "nodeA",
			},
			To: config.LinkEnd{
				Name: "nodeB",
			},
		},
	}

	ctx, err := BuildNodeContext(&nodeA, []config.Node{nodeA, nodeB}, links)
	if err != nil {
		t.Fatalf("BuildNodeContext failed: %v", err)
	}

	linkList := ctx["links"].([]map[string]any)
	local := linkList[0]["local"].(map[string]any)
	remote := linkList[0]["remote"].(map[string]any)

	// Addresses should be derived
	addrA, _ := DeriveIPv6LinkLocalAddressOnly("192.168.10.1")
	addrB, _ := DeriveIPv6LinkLocalAddressOnly("192.168.10.2")

	if local["address"] != addrA {
		t.Fatalf("Expected derived local address %s, got %v", addrA, local["address"])
	}
	if remote["address"] != addrB {
		t.Fatalf("Expected derived remote address %s, got %v", addrB, remote["address"])
	}
	if local["interface"] != "wg42nodeB" {
		t.Fatalf("Expected derived interface wg42nodeB, got %v", local["interface"])
	}
}
