package compiler

import (
	"fmt"
	"net/netip"
	"slices"
	"sort"
	"strconv"
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

// FormatNftIngressNewWhitelist translates a list of whitelist rules into nftables set elements.
// Each rule can be in format "ip:port" or "ip:port/proto" (e.g. "192.168.1.1:443" or "192.168.1.1:443/tcp").
// If /tcp or /udp is not present, it implies tcp+udp.
func FormatNftIngressNewWhitelist(whitelist []string) string {
	cleaned := config.CleanIngressNewWhitelist(whitelist)
	if len(cleaned) == 0 {
		return ""
	}

	seen := make(map[string]bool)
	var elements []string

	for _, entry := range cleaned {
		proto := ""
		raw := entry
		if idx := strings.LastIndex(raw, "/"); idx != -1 {
			suffix := strings.ToLower(raw[idx+1:])
			if suffix == "tcp" || suffix == "udp" {
				proto = suffix
				raw = raw[:idx]
			}
		}

		colonIdx := strings.LastIndex(raw, ":")
		if colonIdx == -1 {
			continue
		}
		ipPart := strings.TrimSpace(raw[:colonIdx])
		portPart := strings.TrimSpace(raw[colonIdx+1:])
		if ipPart == "" || portPart == "" {
			continue
		}

		var protos []string
		if proto != "" {
			protos = []string{proto}
		} else {
			protos = []string{"tcp", "udp"}
		}

		for _, p := range protos {
			elem := fmt.Sprintf("%s . %s . %s", ipPart, p, portPart)
			if !seen[elem] {
				seen[elem] = true
				elements = append(elements, elem)
			}
		}
	}

	if len(elements) == 0 {
		return ""
	}

	sort.Strings(elements)
	return strings.Join(elements, ",\n      ")
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

	// Collect easy42 interface names: defaults to "wg42*", plus manual link interfaces on this node
	easy42Ifnames := []string{`"wg42*"`}
	for _, l := range links {
		var localEnd *config.LinkEnd
		if l.From.Name == node.Name {
			localEnd = &l.From
		} else if l.To.Name == node.Name {
			localEnd = &l.To
		} else {
			continue
		}
		if (l.IsManual() || localEnd.IsManual()) && strings.TrimSpace(localEnd.Interface) != "" {
			iface := strings.TrimSpace(localEnd.Interface)
			quoted := `"` + iface + `"`
			if !slices.Contains(easy42Ifnames, quoted) {
				easy42Ifnames = append(easy42Ifnames, quoted)
			}
		}
	}
	ctx["easy42_ifname"] = "{ " + strings.Join(easy42Ifnames, ", ") + " }"

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
		hasForwardSNAT := pol.ForwardSNAT
		hasDSCP := pol.EffectiveDSCPIngress() != "" || pol.EffectiveDSCPEgress() != ""
		hasMark := strings.TrimSpace(pol.EffectiveMark()) != ""
		hasBlockIngressNew := pol.EffectiveBlockIngressNew() != ""
		if !pol.FilterForward && !pol.FilterInput && !hasSNAT && !hasForwardSNAT && !hasDSCP && !hasMark && !hasBlockIngressNew {
			continue
		}

		cleanID := SanitizeIdentifier(polID)

		dstV4 := ""
		dstV6 := ""
		if len(pol.AllowedDstCIDRs) > 0 {
			dstV4 = FormatNftPrefixList(pol.AllowedDstCIDRs, nil, false)
			dstV6 = FormatNftPrefixList(pol.AllowedDstCIDRs, nil, true)
		}

		disallowedDstV4 := ""
		disallowedDstV6 := ""
		if len(pol.DisallowedDstCIDRs) > 0 {
			disallowedDstV4 = FormatNftPrefixList(pol.DisallowedDstCIDRs, nil, false)
			disallowedDstV6 = FormatNftPrefixList(pol.DisallowedDstCIDRs, nil, true)
		}

		srcV4 := ""
		srcV6 := ""
		if len(pol.AllowedSrcCIDRs) > 0 {
			srcV4 = FormatNftPrefixList(pol.AllowedSrcCIDRs, nil, false)
			srcV6 = FormatNftPrefixList(pol.AllowedSrcCIDRs, nil, true)
		}

		disallowedSrcV4 := ""
		disallowedSrcV6 := ""
		if len(pol.DisallowedSrcCIDRs) > 0 {
			disallowedSrcV4 = FormatNftPrefixList(pol.DisallowedSrcCIDRs, nil, false)
			disallowedSrcV6 = FormatNftPrefixList(pol.DisallowedSrcCIDRs, nil, true)
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
				case "main_ip":
					if strings.TrimSpace(node.IP) != "" {
						snatTargetV4 = "$self_ip"
					}
					if strings.TrimSpace(node.IP6) != "" {
						snatTargetV6 = "$self_ip6"
					}
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

		var fwdSnatTargetV4, fwdSnatTargetV6 string
		fwdSnatAction := "masquerade"
		if hasForwardSNAT {
			target := strings.TrimSpace(pol.ForwardSNATTarget)
			if target == "" || target == "masquerade" {
				fwdSnatAction = "masquerade"
			} else {
				fwdSnatAction = "snat"
				switch target {
				case "main_ip":
					if strings.TrimSpace(node.IP) != "" {
						fwdSnatTargetV4 = "$self_ip"
					}
					if strings.TrimSpace(node.IP6) != "" {
						fwdSnatTargetV6 = "$self_ip6"
					}
				case "external_ip":
					if strings.TrimSpace(node.ExternalIP) != "" {
						fwdSnatTargetV4 = "$external_ip"
					}
					if strings.TrimSpace(node.ExternalIP6) != "" {
						fwdSnatTargetV6 = "$external_ip6"
					}
				default:
					if strings.Contains(target, ":") {
						fwdSnatTargetV6 = target
					} else {
						fwdSnatTargetV4 = target
					}
				}
			}
		}

		dscpIngressIP, err := FormatNftDSCPAction(pol.DSCPIngress, "ip")
		if err != nil {
			return nil, fmt.Errorf("invalid dscp_ingress %q for policy %s: %w", pol.DSCPIngress, polID, err)
		}
		dscpIngressIP6, err := FormatNftDSCPAction(pol.DSCPIngress, "ip6")
		if err != nil {
			return nil, fmt.Errorf("invalid dscp_ingress %q for policy %s: %w", pol.DSCPIngress, polID, err)
		}
		dscpEgressIP, err := FormatNftDSCPAction(pol.DSCPEgress, "ip")
		if err != nil {
			return nil, fmt.Errorf("invalid dscp_egress %q for policy %s: %w", pol.DSCPEgress, polID, err)
		}
		dscpEgressIP6, err := FormatNftDSCPAction(pol.DSCPEgress, "ip6")
		if err != nil {
			return nil, fmt.Errorf("invalid dscp_egress %q for policy %s: %w", pol.DSCPEgress, polID, err)
		}

		var ingressNewWlStr string
		if hasBlockIngressNew && len(pol.IngressNewWhitelist) > 0 {
			ingressNewWlStr = FormatNftIngressNewWhitelist(pol.IngressNewWhitelist)
		}

		nftPolicies = append(nftPolicies, map[string]any{
			"id":                     cleanID,
			"name":                   pol.Name,
			"ifnames":                "{ " + strings.Join(ifnames, ", ") + " }",
			"allowed_dst_v4":         dstV4,
			"allowed_dst_v6":         dstV6,
			"disallowed_dst_v4":      disallowedDstV4,
			"disallowed_dst_v6":      disallowedDstV6,
			"allowed_src_v4":         srcV4,
			"allowed_src_v6":         srcV6,
			"disallowed_src_v4":      disallowedSrcV4,
			"disallowed_src_v6":      disallowedSrcV6,
			"local_v4":               localV4,
			"local_v6":               localV6,
			"filter_forward":         pol.FilterForward,
			"filter_input":           pol.FilterInput,
			"input_allow_icmp":       pol.InputAllowICMP,
			"input_allow_icmp6":      pol.InputAllowICMP6,
			"input_all_tcp":          inputAllTCP,
			"input_tcp_ports":        inputTCPPortsStr,
			"input_all_udp":          inputAllUDP,
			"input_udp_ports":        inputUDPPortsStr,
			"snat":                   hasSNAT,
			"snat_action":            snatAction,
			"snat_condition":         snatCondition,
			"snat_target_v4":         snatTargetV4,
			"snat_target_v6":         snatTargetV6,
			"forward_snat":           hasForwardSNAT,
			"forward_snat_action":    fwdSnatAction,
			"forward_snat_target_v4": fwdSnatTargetV4,
			"forward_snat_target_v6": fwdSnatTargetV6,
			"dscp_ingress":           dscpIngressIP,
			"dscp_ingress_ip6":       dscpIngressIP6,
			"dscp_egress":            dscpEgressIP,
			"dscp_egress_ip6":        dscpEgressIP6,
			"mark":                   strings.TrimSpace(pol.EffectiveMark()),
			"block_ingress_new":      pol.EffectiveBlockIngressNew(),
			"ingress_new_wl":         ingressNewWlStr,
		})
	}
	sort.Slice(nftPolicies, func(i, j int) bool {
		return nftPolicies[i]["id"].(string) < nftPolicies[j]["id"].(string)
	})
	ctx["nft_policies"] = nftPolicies
	ctx["custom_nft_policies"] = nftPolicies

	// Collect interfaces per mark (from effective marks on local link ends)
	markIfnames := make(map[string][]string)
	for _, l := range linksCtx {
		local, _ := l["local"].(map[string]any)
		iface, _ := local["interface"].(string)
		mark, _ := local["mark"].(string)
		mark = strings.TrimSpace(mark)
		if iface == "" || mark == "" {
			continue
		}
		formattedMark, err := FormatNftMarkAction(mark)
		if err != nil {
			return nil, fmt.Errorf("invalid mark format %q for interface %s: %w", mark, iface, err)
		}
		quoted := `"` + iface + `"`
		if !slices.Contains(markIfnames[formattedMark], quoted) {
			markIfnames[formattedMark] = append(markIfnames[formattedMark], quoted)
		}
	}

	var nftMarks []map[string]any
	var markKeys []string
	for k := range markIfnames {
		markKeys = append(markKeys, k)
	}
	sort.Strings(markKeys)

	for _, m := range markKeys {
		ifs := markIfnames[m]
		sort.Strings(ifs)
		var ifnamesStr string
		if len(ifs) == 1 {
			ifnamesStr = ifs[0]
		} else {
			ifnamesStr = "{ " + strings.Join(ifs, ", ") + " }"
		}
		nftMarks = append(nftMarks, map[string]any{
			"mark":    m,
			"ifnames": ifnamesStr,
		})
	}
	ctx["nft_marks"] = nftMarks

	var nftHooksPre []string
	var nftHooksTable []string
	var nftHooksPost []string
	for _, h := range node.ConfigHooks {
		content := strings.TrimSpace(h.Content)
		if content == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(h.Type)) {
		case "nft.pre":
			nftHooksPre = append(nftHooksPre, addHookTemplate(ctx, "nft_pre", content))
		case "nft.table":
			nftHooksTable = append(nftHooksTable, addHookTemplate(ctx, "nft_table", content))
		case "nft", "nft.post", "nft.global":
			nftHooksPost = append(nftHooksPost, addHookTemplate(ctx, "nft_post", content))
		}
	}
	if len(nftHooksPre) > 0 {
		ctx["nft_hooks_pre"] = nftHooksPre
	}
	if len(nftHooksTable) > 0 {
		ctx["nft_hooks_table"] = nftHooksTable
	}
	if len(nftHooksPost) > 0 {
		ctx["nft_hooks"] = nftHooksPost
	}

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

	return tplutil.RenderTemplateWithHooks(tmplContent, ctx, popHookTemplates(ctx))
}

// GenerateNftablesConfig compiles the nftables configuration for a node using the custom template or default embedded template
func GenerateNftablesConfig(
	node *config.Node,
	allNodes []config.Node,
	links []config.Link,
	args ...any,
) (string, error) {
	var customTemplates []config.ConfigTemplate
	for _, arg := range args {
		switch v := arg.(type) {
		case []config.ConfigTemplate:
			customTemplates = v
		case *config.Config:
			if v != nil {
				customTemplates = v.Templates
			}
		case config.Config:
			customTemplates = v.Templates
		}
	}

	var tmplContent string
	var err error
	if node != nil && strings.TrimSpace(node.NftTemplate) != "" {
		tmplContent, err = FindTemplate(config.TemplateTypeNft, node.NftTemplate, customTemplates)
	} else {
		tmplContent, err = GetDefaultNftablesTemplate()
	}
	if err != nil {
		return "", err
	}
	return GenerateNftablesConfigWithTemplate(tmplContent, node, allNodes, links, args...)
}

// FormatNftMarkAction formats a netfilter mark string (which can be either a plain "mark"
// or a "mark/bitmask" format) into the nftables expression to be set in:
//   meta mark set <expression>
//
// Formats:
// 1. Plain mark (e.g. "0x1234", "42", "0x00000080"):
//    Returns the mark unchanged.
// 2. "mark/bitmask" where mark == bitmask (e.g. "0x80/0x80"):
//    Sets the specified bits via bitwise OR:
//    Returns "meta mark or 0x80".
// 3. "mark/bitmask" where mark != bitmask (e.g. "0x40/0xc0" or "0/0x80"):
//    Preserves unmasked bits via AND with ~bitmask, and sets masked bits via OR:
//    Returns "meta mark and 0xffffff3f or 0x40".
func FormatNftMarkAction(rawMark string) (string, error) {
	s := strings.TrimSpace(rawMark)
	if s == "" {
		return "", nil
	}
	if !strings.Contains(s, "/") {
		return s, nil
	}

	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid mark format %q: expected mark or mark/bitmask", rawMark)
	}

	markVal, err := parseUint32(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid mark part %q in %q: %w", parts[0], rawMark, err)
	}

	maskVal, err := parseUint32(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid bitmask part %q in %q: %w", parts[1], rawMark, err)
	}

	if maskVal == 0 {
		return "", fmt.Errorf("invalid bitmask in %q: mask cannot be 0", rawMark)
	}

	if markVal == maskVal {
		return fmt.Sprintf("meta mark or 0x%x", markVal), nil
	}

	andMask := (^maskVal) & 0xffffffff
	orVal := markVal & maskVal
	return fmt.Sprintf("meta mark and 0x%x or 0x%x", andMask, orVal), nil
}

// FormatNftDSCPAction formats a DSCP string (which can be either a plain "value"
// or a "value/mask" format) into the nftables expression to be set in:
//
//	<family> dscp set <expression>
//
// family is "ip" (IPv4) or "ip6" (IPv6). DSCP is a 6-bit field, so both the
// value and the mask must be in the range 0-63 and the mask must be non-zero.
//
// Formats:
//  1. Plain value (e.g. "46", "0x2e", "0"):
//     Returns the value unchanged, so e.g. "ip dscp set 46".
//  2. "value/mask" where value == mask (e.g. "0x20/0x20"):
//     Sets the specified bits via bitwise OR:
//     Returns "ip dscp | 0x20" (i.e. "ip dscp set ip dscp | 0x20").
//  3. "value/mask" where value != mask (e.g. "0x10/0x30" or "0/0x20"):
//     Preserves unmasked bits via AND with ~mask (within the 6-bit DSCP range)
//     and sets masked bits via OR:
//     Returns "ip dscp & 0xf | 0x10" (i.e. "ip dscp set ip dscp & 0xf | 0x10").
func FormatNftDSCPAction(rawDSCP string, family string) (string, error) {
	s := strings.TrimSpace(rawDSCP)
	if s == "" {
		return "", nil
	}
	if family != "ip" && family != "ip6" {
		return "", fmt.Errorf("invalid DSCP family %q: expected ip or ip6", family)
	}
	if !strings.Contains(s, "/") {
		val, err := parseUint32(s)
		if err != nil {
			return "", fmt.Errorf("invalid DSCP value %q: %w", s, err)
		}
		if val > 63 {
			return "", fmt.Errorf("invalid DSCP value %q: value must be 0-63", s)
		}
		return s, nil
	}

	parts := strings.Split(s, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid DSCP format %q: expected value or value/mask", rawDSCP)
	}

	val, err := parseUint32(parts[0])
	if err != nil {
		return "", fmt.Errorf("invalid DSCP value part %q in %q: %w", parts[0], rawDSCP, err)
	}

	mask, err := parseUint32(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid DSCP mask part %q in %q: %w", parts[1], rawDSCP, err)
	}

	if mask == 0 {
		return "", fmt.Errorf("invalid mask in %q: mask cannot be 0", rawDSCP)
	}
	if val > 63 {
		return "", fmt.Errorf("invalid DSCP value %q: value must be 0-63", rawDSCP)
	}
	if mask > 63 {
		return "", fmt.Errorf("invalid DSCP mask %q: mask must be 0-63", rawDSCP)
	}

	if val == mask {
		return fmt.Sprintf("%s dscp | 0x%x", family, val), nil
	}

	andMask := (^mask) & 0x3f
	orVal := val & mask
	return fmt.Sprintf("%s dscp & 0x%x | 0x%x", family, andMask, orVal), nil
}

func parseUint32(s string) (uint32, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty number")
	}
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		v, err := strconv.ParseUint(s[2:], 16, 32)
		return uint32(v), err
	}
	if v, err := strconv.ParseUint(s, 10, 32); err == nil {
		return uint32(v), nil
	}
	v, err := strconv.ParseUint(s, 0, 32)
	return uint32(v), err
}

