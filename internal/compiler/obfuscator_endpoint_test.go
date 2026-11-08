package compiler

import (
	"testing"

	"easy42/internal/config"
)

func TestResolveLinkEndpointObfuscatorOverride(t *testing.T) {
	fromNode := &config.Node{Name: "alpha", IP: "192.168.100.1", Entrypoints: []config.Entrypoint{{IP: "1.1.1.1", Tags: []string{"default"}}}}
	toNode := &config.Node{Name: "beta", IP: "192.168.100.2", Entrypoints: []config.Entrypoint{{IP: "2.2.2.2", Tags: []string{"default"}}}}
	fromEnd := &config.LinkEnd{
		Name:                 "alpha",
		Interface:            "wg42beta",
		ListenPort:           28389,
		Endpoint:             "2.2.2.2:25532",
		Obfuscator:           config.ObfuscatorPhantunClient,
		ObfuscatorParameters: &config.ObfuscatorParameters{LocalPort: 30000, ServerPort: 30001},
	}
	toEnd := &config.LinkEnd{
		Name:       "beta",
		Interface:  "wg42alpha",
		ListenPort: 25532,
		Endpoint:   "1.1.1.1:28389",
		Obfuscator: config.ObfuscatorPhantunServer,
	}

	if got := ResolveLinkEndpoint(fromNode, toNode, fromEnd, toEnd); got != "127.0.0.1:30000" {
		t.Fatalf("expected loopback client endpoint, got %s", got)
	}
	if got := ResolveLinkEndpoint(toNode, fromNode, toEnd, fromEnd); got != "" {
		t.Fatalf("expected empty server endpoint, got %s", got)
	}
	if got := ResolveUnderlyingLinkEndpoint(fromNode, toNode, fromEnd, toEnd); got != "2.2.2.2:25532" {
		t.Fatalf("expected underlying endpoint, got %s", got)
	}
	if got := ResolveUnderlyingLinkEndpoint(toNode, fromNode, toEnd, fromEnd); got != "1.1.1.1:28389" {
		t.Fatalf("expected underlying server endpoint, got %s", got)
	}
}
