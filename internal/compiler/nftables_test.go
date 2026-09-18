package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"easy42/internal/config"
)

func TestFormatNftPrefixList(t *testing.T) {
	tests := []struct {
		name        string
		prefixes    []string
		defaultList []string
		isV6        bool
		expected    string
	}{
		{
			name:        "empty prefixes uses default v4",
			prefixes:    nil,
			defaultList: []string{"172.20.0.0/14"},
			isV6:        false,
			expected:    "{ 172.20.0.0/14 }",
		},
		{
			name:        "empty prefixes uses default v6",
			prefixes:    nil,
			defaultList: []string{"fd00::/8"},
			isV6:        true,
			expected:    "{ fd00::/8 }",
		},
		{
			name: "strips bird braces and eliminates contained subnets",
			prefixes: []string{
				"172.20.0.0/14{21,29}",
				"172.20.10.0/24",
				"10.0.0.0/8+",
			},
			defaultList: []string{"172.20.0.0/14"},
			isV6:        false,
			expected:    "{ 10.0.0.0/8, 172.20.0.0/14 }",
		},
		{
			name: "v6 prefixes deduplication and subnets",
			prefixes: []string{
				"fd00::/8{44,64}",
				"fd42:a159::/48",
				"172.20.0.0/14",
			},
			defaultList: []string{"fd00::/8"},
			isV6:        true,
			expected:    "{ fd00::/8 }",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FormatNftPrefixList(tc.prefixes, tc.defaultList, tc.isV6)
			if got != tc.expected {
				t.Errorf("expected %s, got %s", tc.expected, got)
			}
		})
	}
}

func TestGenerateNftablesConfig(t *testing.T) {
	node := config.Node{
		Name:        "test-node",
		IP:          "192.168.100.1",
		ExternalIP:  "172.20.229.13",
		ExternalIP6: "fd42:a159:f9f0::d",
		ASN:         4224420001,
	}
	allNodes := []config.Node{
		node,
		{Name: "dn42-peer", IsExternal: true},
	}
	links := []config.Link{
		{
			From: config.LinkEnd{Name: "test-node", Interface: "wg42-dn42", Policy: config.PolicyDN42},
			To:   config.LinkEnd{Name: "dn42-peer", Interface: "wg42"},
		},
	}
	netSettings := &config.NetworkSettings{
		PublicASN: 4242421234,
		Prefixes:  []string{"172.20.0.0/14{21,29}", "10.0.0.0/8", "fd00::/8{44,64}"},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, netSettings)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	// Verify required defines and rules
	if !strings.Contains(conf, "define external_ip = 172.20.229.13") {
		t.Errorf("Expected define external_ip in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "define external_ip6 = fd42:a159:f9f0::d") {
		t.Errorf("Expected define external_ip6 in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "destroy table inet easy42") {
		t.Errorf("Expected destroy table inet easy42 in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "table inet easy42 {") {
		t.Errorf("Expected table inet easy42 in conf, got:\n%s", conf)
	}

	// Verify dn42 policy rules
	if !strings.Contains(conf, `define pol_dn42_ifname = { "wg42-dn42" }`) {
		t.Errorf("Expected define pol_dn42_ifname in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, `define pol_dn42_dst_v4 = { 10.0.0.0/8, 172.20.0.0/14 }`) {
		t.Errorf("Expected define pol_dn42_dst_v4 in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, `define pol_dn42_dst_v6 = { fd00::/8 }`) {
		t.Errorf("Expected define pol_dn42_dst_v6 in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "iifname @pol_dn42_ifname ip saddr != @pol_dn42_src_v4 drop") {
		t.Errorf("Expected forward saddr drop rule in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "iifname @pol_dn42_ifname meta l4proto tcp th dport { 179 } accept") {
		t.Errorf("Expected BGP port 179 rule in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "iifname @pol_dn42_ifname meta l4proto udp th dport { 53 } accept") {
		t.Errorf("Expected DNS port 53 rule in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "iifname @pol_dn42_ifname meta l4proto { icmp, ipv6-icmp } accept") {
		t.Errorf("Expected ICMP/ICMP6 rule in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "iifname @pol_dn42_ifname counter drop") {
		t.Errorf("Expected input counter drop in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "ip saddr != @pol_dn42_dst_v4 oifname @pol_dn42_ifname meta nfproto ipv4 snat to $external_ip") {
		t.Errorf("Expected IPv4 snat rule in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "ip6 saddr != @pol_dn42_dst_v6 oifname @pol_dn42_ifname meta nfproto ipv6 snat to $external_ip6") {
		t.Errorf("Expected IPv6 snat rule in conf, got:\n%s", conf)
	}

	// Ensure no hardcoded external_ifname remains
	if strings.Contains(conf, "@external_ifname") {
		t.Errorf("Hardcoded @external_ifname should not be present in conf, got:\n%s", conf)
	}

	// Validate syntax with nft binary if installed
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestGenerateNftablesConfigWithoutExternalIP(t *testing.T) {
	node := config.Node{
		Name: "internal-only",
		IP:   "192.168.100.2",
		ASN:  4224420002,
	}

	conf, err := GenerateNftablesConfig(&node, []config.Node{node}, nil)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	if strings.Contains(conf, "define external_ip =") {
		t.Errorf("Did not expect define external_ip when node has none")
	}
	if strings.Contains(conf, "define external_ip6 =") {
		t.Errorf("Did not expect define external_ip6 when node has none")
	}
	if strings.Contains(conf, "snat to $external_ip") {
		t.Errorf("Did not expect snat rule for external_ip when none configured")
	}

	// Validate syntax with nft binary if installed
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestNetworkPolicyNftables(t *testing.T) {
	node := config.Node{
		Name:        "local-fw",
		IP:          "192.168.100.1",
		ExternalIP:  "172.20.229.13",
		ExternalIP6: "fd42:a159:f9f0::d",
		ASN:         4224420001,
	}
	allNodes := []config.Node{
		node,
		{Name: "peer-int", IP: "192.168.100.2"},
		{Name: "peer-cust", IP: "192.168.100.3"},
	}
	links := []config.Link{
		{
			From: config.LinkEnd{Name: "local-fw", Interface: "wg42intdn42", Policy: config.PolicyDN42},
			To:   config.LinkEnd{Name: "peer-int", Interface: "wg42gw"},
		},
		{
			From: config.LinkEnd{Name: "local-fw", Interface: "wg42custom", Policy: "restricted"},
			To:   config.LinkEnd{Name: "peer-cust", Interface: "wg42gw"},
		},
	}
	netSettings := &config.NetworkSettings{
		Prefixes: []string{"172.20.0.0/14", "fd00::/8"},
	}
	customPolicies := []config.NetworkPolicy{
		{
			ID:              "restricted",
			Name:            "Restricted Partner",
			AllowedDstCIDRs: []string{"172.20.10.0/24"},
			AllowedSrcCIDRs: []string{"172.20.20.0/24"},
			FilterForward:   true,
			FilterInput:     true,
			SNAT: &config.SNATConfig{
				Enabled:   true,
				Condition: "not_dst",
				Target:    "external_ip",
			},
		},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, netSettings, customPolicies)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	// 1. Verify dn42 interface set
	if !strings.Contains(conf, `define pol_dn42_ifname = { "wg42intdn42" }`) {
		t.Errorf("Expected pol_dn42_ifname set, got:\n%s", conf)
	}

	// 2. Verify custom policy sets
	if !strings.Contains(conf, `define pol_restricted_ifname = { "wg42custom" }`) {
		t.Errorf("Expected define pol_restricted_ifname, got:\n%s", conf)
	}
	if !strings.Contains(conf, `define pol_restricted_dst_v4 = { 172.20.10.0/24 }`) {
		t.Errorf("Expected define pol_restricted_dst_v4, got:\n%s", conf)
	}
	if !strings.Contains(conf, `define pol_restricted_src_v4 = { 172.20.20.0/24 }`) {
		t.Errorf("Expected define pol_restricted_src_v4, got:\n%s", conf)
	}

	// 3. Verify forward drops
	if !strings.Contains(conf, `iifname @pol_restricted_ifname ip saddr != @pol_restricted_src_v4 drop`) {
		t.Errorf("Expected saddr drop rule, got:\n%s", conf)
	}
	if !strings.Contains(conf, `iifname @pol_restricted_ifname ip daddr != @pol_restricted_dst_v4 drop`) {
		t.Errorf("Expected daddr drop rule, got:\n%s", conf)
	}

	// 4. Verify input drop
	if !strings.Contains(conf, `iifname @pol_restricted_ifname counter drop`) {
		t.Errorf("Expected input drop rule, got:\n%s", conf)
	}

	// 5. Verify SNAT rule
	if !strings.Contains(conf, `ip saddr != @pol_restricted_dst_v4 oifname @pol_restricted_ifname meta nfproto ipv4 snat to $external_ip`) {
		t.Errorf("Expected custom SNAT rule, got:\n%s", conf)
	}

	// 6. Validate with real nft binary
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestInputFilterProtocolsAndPorts(t *testing.T) {
	node := config.Node{
		Name: "router1",
		IP:   "192.168.100.1",
		ASN:  4224420001,
	}
	allNodes := []config.Node{
		node,
		{Name: "peer-lan", IP: "192.168.100.2"},
	}
	links := []config.Link{
		{
			From: config.LinkEnd{Name: "router1", Interface: "wg42custom", Policy: "lan-policy"},
			To:   config.LinkEnd{Name: "peer-lan", Interface: "wg42"},
		},
	}
	customPolicies := []config.NetworkPolicy{
		{
			ID:              "lan-policy",
			Name:            "LAN Policy with Custom Services",
			FilterInput:     true,
			InputAllowICMP:  true,
			InputAllowICMP6: false,
			InputTCPPorts:   []string{"80", "443", "8000-8080"},
			InputUDPPorts:   []string{"53", "5000-5010"},
		},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, nil, customPolicies)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	// Verify TCP port rule includes 179 + custom ports
	if !strings.Contains(conf, "iifname @pol_lan_policy_ifname meta l4proto tcp th dport { 179, 80, 443, 8000-8080 } accept") {
		t.Errorf("Expected TCP port rule with 179 and custom ports, got:\n%s", conf)
	}

	// Verify UDP port rule
	if !strings.Contains(conf, "iifname @pol_lan_policy_ifname meta l4proto udp th dport { 53, 5000-5010 } accept") {
		t.Errorf("Expected UDP port rule, got:\n%s", conf)
	}

	// Verify ICMP rule (only IPv4 ICMP, not ICMPv6)
	if !strings.Contains(conf, "iifname @pol_lan_policy_ifname meta l4proto icmp accept") {
		t.Errorf("Expected ICMP rule, got:\n%s", conf)
	}
	if strings.Contains(conf, "ipv6-icmp") {
		t.Errorf("Did not expect ipv6-icmp rule, got:\n%s", conf)
	}

	// Verify counter drop
	if !strings.Contains(conf, "iifname @pol_lan_policy_ifname counter drop") {
		t.Errorf("Expected counter drop, got:\n%s", conf)
	}

	// Validate syntax with nft binary if installed
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestInputFilterAllProtocols(t *testing.T) {
	node := config.Node{
		Name: "router1",
		IP:   "192.168.100.1",
		ASN:  4224420001,
	}
	allNodes := []config.Node{
		node,
		{Name: "peer-lan", IP: "192.168.100.2"},
	}
	links := []config.Link{
		{
			From: config.LinkEnd{Name: "router1", Interface: "wg42all", Policy: "all-proto"},
			To:   config.LinkEnd{Name: "peer-lan", Interface: "wg42"},
		},
	}
	customPolicies := []config.NetworkPolicy{
		{
			ID:              "all-proto",
			Name:            "All TCP and UDP Allowed",
			FilterInput:     true,
			InputAllowICMP:  true,
			InputAllowICMP6: true,
			InputTCPPorts:   []string{"all"},
			InputUDPPorts:   []string{"*"},
		},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, nil, customPolicies)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	if !strings.Contains(conf, "iifname @pol_all_proto_ifname meta l4proto tcp accept") {
		t.Errorf("Expected meta l4proto tcp accept, got:\n%s", conf)
	}
	if !strings.Contains(conf, "iifname @pol_all_proto_ifname meta l4proto udp accept") {
		t.Errorf("Expected meta l4proto udp accept, got:\n%s", conf)
	}
	if !strings.Contains(conf, "iifname @pol_all_proto_ifname meta l4proto { icmp, ipv6-icmp } accept") {
		t.Errorf("Expected meta l4proto { icmp, ipv6-icmp } accept, got:\n%s", conf)
	}

	// Validate syntax with nft binary if installed
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestGenerateNftablesConfigWithIP6(t *testing.T) {
	node := config.Node{
		Name: "node-ip6",
		IP:   "192.168.100.1",
		IP6:  "fd42:a159:f9f0::1",
		ASN:  4224420001,
	}

	conf, err := GenerateNftablesConfig(&node, []config.Node{node}, nil)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	if !strings.Contains(conf, "define self_ip6 = fd42:a159:f9f0::1") {
		t.Errorf("Expected define self_ip6 = fd42:a159:f9f0::1, got:\n%s", conf)
	}
	if !strings.Contains(conf, "meta nfproto ipv6 snat to $self_ip6") {
		t.Errorf("Expected ipv6 snat rule to self_ip6, got:\n%s", conf)
	}

	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestLocalNetworksForwardFilter(t *testing.T) {
	nodeBar := config.Node{
		Name:        "bar",
		IP:          "172.20.229.2",
		ExternalIP:  "172.20.229.2",
		ExternalIP6: "fd42:a159:f9f0::2",
		ASN:         4242420002,
	}
	nodeFoo := config.Node{
		Name: "foo",
		IP:   "172.20.229.1",
		ASN:  4242420001,
	}
	nodeExt := config.Node{
		Name:       "external-peer",
		IsExternal: true,
		ASN:        4242429999,
	}
	allNodes := []config.Node{nodeBar, nodeFoo, nodeExt}

	links := []config.Link{
		{
			From: config.LinkEnd{Name: "bar", Interface: "wg42-ext", Policy: config.PolicyDN42},
			To:   config.LinkEnd{Name: "external-peer", Interface: "wg42"},
		},
		{
			From: config.LinkEnd{Name: "bar", Interface: "wg42-foo", Policy: config.PolicyDefault},
			To:   config.LinkEnd{Name: "foo", Interface: "wg42-bar"},
		},
	}

	netSettings := &config.NetworkSettings{
		PublicASN:         4242420000,
		LocalDN42Networks: []string{"172.20.229.0/27", "fd42:a159:f9f0::/48"},
	}

	conf, err := GenerateNftablesConfig(&nodeBar, allNodes, links, netSettings)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	// Verify local definitions
	if !strings.Contains(conf, "define pol_dn42_local_v4 = { 172.20.229.0/27 }") {
		t.Errorf("Expected define pol_dn42_local_v4 in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "define pol_dn42_local_v6 = { fd42:a159:f9f0::/48 }") {
		t.Errorf("Expected define pol_dn42_local_v6 in conf, got:\n%s", conf)
	}

	// Verify local sets
	if !strings.Contains(conf, "elements = $pol_dn42_local_v4") {
		t.Errorf("Expected set pol_dn42_local_v4 in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "elements = $pol_dn42_local_v6") {
		t.Errorf("Expected set pol_dn42_local_v6 in conf, got:\n%s", conf)
	}

	// Verify filter_forward rules for local networks
	expectedRules := []string{
		"iifname @pol_dn42_ifname ip daddr @pol_dn42_local_v4 meta l4proto tcp th dport { 179 } accept",
		"iifname @pol_dn42_ifname ip daddr @pol_dn42_local_v4 meta l4proto udp th dport { 53 } accept",
		"iifname @pol_dn42_ifname ip daddr @pol_dn42_local_v4 meta l4proto icmp accept",
		"iifname @pol_dn42_ifname ip daddr @pol_dn42_local_v4 counter drop",
		"iifname @pol_dn42_ifname ip6 daddr @pol_dn42_local_v6 meta l4proto tcp th dport { 179 } accept",
		"iifname @pol_dn42_ifname ip6 daddr @pol_dn42_local_v6 meta l4proto udp th dport { 53 } accept",
		"iifname @pol_dn42_ifname ip6 daddr @pol_dn42_local_v6 meta l4proto ipv6-icmp accept",
		"iifname @pol_dn42_ifname ip6 daddr @pol_dn42_local_v6 counter drop",
	}
	for _, rule := range expectedRules {
		if !strings.Contains(conf, rule) {
			t.Errorf("Expected rule %q in conf, got:\n%s", rule, conf)
		}
	}

	// Verify nft -c -f validation passes
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}


func TestDSCPNftables(t *testing.T) {
	dscpIngress := 46  // EF
	dscpEgress := 10   // AF11
	node := config.Node{
		Name:        "dscp-node",
		IP:          "192.168.100.1",
		ExternalIP:  "172.20.229.13",
		ExternalIP6: "fd42:a159:f9f0::d",
		ASN:         4224420001,
	}
	allNodes := []config.Node{
		node,
		{Name: "peer-a", IP: "192.168.100.2"},
		{Name: "peer-b", IP: "192.168.100.3"},
	}
	links := []config.Link{
		{
			From: config.LinkEnd{Name: "dscp-node", Interface: "wg42dscp", Policy: "dscp-policy"},
			To:   config.LinkEnd{Name: "peer-a", Interface: "wg42"},
		},
		{
			From: config.LinkEnd{Name: "dscp-node", Interface: "wg42nodsc", Policy: "no-dscp"},
			To:   config.LinkEnd{Name: "peer-b", Interface: "wg42"},
		},
	}
	customPolicies := []config.NetworkPolicy{
		{
			ID:          "dscp-policy",
			Name:        "DSCP Both",
			DSCPIngress: &dscpIngress,
			DSCPEgress:  &dscpEgress,
		},
		{
			ID:            "no-dscp",
			Name:          "No DSCP",
			FilterForward: true,
		},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, nil, customPolicies)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	// Verify DSCP ingress rules (prerouting mangle)
	if !strings.Contains(conf, "chain mangle_prerouting_dscp {") {
		t.Errorf("Expected mangle_prerouting_dscp chain in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "iifname @pol_dscp_policy_ifname ip dscp set 46") {
		t.Errorf("Expected ip dscp set 46 ingress rule, got:\n%s", conf)
	}
	if !strings.Contains(conf, "iifname @pol_dscp_policy_ifname ip6 dscp set 46") {
		t.Errorf("Expected ip6 dscp set 46 ingress rule, got:\n%s", conf)
	}

	// Verify DSCP egress rules (postrouting mangle)
	if !strings.Contains(conf, "chain mangle_postrouting_dscp {") {
		t.Errorf("Expected mangle_postrouting_dscp chain in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "oifname @pol_dscp_policy_ifname ip dscp set 10") {
		t.Errorf("Expected ip dscp set 10 egress rule, got:\n%s", conf)
	}
	if !strings.Contains(conf, "oifname @pol_dscp_policy_ifname ip6 dscp set 10") {
		t.Errorf("Expected ip6 dscp set 10 egress rule, got:\n%s", conf)
	}

	// Verify no DSCP rules for the no-dscp policy
	if strings.Contains(conf, "pol_no_dscp_ifname ip dscp set") {
		t.Errorf("Did not expect DSCP rules for no-dscp policy, got:\n%s", conf)
	}

	// Validate syntax with nft binary if installed
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestDSCPIngressOnly(t *testing.T) {
	dscpVal := 0  // CS0
	node := config.Node{
		Name: "dscp-in",
		IP:   "192.168.100.1",
		ASN:  4224420001,
	}
	allNodes := []config.Node{node, {Name: "peer", IP: "192.168.100.2"}}
	links := []config.Link{
		{
			From: config.LinkEnd{Name: "dscp-in", Interface: "wg42dsci", Policy: "in-only"},
			To:   config.LinkEnd{Name: "peer", Interface: "wg42"},
		},
	}
	customPolicies := []config.NetworkPolicy{
		{
			ID:          "in-only",
			Name:        "Ingress Only",
			DSCPIngress: &dscpVal,
		},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, nil, customPolicies)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	// Should have prerouting chain but NOT postrouting dscp chain
	if !strings.Contains(conf, "chain mangle_prerouting_dscp {") {
		t.Errorf("Expected mangle_prerouting_dscp chain, got:\n%s", conf)
	}
	if !strings.Contains(conf, "iifname @pol_in_only_ifname ip dscp set 0") {
		t.Errorf("Expected ip dscp set 0 rule, got:\n%s", conf)
	}
	if strings.Contains(conf, "chain mangle_postrouting_dscp {") {
		t.Errorf("Did not expect mangle_postrouting_dscp chain, got:\n%s", conf)
	}

	// Validate syntax with nft binary if installed
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestNftablesConfigHooks(t *testing.T) {
	node := config.Node{
		Name: "hook-node",
		IP:   "192.168.100.1",
		ASN:  4224420001,
		ConfigHooks: []config.ConfigHook{
			{
				Type:    "nft.pre",
				Content: "define custom_pre_def = 10.99.0.0/16",
			},
			{
				Type: "nft.table",
				Content: `  chain custom_input_hook {
    type filter hook input priority -10; policy accept;
    tcp dport 8080 accept
  }`,
			},
			{
				Type:    "nft.post",
				Content: "# Custom post table hook\ntable inet custom_table {\n  chain custom_chain { type filter hook output priority 0; policy accept; }\n}",
			},
		},
	}

	conf, err := GenerateNftablesConfig(&node, []config.Node{node}, nil, nil, nil)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	if !strings.Contains(conf, "define custom_pre_def = 10.99.0.0/16") {
		t.Errorf("Expected nft.pre hook in:\n%s", conf)
	}
	if !strings.Contains(conf, "chain custom_input_hook {") || !strings.Contains(conf, "tcp dport 8080 accept") {
		t.Errorf("Expected nft.table hook in:\n%s", conf)
	}
	if !strings.Contains(conf, "table inet custom_table {") {
		t.Errorf("Expected nft.post hook in:\n%s", conf)
	}

	// Validate syntax with nft binary if installed
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestDisallowedCIDRsNftables(t *testing.T) {
	node := config.Node{
		Name:        "gw1",
		IP:          "192.168.10.1",
		ExternalIP:  "172.20.10.1",
		ExternalIP6: "fd42:10::1",
		ASN:         4224420001,
	}
	allNodes := []config.Node{
		node,
		{Name: "peer-dn42", IP: "192.168.10.2"},
		{Name: "peer-masq", IP: "192.168.10.3"},
	}
	links := []config.Link{
		{
			From: config.LinkEnd{Name: "gw1", Interface: "wg42custom", Policy: "dn42-custom"},
			To:   config.LinkEnd{Name: "peer-dn42", Interface: "wg42"},
		},
		{
			From: config.LinkEnd{Name: "gw1", Interface: "wg42masq", Policy: "masq-custom"},
			To:   config.LinkEnd{Name: "peer-masq", Interface: "wg42"},
		},
	}
	customPolicies := []config.NetworkPolicy{
		{
			ID:                 "dn42-custom",
			Name:               "DN42 Custom with Disallowed Subnets",
			AllowedDstCIDRs:    []string{"172.20.0.0/14", "fd00::/8"},
			DisallowedDstCIDRs: []string{"172.20.99.0/24", "fd42:1234:5678::/48"},
			AllowedSrcCIDRs:    []string{"172.20.0.0/14", "fd00::/8"},
			DisallowedSrcCIDRs: []string{"172.20.88.0/24", "fd42:9999:8888::/48"},
			FilterForward:      true,
			SNAT: &config.SNATConfig{
				Enabled:   true,
				Condition: "not_dst",
				Target:    "external_ip",
			},
		},
		{
			ID:                 "masq-custom",
			Name:               "Masquerade Custom with Disallowed Dst",
			AllowedDstCIDRs:    []string{"10.0.0.0/8"},
			DisallowedDstCIDRs: []string{"10.99.0.0/16", "fd00:99::/48"},
			FilterForward:      false,
			SNAT: &config.SNATConfig{
				Enabled:   true,
				Condition: "not_dst",
				Target:    "masquerade",
			},
		},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, nil, customPolicies)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	// 1. Check defines
	expectedDefines := []string{
		`define pol_dn42_custom_disallowed_dst_v4 = { 172.20.99.0/24 }`,
		`define pol_dn42_custom_disallowed_dst_v6 = { fd42:1234:5678::/48 }`,
		`define pol_dn42_custom_disallowed_src_v4 = { 172.20.88.0/24 }`,
		`define pol_dn42_custom_disallowed_src_v6 = { fd42:9999:8888::/48 }`,
		`define pol_masq_custom_disallowed_dst_v4 = { 10.99.0.0/16 }`,
		`define pol_masq_custom_disallowed_dst_v6 = { fd00:99::/48 }`,
	}
	for _, def := range expectedDefines {
		if !strings.Contains(conf, def) {
			t.Errorf("Expected define %q in config:\n%s", def, conf)
		}
	}

	// 2. Check sets
	expectedSets := []string{
		"set pol_dn42_custom_disallowed_dst_v4",
		"set pol_dn42_custom_disallowed_dst_v6",
		"set pol_dn42_custom_disallowed_src_v4",
		"set pol_dn42_custom_disallowed_src_v6",
		"set pol_masq_custom_disallowed_dst_v4",
		"set pol_masq_custom_disallowed_dst_v6",
	}
	for _, s := range expectedSets {
		if !strings.Contains(conf, s) {
			t.Errorf("Expected set %q in config:\n%s", s, conf)
		}
	}

	// 3. Check filter_forward drop rules
	expectedForwardRules := []string{
		`iifname @pol_dn42_custom_ifname ip saddr @pol_dn42_custom_disallowed_src_v4 drop`,
		`iifname @pol_dn42_custom_ifname ip saddr != @pol_dn42_custom_src_v4 drop`,
		`iifname @pol_dn42_custom_ifname ip daddr @pol_dn42_custom_disallowed_dst_v4 drop`,
		`iifname @pol_dn42_custom_ifname ip daddr != @pol_dn42_custom_dst_v4 drop`,
		`iifname @pol_dn42_custom_ifname ip6 saddr @pol_dn42_custom_disallowed_src_v6 drop`,
		`iifname @pol_dn42_custom_ifname ip6 saddr != @pol_dn42_custom_src_v6 drop`,
		`iifname @pol_dn42_custom_ifname ip6 daddr @pol_dn42_custom_disallowed_dst_v6 drop`,
		`iifname @pol_dn42_custom_ifname ip6 daddr != @pol_dn42_custom_dst_v6 drop`,
	}
	for _, r := range expectedForwardRules {
		if !strings.Contains(conf, r) {
			t.Errorf("Expected forward rule %q in config:\n%s", r, conf)
		}
	}

	// 4. Check SNAT rules
	expectedSNATRules := []string{
		`ip saddr @pol_dn42_custom_disallowed_dst_v4 oifname @pol_dn42_custom_ifname meta nfproto ipv4 snat to $external_ip`,
		`ip saddr != @pol_dn42_custom_dst_v4 oifname @pol_dn42_custom_ifname meta nfproto ipv4 snat to $external_ip`,
		`ip6 saddr @pol_dn42_custom_disallowed_dst_v6 oifname @pol_dn42_custom_ifname meta nfproto ipv6 snat to $external_ip6`,
		`ip6 saddr != @pol_dn42_custom_dst_v6 oifname @pol_dn42_custom_ifname meta nfproto ipv6 snat to $external_ip6`,
		`ip saddr @pol_masq_custom_disallowed_dst_v4 oifname @pol_masq_custom_ifname masquerade`,
		`ip saddr != @pol_masq_custom_dst_v4 oifname @pol_masq_custom_ifname masquerade`,
		`ip6 saddr @pol_masq_custom_disallowed_dst_v6 oifname @pol_masq_custom_ifname masquerade`,
	}
	for _, r := range expectedSNATRules {
		if !strings.Contains(conf, r) {
			t.Errorf("Expected SNAT rule %q in config:\n%s", r, conf)
		}
	}

	// 5. Validate with real nft binary
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestGenerateNftablesConfig_NetfilterMark(t *testing.T) {
	node := config.Node{
		Name: "router1",
		IP:   "192.168.100.1",
		ASN:  4224420001,
	}
	allNodes := []config.Node{
		node,
		{Name: "peer1"},
		{Name: "peer2"},
		{Name: "peer3"},
		{Name: "peer4"},
		{Name: "peer5"},
	}

	customPolicies := []config.NetworkPolicy{
		{
			ID:   "pol-marked",
			Name: "Policy with default mark",
			Mark: "0x1234",
		},
	}

	links := []config.Link{
		// Link 1: uses pol-marked, no override -> mark 0x1234
		{
			From: config.LinkEnd{Name: "router1", Interface: "wg42-p1", Policy: "pol-marked"},
			To:   config.LinkEnd{Name: "peer1", Interface: "wg42"},
		},
		// Link 2: uses pol-marked, overrides mark -> 42
		{
			From: config.LinkEnd{Name: "router1", Interface: "wg42-p2", Policy: "pol-marked", Mark: "42"},
			To:   config.LinkEnd{Name: "peer2", Interface: "wg42"},
		},
		// Link 3: uses pol-marked, no override -> also mark 0x1234 (grouped with Link 1)
		{
			From: config.LinkEnd{Name: "router1", Interface: "wg42-p3", Policy: "pol-marked"},
			To:   config.LinkEnd{Name: "peer3", Interface: "wg42"},
		},
		// Link 4: default policy, no mark -> no rule
		{
			From: config.LinkEnd{Name: "router1", Interface: "wg42-p4", Policy: config.PolicyDefault},
			To:   config.LinkEnd{Name: "peer4", Interface: "wg42"},
		},
		// Link 5: default policy, overrides mark -> 0xcafe
		{
			From: config.LinkEnd{Name: "router1", Interface: "wg42-p5", Policy: config.PolicyDefault, Mark: "0xcafe"},
			To:   config.LinkEnd{Name: "peer5", Interface: "wg42"},
		},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, nil, customPolicies)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	// 1. Check chain exists
	if !strings.Contains(conf, "chain mangle_prerouting_mark {") {
		t.Errorf("Expected mangle_prerouting_mark chain in conf, got:\n%s", conf)
	}

	// 2. Check grouped rule for 0x1234 (p1 and p3)
	expectedGrouped := `iifname { "wg42-p1", "wg42-p3" } meta mark set 0x1234`
	if !strings.Contains(conf, expectedGrouped) {
		t.Errorf("Expected grouped rule %q in conf, got:\n%s", expectedGrouped, conf)
	}

	// 3. Check individual rule for 42 (p2)
	expectedP2 := `iifname "wg42-p2" meta mark set 42`
	if !strings.Contains(conf, expectedP2) {
		t.Errorf("Expected rule %q in conf, got:\n%s", expectedP2, conf)
	}

	// 4. Check individual rule for 0xcafe (p5)
	expectedP5 := `iifname "wg42-p5" meta mark set 0xcafe`
	if !strings.Contains(conf, expectedP5) {
		t.Errorf("Expected rule %q in conf, got:\n%s", expectedP5, conf)
	}

	// 5. Ensure p4 is not marked
	if strings.Contains(conf, "wg42-p4") {
		t.Errorf("Did not expect wg42-p4 in mark rules, got:\n%s", conf)
	}

	// 6. Validate with real nft binary
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestNftablesSNAT_MainIP(t *testing.T) {
	node := config.Node{
		Name:        "local-node",
		IP:          "192.168.100.1",
		IP6:         "fd42:a159:f9f0::1",
		ExternalIP:  "172.20.229.13",
		ExternalIP6: "fd42:a159:f9f0::d",
	}
	allNodes := []config.Node{
		node,
		{Name: "peer-node", IP: "192.168.100.2", IP6: "fd42:a159:f9f0::2"},
	}

	customPolicies := []config.NetworkPolicy{
		{
			ID:              "pol_main",
			Name:            "Main IP SNAT Policy",
			AllowedDstCIDRs: []string{"10.0.0.0/8", "fd00::/8"},
			SNAT: &config.SNATConfig{
				Enabled:   true,
				Condition: "not_dst",
				Target:    "main_ip",
			},
		},
	}

	links := []config.Link{
		{
			From: config.LinkEnd{Name: "local-node", Interface: "wg42-main", Policy: "pol_main"},
			To:   config.LinkEnd{Name: "peer-node", Interface: "wg42"},
		},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, nil, customPolicies)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	// Must SNAT to $self_ip and $self_ip6 even though ExternalIP and ExternalIP6 exist on node
	expectedV4 := "ip saddr != @pol_pol_main_dst_v4 oifname @pol_pol_main_ifname meta nfproto ipv4 snat to $self_ip"
	expectedV6 := "ip6 saddr != @pol_pol_main_dst_v6 oifname @pol_pol_main_ifname meta nfproto ipv6 snat to $self_ip6"

	if !strings.Contains(conf, expectedV4) {
		t.Errorf("Expected Main IP IPv4 SNAT rule in conf:\n%s\nGot conf:\n%s", expectedV4, conf)
	}
	if !strings.Contains(conf, expectedV6) {
		t.Errorf("Expected Main IP IPv6 SNAT rule in conf:\n%s\nGot conf:\n%s", expectedV6, conf)
	}

	// Must not SNAT to $external_ip for this policy
	if strings.Contains(conf, "oifname @pol_pol_main_ifname meta nfproto ipv4 snat to $external_ip") {
		t.Errorf("Did not expect $external_ip SNAT target when main_ip is configured")
	}

	// Validate with nft binary
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestNftablesForwardSNAT(t *testing.T) {
	node := config.Node{
		Name:        "local-node",
		IP:          "192.168.100.1",
		IP6:         "fd42:a159:f9f0::1",
		ExternalIP:  "172.20.229.13",
		ExternalIP6: "fd42:a159:f9f0::d",
	}
	allNodes := []config.Node{
		node,
		{Name: "peer-node", IP: "192.168.100.2", IP6: "fd42:a159:f9f0::2"},
	}

	customPolicies := []config.NetworkPolicy{
		{
			ID:          "pol_fwd_masq",
			Name:        "Forward Masquerade Policy",
			ForwardSNAT: true,
			// ForwardSNATTarget empty or "masquerade"
		},
		{
			ID:                "pol_fwd_main",
			Name:              "Forward Main IP Policy",
			ForwardSNAT:       true,
			ForwardSNATTarget: "main_ip",
		},
		{
			ID:                "pol_fwd_ext",
			Name:              "Forward External IP Policy",
			ForwardSNAT:       true,
			ForwardSNATTarget: "external_ip",
		},
	}

	links := []config.Link{
		{
			From: config.LinkEnd{Name: "local-node", Interface: "wg42-fwd-m", Policy: "pol_fwd_masq"},
			To:   config.LinkEnd{Name: "peer-node", Interface: "wg42"},
		},
		{
			From: config.LinkEnd{Name: "local-node", Interface: "wg42-fwd-main", Policy: "pol_fwd_main"},
			To:   config.LinkEnd{Name: "peer-node", Interface: "wg42"},
		},
		{
			From: config.LinkEnd{Name: "local-node", Interface: "wg42-fwd-ext", Policy: "pol_fwd_ext"},
			To:   config.LinkEnd{Name: "peer-node", Interface: "wg42"},
		},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, nil, customPolicies)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	expectedMasq := `iifname @pol_pol_fwd_masq_ifname oifname != @pol_pol_fwd_masq_ifname masquerade`
	if !strings.Contains(conf, expectedMasq) {
		t.Errorf("Expected Forward masquerade rule in conf:\n%s\nGot conf:\n%s", expectedMasq, conf)
	}

	expectedMainV4 := `iifname @pol_pol_fwd_main_ifname oifname != @pol_pol_fwd_main_ifname meta nfproto ipv4 snat to $self_ip`
	expectedMainV6 := `iifname @pol_pol_fwd_main_ifname oifname != @pol_pol_fwd_main_ifname meta nfproto ipv6 snat to $self_ip6`
	if !strings.Contains(conf, expectedMainV4) {
		t.Errorf("Expected Forward Main IP IPv4 rule in conf:\n%s\nGot conf:\n%s", expectedMainV4, conf)
	}
	if !strings.Contains(conf, expectedMainV6) {
		t.Errorf("Expected Forward Main IP IPv6 rule in conf:\n%s\nGot conf:\n%s", expectedMainV6, conf)
	}

	expectedExtV4 := `iifname @pol_pol_fwd_ext_ifname oifname != @pol_pol_fwd_ext_ifname meta nfproto ipv4 snat to $external_ip`
	expectedExtV6 := `iifname @pol_pol_fwd_ext_ifname oifname != @pol_pol_fwd_ext_ifname meta nfproto ipv6 snat to $external_ip6`
	if !strings.Contains(conf, expectedExtV4) {
		t.Errorf("Expected Forward External IP IPv4 rule in conf:\n%s\nGot conf:\n%s", expectedExtV4, conf)
	}
	if !strings.Contains(conf, expectedExtV6) {
		t.Errorf("Expected Forward External IP IPv6 rule in conf:\n%s\nGot conf:\n%s", expectedExtV6, conf)
	}

	// Validate with nft binary
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(conf), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), conf)
		}
	}
}

func TestNftablesPostroutingOrder_OutboundBeforeForward(t *testing.T) {
	node := config.Node{
		Name:        "local-node",
		IP:          "192.168.100.1",
		IP6:         "fd42:a159:f9f0::1",
		ExternalIP:  "172.20.229.13",
		ExternalIP6: "fd42:a159:f9f0::d",
	}
	allNodes := []config.Node{
		node,
		{Name: "peer-a", IP: "192.168.100.2"},
		{Name: "peer-b", IP: "192.168.100.3"},
	}

	// Policy A: has Forward SNAT only
	// Policy B: has Outbound SNAT only
	customPolicies := []config.NetworkPolicy{
		{
			ID:          "pol_a",
			Name:        "Policy A",
			ForwardSNAT: true,
		},
		{
			ID:              "pol_b",
			Name:            "Policy B",
			AllowedDstCIDRs: []string{"10.0.0.0/8"},
			SNAT: &config.SNATConfig{
				Enabled:   true,
				Condition: "not_dst",
				Target:    "external_ip",
			},
		},
	}

	links := []config.Link{
		{
			From: config.LinkEnd{Name: "local-node", Interface: "wg42-a", Policy: "pol_a"},
			To:   config.LinkEnd{Name: "peer-a", Interface: "wg42"},
		},
		{
			From: config.LinkEnd{Name: "local-node", Interface: "wg42-b", Policy: "pol_b"},
			To:   config.LinkEnd{Name: "peer-b", Interface: "wg42"},
		},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, links, nil, customPolicies)
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed: %v", err)
	}

	outboundIndex := strings.Index(conf, "oifname @pol_pol_b_ifname")
	forwardIndex := strings.Index(conf, "iifname @pol_pol_a_ifname oifname != @pol_pol_a_ifname masquerade")

	if outboundIndex == -1 {
		t.Fatalf("Outbound SNAT rule for Policy B not found in conf:\n%s", conf)
	}
	if forwardIndex == -1 {
		t.Fatalf("Forward SNAT rule for Policy A not found in conf:\n%s", conf)
	}

	if outboundIndex > forwardIndex {
		t.Errorf("Expected Outbound SNAT to precede Forward SNAT in postrouting chain. Outbound pos: %d, Forward pos: %d\nConf:\n%s",
			outboundIndex, forwardIndex, conf)
	}

	// Also check when a single policy has both Outbound SNAT and Forward SNAT
	dualPolicy := config.NetworkPolicy{
		ID:              "pol_dual",
		Name:            "Dual SNAT Policy",
		ForwardSNAT:     true,
		AllowedDstCIDRs: []string{"10.0.0.0/8"},
		SNAT: &config.SNATConfig{
			Enabled:   true,
			Condition: "all",
			Target:    "main_ip",
		},
	}
	linksDual := []config.Link{
		{
			From: config.LinkEnd{Name: "local-node", Interface: "wg42-dual", Policy: "pol_dual"},
			To:   config.LinkEnd{Name: "peer-a", Interface: "wg42"},
		},
	}

	confDual, err := GenerateNftablesConfig(&node, allNodes, linksDual, nil, []config.NetworkPolicy{dualPolicy})
	if err != nil {
		t.Fatalf("GenerateNftablesConfig failed for dual: %v", err)
	}

	outboundDualIndex := strings.Index(confDual, "oifname @pol_pol_dual_ifname meta nfproto ipv4 snat to $self_ip")
	forwardDualIndex := strings.Index(confDual, "iifname @pol_pol_dual_ifname oifname != @pol_pol_dual_ifname masquerade")

	if outboundDualIndex == -1 || forwardDualIndex == -1 {
		t.Fatalf("Could not find dual rules in conf:\n%s", confDual)
	}
	if outboundDualIndex > forwardDualIndex {
		t.Errorf("In dual policy, expected Outbound SNAT (%d) before Forward SNAT (%d)\nConf:\n%s",
			outboundDualIndex, forwardDualIndex, confDual)
	}

	// Validate with nft binary
	if nftPath, err := exec.LookPath("nft"); err == nil {
		tmpFile := filepath.Join(t.TempDir(), "easy42.nft")
		if err := os.WriteFile(tmpFile, []byte(confDual), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		cmd := exec.Command(nftPath, "-c", "-f", tmpFile)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("nft -c -f validation failed: %v\nOutput:\n%s\nConfig:\n%s", err, string(out), confDual)
		}
	}
}



