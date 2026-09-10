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
