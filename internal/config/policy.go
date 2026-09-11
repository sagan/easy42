package config

import (
	"fmt"
	"strconv"
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
	LocalNetworks      []string    `json:"local_networks,omitempty"`
	AllowedDstCIDRs    []string    `json:"allowed_dst_cidrs,omitempty"`
	AllowedSrcCIDRs    []string    `json:"allowed_src_cidrs,omitempty"`
	AllowedImportCIDRs []string    `json:"allowed_import_cidrs,omitempty"`
	RejectInternet     bool        `json:"reject_internet"`
	FilterForward      bool        `json:"filter_forward"`
	FilterInput        bool        `json:"filter_input"`
	InputAllowICMP     bool        `json:"input_allow_icmp"`
	InputAllowICMP6    bool        `json:"input_allow_icmp6"`
	InputTCPPorts      []string    `json:"input_tcp_ports,omitempty"`
	InputUDPPorts      []string    `json:"input_udp_ports,omitempty"`
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

// CleanPortList normalizes and validates a list of ports and port ranges.
// It accepts single ports (e.g. "53"), port ranges (e.g. "8000-8080"),
// or wildcard "all" / "*". It splits by commas, whitespace, and newlines,
// validates numeric ranges (1-65535), and deduplicates entries.
func CleanPortList(ports []string) []string {
	if len(ports) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []string

	for _, entry := range ports {
		for _, raw := range strings.FieldsFunc(entry, func(r rune) bool {
			return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
		}) {
			p := strings.TrimSpace(raw)
			if p == "" {
				continue
			}

			// Wildcards
			if strings.EqualFold(p, "all") || p == "*" {
				if !seen["all"] {
					seen["all"] = true
					result = append(result, "all")
				}
				continue
			}

			// Check if port range: low-high
			if parts := strings.Split(p, "-"); len(parts) == 2 {
				low, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
				high, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err1 == nil && err2 == nil && low > 0 && high <= 65535 && low <= high {
					normalized := fmt.Sprintf("%d-%d", low, high)
					if !seen[normalized] {
						seen[normalized] = true
						result = append(result, normalized)
					}
				}
				continue
			}

			// Single port
			if num, err := strconv.Atoi(p); err == nil && num > 0 && num <= 65535 {
				normalized := strconv.Itoa(num)
				if !seen[normalized] {
					seen[normalized] = true
					result = append(result, normalized)
				}
			}
		}
	}

	return result
}

// GetBuiltinPolicies returns the immutable baseline policies always present in easy42.
// For dn42, default CIDRs are populated from global NetworkSettings if provided.
func GetBuiltinPolicies(netSettings *NetworkSettings) []NetworkPolicy {
	var dn42Prefixes []string
	if netSettings != nil && len(netSettings.Prefixes) > 0 {
		dn42Prefixes = CleanPrefixes(netSettings.Prefixes)
	} else {
		dn42Prefixes = []string{"172.20.0.0/14{21,29}", "fd00::/8{44,64}"}
	}

	var localDN42 []string
	if netSettings != nil && len(netSettings.LocalDN42Networks) > 0 {
		localDN42 = CleanPrefixes(netSettings.LocalDN42Networks)
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
			LocalNetworks:      localDN42,
			AllowedDstCIDRs:    dn42Prefixes,
			AllowedSrcCIDRs:    dn42Prefixes,
			AllowedImportCIDRs: dn42Prefixes,
			RejectInternet:     true,
			FilterForward:      true,
			FilterInput:        true,
			InputAllowICMP:     true,
			InputAllowICMP6:    true,
			InputUDPPorts:      []string{"53"},
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

// EffectiveCost returns the active cost for this LinkEnd.
// If Cost is set (not zero), it overrides the policy cost.
// Otherwise, it falls back to the provided policy cost (or 100 if unset/non-positive).
func (l *LinkEnd) EffectiveCost(policyCost int) int {
	if l != nil && l.Cost != 0 {
		return l.Cost
	}
	if policyCost > 0 {
		return policyCost
	}
	return 100
}

// EffectiveCostWithPolicy returns the active cost for this LinkEnd using the provided policy.
// If Cost is set (not zero), it overrides the policy cost.
// Otherwise, it falls back to the policy's effective cost.
func (l *LinkEnd) EffectiveCostWithPolicy(p *NetworkPolicy) int {
	if l != nil && l.Cost != 0 {
		return l.Cost
	}
	if p != nil {
		return p.EffectiveCost()
	}
	return 100
}

