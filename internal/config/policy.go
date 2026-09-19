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

const (
	RoutingPolicyFull          = "full"
	RoutingPolicyStub          = "stub"
	RoutingPolicyReceiveOnly   = "receive_only"
	RoutingPolicyAdvertiseOnly = "advertise_only"
)

const (
	BlockIngressNewDisabled = ""
	BlockIngressNewAll      = "all"
	BlockIngressNewForward  = "forward"
)

// NormalizeBlockIngressNew sanitizes and normalizes block ingress new connection options.
// Defaults to BlockIngressNewDisabled ("") if empty, disabled, or unrecognized.
func NormalizeBlockIngressNew(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "all", "all_ingress", "all-ingress", "ingress":
		return BlockIngressNewAll
	case "forward", "forwarding", "forward_only", "forward-only":
		return BlockIngressNewForward
	case "", "none", "disabled", "off", "false":
		return BlockIngressNewDisabled
	default:
		return BlockIngressNewDisabled
	}
}

// IsValidBlockIngressNew returns true if p is a recognized option or alias.
func IsValidBlockIngressNew(p string) bool {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "", "disabled", "none", "off", "false",
		BlockIngressNewAll, "all_ingress", "all-ingress", "ingress",
		BlockIngressNewForward, "forwarding", "forward_only", "forward-only":
		return true
	default:
		return false
	}
}

// NormalizeRoutingPolicy sanitizes and normalizes routing policy strings.
// Defaults to RoutingPolicyFull ("full") if empty or unknown.
func NormalizeRoutingPolicy(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "stub", "originate_only", "originate-only", "local_only", "local-only", "transit_client", "transit-client":
		return RoutingPolicyStub
	case "receive_only", "receive-only", "import_only", "import-only":
		return RoutingPolicyReceiveOnly
	case "advertise_only", "advertise-only", "export_only", "export-only":
		return RoutingPolicyAdvertiseOnly
	case "full", "transit", "mesh", "":
		return RoutingPolicyFull
	default:
		return RoutingPolicyFull
	}
}

// IsValidRoutingPolicy returns true if p is a recognized routing policy or alias.
func IsValidRoutingPolicy(p string) bool {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case RoutingPolicyFull, RoutingPolicyStub, RoutingPolicyReceiveOnly, RoutingPolicyAdvertiseOnly,
		"originate_only", "originate-only", "local_only", "local-only", "transit_client", "transit-client",
		"import_only", "import-only", "export_only", "export-only", "transit", "mesh":
		return true
	default:
		return false
	}
}

// SNATConfig represents source network address translation options for egress traffic
type SNATConfig struct {
	Enabled   bool   `json:"enabled"`
	Condition string `json:"condition,omitempty"` // "not_dst" (saddr != allowed dst CIDRs) | "all"
	Target    string `json:"target,omitempty"`    // "external_ip" | "masquerade" | "main_ip" | custom IP string
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
	DisallowedDstCIDRs []string    `json:"disallowed_dst_cidrs,omitempty"`
	DisallowedSrcCIDRs []string    `json:"disallowed_src_cidrs,omitempty"`
	RejectInternet     bool        `json:"reject_internet"`
	FilterForward      bool        `json:"filter_forward"`
	FilterInput        bool        `json:"filter_input"`
	InputAllowICMP     bool        `json:"input_allow_icmp"`
	InputAllowICMP6    bool        `json:"input_allow_icmp6"`
	InputTCPPorts      []string    `json:"input_tcp_ports,omitempty"`
	InputUDPPorts      []string    `json:"input_udp_ports,omitempty"`
	SNAT               *SNATConfig `json:"snat,omitempty"`
	ForwardSNAT        bool        `json:"forward_snat,omitempty"`
	ForwardSNATTarget  string      `json:"forward_snat_target,omitempty"` // "masquerade" | "external_ip" | "main_ip" | custom IP string
	DSCPIngress        *int        `json:"dscp_ingress,omitempty"`        // Rewrite DSCP on packets received from the link peer (0-63)
	DSCPEgress         *int        `json:"dscp_egress,omitempty"`         // Rewrite DSCP on packets sent to the link peer (0-63)
	BlockIngressNew    string      `json:"block_ingress_new,omitempty"`   // Block ingress new/invalid connections: "" (disabled) | "all" | "forward"
	ROA4               string      `json:"roa4,omitempty"`
	ROA6               string      `json:"roa6,omitempty"`
	ROAStrict          bool        `json:"roa_strict,omitempty"`
	Fwmark             string      `json:"fwmark,omitempty"`         // Local WireGuard [Interface] FwMark
	Preference         *int        `json:"preference,omitempty"`     // BIRD peer BGP protocol preference
	Mark               string      `json:"mark,omitempty"`           // Netfilter mark for received packets from the link peer
	RoutingPolicy      string      `json:"routing_policy,omitempty"` // BGP meta routing policy ("full", "stub", "receive_only", "advertise_only")
}

// EffectiveBlockIngressNew returns the active block ingress new option or "" if unset/disabled
func (p *NetworkPolicy) EffectiveBlockIngressNew() string {
	if p != nil {
		return NormalizeBlockIngressNew(p.BlockIngressNew)
	}
	return BlockIngressNewDisabled
}

// EffectiveRoutingPolicy returns the active routing policy or "full" if unset
func (p *NetworkPolicy) EffectiveRoutingPolicy() string {
	if p != nil && strings.TrimSpace(p.RoutingPolicy) != "" {
		return NormalizeRoutingPolicy(p.RoutingPolicy)
	}
	return RoutingPolicyFull
}

// EffectiveFwmark returns the configured fwmark or empty string if unset
func (p *NetworkPolicy) EffectiveFwmark() string {
	if p != nil {
		return strings.TrimSpace(p.Fwmark)
	}
	return ""
}

// EffectiveMark returns the configured mark or empty string if unset
func (p *NetworkPolicy) EffectiveMark() string {
	if p != nil {
		return strings.TrimSpace(p.Mark)
	}
	return ""
}

// EffectivePreference returns the configured preference or nil if unset
func (p *NetworkPolicy) EffectivePreference() *int {
	if p != nil {
		return p.Preference
	}
	return nil
}

// EffectiveCost returns the configured cost or default 100 if unset/non-positive
func (p *NetworkPolicy) EffectiveCost() int {
	if p != nil && p.Cost > 0 {
		return p.Cost
	}
	return 100
}

// ValidateDSCP returns a sanitized DSCP pointer: nil if the input is nil,
// otherwise clamped to the valid range 0-63.
func ValidateDSCP(v *int) *int {
	if v == nil {
		return nil
	}
	d := *v
	if d < 0 {
		d = 0
	} else if d > 63 {
		d = 63
	}
	return &d
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
		dn42Prefixes = []string{
			"172.20.0.0/14{21,29}", // dn42
			"172.20.0.0/24{28,32}", // dn42 Anycast
			"172.21.0.0/24{28,32}", // dn42 Anycast
			"172.22.0.0/24{28,32}", // dn42 Anycast
			"172.23.0.0/24{28,32}", // dn42 Anycast
			"172.31.0.0/16+",       // ChaosVPN
			"10.100.0.0/14+",       // ChaosVPN
			"10.0.0.0/8{15,24}",    // Freifunk.net
			"10.127.0.0/16+",       // NeoNetwork
			"fd00::/8{44,64}",      // DN42 ipv6
		}
	}

	var localDN42 []string
	if netSettings != nil && len(netSettings.LocalDN42Networks) > 0 {
		localDN42 = CleanPrefixes(netSettings.LocalDN42Networks)
	}

	var disallowedDN42 []string
	if netSettings != nil {
		if len(netSettings.DisallowedDN42Networks) > 0 {
			disallowedDN42 = CleanPrefixes(netSettings.DisallowedDN42Networks)
		} else if len(netSettings.DisallowedDN42CIDRs) > 0 {
			disallowedDN42 = CleanPrefixes(netSettings.DisallowedDN42CIDRs)
		}
	}

	return []NetworkPolicy{
		{
			ID:             PolicyDefault,
			Name:           "Default (Internal)",
			Description:    "Standard internal mesh policy with Internet route leak protection",
			IsInternal:     true,
			Cost:           100,
			RoutingPolicy:  RoutingPolicyFull,
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
			RoutingPolicy:      RoutingPolicyFull,
			LocalNetworks:      localDN42,
			AllowedDstCIDRs:    dn42Prefixes,
			AllowedSrcCIDRs:    dn42Prefixes,
			DisallowedDstCIDRs: disallowedDN42,
			DisallowedSrcCIDRs: disallowedDN42,
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
			RoutingPolicy:  RoutingPolicyFull,
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
// Custom policies take precedence over built-in policies with the same ID.
func (c *Config) FindPolicy(id string) *NetworkPolicy {
	all := c.GetAllPolicies()
	for i := len(all) - 1; i >= 0; i-- {
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

// EffectiveFwmark returns the active fwmark for this LinkEnd using the provided policy.
// If LinkEnd's Fwmark is defined, it overrides the policy's fwmark.
// Otherwise, it falls back to the policy's fwmark.
func (l *LinkEnd) EffectiveFwmark(p *NetworkPolicy) string {
	if l != nil && strings.TrimSpace(l.Fwmark) != "" {
		return strings.TrimSpace(l.Fwmark)
	}
	if p != nil {
		return strings.TrimSpace(p.Fwmark)
	}
	return ""
}

// EffectivePreference returns the active preference for this LinkEnd using the provided policy.
// If LinkEnd's Preference is defined, it overrides the policy's preference.
// Otherwise, it falls back to the policy's preference.
func (l *LinkEnd) EffectivePreference(p *NetworkPolicy) *int {
	if l != nil && l.Preference != nil {
		return l.Preference
	}
	if p != nil && p.Preference != nil {
		return p.Preference
	}
	return nil
}

// EffectiveMark returns the active mark for this LinkEnd using the provided policy.
// If LinkEnd's Mark is defined, it overrides the policy's mark.
// Otherwise, it falls back to the policy's mark.
func (l *LinkEnd) EffectiveMark(p *NetworkPolicy) string {
	if l != nil && strings.TrimSpace(l.Mark) != "" {
		return strings.TrimSpace(l.Mark)
	}
	if p != nil {
		return strings.TrimSpace(p.Mark)
	}
	return ""
}

// EffectiveRoutingPolicy returns the active routing policy for this LinkEnd using the provided policy.
// If LinkEnd's RoutingPolicy is defined, it overrides the policy's routing policy.
// Otherwise, it falls back to the policy's routing policy.
func (l *LinkEnd) EffectiveRoutingPolicy(p *NetworkPolicy) string {
	if l != nil && strings.TrimSpace(l.RoutingPolicy) != "" {
		return NormalizeRoutingPolicy(l.RoutingPolicy)
	}
	if p != nil {
		return p.EffectiveRoutingPolicy()
	}
	return RoutingPolicyFull
}
