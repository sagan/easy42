package config

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNetworkSettingsUnmarshalJSON(t *testing.T) {
	// Test new prefixes field takes precedence
	newJSON := `{"public_asn": 4242421234, "prefixes": ["10.0.0.0/8+"]}`
	var ns NetworkSettings
	if err := json.Unmarshal([]byte(newJSON), &ns); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !reflect.DeepEqual(ns.Prefixes, []string{"10.0.0.0/8+"}) {
		t.Errorf("Expected prefixes %v, got %v", []string{"10.0.0.0/8+"}, ns.Prefixes)
	}
}

func TestNodeConfigHooksJSON(t *testing.T) {
	nodeJSON := `{
		"name": "node1",
		"asn": 4224420001,
		"config_hooks": [
			{
				"type": "bird",
				"content": "protocol direct { ipv4; }"
			},
			{
				"type": "wg.interface",
				"target": "wg42*",
				"content": "PostUp = true"
			}
		]
	}`

	var node Node
	if err := json.Unmarshal([]byte(nodeJSON), &node); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if len(node.ConfigHooks) != 2 {
		t.Fatalf("Expected 2 config hooks, got %d", len(node.ConfigHooks))
	}
	if node.ConfigHooks[0].Type != "bird" || node.ConfigHooks[0].Content != "protocol direct { ipv4; }" {
		t.Errorf("Unexpected hook[0]: %+v", node.ConfigHooks[0])
	}
	if node.ConfigHooks[1].Type != "wg.interface" || node.ConfigHooks[1].Target != "wg42*" {
		t.Errorf("Unexpected hook[1]: %+v", node.ConfigHooks[1])
	}
}

