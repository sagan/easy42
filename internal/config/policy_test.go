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

func TestGetBuiltinPolicies_DisallowedDN42Networks(t *testing.T) {
	netSettings := &NetworkSettings{
		DisallowedDN42Networks: []string{"fd42:1234:5678::/48", "172.20.99.0/24"},
	}
	policies := GetBuiltinPolicies(netSettings)
	for _, p := range policies {
		if p.ID == PolicyDN42 {
			if len(p.DisallowedDstCIDRs) != 2 {
				t.Fatalf("expected 2 DisallowedDstCIDRs in PolicyDN42, got %v", p.DisallowedDstCIDRs)
			}
			if p.DisallowedDstCIDRs[0] != "fd42:1234:5678::/48" || p.DisallowedDstCIDRs[1] != "172.20.99.0/24" {
				t.Errorf("unexpected DisallowedDstCIDRs: %v", p.DisallowedDstCIDRs)
			}
			if len(p.DisallowedSrcCIDRs) != 2 {
				t.Fatalf("expected 2 DisallowedSrcCIDRs in PolicyDN42, got %v", p.DisallowedSrcCIDRs)
			}
			if p.DisallowedSrcCIDRs[0] != "fd42:1234:5678::/48" || p.DisallowedSrcCIDRs[1] != "172.20.99.0/24" {
				t.Errorf("unexpected DisallowedSrcCIDRs: %v", p.DisallowedSrcCIDRs)
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

func TestEffectiveFwmarkAndPreference(t *testing.T) {
	var end *LinkEnd
	var pol *NetworkPolicy

	// 1. Both nil
	if fw := end.EffectiveFwmark(pol); fw != "" {
		t.Errorf("expected empty fwmark for nil end and pol, got %q", fw)
	}
	if pref := end.EffectivePreference(pol); pref != nil {
		t.Errorf("expected nil preference for nil end and pol, got %v", pref)
	}

	// 2. Policy defined, LinkEnd unset -> Policy value used
	polPref := 120
	pol = &NetworkPolicy{
		Fwmark:     "51820",
		Preference: &polPref,
	}
	end = &LinkEnd{}

	if fw := end.EffectiveFwmark(pol); fw != "51820" {
		t.Errorf("expected policy fwmark '51820', got %q", fw)
	}
	if pref := end.EffectivePreference(pol); pref == nil || *pref != 120 {
		t.Errorf("expected policy preference 120, got %v", pref)
	}

	// 3. LinkEnd defined -> LinkEnd overwrites NetworkPolicy
	endPref := 200
	end.Fwmark = "0xca64"
	end.Preference = &endPref

	if fw := end.EffectiveFwmark(pol); fw != "0xca64" {
		t.Errorf("expected linkEnd fwmark '0xca64' overriding policy, got %q", fw)
	}
	if pref := end.EffectivePreference(pol); pref == nil || *pref != 200 {
		t.Errorf("expected linkEnd preference 200 overriding policy, got %v", pref)
	}

	// 4. LinkEnd with preference 0 overrides policy preference
	zeroPref := 0
	end.Preference = &zeroPref
	if pref := end.EffectivePreference(pol); pref == nil || *pref != 0 {
		t.Errorf("expected linkEnd preference 0 overriding policy, got %v", pref)
	}

	// 5. LinkEnd without policy
	if fw := end.EffectiveFwmark(nil); fw != "0xca64" {
		t.Errorf("expected linkEnd fwmark '0xca64' with nil policy, got %q", fw)
	}
	if pref := end.EffectivePreference(nil); pref == nil || *pref != 0 {
		t.Errorf("expected linkEnd preference 0 with nil policy, got %v", pref)
	}
}

func TestEffectiveMark(t *testing.T) {
	// 1. Unset policy and unset linkEnd
	var pol *NetworkPolicy
	var end *LinkEnd

	if m := pol.EffectiveMark(); m != "" {
		t.Errorf("expected empty mark for nil policy, got %q", m)
	}
	if m := end.EffectiveMark(pol); m != "" {
		t.Errorf("expected empty mark for nil linkEnd with nil policy, got %q", m)
	}

	pol = &NetworkPolicy{}
	end = &LinkEnd{}
	if m := pol.EffectiveMark(); m != "" {
		t.Errorf("expected empty mark for empty policy, got %q", m)
	}
	if m := end.EffectiveMark(pol); m != "" {
		t.Errorf("expected empty mark for empty linkEnd with empty policy, got %q", m)
	}

	// 2. Policy defined, LinkEnd unset -> Policy value used
	pol = &NetworkPolicy{
		Mark: "0x1234",
	}
	end = &LinkEnd{}
	if m := pol.EffectiveMark(); m != "0x1234" {
		t.Errorf("expected policy mark '0x1234', got %q", m)
	}
	if m := end.EffectiveMark(pol); m != "0x1234" {
		t.Errorf("expected policy mark '0x1234' on linkEnd, got %q", m)
	}

	// 3. LinkEnd defined -> LinkEnd overrides NetworkPolicy
	end.Mark = "42"
	if m := end.EffectiveMark(pol); m != "42" {
		t.Errorf("expected linkEnd mark '42' overriding policy, got %q", m)
	}

	// 4. LinkEnd without policy
	if m := end.EffectiveMark(nil); m != "42" {
		t.Errorf("expected linkEnd mark '42' with nil policy, got %q", m)
	}
}

func TestEffectiveRoutingPolicy(t *testing.T) {
	// 1. Unset policy and unset linkEnd -> default "full"
	var pol *NetworkPolicy
	var end *LinkEnd

	if rp := pol.EffectiveRoutingPolicy(); rp != RoutingPolicyFull {
		t.Errorf("expected %q for nil policy, got %q", RoutingPolicyFull, rp)
	}
	if rp := end.EffectiveRoutingPolicy(pol); rp != RoutingPolicyFull {
		t.Errorf("expected %q for nil linkEnd with nil policy, got %q", RoutingPolicyFull, rp)
	}

	pol = &NetworkPolicy{}
	end = &LinkEnd{}
	if rp := pol.EffectiveRoutingPolicy(); rp != RoutingPolicyFull {
		t.Errorf("expected %q for empty policy, got %q", RoutingPolicyFull, rp)
	}
	if rp := end.EffectiveRoutingPolicy(pol); rp != RoutingPolicyFull {
		t.Errorf("expected %q for empty linkEnd with empty policy, got %q", RoutingPolicyFull, rp)
	}

	// 2. Policy defined, LinkEnd unset -> Policy value used
	pol = &NetworkPolicy{RoutingPolicy: RoutingPolicyStub}
	if rp := pol.EffectiveRoutingPolicy(); rp != RoutingPolicyStub {
		t.Errorf("expected %q for policy, got %q", RoutingPolicyStub, rp)
	}
	if rp := end.EffectiveRoutingPolicy(pol); rp != RoutingPolicyStub {
		t.Errorf("expected %q on linkEnd from policy, got %q", RoutingPolicyStub, rp)
	}

	// 3. LinkEnd defined -> overrides NetworkPolicy
	end.RoutingPolicy = RoutingPolicyReceiveOnly
	if rp := end.EffectiveRoutingPolicy(pol); rp != RoutingPolicyReceiveOnly {
		t.Errorf("expected %q overriding policy, got %q", RoutingPolicyReceiveOnly, rp)
	}

	// 4. LinkEnd without policy
	if rp := end.EffectiveRoutingPolicy(nil); rp != RoutingPolicyReceiveOnly {
		t.Errorf("expected %q with nil policy, got %q", RoutingPolicyReceiveOnly, rp)
	}

	// 5. Aliases resolution
	polAlias := &NetworkPolicy{RoutingPolicy: "originate_only"}
	if rp := polAlias.EffectiveRoutingPolicy(); rp != RoutingPolicyStub {
		t.Errorf("expected alias originate_only to resolve to %q, got %q", RoutingPolicyStub, rp)
	}

	polAlias2 := &NetworkPolicy{RoutingPolicy: "transit_client"}
	if rp := polAlias2.EffectiveRoutingPolicy(); rp != RoutingPolicyStub {
		t.Errorf("expected alias transit_client to resolve to %q, got %q", RoutingPolicyStub, rp)
	}

	polAlias3 := &NetworkPolicy{RoutingPolicy: "import_only"}
	if rp := polAlias3.EffectiveRoutingPolicy(); rp != RoutingPolicyReceiveOnly {
		t.Errorf("expected alias import_only to resolve to %q, got %q", RoutingPolicyReceiveOnly, rp)
	}

	polAlias4 := &NetworkPolicy{RoutingPolicy: "export_only"}
	if rp := polAlias4.EffectiveRoutingPolicy(); rp != RoutingPolicyAdvertiseOnly {
		t.Errorf("expected alias export_only to resolve to %q, got %q", RoutingPolicyAdvertiseOnly, rp)
	}
}

func TestRoutingPolicyHelpers(t *testing.T) {
	if !IsValidRoutingPolicy("full") || !IsValidRoutingPolicy("stub") || !IsValidRoutingPolicy("receive_only") || !IsValidRoutingPolicy("advertise_only") {
		t.Errorf("expected standard routing policies to be valid")
	}
	if !IsValidRoutingPolicy("originate_only") || !IsValidRoutingPolicy("local_only") || !IsValidRoutingPolicy("import_only") || !IsValidRoutingPolicy("export_only") {
		t.Errorf("expected aliases to be valid")
	}
	if IsValidRoutingPolicy("invalid_policy_name_xyz") {
		t.Errorf("expected invalid policy name to return false")
	}

	if NormalizeRoutingPolicy("originate_only") != RoutingPolicyStub {
		t.Errorf("expected originate_only to normalize to stub")
	}
	if NormalizeRoutingPolicy("unknown_fallback") != RoutingPolicyFull {
		t.Errorf("expected unknown to normalize to full")
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

func TestValidateDSCP(t *testing.T) {
	if got := ValidateDSCP(nil); got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}

	val := -5
	if got := ValidateDSCP(&val); got == nil || *got != 0 {
		t.Errorf("expected 0 for negative input, got %v", got)
	}

	val = 70
	if got := ValidateDSCP(&val); got == nil || *got != 63 {
		t.Errorf("expected 63 for input > 63, got %v", got)
	}

	val = 46
	if got := ValidateDSCP(&val); got == nil || *got != 46 {
		t.Errorf("expected 46 for valid input, got %v", got)
	}
}

