package config

import (
	"strings"
)

const (
	PolicyDefault = "default"
	PolicyDN42    = "dn42"
	PolicyNone    = "none"
)

// SNATConfig represents source network address translation options for egress traffic
type SNATConfig struct {
	Enabled   bool   `json:"enabled"`
	Condition string `json:"condition,omitempty"` // "not_dst" (saddr != allowed dst CIDRs) | "all"
	Target    string `json:"target,omitempty"`    // "external_ip" | "masquerade" | custom IP string
}

// NetworkPolicy defines routing and firewall controls deployed on a link endpoint
type NetworkPolicy struct {
	ID                 string      `json:"id"`
	Name               string      `json:"name"`
	Description        string      `json:"description,omitempty"`
	IsInternal         bool        `json:"is_internal,omitempty"` // true for built-in virtual policies (read-only)
	Cost               int         `json:"cost,omitempty"`        // Cost deducted from bgp_local_pref on internal hops (default 100)
	AllowedDstCIDRs    []string    `json:"allowed_dst_cidrs,omitempty"`
	AllowedSrcCIDRs    []string    `json:"allowed_src_cidrs,omitempty"`
	AllowedImportCIDRs []string    `json:"allowed_import_cidrs,omitempty"`
	RejectInternet     bool        `json:"reject_internet"`
	FilterForward      bool        `json:"filter_forward"`
	FilterInput        bool        `json:"filter_input"`
	SNAT               *SNATConfig `json:"snat,omitempty"`
	ROA4               string      `json:"roa4,omitempty"`
	ROA6               string      `json:"roa6,omitempty"`
	ROAStrict          bool        `json:"roa_strict,omitempty"`
}

// EffectiveCost returns the configured cost or default 100 if unset/non-positive
func (p *NetworkPolicy) EffectiveCost() int {
	if p != nil && p.Cost > 0 {
		return p.Cost
	}
	return 100
}

// GetBuiltinPolicies returns the three standard built-in virtual policies: default, dn42, none.
// For dn42, default CIDRs are populated from global NetworkSettings if provided.
func GetBuiltinPolicies(netSettings *NetworkSettings) []NetworkPolicy {
	var dn42Prefixes []string
	if netSettings != nil && len(netSettings.Prefixes) > 0 {
		dn42Prefixes = CleanPrefixes(netSettings.Prefixes)
	} else {
		dn42Prefixes = []string{"172.20.0.0/14{21,29}", "fd00::/8{44,64}"}
	}

	return []NetworkPolicy{
		{
			ID:             PolicyDefault,
			Name:           "Default (Internal)",
			Description:    "Standard internal mesh policy with Internet route leak protection",
			IsInternal:     true,
			Cost:           100,
			RejectInternet: true,
			FilterForward:  false,
			FilterInput:    false,
		},
		{
			ID:                 PolicyDN42,
			Name:               "DN42 / External Peering",
			Description:        "Peering policy for external/DN42 networks with BGP route filtering, ingress/egress firewall, and SNAT",
			IsInternal:         true,
			Cost:               100,
			AllowedDstCIDRs:    dn42Prefixes,
			AllowedSrcCIDRs:    dn42Prefixes,
			AllowedImportCIDRs: dn42Prefixes,
			RejectInternet:     true,
			FilterForward:      true,
			FilterInput:        true,
			SNAT: &SNATConfig{
				Enabled:   true,
				Condition: "not_dst",
				Target:    "external_ip",
			},
			ROA4: "https://dn42.burble.com/roa/dn42_roa_bird2_4.conf",
			ROA6: "https://dn42.burble.com/roa/dn42_roa_bird2_6.conf",
		},
		{
			ID:             PolicyNone,
			Name:           "None (Unrestricted)",
			Description:    "Fully unrestricted routing and traffic forwarding without filters",
			IsInternal:     true,
			Cost:           100,
			RejectInternet: false,
			FilterForward:  false,
			FilterInput:    false,
		},
	}
}

// GetAllPolicies returns the combined list of built-in policies and user-defined policies.
func (c *Config) GetAllPolicies() []NetworkPolicy {
	var netSettings *NetworkSettings
	if c != nil {
		netSettings = &c.NetworkSettings
	}
	builtins := GetBuiltinPolicies(netSettings)
	if c == nil || len(c.NetworkPolicies) == 0 {
		return builtins
	}

	res := make([]NetworkPolicy, 0, len(builtins)+len(c.NetworkPolicies))
	res = append(res, builtins...)
	res = append(res, c.NetworkPolicies...)
	return res
}

// FindPolicy looks up a policy by ID in built-in and user-defined policies.
func (c *Config) FindPolicy(id string) *NetworkPolicy {
	all := c.GetAllPolicies()
	for i := range all {
		if all[i].ID == id {
			return &all[i]
		}
	}
	return nil
}

// EffectivePolicy returns the active policy ID for this LinkEnd.
// If not explicitly configured, external peer endpoints default to "dn42" and internal endpoints to "default".
func (l *LinkEnd) EffectivePolicy(isRemoteExternal bool) string {
	if l != nil && strings.TrimSpace(l.Policy) != "" {
		return strings.TrimSpace(l.Policy)
	}
	if isRemoteExternal {
		return PolicyDN42
	}
	return PolicyDefault
}
