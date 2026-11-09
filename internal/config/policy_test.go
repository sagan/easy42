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

	// Negative cost tests
	polNeg := &NetworkPolicy{Cost: -50}
	if c := polNeg.EffectiveCost(); c != -50 {
		t.Errorf("policy struct cost -50: expected -50, got %d", c)
	}
	if c := end.EffectiveCostWithPolicy(polNeg); c != 30 {
		t.Errorf("explicit cost 30 overrides negative policy: expected 30, got %d", c)
	}
	end.Cost = -40
	if c := end.EffectiveCost(50); c != -40 {
		t.Errorf("explicit negative cost -40 with policy cost 50: expected -40, got %d", c)
	}
	if c := end.EffectiveCostWithPolicy(polNeg); c != -40 {
		t.Errorf("explicit negative cost -40 with policy struct cost -50: expected -40, got %d", c)
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

	// 5. LinkEnd Mark with bitmask syntax
	end.Mark = "0x80/0x80"
	if m := end.EffectiveMark(pol); m != "0x80/0x80" {
		t.Errorf("expected linkEnd mark '0x80/0x80', got %q", m)
	}

	// 6. NetworkPolicy Mark with bitmask syntax
	polBitmask := &NetworkPolicy{
		Mark: "0x40/0xc0",
	}
	endEmpty := &LinkEnd{}
	if m := polBitmask.EffectiveMark(); m != "0x40/0xc0" {
		t.Errorf("expected policy mark '0x40/0xc0', got %q", m)
	}
	if m := endEmpty.EffectiveMark(polBitmask); m != "0x40/0xc0" {
		t.Errorf("expected linkEnd mark from policy '0x40/0xc0', got %q", m)
	}

	// 7. Fwmark does NOT affect EffectiveMark
	polFw := &NetworkPolicy{
		Fwmark: "51820",
	}
	if m := polFw.EffectiveMark(); m != "" {
		t.Errorf("expected empty policy mark when only Fwmark is set, got %q", m)
	}
	if m := endEmpty.EffectiveMark(polFw); m != "" {
		t.Errorf("expected empty linkEnd mark when only policy Fwmark is set, got %q", m)
	}

	endFw := &LinkEnd{
		Fwmark: "0xca64",
	}
	if m := endFw.EffectiveMark(nil); m != "" {
		t.Errorf("expected empty linkEnd mark when only linkEnd Fwmark is set, got %q", m)
	}
	if m := endFw.EffectiveMark(polFw); m != "" {
		t.Errorf("expected empty linkEnd mark when only Fwmark is set on linkEnd and policy, got %q", m)
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

func TestEffectiveAddPaths(t *testing.T) {
	// 1. Nil policy and nil LinkEnd
	var pol *NetworkPolicy
	var end *LinkEnd
	if ap := pol.EffectiveAddPaths(); ap != AddPathsOff {
		t.Errorf("expected %q for nil policy, got %q", AddPathsOff, ap)
	}
	if ap := end.EffectiveAddPaths(pol); ap != AddPathsOff {
		t.Errorf("expected %q for nil linkEnd with nil policy, got %q", AddPathsOff, ap)
	}

	// 2. Empty policy and LinkEnd
	pol = &NetworkPolicy{}
	end = &LinkEnd{}
	if ap := pol.EffectiveAddPaths(); ap != AddPathsOff {
		t.Errorf("expected %q for empty policy, got %q", AddPathsOff, ap)
	}
	if ap := end.EffectiveAddPaths(pol); ap != AddPathsOff {
		t.Errorf("expected %q for empty linkEnd with empty policy, got %q", AddPathsOff, ap)
	}

	// 3. Policy has add_paths "on", LinkEnd unset -> inherits "on"
	pol = &NetworkPolicy{AddPaths: AddPathsOn}
	if ap := pol.EffectiveAddPaths(); ap != AddPathsOn {
		t.Errorf("expected %q for policy, got %q", AddPathsOn, ap)
	}
	if ap := end.EffectiveAddPaths(pol); ap != AddPathsOn {
		t.Errorf("expected %q on linkEnd inherited from policy, got %q", AddPathsOn, ap)
	}

	// 4. LinkEnd overrides policy with "rx"
	end.AddPaths = AddPathsRx
	if ap := end.EffectiveAddPaths(pol); ap != AddPathsRx {
		t.Errorf("expected %q overriding policy, got %q", AddPathsRx, ap)
	}

	// 5. LinkEnd overrides policy with "off"
	end.AddPaths = AddPathsOff
	if ap := end.EffectiveAddPaths(pol); ap != AddPathsOff {
		t.Errorf("expected %q overriding policy, got %q", AddPathsOff, ap)
	}

	// 6. LinkEnd overrides policy with "tx"
	end.AddPaths = AddPathsTx
	if ap := end.EffectiveAddPaths(pol); ap != AddPathsTx {
		t.Errorf("expected %q overriding policy, got %q", AddPathsTx, ap)
	}

	// 7. LinkEnd with "inherit" falls back to policy
	end.AddPaths = "inherit"
	if ap := end.EffectiveAddPaths(pol); ap != AddPathsOn {
		t.Errorf("expected inherit to revert to policy %q, got %q", AddPathsOn, ap)
	}

	// 8. LinkEnd without policy
	end.AddPaths = AddPathsRx
	if ap := end.EffectiveAddPaths(nil); ap != AddPathsRx {
		t.Errorf("expected %q with nil policy, got %q", AddPathsRx, ap)
	}
}

func TestAddPathsHelpers(t *testing.T) {
	if !IsValidAddPaths("off") || !IsValidAddPaths("on") || !IsValidAddPaths("rx") || !IsValidAddPaths("tx") {
		t.Errorf("expected standard add paths options to be valid")
	}
	if !IsValidAddPaths("both") || !IsValidAddPaths("receive") || !IsValidAddPaths("transmit") || !IsValidAddPaths("disabled") {
		t.Errorf("expected aliases to be valid")
	}
	if IsValidAddPaths("invalid_option_xyz") {
		t.Errorf("expected invalid option to return false")
	}

	if NormalizeAddPaths("on") != AddPathsOn {
		t.Errorf("expected 'on' to normalize to %q", AddPathsOn)
	}
	if NormalizeAddPaths("both") != AddPathsOn {
		t.Errorf("expected 'both' to normalize to %q", AddPathsOn)
	}
	if NormalizeAddPaths("rx") != AddPathsRx {
		t.Errorf("expected 'rx' to normalize to %q", AddPathsRx)
	}
	if NormalizeAddPaths("receive") != AddPathsRx {
		t.Errorf("expected 'receive' to normalize to %q", AddPathsRx)
	}
	if NormalizeAddPaths("tx") != AddPathsTx {
		t.Errorf("expected 'tx' to normalize to %q", AddPathsTx)
	}
	if NormalizeAddPaths("transmit") != AddPathsTx {
		t.Errorf("expected 'transmit' to normalize to %q", AddPathsTx)
	}
	if NormalizeAddPaths("off") != AddPathsOff {
		t.Errorf("expected 'off' to normalize to %q", AddPathsOff)
	}
	if NormalizeAddPaths("none") != AddPathsOff {
		t.Errorf("expected 'none' to normalize to %q", AddPathsOff)
	}
	if NormalizeAddPaths("unknown_fallback") != AddPathsOff {
		t.Errorf("expected unknown to normalize to %q", AddPathsOff)
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
	if got := ValidateDSCP(""); got != "" {
		t.Errorf("expected empty for empty input, got %q", got)
	}
	if got := ValidateDSCP("   "); got != "" {
		t.Errorf("expected empty for whitespace input, got %q", got)
	}
	if got := ValidateDSCP("46"); got != "46" {
		t.Errorf("expected 46, got %q", got)
	}
	if got := ValidateDSCP("  46  "); got != "46" {
		t.Errorf("expected trimmed 46, got %q", got)
	}
	if got := ValidateDSCP("0x2e"); got != "0x2e" {
		t.Errorf("expected 0x2e, got %q", got)
	}
	if got := ValidateDSCP("0x20/0x20"); got != "0x20/0x20" {
		t.Errorf("expected 0x20/0x20, got %q", got)
	}
	if got := ValidateDSCP(" 0x20 / 0x20 "); got != "0x20/0x20" {
		t.Errorf("expected normalized 0x20/0x20, got %q", got)
	}
	if got := ValidateDSCP("0/0x20"); got != "0/0x20" {
		t.Errorf("expected 0/0x20, got %q", got)
	}
}

func TestBlockIngressNewHelpers(t *testing.T) {
	// Standard values
	if !IsValidBlockIngressNew("") || !IsValidBlockIngressNew("all") || !IsValidBlockIngressNew("forward") {
		t.Errorf("expected standard block_ingress_new options to be valid")
	}
	// Aliases
	if !IsValidBlockIngressNew("disabled") || !IsValidBlockIngressNew("none") || !IsValidBlockIngressNew("off") ||
		!IsValidBlockIngressNew("all_ingress") || !IsValidBlockIngressNew("forwarding") || !IsValidBlockIngressNew("forward_only") {
		t.Errorf("expected aliases to be valid")
	}
	if IsValidBlockIngressNew("some_invalid_option") {
		t.Errorf("expected invalid option to return false")
	}

	// Normalization
	if got := NormalizeBlockIngressNew("all"); got != BlockIngressNewAll {
		t.Errorf("expected 'all' to normalize to %q, got %q", BlockIngressNewAll, got)
	}
	if got := NormalizeBlockIngressNew("all_ingress"); got != BlockIngressNewAll {
		t.Errorf("expected 'all_ingress' to normalize to %q, got %q", BlockIngressNewAll, got)
	}
	if got := NormalizeBlockIngressNew("forward"); got != BlockIngressNewForward {
		t.Errorf("expected 'forward' to normalize to %q, got %q", BlockIngressNewForward, got)
	}
	if got := NormalizeBlockIngressNew("forwarding"); got != BlockIngressNewForward {
		t.Errorf("expected 'forwarding' to normalize to %q, got %q", BlockIngressNewForward, got)
	}
	if got := NormalizeBlockIngressNew("disabled"); got != BlockIngressNewDisabled {
		t.Errorf("expected 'disabled' to normalize to %q, got %q", BlockIngressNewDisabled, got)
	}
	if got := NormalizeBlockIngressNew("none"); got != BlockIngressNewDisabled {
		t.Errorf("expected 'none' to normalize to %q, got %q", BlockIngressNewDisabled, got)
	}
	if got := NormalizeBlockIngressNew(""); got != BlockIngressNewDisabled {
		t.Errorf("expected empty string to normalize to %q, got %q", BlockIngressNewDisabled, got)
	}
	if got := NormalizeBlockIngressNew("invalid"); got != BlockIngressNewDisabled {
		t.Errorf("expected unrecognized string to normalize to default %q, got %q", BlockIngressNewDisabled, got)
	}

	// Effective on nil policy
	var nilPol *NetworkPolicy
	if got := nilPol.EffectiveBlockIngressNew(); got != BlockIngressNewDisabled {
		t.Errorf("expected nil policy to return disabled, got %q", got)
	}

	// Effective on policy
	pol := &NetworkPolicy{BlockIngressNew: "forward"}
	if got := pol.EffectiveBlockIngressNew(); got != BlockIngressNewForward {
		t.Errorf("expected 'forward', got %q", got)
	}

	pol2 := &NetworkPolicy{BlockIngressNew: "all_ingress"}
	if got := pol2.EffectiveBlockIngressNew(); got != BlockIngressNewAll {
		t.Errorf("expected 'all', got %q", got)
	}
}

func TestCleanIngressNewWhitelist(t *testing.T) {
	// nil / empty
	if got := CleanIngressNewWhitelist(nil); got != nil {
		t.Errorf("expected nil for nil input, got %v", got)
	}
	if got := CleanIngressNewWhitelist([]string{}); got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}

	// standard elements
	input := []string{
		"192.168.1.1:443",
		"192.168.1.1:443/tcp",
		"192.168.1.1:443/UDP",
		"10.0.0.0/24:80",
		"10.0.0.0/24:80/tcp",
		"172.16.1.1:8000-8080",
		"172.16.1.1:8000-8080/udp",
	}
	cleaned := CleanIngressNewWhitelist(input)
	expected := []string{
		"192.168.1.1:443",
		"192.168.1.1:443/tcp",
		"192.168.1.1:443/udp",
		"10.0.0.0/24:80",
		"10.0.0.0/24:80/tcp",
		"172.16.1.1:8000-8080",
		"172.16.1.1:8000-8080/udp",
	}
	if len(cleaned) != len(expected) {
		t.Fatalf("expected %d elements, got %d: %v", len(expected), len(cleaned), cleaned)
	}
	for i, v := range expected {
		if cleaned[i] != v {
			t.Errorf("expected element %d to be %q, got %q", i, v, cleaned[i])
		}
	}

	// whitespace, commas, newlines splitting and deduplication
	multilineInput := []string{
		"192.168.1.1:443, 192.168.1.1:443/tcp\n192.168.1.1:443",
	}
	cleanedMulti := CleanIngressNewWhitelist(multilineInput)
	if len(cleanedMulti) != 2 {
		t.Fatalf("expected 2 deduplicated elements, got %d: %v", len(cleanedMulti), cleanedMulti)
	}

	// invalid elements filtered out
	invalidInput := []string{
		"invalid",
		"999.999.999.999:80",
		"192.168.1.1:0",
		"192.168.1.1:70000",
		"192.168.1.1:80-70",
		"fd00::1:443", // IPv6 not accepted for ipv4 set
	}
	if got := CleanIngressNewWhitelist(invalidInput); len(got) != 0 {
		t.Errorf("expected empty for invalid entries, got %v", got)
	}
}

func TestEffectiveIngressNewWhitelist(t *testing.T) {
	var nilPol *NetworkPolicy
	if got := nilPol.EffectiveIngressNewWhitelist(); got != nil {
		t.Errorf("expected nil for nil policy, got %v", got)
	}

	disabledPol := &NetworkPolicy{
		BlockIngressNew:     BlockIngressNewDisabled,
		IngressNewWhitelist: []string{"192.168.1.1:443"},
	}
	if got := disabledPol.EffectiveIngressNewWhitelist(); got != nil {
		t.Errorf("expected nil when BlockIngressNew is disabled, got %v", got)
	}

	enabledPol := &NetworkPolicy{
		BlockIngressNew:     BlockIngressNewAll,
		IngressNewWhitelist: []string{"192.168.1.1:443"},
	}
	if got := enabledPol.EffectiveIngressNewWhitelist(); len(got) != 1 || got[0] != "192.168.1.1:443" {
		t.Errorf("expected ['192.168.1.1:443'], got %v", got)
	}
}

func TestEffectiveBirdFilters(t *testing.T) {
	var nilPol *NetworkPolicy
	if nilPol.EffectiveBirdImportFilter() != "" {
		t.Errorf("expected empty string for nil policy import filter")
	}
	if nilPol.EffectiveBirdExportFilter() != "" {
		t.Errorf("expected empty string for nil policy export filter")
	}

	emptyPol := &NetworkPolicy{}
	if emptyPol.EffectiveBirdImportFilter() != "" {
		t.Errorf("expected empty string for unset import filter")
	}
	if emptyPol.EffectiveBirdExportFilter() != "" {
		t.Errorf("expected empty string for unset export filter")
	}

	pol := &NetworkPolicy{
		BirdImportFilter: "  import rule;  ",
		BirdExportFilter: "  export rule;  ",
	}
	if pol.EffectiveBirdImportFilter() != "import rule;" {
		t.Errorf("expected trimmed import rule, got %q", pol.EffectiveBirdImportFilter())
	}
	if pol.EffectiveBirdExportFilter() != "export rule;" {
		t.Errorf("expected trimmed export rule, got %q", pol.EffectiveBirdExportFilter())
	}

	// Test bird_export_fillter alias
	aliasPol := &NetworkPolicy{
		BirdExportFillter: "  alias export rule;  ",
	}
	if aliasPol.EffectiveBirdExportFilter() != "alias export rule;" {
		t.Errorf("expected trimmed alias export rule, got %q", aliasPol.EffectiveBirdExportFilter())
	}
}

func TestEffectiveBirdSnippets(t *testing.T) {
	var nilPol *NetworkPolicy
	if nilPol.EffectiveBirdPreSnippet() != "" {
		t.Errorf("expected empty string for nil policy pre snippet")
	}
	if nilPol.EffectiveBirdPostSnippet() != "" {
		t.Errorf("expected empty string for nil policy post snippet")
	}

	emptyPol := &NetworkPolicy{}
	if emptyPol.EffectiveBirdPreSnippet() != "" {
		t.Errorf("expected empty string for unset pre snippet")
	}
	if emptyPol.EffectiveBirdPostSnippet() != "" {
		t.Errorf("expected empty string for unset post snippet")
	}

	pol := &NetworkPolicy{
		BirdPreSnippet:  "  define FOO = 1;  ",
		BirdPostSnippet: "  protocol direct { };  ",
	}
	if pol.EffectiveBirdPreSnippet() != "define FOO = 1;" {
		t.Errorf("expected trimmed pre snippet, got %q", pol.EffectiveBirdPreSnippet())
	}
	if pol.EffectiveBirdPostSnippet() != "protocol direct { };" {
		t.Errorf("expected trimmed post snippet, got %q", pol.EffectiveBirdPostSnippet())
	}
}
