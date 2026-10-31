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

func TestBuildNodeEnv_NodeData(t *testing.T) {
	node := config.Node{
		Name:      "edge-1",
		Interface: "eth0",
		IP:        "192.168.100.1",
		Note: "# Edge Server\n```json\n{\n  \"type\": \"server\",\n  \"meta\": {\n    \"id\": 1,\n    \"level\": \"center\",\n    \"data\": {\n      \"nested_deep\": 42\n    }\n  },\n  \"tags\": [\"a\", \"b\"],\n  \"enabled\": true,\n  \"description\": \"Main gateway server\"\n}\n```\n",
	}

	envStr := buildNodeEnv(node)

	// User prompt exact requirements:
	if !strings.Contains(envStr, "NODE_DATA_TYPE=server") {
		t.Errorf("expected NODE_DATA_TYPE=server in env, got:\n%s", envStr)
	}
	if !strings.Contains(envStr, "NODE_DATA_META_ID=1") {
		t.Errorf("expected NODE_DATA_META_ID=1 in env, got:\n%s", envStr)
	}
	if !strings.Contains(envStr, "NODE_DATA_LEVEL=center") {
		t.Errorf("expected NODE_DATA_LEVEL=center in env, got:\n%s", envStr)
	}
	if !strings.Contains(envStr, "NODE_DATA_META_LEVEL=center") {
		t.Errorf("expected NODE_DATA_META_LEVEL=center in env, got:\n%s", envStr)
	}

	// 1-layer hierarchy constraint: NODE_DATA_META_DATA_* must not be set
	if strings.Contains(envStr, "NODE_DATA_META_DATA") {
		t.Errorf("expected no NODE_DATA_META_DATA_* in env, got:\n%s", envStr)
	}

	// Arrays should be ignored (only primitive value fields)
	if strings.Contains(envStr, "NODE_DATA_TAGS") {
		t.Errorf("expected arrays to be omitted from env, got:\n%s", envStr)
	}

	// Boolean primitive
	if !strings.Contains(envStr, "NODE_DATA_ENABLED=true") {
		t.Errorf("expected NODE_DATA_ENABLED=true in env, got:\n%s", envStr)
	}

	// String with spaces should be quoted safely
	if !strings.Contains(envStr, `NODE_DATA_DESCRIPTION="Main gateway server"`) {
		t.Errorf("expected quoted NODE_DATA_DESCRIPTION in env, got:\n%s", envStr)
	}

	// Test node without note
	emptyNode := config.Node{Name: "empty", IP: "10.0.0.1"}
	emptyEnv := buildNodeEnv(emptyNode)
	if strings.Contains(emptyEnv, "NODE_DATA_") {
		t.Errorf("expected no NODE_DATA_ in empty note env, got:\n%s", emptyEnv)
	}
}
