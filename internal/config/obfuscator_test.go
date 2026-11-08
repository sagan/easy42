package config

import (
	"strings"
	"testing"
)

func validClientParameters() *ObfuscatorParameters {
	return &ObfuscatorParameters{
		LocalPort:  30000,
		ServerPort: 30001,
		TunName:    "tunptpeer",
		TunLocal:   "192.168.200.1",
		TunPeer:    "192.168.200.2",
		ExtraArgs:  []string{"--tun-mtu", "1400"},
	}
}

func validServerParameters() *ObfuscatorParameters {
	return &ObfuscatorParameters{
		ServerPort:    30001,
		WireGuardPort: 25532,
		TunName:       "tunptother",
		TunLocal:      "192.168.200.2",
		TunPeer:       "192.168.200.215",
	}
}

func TestNormalizeObfuscatorRole(t *testing.T) {
	tests := map[string]string{
		"":                 "none",
		"NONE":             "none",
		" Phantun_Client ": "phantun_client",
		"PHANTUN_SERVER":   "phantun_server",
		"unknown":          "unknown",
	}
	for in, want := range tests {
		if got := NormalizeObfuscatorRole(in); got != want {
			t.Errorf("NormalizeObfuscatorRole(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestValidateObfuscatorParameters(t *testing.T) {
	if err := ValidateObfuscatorParameters(ObfuscatorPhantunClient, validClientParameters()); err != nil {
		t.Fatalf("expected valid client parameters: %v", err)
	}
	if err := ValidateObfuscatorParameters(ObfuscatorPhantunServer, validServerParameters()); err != nil {
		t.Fatalf("expected valid server parameters: %v", err)
	}

	bad := validClientParameters()
	bad.TunName = strings.Repeat("a", 16)
	if err := ValidateObfuscatorParameters(ObfuscatorPhantunClient, bad); err == nil {
		t.Fatal("expected overlong tun name to be rejected")
	}

	bad = validClientParameters()
	bad.ExtraArgs = []string{"--foo;rm"}
	if err := ValidateObfuscatorParameters(ObfuscatorPhantunClient, bad); err == nil {
		t.Fatal("expected unsafe extra arg to be rejected")
	}

	bad = validServerParameters()
	bad.WireGuardPort = 0
	if err := ValidateObfuscatorParameters(ObfuscatorPhantunServer, bad); err == nil {
		t.Fatal("expected missing server wireguard port to be rejected")
	}
}

func TestValidateLinkObfuscator(t *testing.T) {
	link := &Link{
		From: LinkEnd{Name: "a", Interface: "wg42b", Obfuscator: ObfuscatorPhantunClient, ObfuscatorParameters: validClientParameters()},
		To:   LinkEnd{Name: "b", Interface: "wg42a", Obfuscator: ObfuscatorPhantunServer, ObfuscatorParameters: validServerParameters()},
	}
	fromNode := &Node{Name: "a", Mode: "agent"}
	toNode := &Node{Name: "b", Mode: "agent"}
	if err := ValidateLinkObfuscator(link, fromNode, toNode); err != nil {
		t.Fatalf("expected valid link: %v", err)
	}

	sshNode := &Node{Name: "b", Mode: "ssh"}
	if err := ValidateLinkObfuscator(link, fromNode, sshNode); err == nil {
		t.Fatal("expected SSH-mode endpoint to be rejected")
	}

	link.To.Obfuscator = ObfuscatorPhantunClient
	if err := ValidateLinkObfuscator(link, fromNode, toNode); err == nil {
		t.Fatal("expected non-complementary roles to be rejected")
	}
}

func TestObfuscatorServiceName(t *testing.T) {
	if got := ObfuscatorServiceName("wg42peer"); got != "wg42peer-obfuscator" {
		t.Fatalf("unexpected service name: %s", got)
	}
	if got := ObfuscatorServiceName("wg42peer-obfuscator"); got != "wg42peer-obfuscator" {
		t.Fatalf("expected idempotent service naming, got: %s", got)
	}
}

func TestEffectiveMTU(t *testing.T) {
	tests := []struct {
		name     string
		end      *LinkEnd
		expected int
	}{
		{name: "none", end: &LinkEnd{MTU: 1420}, expected: 1420},
		{name: "client", end: &LinkEnd{MTU: 1420, Obfuscator: ObfuscatorPhantunClient}, expected: 1408},
		{name: "server", end: &LinkEnd{MTU: 1420, Obfuscator: ObfuscatorPhantunServer}, expected: 1408},
		{name: "small", end: &LinkEnd{MTU: 10, Obfuscator: ObfuscatorPhantunClient}, expected: 1},
		{name: "unset", end: &LinkEnd{Obfuscator: ObfuscatorPhantunClient}, expected: 0},
	}
	for _, tc := range tests {
		if got := tc.end.EffectiveMTU(); got != tc.expected {
			t.Errorf("%s: expected effective MTU %d, got %d", tc.name, tc.expected, got)
		}
	}
}

func TestParseAndValidateConfigChange_ObfuscatorLinkEnd(t *testing.T) {
	snippet := `{
		"links": [
			{
				"from": {
					"name": "node-a",
					"obfuscator": "phantun_client",
					"obfuscator_parameters": {
						"local_port": 30000,
						"server_port": 30001,
						"tun_name": "tunptnode-b",
						"tun_local": "192.168.200.1",
						"tun_peer": "192.168.200.2",
						"extra_args": ["--tun-mtu", "1400"]
					}
				},
				"to": {
					"name": "node-b",
					"obfuscator": "phantun_server",
					"obfuscator_parameters": {
						"server_port": 30001,
						"wireguard_port": 25532,
						"tun_name": "tunptnode-a",
						"tun_local": "192.168.200.2",
						"tun_peer": "192.168.200.1"
					}
				}
			}
		]
	}`

	patch, _, err := ParseAndValidateConfigChange([]byte(snippet))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if patch.Links == nil || len(*patch.Links) != 1 {
		t.Fatalf("expected one link, got %+v", patch.Links)
	}
	link := (*patch.Links)[0]
	if link.From.Obfuscator == nil || *link.From.Obfuscator != ObfuscatorPhantunClient {
		t.Fatalf("expected client obfuscator, got %+v", link.From.Obfuscator)
	}
	if link.From.ObfuscatorParameters == nil || link.From.ObfuscatorParameters.LocalPort != 30000 {
		t.Fatalf("expected parsed client parameters, got %+v", link.From.ObfuscatorParameters)
	}
	if link.To.Obfuscator == nil || *link.To.Obfuscator != ObfuscatorPhantunServer {
		t.Fatalf("expected server obfuscator, got %+v", link.To.Obfuscator)
	}
	if link.To.ObfuscatorParameters == nil || link.To.ObfuscatorParameters.WireGuardPort != 25532 {
		t.Fatalf("expected parsed server parameters, got %+v", link.To.ObfuscatorParameters)
	}
}
