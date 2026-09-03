package ssh

import (
	"testing"
	"time"

	"easy42/internal/config"
)

func TestDetermineTargetMTU(t *testing.T) {
	ifaces := []config.InterfaceInfo{
		{
			Name:      "lo",
			Addresses: []string{"127.0.0.1/8"},
			Up:        true,
			MTU:       65536,
		},
		{
			Name:      "eth0",
			Addresses: []string{"192.168.1.50/24"},
			Up:        true,
			MTU:       1500,
		},
		{
			Name:      "eth1",
			Addresses: []string{"10.0.0.50/24"},
			Up:        true,
			MTU:       9000,
		},
	}

	// Direct match with host IP
	mtu := determineTargetMTU(nil, "192.168.1.50", ifaces, "")
	if mtu != 1500 {
		t.Errorf("Expected MTU 1500 for eth0, got %d", mtu)
	}

	mtu9k := determineTargetMTU(nil, "10.0.0.50", ifaces, "")
	if mtu9k != 9000 {
		t.Errorf("Expected MTU 9000 for eth1, got %d", mtu9k)
	}

	// Match via suggested interface
	mtuSugg := determineTargetMTU(nil, "hostname.domain", ifaces, "eth1")
	if mtuSugg != 9000 {
		t.Errorf("Expected MTU 9000 via suggested interface eth1, got %d", mtuSugg)
	}

	// Fallback to 1500 when no matching interface or empty
	mtuFallback := determineTargetMTU(nil, "unknown-host", nil, "")
	if mtuFallback != 1500 {
		t.Errorf("Expected fallback MTU 1500, got %d", mtuFallback)
	}
}

func TestSanitizeNodeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"my-host", "my-host"},
		{"My_Host.Local", "my-host-loc"}, // sanitized and truncated to 11 chars
		{"!!!", "node"},
		{"node-with-a-very-long-name", "node-with-a"},
	}

	for _, tt := range tests {
		got := sanitizeNodeName(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeNodeName(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDetermineSuggestedASN(t *testing.T) {
	// 1. Empty network: random 429942XXXX
	asnEmpty := DetermineSuggestedASN(nil)
	if asnEmpty < 4299420000 || asnEmpty > 4299429999 {
		t.Errorf("Expected ASN in DN42 range, got %d", asnEmpty)
	}

	// 2. Single node: should match that node's ASN
	t1 := time.Now().Add(-10 * time.Minute)
	nodes := []config.Node{
		{Name: "node-a", ASN: 4299421234, ModifiedAt: t1},
	}
	if got := DetermineSuggestedASN(nodes); got != 4299421234 {
		t.Errorf("Expected single node ASN 4299421234, got %d", got)
	}

	// 3. Multiple nodes: should pick the most recently modified node's ASN
	t2 := time.Now().Add(-5 * time.Minute)
	nodes = append(nodes, config.Node{
		Name:       "node-b",
		ASN:        4299425678,
		ModifiedAt: t2,
	})
	if got := DetermineSuggestedASN(nodes); got != 4299425678 {
		t.Errorf("Expected most recent node-b ASN 4299425678, got %d", got)
	}

	// 4. Update node-a with newer timestamp: should now pick node-a's ASN
	t3 := time.Now()
	nodes[0].ModifiedAt = t3
	nodes[0].ASN = 4299429999
	if got := DetermineSuggestedASN(nodes); got != 4299429999 {
		t.Errorf("Expected updated node-a ASN 4299429999, got %d", got)
	}

	// 5. Legacy nodes without ModifiedAt: pick the last valid node
	legacyNodes := []config.Node{
		{Name: "legacy-1", ASN: 4299420001},
		{Name: "legacy-2", ASN: 4299420002},
	}
	if got := DetermineSuggestedASN(legacyNodes); got != 4299420002 {
		t.Errorf("Expected last legacy node ASN 4299420002, got %d", got)
	}
}
