package ssh

import (
	"testing"

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
