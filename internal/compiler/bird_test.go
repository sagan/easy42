package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
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

func validateBirdSyntax(t *testing.T, conf string) {
	birdPath, err := exec.LookPath("bird")
	if err != nil || birdPath == "" {
		return
	}
	testConf := conf
	lines := strings.Split(testConf, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "include \"/etc/easy42_") {
			start := strings.Index(trimmed, "\"/etc/") + len("\"/etc/")
			end := strings.LastIndex(trimmed, "\"")
			if start > 0 && end > start {
				fileName := trimmed[start:end]
				stubPath := filepath.Join(os.TempDir(), fileName)
				_ = os.WriteFile(stubPath, []byte("# dummy\n"), 0644)
				defer os.Remove(stubPath)
				testConf = strings.ReplaceAll(testConf, "/etc/"+fileName, stubPath)
			}
		}
	}
	tmpFile, err := os.CreateTemp("", "bird_test_*.conf")
	if err != nil {
		t.Fatalf("Failed to create temp file for bird check: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString(testConf); err != nil {
		t.Fatalf("Failed to write temp file for bird check: %v", err)
	}
	_ = tmpFile.Close()

	cmd := exec.Command(birdPath, "-p", "-c", tmpFile.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("BIRD syntax validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), testConf)
	}
}

func TestBuildNodeContextAndGenerateBirdConfig(t *testing.T) {
	node1 := config.Node{
		Name:      "router1",
		Host:      "10.0.0.1",
		IP:        "192.168.100.1",
		Interface: "eth0",
		ASN:       4224420001,
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
		ASN:       4224420002,
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
	if remoteNode1["asn"].(uint64) != 4224420002 {
		t.Fatalf("Expected remote ASN 4224420002, got %v", remoteNode1["asn"])
	}

	// 2. Generate BIRD config for Node 1
	conf1, err := GenerateBirdConfig(&node1, allNodes, links)
	if err != nil {
		t.Fatalf("GenerateBirdConfig node1 failed: %v", err)
	}

	expectedSnippets := []string{
		"define SELF_IP = 192.168.100.1;",
		"define SELF_AS = 4224420001;",
		"define TABLE = 254;",
		"router id SELF_IP;",
		"protocol kernel kernel_v4",
		"if source ~ [ RTS_BGP ] then accept;",
		"protocol kernel kernel_100 {",
		"kernel table 100;",
		"if net ~ 10.0.0.0/8+ then accept;",
		"if net ~ 192.168.0.0/16+ then accept;",
		"protocol static static_self {",
		"route 192.168.100.1/32 reject;",
		"protocol static static_v4 {",
		"route 192.168.100.0/24 reject;",
		"template bgp easy42_peer",
		"protocol bgp 'easy42_peer_router2' from easy42_peer {",
		"local fe80::c0a8:6401 as SELF_AS;",
		"neighbor fe80::c0a8:6402 % 'wg42router2' as 4224420002;",
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
		"define SELF_AS = 4224420002;",
		"protocol bgp 'easy42_peer_router1' from easy42_peer {",
		"local fe80::c0a8:6402 as SELF_AS;",
		"neighbor fe80::c0a8:6401 % 'wg42router1' as 4224420001;",
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
		ASN:  4224420001,
	}
	nodeB := config.Node{
		Name: "nodeB",
		IP:   "192.168.10.2",
		ASN:  4224420002,
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

func TestBGPConfederationAndExternalPeering(t *testing.T) {
	nodeManaged := config.Node{
		Name:      "router1",
		IP:        "192.168.100.1",
		Interface: "lo",
		ASN:       4224420001,
	}
	nodeExternal := config.Node{
		Name:       "dn42peer",
		IsExternal: true,
		ASN:        4242421234,
	}

	netSettings := &config.NetworkSettings{
		PublicASN: 4242429999,
		Prefixes:  []string{"172.20.10.0/24"},
	}

	link := config.Link{
		From: config.LinkEnd{
			Name:       "router1",
			Interface:  "wg42-dn42peer",
			Address:    "fe80::1/64",
			ListenPort: 51820,
		},
		To: config.LinkEnd{
			Name:      "dn42peer",
			Interface: "wg42router1",
			Address:   "fe80::2/64",
			Endpoint:  "peer.dn42.net:51820",
			PublicKey: "PeErPuBlIcKeY1234567890=",
		},
	}

	conf, err := GenerateBirdConfig(&nodeManaged, []config.Node{nodeManaged, nodeExternal}, []config.Link{link}, netSettings)
	if err != nil {
		t.Fatalf("GenerateBirdConfig with external peer failed: %v", err)
	}

	expectedSnippets := []string{
		"define CONFED_AS = 4242429999;",
		"confederation CONFED_AS;",
		"confederation member yes;",
		"template bgp external_peer {",
		"protocol bgp 'ext_peer_dn42peer' from external_peer {",
		"local fe80::1 as CONFED_AS;",
		"neighbor fe80::2 % 'wg42-dn42peer' as 4242421234;",
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(conf, s) {
			t.Errorf("Missing expected snippet in config with external peer:\nSnippet: %s\nGenerated:\n%s", s, conf)
		}
	}
}

func TestExternalTableRouting(t *testing.T) {
	nodeManaged := config.Node{
		Name:          "router1",
		IP:            "192.168.100.1",
		Interface:     "lo",
		ASN:           4224420001,
		Table:         254,
		ExternalTable: 100,
	}
	nodeExternal := config.Node{
		Name:       "dn42peer",
		IsExternal: true,
		ASN:        4242421234,
	}

	netSettings := &config.NetworkSettings{
		PublicASN: 4242429999,
		Prefixes:  []string{"172.20.10.0/24"},
	}

	link := config.Link{
		From: config.LinkEnd{
			Name:       "router1",
			Interface:  "wg42-dn42peer",
			Address:    "fe80::1/64",
			ListenPort: 51820,
		},
		To: config.LinkEnd{
			Name:      "dn42peer",
			Interface: "wg42router1",
			Address:   "fe80::2/64",
			Endpoint:  "peer.dn42.net:51820",
			PublicKey: "PeErPuBlIcKeY1234567890=",
		},
	}

	// 1. When ExternalTable is set (100) and different from Table (254)
	conf, err := GenerateBirdConfig(&nodeManaged, []config.Node{nodeManaged, nodeExternal}, []config.Link{link}, netSettings)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}

	expectedSnippets := []string{
		"define TABLE = 254;",
		"define EXTERNAL_TABLE = 100;",
		"ipv4 table ext_table4;",
		"ipv6 table ext_table6;",
		"define COMM_EXTERNAL = (CONFED_AS, 1, 1);",
		"protocol kernel kernel_v4 {",
		"if (COMM_EXTERNAL ~ bgp_large_community) then reject;",
		"if source ~ [ RTS_BGP ] then accept;",
		"kernel table TABLE;",
		"protocol pipe pipe_ext_v4 {",
		"table master4;",
		"peer table ext_table4;",
		"if (source ~ [ RTS_BGP ]) && (COMM_EXTERNAL ~ bgp_large_community) then accept;",
		"protocol kernel kernel_ext_v4 {",
		"table ext_table4;",
		"krt_prefsrc = SELF_IP;",
		"kernel table EXTERNAL_TABLE;",
		"protocol kernel kernel_v6 {",
		"protocol pipe pipe_ext_v6 {",
		"table master6;",
		"peer table ext_table6;",
		"protocol kernel kernel_ext_v6 {",
		"table ext_table6;",
		"bgp_large_community.add(COMM_EXTERNAL);",
	}

	for _, s := range expectedSnippets {
		if !strings.Contains(conf, s) {
			t.Errorf("Missing expected snippet in config with ExternalTable=100:\nSnippet: %s\nGenerated:\n%s", s, conf)
		}
	}

	// Validate configuration syntax using BIRD if installed
	validateBirdSyntax(t, conf)

	// 2. When ExternalTable == Table (254), it should NOT generate separated external kernel tables
	nodeManagedSameTable := nodeManaged
	nodeManagedSameTable.ExternalTable = 254
	confSame, err := GenerateBirdConfig(&nodeManagedSameTable, []config.Node{nodeManagedSameTable, nodeExternal}, []config.Link{link}, netSettings)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}

	if strings.Contains(confSame, "define EXTERNAL_TABLE") {
		t.Errorf("Did not expect define EXTERNAL_TABLE when ExternalTable == Table:\n%s", confSame)
	}
	if strings.Contains(confSame, "protocol kernel kernel_ext_v4") {
		t.Errorf("Did not expect protocol kernel kernel_ext_v4 when ExternalTable == Table:\n%s", confSame)
	}
	if strings.Contains(confSame, "protocol pipe pipe_ext_v4") {
		t.Errorf("Did not expect protocol pipe pipe_ext_v4 when ExternalTable == Table:\n%s", confSame)
	}

	// 3. When ExternalTable == 0 (unset), it should NOT generate separated external kernel tables
	nodeManagedNoExt := nodeManaged
	nodeManagedNoExt.ExternalTable = 0
	confNoExt, err := GenerateBirdConfig(&nodeManagedNoExt, []config.Node{nodeManagedNoExt, nodeExternal}, []config.Link{link}, netSettings)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}

	if strings.Contains(confNoExt, "define EXTERNAL_TABLE") {
		t.Errorf("Did not expect define EXTERNAL_TABLE when ExternalTable == 0:\n%s", confNoExt)
	}
	if strings.Contains(confNoExt, "protocol kernel kernel_ext_v4") {
		t.Errorf("Did not expect protocol kernel kernel_ext_v4 when ExternalTable == 0:\n%s", confNoExt)
	}
	if strings.Contains(confNoExt, "protocol pipe pipe_ext_v4") {
		t.Errorf("Did not expect protocol pipe pipe_ext_v4 when ExternalTable == 0:\n%s", confNoExt)
	}

	// 4. When ExternalIP is set on a node with ExternalTable
	nodeWithExtIP := nodeManaged
	nodeWithExtIP.ExternalIP = "172.20.100.1"
	confExtIP, err := GenerateBirdConfig(&nodeWithExtIP, []config.Node{nodeWithExtIP, nodeExternal}, []config.Link{link}, netSettings)
	if err != nil {
		t.Fatalf("GenerateBirdConfig with ExternalIP failed: %v", err)
	}
	if !strings.Contains(confExtIP, "define EXTERNAL_IP = 172.20.100.1;") {
		t.Errorf("Expected define EXTERNAL_IP = 172.20.100.1;\nGenerated:\n%s", confExtIP)
	}
	if !strings.Contains(confExtIP, "krt_prefsrc = EXTERNAL_IP;") {
		t.Errorf("Expected krt_prefsrc = EXTERNAL_IP; in kernel_ext_v4 when ExternalIP is set\nGenerated:\n%s", confExtIP)
	}
	// Validate syntax with bird -p
	validateBirdSyntax(t, confExtIP)

	// 5. Internal node without direct external links, but with ExternalTable configured
	// It receives external routes transitively over easy42_peer tagged with COMM_EXTERNAL
	nodeInternal := config.Node{
		Name:          "internal-router",
		Host:          "10.0.0.2",
		IP:            "192.168.100.2",
		ExternalIP:    "172.20.100.2",
		ASN:           4224420002,
		Table:         254,
		ExternalTable: 100,
	}
	internalLink := config.Link{
		From: config.LinkEnd{
			Name:      "internal-router",
			Interface: "wg42router1",
			Address:   "fe80::2/64",
		},
		To: config.LinkEnd{
			Name:      "router1",
			Interface: "wg42internal-router",
			Address:   "fe80::1/64",
		},
	}
	confInternal, err := GenerateBirdConfig(&nodeInternal, []config.Node{nodeInternal, nodeManaged}, []config.Link{internalLink}, netSettings)
	if err != nil {
		t.Fatalf("GenerateBirdConfig for internal node failed: %v", err)
	}
	expectedInternalSnippets := []string{
		"define COMM_EXTERNAL = (CONFED_AS, 1, 1);",
		"define EXTERNAL_IP = 172.20.100.2;",
		"define EXTERNAL_TABLE = 100;",
		"if (COMM_EXTERNAL ~ bgp_large_community) then reject;",
		"protocol pipe pipe_ext_v4 {",
		"if (source ~ [ RTS_BGP ]) && (COMM_EXTERNAL ~ bgp_large_community) then accept;",
		"protocol kernel kernel_ext_v4 {",
		"krt_prefsrc = EXTERNAL_IP;",
	}
	for _, s := range expectedInternalSnippets {
		if !strings.Contains(confInternal, s) {
			t.Errorf("Internal node config missing expected snippet:\nSnippet: %s\nGenerated:\n%s", s, confInternal)
		}
	}
	if strings.Contains(confInternal, "template bgp external_peer") {
		t.Errorf("Internal node without external links should not have template bgp external_peer:\n%s", confInternal)
	}
	validateBirdSyntax(t, confInternal)
}

func TestStaticRoutesV4AndV6(t *testing.T) {
	node := config.Node{
		Name: "router1",
		Host: "10.0.0.1",
		IP:   "192.168.100.1",
		ASN:  4224420001,
		StaticRoutes: []string{
			"192.168.100.0/24",
			"10.0.0.0/8",
			"fd00:42::/48",
			"2001:db8:1::/64",
		},
	}

	ctx, err := BuildNodeContext(&node, []config.Node{node}, nil)
	if err != nil {
		t.Fatalf("BuildNodeContext failed: %v", err)
	}

	v4Routes, ok := ctx["static_routes_v4"].([]string)
	if !ok || len(v4Routes) != 2 {
		t.Fatalf("Expected 2 static_routes_v4, got %v", ctx["static_routes_v4"])
	}
	if v4Routes[0] != "192.168.100.0/24" || v4Routes[1] != "10.0.0.0/8" {
		t.Errorf("Unexpected static_routes_v4 content: %v", v4Routes)
	}

	v6Routes, ok := ctx["static_routes_v6"].([]string)
	if !ok || len(v6Routes) != 2 {
		t.Fatalf("Expected 2 static_routes_v6, got %v", ctx["static_routes_v6"])
	}
	if v6Routes[0] != "fd00:42::/48" || v6Routes[1] != "2001:db8:1::/64" {
		t.Errorf("Unexpected static_routes_v6 content: %v", v6Routes)
	}

	conf, err := GenerateBirdConfig(&node, []config.Node{node}, nil)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}

	expectedSnippets := []string{
		"protocol static static_v4 {",
		"    ipv4;",
		"    route 192.168.100.0/24 reject;",
		"    route 10.0.0.0/8 reject;",
		"protocol static static_v6 {",
		"    ipv6;",
		"    route fd00:42::/48 reject;",
		"    route 2001:db8:1::/64 reject;",
	}
	for _, s := range expectedSnippets {
		if !strings.Contains(conf, s) {
			t.Errorf("Missing expected snippet in generated config: %q", s)
		}
	}

	// Test with only IPv6 static routes
	nodeOnlyV6 := config.Node{
		Name: "router1",
		Host: "10.0.0.1",
		IP:   "192.168.100.1",
		ASN:  4224420001,
		StaticRoutes: []string{
			"fd00:42::/48",
		},
	}
	confV6, err := GenerateBirdConfig(&nodeOnlyV6, []config.Node{nodeOnlyV6}, nil)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}
	if strings.Contains(confV6, "protocol static static_v4") {
		t.Errorf("Expected NO static_v4 when only IPv6 routes provided")
	}
	if !strings.Contains(confV6, "protocol static static_v6") {
		t.Errorf("Expected static_v6 when IPv6 routes provided")
	}

	// Test with only IPv4 static routes
	nodeOnlyV4 := config.Node{
		Name: "router1",
		Host: "10.0.0.1",
		IP:   "192.168.100.1",
		ASN:  4224420001,
		StaticRoutes: []string{
			"192.168.100.0/24",
		},
	}
	confV4, err := GenerateBirdConfig(&nodeOnlyV4, []config.Node{nodeOnlyV4}, nil)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}
	if !strings.Contains(confV4, "protocol static static_v4") {
		t.Errorf("Expected static_v4 when IPv4 routes provided")
	}
	if strings.Contains(confV4, "protocol static static_v6") {
		t.Errorf("Expected NO static_v6 when only IPv4 routes provided")
	}

	// Test with empty static routes
	nodeNoStatic := config.Node{
		Name: "router1",
		Host: "10.0.0.1",
		IP:   "192.168.100.1",
		ASN:  4224420001,
	}
	confNoStatic, err := GenerateBirdConfig(&nodeNoStatic, []config.Node{nodeNoStatic}, nil)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}
	if strings.Contains(confNoStatic, "protocol static static_v4") {
		t.Errorf("Expected NO static_v4 when no static routes provided")
	}
	if strings.Contains(confNoStatic, "protocol static static_v6") {
		t.Errorf("Expected NO static_v6 when no static routes provided")
	}
}

func TestBIRDConfigWithCorruptedSplitPrefixes(t *testing.T) {
	nodeManaged := config.Node{
		Name:      "dedirock",
		IP:        "192.168.110.203",
		Interface: "wg0",
		ASN:       4224420001,
	}
	nodeExternal := config.Node{
		Name:       "iedon-uk",
		IsExternal: true,
		ASN:        4242422189,
	}
	link := config.Link{
		From: config.LinkEnd{
			Name:       "dedirock",
			Interface:  "wg42-iedon-uk",
			Address:    "fe80::1120/64",
			ListenPort: 22189,
		},
		To: config.LinkEnd{
			Name:      "iedon-uk",
			Interface: "wg42dedirock",
			Address:   "fd42:4242:2189:122::1",
		},
	}

	netSettings := &config.NetworkSettings{
		PublicASN: 4242421120,
		Prefixes: []string{
			"172.20.0.0/14{21",
			"29}",
			"172.20.0.0/24{28",
			"32}",
			"172.31.0.0/16+",
			"fd00::/8{44",
			"64}",
		},
	}

	conf, err := GenerateBirdConfig(&nodeManaged, []config.Node{nodeManaged, nodeExternal}, []config.Link{link}, netSettings)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}

	expectedV4 := "define EXT_PREFIXES_V4 = [ 172.20.0.0/14{21,29}, 172.20.0.0/24{28,32}, 172.31.0.0/16+ ];"
	expectedV6 := "define EXT_PREFIXES_V6 = [ fd00::/8{44,64} ];"

	if !strings.Contains(conf, expectedV4) {
		t.Errorf("Expected IPv4 prefix definition %q, got:\n%s", expectedV4, conf)
	}
	if !strings.Contains(conf, expectedV6) {
		t.Errorf("Expected IPv6 prefix definition %q, got:\n%s", expectedV6, conf)
	}
	if strings.Contains(conf, "64}") && !strings.Contains(conf, "{44,64}") {
		t.Errorf("Config contains stray 64}")
	}

	if !strings.Contains(conf, `interface "wg42-iedon-uk";`) {
		t.Errorf("Expected interface definition in BGP ext_peer block, got:\n%s", conf)
	}

	// If BIRD binary is installed on the host, run real syntax validation
	validateBirdSyntax(t, conf)
}

func TestNetworkPolicyBIRD(t *testing.T) {
	nodeLocal := config.Node{
		Name: "local-gw",
		IP:   "192.168.100.1",
		ASN:  4224420001,
	}
	nodeDefault := config.Node{Name: "node-def", IP: "192.168.100.2", ASN: 4224420002}
	nodeNone := config.Node{Name: "node-none", IP: "192.168.100.3", ASN: 4224420003}
	nodeDN42 := config.Node{Name: "node-dn42", IP: "192.168.100.4", ASN: 4242421234, IsExternal: true}
	nodeGuest := config.Node{Name: "node-guest", IP: "192.168.100.5", ASN: 4224420005}

	allNodes := []config.Node{nodeLocal, nodeDefault, nodeNone, nodeDN42, nodeGuest}

	links := []config.Link{
		{
			From: config.LinkEnd{Name: "local-gw", Interface: "wg42def", Policy: config.PolicyDefault},
			To:   config.LinkEnd{Name: "node-def", Interface: "wg42gw"},
		},
		{
			From: config.LinkEnd{Name: "local-gw", Interface: "wg42none", Policy: config.PolicyNone},
			To:   config.LinkEnd{Name: "node-none", Interface: "wg42gw"},
		},
		{
			From: config.LinkEnd{Name: "local-gw", Interface: "wg42-dn42", Policy: config.PolicyDN42},
			To:   config.LinkEnd{Name: "node-dn42", Interface: "wg42gw"},
		},
		{
			From: config.LinkEnd{Name: "local-gw", Interface: "wg42guest", Policy: "guest-pol"},
			To:   config.LinkEnd{Name: "node-guest", Interface: "wg42gw"},
		},
	}

	netSettings := &config.NetworkSettings{
		PublicASN: 4242420000,
		Prefixes:  []string{"172.20.0.0/14"},
	}

	customPolicies := []config.NetworkPolicy{
		{
			ID:                 "guest-pol",
			Name:               "Guest Partner",
			RejectInternet:     true,
			AllowedImportCIDRs: []string{"172.20.50.0/24"},
			AllowedDstCIDRs:    []string{"172.20.10.0/24"},
		},
	}

	conf, err := GenerateBirdConfig(&nodeLocal, allNodes, links, netSettings, customPolicies)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}

	// 1. Verify default peer template rejects internet on both import and export
	if !strings.Contains(conf, "template bgp easy42_peer") {
		t.Errorf("Expected template easy42_peer")
	}
	if !strings.Contains(conf, "protocol bgp 'easy42_peer_node-def' from easy42_peer") {
		t.Errorf("Expected easy42_peer_node-def protocol")
	}

	// 2. Verify none_peer template and protocol
	if !strings.Contains(conf, "template bgp none_peer") {
		t.Errorf("Expected template none_peer")
	}
	if !strings.Contains(conf, "protocol bgp 'peer_node-none' from none_peer") {
		t.Errorf("Expected peer_node-none protocol from none_peer")
	}

	// 3. Verify dn42 external_peer template and protocol
	if !strings.Contains(conf, "template bgp external_peer") {
		t.Errorf("Expected template external_peer")
	}
	if !strings.Contains(conf, "protocol bgp 'ext_peer_node-dn42' from external_peer") {
		t.Errorf("Expected ext_peer_node-dn42 protocol from external_peer")
	}

	// 4. Verify custom policy template and protocol
	if !strings.Contains(conf, "define POL_guest_pol_IMPORT_V4 = [ 172.20.50.0/24 ];") {
		t.Errorf("Expected POL_guest_pol_IMPORT_V4 define")
	}
	if !strings.Contains(conf, "define POL_guest_pol_EXPORT_V4 = [ 172.20.10.0/24 ];") {
		t.Errorf("Expected POL_guest_pol_EXPORT_V4 define")
	}
	if !strings.Contains(conf, "template bgp pol_peer_guest_pol") {
		t.Errorf("Expected template bgp pol_peer_guest_pol")
	}
	if !strings.Contains(conf, "protocol bgp 'peer_node-guest' from pol_peer_guest_pol") {
		t.Errorf("Expected peer_node-guest protocol from pol_peer_guest_pol")
	}

	// 5. Verify DN42 ROA tables and functions
	if !strings.Contains(conf, "roa4 table dn42_roa4;") {
		t.Errorf("Expected roa4 table dn42_roa4")
	}
	if !strings.Contains(conf, "roa6 table dn42_roa6;") {
		t.Errorf("Expected roa6 table dn42_roa6")
	}
	if !strings.Contains(conf, "function dn42_roa_check()") {
		t.Errorf("Expected function dn42_roa_check")
	}
	if !strings.Contains(conf, "if ! dn42_roa_check() then reject;") {
		t.Errorf("Expected dn42_roa_check reject in import filter")
	}

	// 6. Test bird binary validation if available
	validateBirdSyntax(t, conf)
}

func TestROAPoliciesBIRD(t *testing.T) {
	nodeLocal := config.Node{
		Name: "router1",
		IP:   "192.168.10.1",
		ASN:  4224420001,
	}
	nodePeer := config.Node{
		Name: "router2",
		IP:   "192.168.10.2",
		ASN:  4224420002,
	}

	links := []config.Link{
		{
			From: config.LinkEnd{Name: "router1", Interface: "wg42custom", Policy: "custom-net"},
			To:   config.LinkEnd{Name: "router2", Interface: "wg42gw"},
		},
	}

	customPolicies := []config.NetworkPolicy{
		{
			ID:        "custom-net",
			Name:      "Custom Net with Strict ROA",
			ROA4:      "https://example.com/roa4.conf",
			ROA6:      "/var/data/roa6.conf",
			ROAStrict: true,
		},
	}

	conf, err := GenerateBirdConfig(&nodeLocal, []config.Node{nodeLocal, nodePeer}, links, customPolicies)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}

	if !strings.Contains(conf, "roa4 table custom_net_roa4;") {
		t.Errorf("Expected custom_net_roa4 table definition")
	}
	if !strings.Contains(conf, "roa6 table custom_net_roa6;") {
		t.Errorf("Expected custom_net_roa6 table definition")
	}
	if !strings.Contains(conf, "include \"/etc/easy42_custom_net_roa4.conf\";") {
		t.Errorf("Expected include for custom_net_roa4")
	}
	if !strings.Contains(conf, "include \"/etc/easy42_custom_net_roa6.conf\";") {
		t.Errorf("Expected include for custom_net_roa6")
	}
	if !strings.Contains(conf, "function custom_net_roa_check()") {
		t.Errorf("Expected function custom_net_roa_check()")
	}
	if !strings.Contains(conf, "if ! custom_net_roa_check() then reject;") {
		t.Errorf("Expected custom_net_roa_check reject in template")
	}
	// Under strict mode, ROA_UNKNOWN should not return true
	if strings.Contains(conf, "if roa_check(custom_net_roa4, net, bgp_path.last) = ROA_UNKNOWN then return true;") {
		t.Errorf("Did not expect ROA_UNKNOWN acceptance when ROAStrict is true")
	}
}

func TestBGPConfederationHopPreference(t *testing.T) {
	node1 := config.Node{
		Name:      "alihk",
		IP:        "192.168.110.210",
		Interface: "wg0",
		ASN:       4224420210,
	}
	node2 := config.Node{
		Name:      "dedirock",
		IP:        "192.168.110.203",
		Interface: "wg0",
		ASN:       4224420203,
	}

	netSettings := &config.NetworkSettings{
		PublicASN: 4242421120,
		Prefixes:  []string{"172.20.0.0/14{21,29}"},
	}

	link := config.Link{
		From: config.LinkEnd{
			Name:      "alihk",
			Interface: "wg42dedirock",
			Address:   "fe80::c0a8:6ed2/64",
		},
		To: config.LinkEnd{
			Name:      "dedirock",
			Interface: "wg42alihk",
			Address:   "fe80::c0a8:6ecb/64",
		},
	}

	// 1. With Confederation (PublicASN set)
	confWithConfed, err := GenerateBirdConfig(&node1, []config.Node{node1, node2}, []config.Link{link}, netSettings)
	if err != nil {
		t.Fatalf("GenerateBirdConfig failed: %v", err)
	}

	expectedSnippets := []string{
		"define DEFAULT_LOCAL_PREF = 10000;",
		"# Prefer routes with fewer confederation hops / lower policy cost",
		"if !defined(bgp_local_pref) then bgp_local_pref = DEFAULT_LOCAL_PREF;",
		"if bgp_local_pref > 100 then bgp_local_pref = bgp_local_pref - 100; else bgp_local_pref = 1;",
	}
	for _, s := range expectedSnippets {
		if !strings.Contains(confWithConfed, s) {
			t.Errorf("Expected snippet %q in generated confederation config:\n%s", s, confWithConfed)
		}
	}

	// Syntax validation with BIRD
	if birdPath, err := exec.LookPath("bird"); err == nil {
		tmpFile, err := os.CreateTemp("", "bird_confed_test_*.conf")
		if err != nil {
			t.Fatalf("Failed to create temp bird file: %v", err)
		}
		defer os.Remove(tmpFile.Name())
		if _, err := tmpFile.WriteString(confWithConfed); err != nil {
			t.Fatalf("Failed to write temp bird file: %v", err)
		}
		tmpFile.Close()

		cmd := exec.Command(birdPath, "-p", "-c", tmpFile.Name())
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("BIRD syntax validation failed: %v\nOutput:\n%s\nConf:\n%s", err, string(out), confWithConfed)
		}
	}

	// 2. Custom policy with custom cost (e.g. 50)
	customPol := config.NetworkPolicy{
		ID:   "fast-track",
		Name: "Fast Track",
		Cost: 50,
	}
	linkCustom := config.Link{
		From: config.LinkEnd{
			Name:      "alihk",
			Interface: "wg42dedirock",
			Address:   "fe80::c0a8:6ed2/64",
			Policy:    "fast-track",
		},
		To: config.LinkEnd{
			Name:      "dedirock",
			Interface: "wg42alihk",
			Address:   "fe80::c0a8:6ecb/64",
		},
	}
	confWithCustomCost, err := GenerateBirdConfig(&node1, []config.Node{node1, node2}, []config.Link{linkCustom}, netSettings, []config.NetworkPolicy{customPol})
	if err != nil {
		t.Fatalf("GenerateBirdConfig with custom policy failed: %v", err)
	}
	expectedCustomSnippets := []string{
		"template bgp pol_peer_fast_track",
		"if bgp_local_pref > 50 then bgp_local_pref = bgp_local_pref - 50; else bgp_local_pref = 1;",
	}
	for _, s := range expectedCustomSnippets {
		if !strings.Contains(confWithCustomCost, s) {
			t.Errorf("Expected custom snippet %q in config:\n%s", s, confWithCustomCost)
		}
	}
	if birdPath, err := exec.LookPath("bird"); err == nil {
		tmpFile, err := os.CreateTemp("", "bird_confed_custom_test_*.conf")
		if err == nil {
			defer os.Remove(tmpFile.Name())
			_, _ = tmpFile.WriteString(confWithCustomCost)
			_ = tmpFile.Close()
			cmd := exec.Command(birdPath, "-p", "-c", tmpFile.Name())
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("BIRD validation failed on custom cost config: %v\nOutput:\n%s", err, string(out))
			}
		}
	}

	// 3. Without Confederation (no PublicASN)
	confWithoutConfed, err := GenerateBirdConfig(&node1, []config.Node{node1, node2}, []config.Link{link})
	if err != nil {
		t.Fatalf("GenerateBirdConfig without confederation failed: %v", err)
	}
	if strings.Contains(confWithoutConfed, "Prefer routes with fewer confederation hops") {
		t.Errorf("Did not expect confederation hop snippet when no confederation configured")
	}
}
