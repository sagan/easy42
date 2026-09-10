package compiler

import (
	"fmt"
	"net"
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

func TestGetDefaultWgTemplate(t *testing.T) {
	tmpl, err := GetDefaultWgTemplate()
	if err != nil {
		t.Fatalf("GetDefaultWgTemplate failed: %v", err)
	}
	if !strings.Contains(tmpl, "[Interface]") || !strings.Contains(tmpl, "[Peer]") {
		t.Fatalf("Expected Interface and Peer sections in default wg template, got:\n%s", tmpl)
	}
}

func TestGenerateWgConfigContentWithTemplate(t *testing.T) {
	nodeA := &config.Node{
		Name: "node-a",
		IP:   "192.168.100.1",
	}
	nodeB := &config.Node{
		Name: "node-b",
		IP:   "192.168.100.2",
	}
	endA := &config.LinkEnd{
		Name:       "node-a",
		ListenPort: 51820,
	}
	endB := &config.LinkEnd{
		Name:      "node-b",
		PublicKey: "peer-pub-key-123",
	}

	customTmpl := `# Custom WG config for {{ .self_node.Name }} -> {{ .peer_node.Name }}
[Interface]
ListenPort = {{ .listen_port }}
# Custom note: MTU is {{ .mtu }}

[Peer]
PublicKey = {{ .peer_public_key }}
AllowedIPs = {{ .allowed_ips }}
`
	rendered, err := GenerateWgConfigContentWithTemplate(customTmpl, nodeA, nodeB, endA, endB, nil)
	if err != nil {
		t.Fatalf("GenerateWgConfigContentWithTemplate failed: %v", err)
	}
	expectedHeader := "# Custom WG config for node-a -> node-b"
	if !strings.Contains(rendered, expectedHeader) {
		t.Fatalf("Expected %q, got:\n%s", expectedHeader, rendered)
	}
	if !strings.Contains(rendered, "ListenPort = 51820") {
		t.Fatalf("Expected ListenPort = 51820, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "PublicKey = peer-pub-key-123") {
		t.Fatalf("Expected PublicKey = peer-pub-key-123, got:\n%s", rendered)
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

	// Test UseIp = false vs true with domain endpoint
	origLookup := DefaultLookupIP
	defer func() { DefaultLookupIP = origLookup }()
	DefaultLookupIP = func(host string) ([]net.IP, error) {
		if host == "vpn.example.com" {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		}
		return nil, fmt.Errorf("unknown host")
	}

	endA.Endpoint = "vpn.example.com:51820"
	endA.UseIp = false
	confDomain, err := GenerateWgConfigContent(nodeA, nodeB, endA, endB, vault)
	if err != nil {
		t.Fatalf("GenerateWgConfigContent with domain failed: %v", err)
	}
	if !strings.Contains(confDomain, "Endpoint = vpn.example.com:51820") {
		t.Fatalf("Expected domain endpoint, got:\n%s", confDomain)
	}

	// Now set UseIp = true
	endA.UseIp = true
	confIP, err := GenerateWgConfigContent(nodeA, nodeB, endA, endB, vault)
	if err != nil {
		t.Fatalf("GenerateWgConfigContent with UseIp failed: %v", err)
	}
	if !strings.Contains(confIP, "Endpoint = 93.184.216.34:51820") {
		t.Fatalf("Expected resolved IP endpoint 93.184.216.34:51820, got:\n%s", confIP)
	}
}

func TestResolveEndpointToIP(t *testing.T) {
	origLookup := DefaultLookupIP
	defer func() { DefaultLookupIP = origLookup }()

	DefaultLookupIP = func(host string) ([]net.IP, error) {
		switch host {
		case "peer.example.org":
			return []net.IP{net.ParseIP("203.0.113.50")}, nil
		case "ipv6.example.org":
			return []net.IP{net.ParseIP("2001:db8::10")}, nil
		case "fail.example.org":
			return nil, fmt.Errorf("dns lookup failed")
		default:
			return nil, fmt.Errorf("host not found")
		}
	}

	// 1. Domain with IPv4 resolution
	res1 := ResolveEndpointToIP("peer.example.org:51820")
	if res1 != "203.0.113.50:51820" {
		t.Errorf("Expected 203.0.113.50:51820, got %s", res1)
	}

	// 2. Domain with IPv6 resolution
	res2 := ResolveEndpointToIP("ipv6.example.org:51820")
	if res2 != "[2001:db8::10]:51820" {
		t.Errorf("Expected [2001:db8::10]:51820, got %s", res2)
	}

	// 3. Already IPv4
	res3 := ResolveEndpointToIP("198.51.100.1:51820")
	if res3 != "198.51.100.1:51820" {
		t.Errorf("Expected 198.51.100.1:51820, got %s", res3)
	}

	// 4. Already IPv6
	res4 := ResolveEndpointToIP("[2001:db8::1]:51820")
	if res4 != "[2001:db8::1]:51820" {
		t.Errorf("Expected [2001:db8::1]:51820, got %s", res4)
	}

	// 5. DNS failure fallback
	res5 := ResolveEndpointToIP("fail.example.org:51820")
	if res5 != "fail.example.org:51820" {
		t.Errorf("Expected fail.example.org:51820, got %s", res5)
	}

	// 6. Empty endpoint
	res6 := ResolveEndpointToIP("")
	if res6 != "" {
		t.Errorf("Expected empty string, got %s", res6)
	}
}

func TestGetInterfaceName(t *testing.T) {
	// Internal peer (default or isExternal=false)
	// Max length 11 chars, prefix "wg42"
	if iface := GetInterfaceName("router1"); iface != "wg42router1" {
		t.Errorf("Expected wg42router1, got %s", iface)
	}
	if iface := GetInterfaceName("abcdefghijk"); iface != "wg42abcdefghijk" {
		t.Errorf("Expected wg42abcdefghijk, got %s", iface)
	}
	if iface := GetInterfaceName("abcdefghijklmno"); iface != "wg42abcdefghijk" {
		t.Errorf("Expected wg42abcdefghijk (truncated to 11 chars), got %s", iface)
	}
	if iface := GetInterfaceName("router1", false); iface != "wg42router1" {
		t.Errorf("Expected wg42router1, got %s", iface)
	}

	// External peer (isExternal=true)
	// Max length 10 chars, prefix "wg42-"
	if iface := GetInterfaceName("dn42peer", true); iface != "wg42-dn42peer" {
		t.Errorf("Expected wg42-dn42peer, got %s", iface)
	}
	if iface := GetInterfaceName("abcdefghij", true); iface != "wg42-abcdefghij" {
		t.Errorf("Expected wg42-abcdefghij, got %s", iface)
	}
	if iface := GetInterfaceName("abcdefghijklmno", true); iface != "wg42-abcdefghij" {
		t.Errorf("Expected wg42-abcdefghij (truncated to 10 chars), got %s", iface)
	}
	if iface := GetExternalInterfaceName("dn42peer"); iface != "wg42-dn42peer" {
		t.Errorf("Expected wg42-dn42peer, got %s", iface)
	}
}

func TestResolveLinkEndpointExternalNode(t *testing.T) {
	managedNode := &config.Node{
		Name: "dedirock",
		IP:   "192.168.110.203",
		Entrypoints: []config.Entrypoint{
			{
				IP:   "dedirock.s.sagan.me",
				Tags: []string{"default"},
				MTU:  1500,
			},
			{
				IP:   "us0.dn42.sagan.me",
				Tags: []string{"external"},
				MTU:  1500,
			},
		},
	}
	externalNode := &config.Node{
		Name:       "iedon-uk",
		IsExternal: true,
		Tags:       []string{"external"},
		Entrypoints: []config.Entrypoint{
			{
				IP:   "uk-lon.dn42.iedon.net",
				Tags: []string{"external"},
			},
		},
	}

	fromEnd := &config.LinkEnd{
		Name:       "dedirock",
		ListenPort: 22189,
		Endpoint:   "uk-lon.dn42.iedon.net:42569",
	}
	toEnd := &config.LinkEnd{
		Name:       "iedon-uk",
		ListenPort: 0,
		Endpoint:   "",
	}

	// 1. Managed node connects to external peer:
	epFrom := ResolveLinkEndpoint(managedNode, externalNode, fromEnd, toEnd)
	if epFrom != "uk-lon.dn42.iedon.net:42569" {
		t.Errorf("Expected dedirock's peer endpoint to be uk-lon.dn42.iedon.net:42569, got %s", epFrom)
	}

	// 2. External node connects to managed peer:
	epTo := ResolveLinkEndpoint(externalNode, managedNode, toEnd, fromEnd)
	if epTo != "us0.dn42.sagan.me:22189" {
		t.Errorf("Expected iedon-uk's peer endpoint to be us0.dn42.sagan.me:22189, got %s", epTo)
	}

	// 3. Even if toEnd.Endpoint was mistakenly set to the external peer's own endpoint:
	toEndCorrupted := &config.LinkEnd{
		Name:       "iedon-uk",
		ListenPort: 0,
		Endpoint:   "uk-lon.dn42.iedon.net:42569",
	}
	epToFixed := ResolveLinkEndpoint(externalNode, managedNode, toEndCorrupted, fromEnd)
	if epToFixed != "us0.dn42.sagan.me:22189" {
		t.Errorf("Expected iedon-uk's peer endpoint to be derived as us0.dn42.sagan.me:22189, got %s", epToFixed)
	}
}
