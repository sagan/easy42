package engine

import (
	"testing"

	"easy42/internal/config"
)

func TestExternalNodeDynamicEndpointResolution(t *testing.T) {
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)
	rawPass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := NewManager(store)
	if err := mgr.Unlock(rawPass); err != nil {
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	// 1. Internal node "alice" with an endpoint of "external" tag
	alice := config.Node{
		Name:      "alice",
		Host:      "10.0.0.1",
		IP:        "192.168.100.1",
		Interface: "lo",
		ASN:       4242420001,
		Entrypoints: []config.Entrypoint{
			{
				IP:   "alice.com",
				Tags: []string{"external"},
			},
		},
	}
	if err := mgr.AddNode(alice); err != nil {
		t.Fatalf("AddNode alice failed: %v", err)
	}

	// 2. External node "bob" with default "external" tag empty endpoint
	bob := config.Node{
		Name:       "bob",
		IsExternal: true,
		ASN:        4242420002,
		Entrypoints: []config.Entrypoint{
			{
				IP:   "",
				Tags: []string{"external"},
			},
		},
	}
	if err := mgr.AddNode(bob); err != nil {
		t.Fatalf("AddNode bob failed: %v", err)
	}

	// 3. Create link between alice and bob with empty endpoints (or undefined)
	fromEnd := config.LinkEnd{
		Name:       "alice",
		ListenPort: 10000,
		Address:    "fe80::1/64",
		Endpoint:   "",
	}
	toEnd := config.LinkEnd{
		Name:       "bob",
		ListenPort: 0,
		Address:    "fe80::2/64",
		Endpoint:   "",
	}

	link, err := mgr.AddLinkAdvanced("alice", "bob", &fromEnd, &toEnd, []string{"dn42"})
	if err != nil {
		t.Fatalf("AddLinkAdvanced failed: %v", err)
	}

	// Alice -> Bob: remote endpoint for bob should be empty (dynamic)
	if link.From.ResolvedEndpoint != "" {
		t.Errorf("Expected alice (From) resolved endpoint to be empty (<dynamic>), got %q", link.From.ResolvedEndpoint)
	}
	if link.From.Endpoint != "" {
		t.Errorf("Expected alice (From) endpoint to be empty, got %q", link.From.Endpoint)
	}

	// Bob -> Alice: remote endpoint for alice should be alice.com:10000
	if link.To.ResolvedEndpoint != "alice.com:10000" {
		t.Errorf("Expected bob (To) resolved endpoint to be 'alice.com:10000', got %q", link.To.ResolvedEndpoint)
	}
	if link.To.Endpoint != "alice.com:10000" {
		t.Errorf("Expected bob (To) endpoint to be 'alice.com:10000', got %q", link.To.Endpoint)
	}

	// 4. Test when toEnd.Endpoint was mistakenly populated with alice's endpoint (the UI bug)
	toEndWithAliceEP := config.LinkEnd{
		Name:       "bob",
		ListenPort: 0,
		Address:    "fe80::2/64",
		Endpoint:   "alice.com:10000",
	}
	link2, err := mgr.AddLinkAdvanced("alice", "bob", &fromEnd, &toEndWithAliceEP, []string{"dn42"})
	if err != nil {
		t.Fatalf("AddLinkAdvanced with toEnd populated failed: %v", err)
	}

	if link2.From.ResolvedEndpoint != "" {
		t.Errorf("Expected alice (From) resolved endpoint to remain empty even if toEnd had alice's endpoint, got %q", link2.From.ResolvedEndpoint)
	}
	if link2.From.Endpoint != "" {
		t.Errorf("Expected alice (From) endpoint to remain empty even if toEnd had alice's endpoint, got %q", link2.From.Endpoint)
	}
	if link2.To.ResolvedEndpoint != "alice.com:10000" {
		t.Errorf("Expected bob (To) resolved endpoint to be 'alice.com:10000', got %q", link2.To.ResolvedEndpoint)
	}

	// 5. Test when bob has an IP specified in its entrypoint
	bobWithIP := config.Node{
		Name:       "carol",
		IsExternal: true,
		ASN:        4242420003,
		Entrypoints: []config.Entrypoint{
			{
				IP:   "carol.com",
				Tags: []string{"external"},
				Ports: []config.PortSpec{
					{Port: 24242},
				},
			},
		},
	}
	if err := mgr.AddNode(bobWithIP); err != nil {
		t.Fatalf("AddNode carol failed: %v", err)
	}

	fromEndCarol := config.LinkEnd{
		Name:       "alice",
		ListenPort: 10002,
		Address:    "fe80::1/64",
	}
	toEndCarol := config.LinkEnd{
		Name:       "carol",
		ListenPort: 24242,
		Address:    "fe80::3/64",
	}

	linkCarol, err := mgr.AddLinkAdvanced("alice", "carol", &fromEndCarol, &toEndCarol, []string{"dn42"})
	if err != nil {
		t.Fatalf("AddLinkAdvanced for carol failed: %v", err)
	}

	if linkCarol.From.ResolvedEndpoint != "carol.com:24242" {
		t.Errorf("Expected alice (From) resolved endpoint to be 'carol.com:24242', got %q", linkCarol.From.ResolvedEndpoint)
	}
	if linkCarol.To.ResolvedEndpoint != "alice.com:10002" {
		t.Errorf("Expected carol (To) resolved endpoint to be 'alice.com:10002', got %q", linkCarol.To.ResolvedEndpoint)
	}
}
