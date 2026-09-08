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
	allNodes := []config.Node{node}
	netSettings := &config.NetworkSettings{
		PublicASN: 4242421234,
		Prefixes:  []string{"172.20.0.0/14{21,29}", "10.0.0.0/8", "fd00::/8{44,64}"},
	}

	conf, err := GenerateNftablesConfig(&node, allNodes, nil, netSettings)
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
	if !strings.Contains(conf, "define external_prefix = { 10.0.0.0/8, 172.20.0.0/14 }") {
		t.Errorf("Expected external_prefix set in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "define external_prefix6 = { fd00::/8 }") {
		t.Errorf("Expected external_prefix6 set in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "destroy table inet easy42") {
		t.Errorf("Expected destroy table inet easy42 in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "table inet easy42 {") {
		t.Errorf("Expected table inet easy42 in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "ip saddr != @external_prefix oifname @external_ifname meta nfproto ipv4 snat to $external_ip") {
		t.Errorf("Expected IPv4 snat rule in conf, got:\n%s", conf)
	}
	if !strings.Contains(conf, "ip6 saddr != @external_prefix6 oifname @external_ifname meta nfproto ipv6 snat to $external_ip6") {
		t.Errorf("Expected IPv6 snat rule in conf, got:\n%s", conf)
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
