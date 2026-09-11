package compiler

import (
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strings"

	"easy42/internal/config"
	"easy42/util/tplutil"
)

// DefaultNftablesConfigPath is the standard destination path for generated nftables rules on managed devices
const DefaultNftablesConfigPath = "/etc/easy42.nft"

// GetDefaultNftablesTemplate returns the content of the embedded easy42.nft.tmpl template
func GetDefaultNftablesTemplate() (string, error) {
	data, err := templatesFS.ReadFile("templates/easy42.nft.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to read embedded nftables template: %w", err)
	}
	return string(data), nil
}

// FormatNftPrefixList formats a list of IP prefixes into an nftables set elements string like "{ 172.20.0.0/14 }"
// It strips BIRD-specific syntax (e.g. {21,29} or +), deduplicates, and filters out overlapping/redundant subnets
// to prevent nftables from failing with "Error: conflicting intervals specified".
func FormatNftPrefixList(prefixes []string, defaultList []string, isV6 bool) string {
	cleaned := config.CleanPrefixes(prefixes)
	var parsedList []netip.Prefix

	for _, raw := range cleaned {
		pStr := strings.TrimSpace(raw)
		if pStr == "" {
			continue
		}
		// Strip BIRD prefix length constraint braces e.g. {21,29}
		if idx := strings.Index(pStr, "{"); idx != -1 {
			pStr = strings.TrimSpace(pStr[:idx])
		}
		// Strip BIRD trailing '+' or '-'
		pStr = strings.TrimRight(pStr, "+-")

		// If no mask specified, append /32 for v4 or /128 for v6
		if !strings.Contains(pStr, "/") {
			if strings.Contains(pStr, ":") {
				pStr += "/128"
			} else {
				pStr += "/32"
			}
		}

		p, err := netip.ParsePrefix(pStr)
		if err != nil {
			continue
		}

		if isV6 && p.Addr().Is6() {
			parsedList = append(parsedList, p.Masked())
		} else if !isV6 && p.Addr().Is4() {
			parsedList = append(parsedList, p.Masked())
		}
	}

	// Filter out nested subnets to avoid nftables conflicting interval error
	// If prefix b contains prefix a (where b has fewer bits / larger subnet), omit a
	var nonOverlapping []netip.Prefix
	for i, a := range parsedList {
		isCovered := false
		for j, b := range parsedList {
			if i != j {
				// If b has shorter prefix and b contains a
				if b.Bits() < a.Bits() && b.Contains(a.Addr()) {
					isCovered = true
					break
				}
				// If same bits and same prefix, deduplicate by index
				if b.Bits() == a.Bits() && b == a && j < i {
					isCovered = true
					break
				}
			}
		}
		if !isCovered {
			nonOverlapping = append(nonOverlapping, a)
		}
	}

	// Sort prefixes for stable output
	sort.Slice(nonOverlapping, func(i, j int) bool {
		return nonOverlapping[i].String() < nonOverlapping[j].String()
	})

	var result []string
	for _, p := range nonOverlapping {
		result = append(result, p.String())
	}

	if len(result) == 0 {
		result = defaultList
	}
	if len(result) == 0 {
		return ""
	}

	return "{ " + strings.Join(result, ", ") + " }"
}

// BuildNftablesNodeContext prepares the template context specifically for nftables generation
func BuildNftablesNodeContext(
	node *config.Node,
	allNodes []config.Node,
	links []config.Link,
	args ...any,
) (map[string]any, error) {
	ctx, err := BuildNodeContext(node, allNodes, links, args...)
	if err != nil {
		return nil, err
	}

	var settings *config.NetworkSettings
	var customPolicies []config.NetworkPolicy
	for _, arg := range args {
		switch v := arg.(type) {
		case *config.NetworkSettings:
			settings = v
		case config.NetworkSettings:
			settings = &v
		case []config.NetworkPolicy:
			customPolicies = v
		case *config.Config:
			if v != nil {
				settings = &v.NetworkSettings
				customPolicies = v.NetworkPolicies
			}
		}
	}

	var prefixes []string
	if settings != nil {
		prefixes = settings.Prefixes
	}

	ctx["nft_prefixes_v4"] = FormatNftPrefixList(prefixes, []string{"172.20.0.0/14"}, false)
	ctx["nft_prefixes_v6"] = FormatNftPrefixList(prefixes, []string{"fd00::/8"}, true)

	// Collect interfaces per policy
	policyIfnames := make(map[string][]string)

	linksCtx, _ := ctx["links"].([]map[string]any)
	for _, l := range linksCtx {
		local, _ := l["local"].(map[string]any)
		iface, _ := local["interface"].(string)
		polID, _ := l["policy_id"].(string)
		if iface == "" {
			continue
		}
		if (polID == "" || polID == config.PolicyDefault) && l["is_dn42"] == true {
			polID = config.PolicyDN42
		} else if polID == "" {
			polID = config.PolicyDefault
		}
		quoted := `"` + iface + `"`
		if !slices.Contains(policyIfnames[polID], quoted) {
			policyIfnames[polID] = append(policyIfnames[polID], quoted)
		}
	}

	if dn42Ifs, ok := policyIfnames[config.PolicyDN42]; ok && len(dn42Ifs) > 0 {
		ctx["nft_external_ifname"] = "{ " + strings.Join(dn42Ifs, ", ") + " }"
	} else {
		ctx["nft_external_ifname"] = ""
	}

	// Resolve all policies (built-in and custom)
	cfgPolicies := &config.Config{
		NetworkPolicies: customPolicies,
	}
	if settings != nil {
		cfgPolicies.NetworkSettings = *settings
	}
	allPolicies := cfgPolicies.GetAllPolicies()
	policyMap := make(map[string]config.NetworkPolicy)
	for _, p := range allPolicies {
		policyMap[p.ID] = p
	}

	var nftPolicies []map[string]any
	for polID, ifnames := range policyIfnames {
		pol, ok := policyMap[polID]
		if !ok {
			continue
		}
		hasSNAT := pol.SNAT != nil && pol.SNAT.Enabled
		if !pol.FilterForward && !pol.FilterInput && !hasSNAT {
			continue
		}

		cleanID := SanitizeIdentifier(polID)

		dstV4 := ""
		dstV6 := ""
		if len(pol.AllowedDstCIDRs) > 0 {
			dstV4 = FormatNftPrefixList(pol.AllowedDstCIDRs, nil, false)
			dstV6 = FormatNftPrefixList(pol.AllowedDstCIDRs, nil, true)
		}

		srcV4 := ""
		srcV6 := ""
		if len(pol.AllowedSrcCIDRs) > 0 {
			srcV4 = FormatNftPrefixList(pol.AllowedSrcCIDRs, nil, false)
			srcV6 = FormatNftPrefixList(pol.AllowedSrcCIDRs, nil, true)
		}

		localV4 := ""
		localV6 := ""
		if len(pol.LocalNetworks) > 0 {
			localV4 = FormatNftPrefixList(pol.LocalNetworks, nil, false)
			localV6 = FormatNftPrefixList(pol.LocalNetworks, nil, true)
		}

		// Input traffic filtering
		var inputAllTCP, inputAllUDP bool
		var inputTCPPortsStr, inputUDPPortsStr string

		if pol.FilterInput {
			// TCP ports: BGP (179) is always allowed.
			tcpList := config.CleanPortList(pol.InputTCPPorts)
			for _, p := range tcpList {
				if p == "all" || p == "*" {
					inputAllTCP = true
					break
				}
			}
			if !inputAllTCP {
				ports := []string{"179"}
				for _, p := range tcpList {
					if p != "179" && !slices.Contains(ports, p) {
						ports = append(ports, p)
					}
				}
				inputTCPPortsStr = "{ " + strings.Join(ports, ", ") + " }"
			}

			// UDP ports
			udpList := config.CleanPortList(pol.InputUDPPorts)
			for _, p := range udpList {
				if p == "all" || p == "*" {
					inputAllUDP = true
					break
				}
			}
			if !inputAllUDP && len(udpList) > 0 {
				inputUDPPortsStr = "{ " + strings.Join(udpList, ", ") + " }"
			}
		}

		var snatTargetV4, snatTargetV6 string
		snatAction := "snat"
		snatCondition := "not_dst"
		if hasSNAT {
			if pol.SNAT.Condition != "" {
				snatCondition = pol.SNAT.Condition
			}
			if pol.SNAT.Target == "masquerade" {
				snatAction = "masquerade"
			} else {
				switch pol.SNAT.Target {
				case "external_ip", "":
					if strings.TrimSpace(node.ExternalIP) != "" {
						snatTargetV4 = "$external_ip"
					}
					if strings.TrimSpace(node.ExternalIP6) != "" {
						snatTargetV6 = "$external_ip6"
					}
				default:
					if strings.Contains(pol.SNAT.Target, ":") {
						snatTargetV6 = pol.SNAT.Target
					} else {
						snatTargetV4 = pol.SNAT.Target
					}
				}
			}
		}

		nftPolicies = append(nftPolicies, map[string]any{
			"id":                cleanID,
			"name":              pol.Name,
			"ifnames":           "{ " + strings.Join(ifnames, ", ") + " }",
			"allowed_dst_v4":    dstV4,
			"allowed_dst_v6":    dstV6,
			"allowed_src_v4":    srcV4,
			"allowed_src_v6":    srcV6,
			"local_v4":          localV4,
			"local_v6":          localV6,
			"filter_forward":    pol.FilterForward,
			"filter_input":      pol.FilterInput,
			"input_allow_icmp":  pol.InputAllowICMP,
			"input_allow_icmp6": pol.InputAllowICMP6,
			"input_all_tcp":     inputAllTCP,
			"input_tcp_ports":   inputTCPPortsStr,
			"input_all_udp":     inputAllUDP,
			"input_udp_ports":   inputUDPPortsStr,
			"snat":              hasSNAT,
			"snat_action":       snatAction,
			"snat_condition":    snatCondition,
			"snat_target_v4":    snatTargetV4,
			"snat_target_v6":    snatTargetV6,
		})
	}
	sort.Slice(nftPolicies, func(i, j int) bool {
		return nftPolicies[i]["id"].(string) < nftPolicies[j]["id"].(string)
	})
	ctx["nft_policies"] = nftPolicies
	ctx["custom_nft_policies"] = nftPolicies

	return ctx, nil
}

// GenerateNftablesConfigWithTemplate generates nftables config using a custom template
func GenerateNftablesConfigWithTemplate(
	tmplContent string,
	node *config.Node,
	allNodes []config.Node,
	links []config.Link,
	args ...any,
) (string, error) {
	ctx, err := BuildNftablesNodeContext(node, allNodes, links, args...)
	if err != nil {
		return "", err
	}

	return tplutil.RenderTemplate(tmplContent, ctx)
}

// GenerateNftablesConfig compiles the nftables configuration for a node using the default embedded template
func GenerateNftablesConfig(
	node *config.Node,
	allNodes []config.Node,
	links []config.Link,
	args ...any,
) (string, error) {
	tmplContent, err := GetDefaultNftablesTemplate()
	if err != nil {
		return "", err
	}
	return GenerateNftablesConfigWithTemplate(tmplContent, node, allNodes, links, args...)
}
