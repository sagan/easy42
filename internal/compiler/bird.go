package compiler

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"easy42/internal/config"
	"easy42/util/tplutil"
)

// DefaultBirdConfigPath is the standard destination path for generated BIRD config on managed devices
const DefaultBirdConfigPath = "/etc/bird_easy42.conf"

//go:embed templates/*
var templatesFS embed.FS

// GetDefaultBirdTemplate returns the content of the embedded easy42_bird.conf.tmpl template
func GetDefaultBirdTemplate() (string, error) {
	data, err := templatesFS.ReadFile("templates/easy42_bird.conf.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to read embedded bird template: %w", err)
	}
	return string(data), nil
}

// BuildNodeContext converts a Node and its connected links into a context map suitable for template execution.
// The resulting context has the structure:
//
//	{
//	  "name": "node1",
//	  "ip": "192.168.100.1",
//	  "asn": 4224420001,
//	  "table": 254,
//	  "static_routes": [...],
//	  "static_routes_v4": [...],
//	  "static_routes_v6": [...],
//	  "routes": [...],
//	  "links": [
//	    {
//	      "tags": [...],
//	      "local": { "name": ..., "interface": ..., "address": ... },
//	      "remote": { "name": ..., "interface": ..., "address": ... },
//	      "remote_node": { "name": ..., "asn": ..., "ip": ... }
//	    }
//	  ]
//
// BuildNodeContext converts a Node and its connected links into a context map suitable for template execution.
func BuildNodeContext(node *config.Node, allNodes []config.Node, links []config.Link, args ...any) (map[string]any, error) {
	if node == nil {
		return nil, fmt.Errorf("node cannot be nil")
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

	// 1. Convert node struct to map[string]any via JSON serialization to preserve json tag naming
	data, err := json.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize node %s: %w", node.Name, err)
	}

	var ctx map[string]any
	if err := json.Unmarshal(data, &ctx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal node %s into context: %w", node.Name, err)
	}

	// 2. Ensure integer types for ASN and table (avoid json float64 exponential formatting like 4.29942e+09)
	table := node.Table
	if table <= 0 {
		table = 254
	}
	ctx["table"] = table
	ctx["routing_table"] = table
	ctx["asn"] = node.ASN
	extTable := node.ExternalTable
	useExtTable := extTable > 0 && extTable != table
	ctx["external_table"] = extTable
	ctx["use_external_table"] = useExtTable

	inetTable := node.InternetTable
	useInetTable := inetTable > 0 && inetTable != table
	ctx["internet_table"] = inetTable
	ctx["use_internet_table"] = useInetTable
	ctx["ip"] = node.IP

	ip6 := strings.TrimSpace(node.IP6)
	if idx := strings.Index(ip6, "/"); idx != -1 {
		ip6 = strings.TrimSpace(ip6[:idx])
	}
	if ip6 != "" {
		ctx["ip6"] = ip6
	} else {
		delete(ctx, "ip6")
	}

	extIP := strings.TrimSpace(node.ExternalIP)
	if extIP != "" {
		ctx["external_ip"] = extIP
	} else {
		delete(ctx, "external_ip")
	}

	extIP6 := strings.TrimSpace(node.ExternalIP6)
	if extIP6 != "" {
		ctx["external_ip6"] = extIP6
	} else {
		delete(ctx, "external_ip6")
	}

	// NetworkSettings / BGP Confederation / Community support
	var commExternal string
	if settings != nil && settings.PublicASN > 0 {
		ctx["confed_as"] = settings.PublicASN
		commExternal = fmt.Sprintf("(%d, 1, 1)", settings.PublicASN)
	} else {
		commExternal = "(4224420000, 1, 1)"
	}
	ctx["comm_external"] = commExternal

	var prefixes []string
	if settings != nil {
		prefixes = settings.Prefixes
	}
	extV4 := formatPrefixList(prefixes, []string{"172.20.0.0/14{21,29}"}, false)
	extV6 := formatPrefixList(prefixes, []string{"fd00::/8{44,64}"}, true)
	ctx["ext_prefixes_v4"] = extV4
	ctx["ext_prefixes_v6"] = extV6

	// 3. Normalize routes and static routes
	routes := node.Routes
	var routeMaps []map[string]any
	for _, r := range routes {
		routeMaps = append(routeMaps, map[string]any{
			"table":    r.Table,
			"prefixes": r.Prefixes,
		})
	}
	ctx["routes"] = routeMaps
	ctx["kernel_routes"] = routeMaps

	var staticV4 []string
	var staticV6 []string
	for _, sr := range node.StaticRoutes {
		trimmed := strings.TrimSpace(sr)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, ":") {
			staticV6 = append(staticV6, trimmed)
		} else {
			staticV4 = append(staticV4, trimmed)
		}
	}
	if staticV4 == nil {
		staticV4 = []string{}
	}
	if staticV6 == nil {
		staticV6 = []string{}
	}

	if node.StaticRoutes == nil {
		ctx["static_routes"] = []string{}
	} else {
		ctx["static_routes"] = node.StaticRoutes
	}
	ctx["static_routes_v4"] = staticV4
	ctx["static_routes_v6"] = staticV6

	// 4. Index allNodes by name for fast lookup
	nodeByName := make(map[string]*config.Node)
	for i := range allNodes {
		nodeByName[allNodes[i].Name] = &allNodes[i]
	}

	// 5. Build links for this node
	type rawLinkInfo struct {
		link             config.Link
		localEnd         *config.LinkEnd
		remoteEnd        *config.LinkEnd
		remoteNode       *config.Node
		policyID         string
		pol              config.NetworkPolicy
		isRemoteExternal bool
		isExternal       bool
		isDN42           bool
		linkCost         int
	}

	var rawLinks []rawLinkInfo
	hasExternalLinks := false
	hasNonePolicy := false
	usedPolicyMap := make(map[string]config.NetworkPolicy)
	policyCostsMap := make(map[string]map[int]bool)

	for _, l := range links {
		var localEnd, remoteEnd *config.LinkEnd
		var remoteNode *config.Node

		if l.From.Name == node.Name {
			localEnd = &l.From
			remoteEnd = &l.To
			remoteNode = nodeByName[l.To.Name]
		} else if l.To.Name == node.Name {
			localEnd = &l.To
			remoteEnd = &l.From
			remoteNode = nodeByName[l.From.Name]
		} else {
			continue
		}

		isRemoteExternal := remoteNode != nil && remoteNode.IsExternal
		policyID := localEnd.EffectivePolicy(isRemoteExternal)
		pol, ok := policyMap[policyID]
		if !ok {
			if isRemoteExternal {
				policyID = config.PolicyDN42
				pol = policyMap[config.PolicyDN42]
			} else {
				policyID = config.PolicyDefault
				pol = policyMap[config.PolicyDefault]
			}
		}
		usedPolicyMap[policyID] = pol

		isExternal := false
		if isRemoteExternal || node.IsExternal || policyID == config.PolicyDN42 {
			isExternal = true
			hasExternalLinks = true
		}

		isDN42 := (policyID == config.PolicyDN42)
		if policyID == config.PolicyNone {
			hasNonePolicy = true
		}

		linkCost := localEnd.EffectiveCostWithPolicy(&pol)
		if policyCostsMap[policyID] == nil {
			policyCostsMap[policyID] = make(map[int]bool)
		}
		policyCostsMap[policyID][linkCost] = true

		rawLinks = append(rawLinks, rawLinkInfo{
			link:             l,
			localEnd:         localEnd,
			remoteEnd:        remoteEnd,
			remoteNode:       remoteNode,
			policyID:         policyID,
			pol:              pol,
			isRemoteExternal: isRemoteExternal,
			isExternal:       isExternal,
			isDN42:           isDN42,
			linkCost:         linkCost,
		})
	}

	defaultCost := 100
	if p, ok := policyMap[config.PolicyDefault]; ok {
		defaultCost = p.EffectiveCost()
	}
	if costs, ok := policyCostsMap[config.PolicyDefault]; ok && len(costs) == 1 {
		for c := range costs {
			defaultCost = c
		}
	}

	noneCost := 100
	if p, ok := policyMap[config.PolicyNone]; ok {
		noneCost = p.EffectiveCost()
	}
	if costs, ok := policyCostsMap[config.PolicyNone]; ok && len(costs) == 1 {
		for c := range costs {
			noneCost = c
		}
	}

	customBaseCosts := make(map[string]int)
	for id, pol := range usedPolicyMap {
		base := pol.EffectiveCost()
		if costs, ok := policyCostsMap[id]; ok && len(costs) == 1 {
			for c := range costs {
				base = c
			}
		}
		customBaseCosts[id] = base
	}

	var nodeLinks []map[string]any
	for _, rl := range rawLinks {
		bgpTemplate := "easy42_peer"
		localAS := "SELF_AS"

		switch rl.policyID {
		case config.PolicyDefault:
			if rl.linkCost == defaultCost {
				bgpTemplate = "easy42_peer"
			} else {
				bgpTemplate = fmt.Sprintf("easy42_peer_cost_%d", rl.linkCost)
			}
		case config.PolicyDN42:
			bgpTemplate = "external_peer"
			if settings != nil && settings.PublicASN > 0 {
				localAS = "CONFED_AS"
			}
		case config.PolicyNone:
			if rl.linkCost == noneCost {
				bgpTemplate = "none_peer"
			} else {
				bgpTemplate = fmt.Sprintf("none_peer_cost_%d", rl.linkCost)
			}
		default:
			cleanID := SanitizeIdentifier(rl.policyID)
			baseCost := customBaseCosts[rl.policyID]
			if rl.linkCost == baseCost {
				bgpTemplate = "pol_peer_" + cleanID
			} else {
				bgpTemplate = fmt.Sprintf("pol_peer_%s_cost_%d", cleanID, rl.linkCost)
			}
		}

		localMap := linkEndToContextMap(rl.localEnd, node, rl.remoteEnd.Name, rl.isRemoteExternal)
		localMap["policy"] = rl.policyID
		localMap["cost"] = rl.linkCost
		remoteMap := linkEndToContextMap(rl.remoteEnd, rl.remoteNode, rl.localEnd.Name, node.IsExternal)

		var remoteNodeMap map[string]any
		if rl.remoteNode != nil {
			nodeData, _ := json.Marshal(rl.remoteNode)
			_ = json.Unmarshal(nodeData, &remoteNodeMap)
			// Ensure table defaults for remote_node as well
			rTable := rl.remoteNode.Table
			if rTable <= 0 {
				rTable = 254
			}
			remoteNodeMap["table"] = rTable
			remoteNodeMap["routing_table"] = rTable
			remoteNodeMap["external_table"] = rl.remoteNode.ExternalTable
			remoteNodeMap["use_external_table"] = rl.remoteNode.ExternalTable > 0 && rl.remoteNode.ExternalTable != rTable
			remoteNodeMap["internet_table"] = rl.remoteNode.InternetTable
			remoteNodeMap["use_internet_table"] = rl.remoteNode.InternetTable > 0 && rl.remoteNode.InternetTable != rTable
			remoteNodeMap["asn"] = rl.remoteNode.ASN
			remoteNodeMap["name"] = rl.remoteNode.Name
			remoteNodeMap["ip"] = rl.remoteNode.IP
			remoteNodeMap["ip6"] = rl.remoteNode.IP6
			remoteNodeMap["external_ip"] = rl.remoteNode.ExternalIP
			remoteNodeMap["external_ip6"] = rl.remoteNode.ExternalIP6
			remoteNodeMap["interface"] = rl.remoteNode.Interface
			remoteNodeMap["is_external"] = rl.remoteNode.IsExternal
		} else {
			remoteNodeMap = map[string]any{
				"name":        rl.remoteEnd.Name,
				"asn":         uint64(0),
				"is_external": false,
			}
		}

		tags := rl.link.Tags
		if tags == nil {
			tags = []string{}
		}

		nodeLinks = append(nodeLinks, map[string]any{
			"tags":         tags,
			"local":        localMap,
			"remote":       remoteMap,
			"remote_node":  remoteNodeMap,
			"is_external":  rl.isExternal,
			"is_dn42":      rl.isDN42,
			"policy_id":    rl.policyID,
			"bgp_template": bgpTemplate,
			"local_as":     localAS,
			"cost":         rl.linkCost,
		})
	}

	var customDefaultTemplates []map[string]any
	if defaultCosts, ok := policyCostsMap[config.PolicyDefault]; ok {
		var extraCosts []int
		for c := range defaultCosts {
			if c != defaultCost {
				extraCosts = append(extraCosts, c)
			}
		}
		sort.Ints(extraCosts)
		for _, c := range extraCosts {
			customDefaultTemplates = append(customDefaultTemplates, map[string]any{
				"template_name": fmt.Sprintf("easy42_peer_cost_%d", c),
				"cost":          c,
			})
		}
	}

	var customNoneTemplates []map[string]any
	if noneCosts, ok := policyCostsMap[config.PolicyNone]; ok {
		var extraCosts []int
		for c := range noneCosts {
			if c != noneCost {
				extraCosts = append(extraCosts, c)
			}
		}
		sort.Ints(extraCosts)
		for _, c := range extraCosts {
			customNoneTemplates = append(customNoneTemplates, map[string]any{
				"template_name": fmt.Sprintf("none_peer_cost_%d", c),
				"cost":          c,
			})
		}
	}

	var customPolicyPrefixes []map[string]any
	var customBirdPolicies []map[string]any

	var customPolicyIDs []string
	for id := range usedPolicyMap {
		if id == config.PolicyDefault || id == config.PolicyDN42 || id == config.PolicyNone {
			continue
		}
		customPolicyIDs = append(customPolicyIDs, id)
	}
	sort.Strings(customPolicyIDs)

	for _, id := range customPolicyIDs {
		pol := usedPolicyMap[id]
		cleanID := SanitizeIdentifier(id)

		importV4 := formatPrefixList(pol.AllowedImportCIDRs, nil, false)
		importV6 := formatPrefixList(pol.AllowedImportCIDRs, nil, true)
		exportV4 := formatPrefixList(pol.AllowedDstCIDRs, nil, false)
		exportV6 := formatPrefixList(pol.AllowedDstCIDRs, nil, true)

		hasRoa := strings.TrimSpace(pol.ROA4) != "" || strings.TrimSpace(pol.ROA6) != ""
		roaFn := cleanID + "_roa_check"

		customPolicyPrefixes = append(customPolicyPrefixes, map[string]any{
			"id":                 cleanID,
			"has_import_v4":      importV4 != "",
			"has_import_v6":      importV6 != "",
			"import_prefixes_v4": importV4,
			"import_prefixes_v6": importV6,
			"has_export_v4":      exportV4 != "",
			"has_export_v6":      exportV6 != "",
			"export_prefixes_v4": exportV4,
			"export_prefixes_v6": exportV6,
		})

		baseCost := customBaseCosts[id]
		baseTmplName := "pol_peer_" + cleanID

		customBirdPolicies = append(customBirdPolicies, map[string]any{
			"id":                 cleanID,
			"name":               pol.Name,
			"template_name":      baseTmplName,
			"cost":               baseCost,
			"reject_internet":    pol.RejectInternet,
			"has_import_v4":      importV4 != "",
			"has_import_v6":      importV6 != "",
			"import_prefixes_v4": importV4,
			"import_prefixes_v6": importV6,
			"has_export_v4":      exportV4 != "",
			"has_export_v6":      exportV6 != "",
			"export_prefixes_v4": exportV4,
			"export_prefixes_v6": exportV6,
			"has_roa":            hasRoa,
			"roa_fn":             roaFn,
		})

		if costs, ok := policyCostsMap[id]; ok {
			var extraCosts []int
			for c := range costs {
				if c != baseCost {
					extraCosts = append(extraCosts, c)
				}
			}
			sort.Ints(extraCosts)
			for _, c := range extraCosts {
				customBirdPolicies = append(customBirdPolicies, map[string]any{
					"id":                 cleanID,
					"name":               pol.Name,
					"template_name":      fmt.Sprintf("pol_peer_%s_cost_%d", cleanID, c),
					"cost":               c,
					"reject_internet":    pol.RejectInternet,
					"has_import_v4":      importV4 != "",
					"has_import_v6":      importV6 != "",
					"import_prefixes_v4": importV4,
					"import_prefixes_v6": importV6,
					"has_export_v4":      exportV4 != "",
					"has_export_v6":      exportV6 != "",
					"export_prefixes_v4": exportV4,
					"export_prefixes_v6": exportV6,
					"has_roa":            hasRoa,
					"roa_fn":             roaFn,
				})
			}
		}
	}

	var roaPolicies []map[string]any
	hasDN42Roa := false
	hasDefaultRoa := false

	var usedPolicyIDs []string
	for id := range usedPolicyMap {
		usedPolicyIDs = append(usedPolicyIDs, id)
	}
	sort.Strings(usedPolicyIDs)

	for _, id := range usedPolicyIDs {
		pol := usedPolicyMap[id]
		hasROA4 := strings.TrimSpace(pol.ROA4) != ""
		hasROA6 := strings.TrimSpace(pol.ROA6) != ""
		if !hasROA4 && !hasROA6 {
			continue
		}
		cleanID := SanitizeIdentifier(id)
		roaPolicies = append(roaPolicies, map[string]any{
			"id":         cleanID,
			"name":       pol.Name,
			"has_roa4":   hasROA4,
			"has_roa6":   hasROA6,
			"table4":     cleanID + "_roa4",
			"table6":     cleanID + "_roa6",
			"proto4":     cleanID + "_roa4_static",
			"proto6":     cleanID + "_roa6_static",
			"file4":      fmt.Sprintf("/etc/easy42_%s_roa4.conf", cleanID),
			"file6":      fmt.Sprintf("/etc/easy42_%s_roa6.conf", cleanID),
			"check_fn":   cleanID + "_roa_check",
			"roa_strict": pol.ROAStrict,
		})
		if id == config.PolicyDN42 {
			hasDN42Roa = true
		}
		if id == config.PolicyDefault {
			hasDefaultRoa = true
		}
	}

	ctx["links"] = nodeLinks
	ctx["default_cost"] = defaultCost
	ctx["none_cost"] = noneCost
	ctx["has_external_links"] = hasExternalLinks
	ctx["has_none_policy"] = hasNonePolicy
	ctx["custom_default_templates"] = customDefaultTemplates
	ctx["custom_none_templates"] = customNoneTemplates
	ctx["custom_policy_prefixes"] = customPolicyPrefixes
	ctx["custom_bird_policies"] = customBirdPolicies
	ctx["roa_policies"] = roaPolicies
	ctx["has_dn42_roa"] = hasDN42Roa
	ctx["has_default_roa"] = hasDefaultRoa
	return ctx, nil
}

// SanitizeIdentifier turns an arbitrary string into a safe alphanumeric+underscore identifier
func SanitizeIdentifier(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	res := strings.Trim(sb.String(), "_")
	if res == "" {
		return "item"
	}
	return res
}

// formatPrefixList formats a list of IP prefixes into a BIRD set string like "[ 172.20.0.0/14{21,29} ]"
func formatPrefixList(prefixes []string, defaultList []string, isV6 bool) string {
	cleaned := config.CleanPrefixes(prefixes)
	var list []string
	for _, p := range cleaned {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if isV6 && strings.Contains(p, ":") {
			list = append(list, p)
		} else if !isV6 && strings.Contains(p, ".") && !strings.Contains(p, ":") {
			list = append(list, p)
		}
	}
	if len(list) == 0 {
		list = defaultList
	}
	if len(list) == 0 {
		return ""
	}
	return "[ " + strings.Join(list, ", ") + " ]"
}

// linkEndToContextMap transforms a LinkEnd into a map for BIRD template use.
// It ensures address is a pure IPv6 address without CIDR suffix (required by BIRD neighbor syntax)
// and ensures interface name is populated.
func linkEndToContextMap(end *config.LinkEnd, node *config.Node, peerName string, isExternal ...bool) map[string]any {
	addr := ""
	if end != nil && end.Address != "" {
		addr = strings.TrimSpace(end.Address)
		// Strip CIDR /64 if present
		if idx := strings.Index(addr, "/"); idx != -1 {
			addr = addr[:idx]
		}
	} else if node != nil && node.IP != "" {
		derived, err := DeriveIPv6LinkLocalAddressOnly(node.IP)
		if err == nil {
			addr = derived
		}
	}

	iface := ""
	if end != nil && end.Interface != "" {
		iface = end.Interface
	} else if peerName != "" {
		ext := len(isExternal) > 0 && isExternal[0]
		iface = GetInterfaceName(peerName, ext)
	}

	name := ""
	listenPort := 0
	endpoint := ""
	pubKey := ""
	keepalive := 0
	mtu := 0
	cost := 0

	if end != nil {
		name = end.Name
		listenPort = end.ListenPort
		endpoint = end.Endpoint
		pubKey = end.PublicKey
		keepalive = end.PersistentKeepalive
		mtu = end.MTU
		cost = end.Cost
	}

	isLinkLocal := strings.HasPrefix(strings.ToLower(addr), "fe80:")

	return map[string]any{
		"name":                 name,
		"interface":            iface,
		"address":              addr,
		"is_link_local":        isLinkLocal,
		"listen_port":          listenPort,
		"endpoint":             endpoint,
		"public_key":           pubKey,
		"persistent_keepalive": keepalive,
		"mtu":                  mtu,
		"cost":                 cost,
	}
}

// GenerateBirdConfigWithTemplate executes a custom template with the node context
func GenerateBirdConfigWithTemplate(
	tmplContent string,
	node *config.Node,
	allNodes []config.Node,
	links []config.Link,
	args ...any,
) (string, error) {
	ctx, err := BuildNodeContext(node, allNodes, links, args...)
	if err != nil {
		return "", err
	}
	return tplutil.RenderTemplate(tmplContent, ctx)
}

// GenerateBirdConfig compiles the BIRD configuration for a node using the default embedded template
func GenerateBirdConfig(
	node *config.Node,
	allNodes []config.Node,
	links []config.Link,
	args ...any,
) (string, error) {
	tmplContent, err := GetDefaultBirdTemplate()
	if err != nil {
		return "", err
	}
	return GenerateBirdConfigWithTemplate(tmplContent, node, allNodes, links, args...)
}
