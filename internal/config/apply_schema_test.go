package config

import (
	"testing"
)

func TestParseAndValidateConfigChange_Nodes(t *testing.T) {
	jsonSnippet := `
	{
		"nodes": [
			{
				"name": "foobar",
				"static_routes": [
					"1.2.3.4",
					"192.168.100.0/24"
				]
			}
		]
	}`

	patch, format, err := ParseAndValidateConfigChange([]byte(jsonSnippet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if format != "json" {
		t.Fatalf("expected format 'json', got '%s'", format)
	}
	if patch.Nodes == nil || len(*patch.Nodes) != 1 {
		t.Fatalf("expected 1 node patch, got: %+v", patch.Nodes)
	}
	node := (*patch.Nodes)[0]
	if node.Name != "foobar" {
		t.Errorf("expected node name 'foobar', got '%s'", node.Name)
	}
	if node.StaticRoutes == nil || len(*node.StaticRoutes) != 2 {
		t.Fatalf("expected 2 static routes, got %+v", node.StaticRoutes)
	}
}

func TestParseAndValidateConfigChange_Links(t *testing.T) {
	jsonSnippet := `
	{
		"links": [
			{
				"from": {
					"name": "node-a",
					"cost": 50
				},
				"to": {
					"name": "node-b",
					"cost": 50
				}
			}
		]
	}`

	patch, format, err := ParseAndValidateConfigChange([]byte(jsonSnippet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if format != "json" {
		t.Fatalf("expected format 'json', got '%s'", format)
	}
	if patch.Links == nil || len(*patch.Links) != 1 {
		t.Fatalf("expected 1 link patch, got: %+v", patch.Links)
	}
	link := (*patch.Links)[0]
	if link.From == nil || link.From.Name != "node-a" {
		t.Errorf("expected from node-a, got %+v", link.From)
	}
	if link.To == nil || link.To.Name != "node-b" {
		t.Errorf("expected to node-b, got %+v", link.To)
	}
}

func TestParseAndValidateConfigChange_RejectsShorthand(t *testing.T) {
	shorthands := []string{
		`{"name": "foobar", "static_routes": ["1.2.3.4"]}`,
		`{"from": {"name": "n1"}, "to": {"name": "n2"}}`,
	}

	for _, s := range shorthands {
		_, _, err := ParseAndValidateConfigChange([]byte(s))
		if err == nil {
			t.Errorf("expected shorthand %s to be rejected, got nil error", s)
		}
	}
}

func TestParseAndValidateConfigChange_YAML(t *testing.T) {
	yamlSnippet := `
nodes:
  - name: foobar
    asn: 4224420202
    static_routes:
      - 10.0.0.0/8
      - 172.20.0.0/14
`
	patch, format, err := ParseAndValidateConfigChange([]byte(yamlSnippet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if format != "yaml" {
		t.Fatalf("expected format 'yaml', got '%s'", format)
	}
	if patch.Nodes == nil || len(*patch.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %+v", patch.Nodes)
	}
	node := (*patch.Nodes)[0]
	if node.ASN == nil || *node.ASN != 4224420202 {
		t.Errorf("expected ASN 4224420202, got %v", node.ASN)
	}
}

func TestParseAndValidateConfigChange_ForbiddenRootFields(t *testing.T) {
	snippets := []string{
		`{"password_hash": "evil"}`,
		`{"encrypted_dek": "evil"}`,
		`{"session_secret": "evil"}`,
	}

	for _, s := range snippets {
		_, _, err := ParseAndValidateConfigChange([]byte(s))
		if err == nil {
			t.Errorf("expected error for forbidden snippet %s, got nil", s)
		}
	}
}

func TestParseAndValidateConfigChange_ForbiddenDerivativeFields(t *testing.T) {
	// Node modified_at or agent_last_seen inside nodes array
	s1 := `{"nodes": [{"name": "n1", "modified_at": "2026-01-01T00:00:00Z"}]}`
	if _, _, err := ParseAndValidateConfigChange([]byte(s1)); err == nil {
		t.Errorf("expected error for node modified_at, got nil")
	}

	s2 := `{"nodes": [{"name": "n1", "agent_version": "1.0.0"}]}`
	if _, _, err := ParseAndValidateConfigChange([]byte(s2)); err == nil {
		t.Errorf("expected error for node agent_version, got nil")
	}

	// Link resolved_endpoint inside links array
	s3 := `{"links": [{"from": {"name": "n1"}, "to": {"name": "n2"}, "resolved_endpoint": "1.2.3.4"}]}`
	if _, _, err := ParseAndValidateConfigChange([]byte(s3)); err == nil {
		t.Errorf("expected error for link resolved_endpoint, got nil")
	}
}

func TestParseAndValidateConfigChange_UnknownField(t *testing.T) {
	s := `{"nodes": [{"name": "n1", "unrecognized_field": "val"}]}`
	if _, _, err := ParseAndValidateConfigChange([]byte(s)); err == nil {
		t.Errorf("expected error for unknown field, got nil")
	}
}

func TestParseAndValidateConfigChange_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		snippet string
	}{
		{"invalid node name", `{"nodes": [{"name": "bad name with spaces"}]}`},
		{"invalid IPv4", `{"nodes": [{"name": "n1", "ip": "999.999.999.999"}]}`},
		{"invalid IPv6", `{"nodes": [{"name": "n1", "ip6": "not:ipv6"}]}`},
		{"invalid CIDR", `{"nodes": [{"name": "n1", "static_routes": ["invalid/cidr/bad"]}]}`},
		{"zero ASN", `{"nodes": [{"name": "n1", "asn": 0}]}`},
		{"negative table", `{"nodes": [{"name": "n1", "table": -5}]}`},
		{"same link endpoints", `{"links": [{"from": {"name": "same"}, "to": {"name": "same"}}]}`},
		{"invalid port", `{"links": [{"from": {"name": "n1", "listen_port": 999999}, "to": {"name": "n2"}}]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := ParseAndValidateConfigChange([]byte(tt.snippet)); err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}
