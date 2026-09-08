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
