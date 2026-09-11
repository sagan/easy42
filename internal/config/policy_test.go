package config

import (
	"testing"
)

func TestGetBuiltinPolicies(t *testing.T) {
	policies := GetBuiltinPolicies(nil)
	if len(policies) != 3 {
		t.Fatalf("expected 3 builtin policies, got %d", len(policies))
	}

	foundDefault := false
	foundDN42 := false
	foundNone := false

	for _, p := range policies {
		if !p.IsInternal {
			t.Errorf("expected policy %s to be internal", p.ID)
		}
		switch p.ID {
		case PolicyDefault:
			foundDefault = true
			if !p.RejectInternet {
				t.Errorf("expected default policy to reject internet")
			}
			if p.FilterForward || p.FilterInput || p.SNAT != nil {
				t.Errorf("expected default policy to not have firewall/snat filters")
			}
		case PolicyDN42:
			foundDN42 = true
			if !p.FilterForward || !p.FilterInput || p.SNAT == nil {
				t.Errorf("expected dn42 policy to have firewall and snat filters")
			}
		case PolicyNone:
			foundNone = true
			if p.RejectInternet || p.FilterForward || p.FilterInput || p.SNAT != nil {
				t.Errorf("expected none policy to have no restrictions")
			}
		}
	}

	if !foundDefault || !foundDN42 || !foundNone {
		t.Errorf("missing expected built-in policies: default=%v, dn42=%v, none=%v", foundDefault, foundDN42, foundNone)
	}
}

func TestGetBuiltinPolicies_LocalDN42Networks(t *testing.T) {
	netSettings := &NetworkSettings{
		LocalDN42Networks: []string{"172.20.229.0/27", "fd00:dead:beef::/48"},
	}
	policies := GetBuiltinPolicies(netSettings)
	for _, p := range policies {
		if p.ID == PolicyDN42 {
			if len(p.LocalNetworks) != 2 {
				t.Fatalf("expected 2 LocalNetworks in PolicyDN42, got %v", p.LocalNetworks)
			}
			if p.LocalNetworks[0] != "172.20.229.0/27" || p.LocalNetworks[1] != "fd00:dead:beef::/48" {
				t.Errorf("unexpected LocalNetworks: %v", p.LocalNetworks)
			}
			return
		}
	}
	t.Fatalf("PolicyDN42 not found")
}

func TestEffectivePolicy(t *testing.T) {
	var end *LinkEnd
	if p := end.EffectivePolicy(false); p != PolicyDefault {
		t.Errorf("nil end with internal remote: expected default, got %s", p)
	}
	if p := end.EffectivePolicy(true); p != PolicyDN42 {
		t.Errorf("nil end with external remote: expected dn42, got %s", p)
	}

	end = &LinkEnd{}
	if p := end.EffectivePolicy(false); p != PolicyDefault {
		t.Errorf("empty end with internal remote: expected default, got %s", p)
	}
	if p := end.EffectivePolicy(true); p != PolicyDN42 {
		t.Errorf("empty end with external remote: expected dn42, got %s", p)
	}

	end.Policy = PolicyNone
	if p := end.EffectivePolicy(true); p != PolicyNone {
		t.Errorf("explicit none end with external remote: expected none, got %s", p)
	}

	end.Policy = "custom-dmz"
	if p := end.EffectivePolicy(false); p != "custom-dmz" {
		t.Errorf("explicit custom end: expected custom-dmz, got %s", p)
	}
}

func TestEffectiveCost(t *testing.T) {
	var end *LinkEnd
	if c := end.EffectiveCost(50); c != 50 {
		t.Errorf("nil end with policy cost 50: expected 50, got %d", c)
	}
	if c := end.EffectiveCost(0); c != 100 {
		t.Errorf("nil end with policy cost 0: expected 100, got %d", c)
	}

	end = &LinkEnd{}
	if c := end.EffectiveCost(50); c != 50 {
		t.Errorf("empty end with policy cost 50: expected 50, got %d", c)
	}

	pol := &NetworkPolicy{Cost: 75}
	if c := end.EffectiveCostWithPolicy(pol); c != 75 {
		t.Errorf("empty end with policy struct cost 75: expected 75, got %d", c)
	}

	// Cost set (not zero) overrides policy cost
	end.Cost = 30
	if c := end.EffectiveCost(50); c != 30 {
		t.Errorf("explicit cost 30 with policy cost 50: expected 30, got %d", c)
	}
	if c := end.EffectiveCostWithPolicy(pol); c != 30 {
		t.Errorf("explicit cost 30 with policy struct cost 75: expected 30, got %d", c)
	}
}

func TestCleanPortList(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "nil or empty input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "single ports comma separated",
			input:    []string{"80, 443, 53"},
			expected: []string{"80", "443", "53"},
		},
		{
			name:     "port ranges and single ports with spaces and duplicates",
			input:    []string{"80", "8000-8080", " 80 ", "443", "8000-8080"},
			expected: []string{"80", "8000-8080", "443"},
		},
		{
			name:     "wildcard all",
			input:    []string{"ALL"},
			expected: []string{"all"},
		},
		{
			name:     "wildcard star",
			input:    []string{"*"},
			expected: []string{"all"},
		},
		{
			name:     "filters invalid ports",
			input:    []string{"0", "65536", "-1", "abc", "80", "90-80", "100-200"},
			expected: []string{"80", "100-200"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CleanPortList(tc.input)
			if len(got) != len(tc.expected) {
				t.Fatalf("expected len %d, got %d: %v", len(tc.expected), len(got), got)
			}
			for i := range got {
				if got[i] != tc.expected[i] {
					t.Errorf("at index %d: expected %s, got %s", i, tc.expected[i], got[i])
				}
			}
		})
	}
}

