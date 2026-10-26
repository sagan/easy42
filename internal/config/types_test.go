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

func TestFlapAlertedSettings(t *testing.T) {
	faJSON := `{
		"flapalerted_address": "fd42:a159:f9f0:8000::c8",
		"flapalerted_port": 1790,
		"flapalerted_asn": 4224420000
	}`
	var ns NetworkSettings
	if err := json.Unmarshal([]byte(faJSON), &ns); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if ns.GetFlapAlertedAddress() != "fd42:a159:f9f0:8000::c8" {
		t.Errorf("Expected address fd42:a159:f9f0:8000::c8, got %s", ns.GetFlapAlertedAddress())
	}
	if ns.GetFlapAlertedPort() != 1790 {
		t.Errorf("Expected port 1790, got %d", ns.GetFlapAlertedPort())
	}
	if ns.GetFlapAlertedASN() != 4224420000 {
		t.Errorf("Expected ASN 4224420000, got %d", ns.GetFlapAlertedASN())
	}

	// Test default helpers when empty
	var emptyNS NetworkSettings
	if emptyNS.GetFlapAlertedPort() != 1790 {
		t.Errorf("Expected default port 1790, got %d", emptyNS.GetFlapAlertedPort())
	}
	if emptyNS.GetFlapAlertedASN() != 4224420000 {
		t.Errorf("Expected default ASN 4224420000, got %d", emptyNS.GetFlapAlertedASN())
	}

	// Test URL derivation and custom URLs
	if ns.GetFlapAlertedURL() != "http://[fd42:a159:f9f0:8000::c8]:8699" {
		t.Errorf("Expected derived IPv6 URL http://[fd42:a159:f9f0:8000::c8]:8699, got %s", ns.GetFlapAlertedURL())
	}
	ipv4NS := NetworkSettings{
		FlapAlertedAddress: "10.0.0.5",
	}
	if ipv4NS.GetFlapAlertedURL() != "http://10.0.0.5:8699" {
		t.Errorf("Expected derived IPv4 URL http://10.0.0.5:8699, got %s", ipv4NS.GetFlapAlertedURL())
	}
	if emptyNS.GetFlapAlertedURL() != "" {
		t.Errorf("Expected empty URL when unconfigured, got %s", emptyNS.GetFlapAlertedURL())
	}

	customNS := NetworkSettings{
		FlapAlertedAddress: "fd42:a159:f9f0:8000::c8",
		FlapAlertedURL:     "https://fa.example.com",
	}
	if customNS.GetFlapAlertedURL() != "https://fa.example.com" {
		t.Errorf("Expected custom URL https://fa.example.com, got %s", customNS.GetFlapAlertedURL())
	}

	customWithoutScheme := NetworkSettings{
		FlapAlertedURL: "fa.example.com:8699",
	}
	if customWithoutScheme.GetFlapAlertedURL() != "http://fa.example.com:8699" {
		t.Errorf("Expected custom URL with prepended http://, got %s", customWithoutScheme.GetFlapAlertedURL())
	}
}

func TestNodeEnableFA(t *testing.T) {
	nodeJSON := `{"name": "router1", "asn": 4224420001, "enable_fa": true}`
	var node Node
	if err := json.Unmarshal([]byte(nodeJSON), &node); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !node.EnableFA {
		t.Errorf("Expected EnableFA to be true")
	}

	data, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if !reflect.DeepEqual(true, reflect.ValueOf(node.EnableFA).Bool()) {
		t.Errorf("Expected EnableFA true")
	}
	var roundTrip Node
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("roundtrip unmarshal failed: %v", err)
	}
	if !roundTrip.EnableFA {
		t.Errorf("Expected roundTrip.EnableFA to be true")
	}
}

func TestNetworkSettingsName(t *testing.T) {
	var emptyNS NetworkSettings
	if emptyNS.GetName() != "Easy42" {
		t.Errorf("Expected 'Easy42', got '%s'", emptyNS.GetName())
	}

	customNS := NetworkSettings{Name: "NeoMesh"}
	if customNS.GetName() != "NeoMesh" {
		t.Errorf("Expected 'NeoMesh', got '%s'", customNS.GetName())
	}

	whitespaceNS := NetworkSettings{Name: "   "}
	if whitespaceNS.GetName() != "Easy42" {
		t.Errorf("Expected 'Easy42', got '%s'", whitespaceNS.GetName())
	}
}


