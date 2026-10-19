package engine

import (
	"strings"
	"testing"

	"easy42/internal/config"
)

func TestBuildConfigureIPPayload(t *testing.T) {
	node := config.Node{
		Name:        "test-node",
		Interface:   "dummy0",
		IP:          "192.168.142.200/32",
		IP6:         "fd42:a159:f9f0:8000::c8/128",
		ExternalIP:  "172.20.229.1",
		ExternalIP6: "fd42:a159:f9f0::1",
	}

	payload := buildConfigureIPPayload(node)
	if payload == nil {
		t.Fatalf("expected non-nil payload")
	}

	envBytes, ok := payload["node_ip.env"]
	if !ok || len(envBytes) == 0 {
		t.Fatalf("expected node_ip.env in payload")
	}

	envStr := string(envBytes)
	if !strings.Contains(envStr, `NODE_NAME="test-node"`) {
		t.Errorf("expected NODE_NAME in env, got: %s", envStr)
	}
	if !strings.Contains(envStr, `NODE_INTERFACE="dummy0"`) {
		t.Errorf("expected NODE_INTERFACE in env, got: %s", envStr)
	}
	if !strings.Contains(envStr, `NODE_IP="192.168.142.200"`) {
		t.Errorf("expected NODE_IP cleaned without /32 in env, got: %s", envStr)
	}
	if !strings.Contains(envStr, `NODE_IP6="fd42:a159:f9f0:8000::c8"`) {
		t.Errorf("expected NODE_IP6 cleaned without /128 in env, got: %s", envStr)
	}
	if !strings.Contains(envStr, `NODE_EXTERNAL_IP="172.20.229.1"`) {
		t.Errorf("expected NODE_EXTERNAL_IP in env, got: %s", envStr)
	}
	if !strings.Contains(envStr, `NODE_EXTERNAL_IP6="fd42:a159:f9f0::1"`) {
		t.Errorf("expected NODE_EXTERNAL_IP6 in env, got: %s", envStr)
	}

	if !strings.Contains(envStr, `HAS_CUSTOM_SCRIPT="0"`) {
		t.Errorf("expected HAS_CUSTOM_SCRIPT=\"0\" in env, got: %s", envStr)
	}

	// Verify dynamic status.sh and run.sh
	statusBytes, ok := payload["status.sh"]
	if !ok || len(statusBytes) == 0 {
		t.Fatalf("expected status.sh in payload")
	}
	if !strings.Contains(string(statusBytes), `NODE_INTERFACE="dummy0"`) {
		t.Errorf("expected dynamic status.sh to have NODE_INTERFACE embedded")
	}

	runBytes, ok := payload["run.sh"]
	if !ok || len(runBytes) == 0 {
		t.Fatalf("expected run.sh in payload")
	}
	if !strings.Contains(string(runBytes), `NODE_INTERFACE="dummy0"`) {
		t.Errorf("expected dynamic run.sh to have NODE_INTERFACE embedded")
	}
}

func TestBuildConfigureNetworkPayloadWithCustomScript(t *testing.T) {
	node := config.Node{
		Name:                "test-node",
		Interface:           "eth0",
		IP:                  "192.168.100.1",
		CustomNetworkScript: "ip rule del table 42\nip rule add table 42 prio 100\n",
	}

	payload := buildConfigureNetworkPayload(node)
	if payload == nil {
		t.Fatalf("expected non-nil payload")
	}

	envBytes, ok := payload["node_ip.env"]
	if !ok || len(envBytes) == 0 {
		t.Fatalf("expected node_ip.env in payload")
	}
	if !strings.Contains(string(envBytes), `HAS_CUSTOM_SCRIPT="1"`) {
		t.Errorf("expected HAS_CUSTOM_SCRIPT=\"1\" in env, got: %s", string(envBytes))
	}

	customBytes, ok := payload["custom_network.sh"]
	if !ok || len(customBytes) == 0 {
		t.Fatalf("expected custom_network.sh in payload")
	}
	if !strings.Contains(string(customBytes), "ip rule add table 42 prio 100") {
		t.Errorf("unexpected custom_network.sh content: %s", string(customBytes))
	}
}

func TestCleanCIDR(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"192.168.1.1/32", "192.168.1.1"},
		{"192.168.1.1", "192.168.1.1"},
		{"fd42::1/128", "fd42::1"},
		{"  10.0.0.1/24  ", "10.0.0.1"},
		{"", ""},
	}

	for _, tt := range tests {
		actual := cleanCIDR(tt.input)
		if actual != tt.expected {
			t.Errorf("cleanCIDR(%q) = %q, expected %q", tt.input, actual, tt.expected)
		}
	}
}
