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
// formatVariantTemplateName constructs a specialized BGP template name when cost or routing policy differs from base.
func formatVariantTemplateName(base string, routingPolicy, baseRoutingPolicy string, cost, baseCost int) string {
	if routingPolicy == baseRoutingPolicy && cost == baseCost {
		return base
	}
	name := base
	if routingPolicy != baseRoutingPolicy {
		name += "_" + routingPolicy
	}
	if cost != baseCost {
		name += fmt.Sprintf("_cost_%d", cost)
	}
	return name
}

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

	if node.Metric != nil {
		ctx["metric"] = *node.Metric
		ctx["has_metric"] = true
	} else {
		delete(ctx, "metric")
		delete(ctx, "has_metric")
	}

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

	dn42Pol, hasDN42 := policyMap[config.PolicyDN42]
	var dn42ImportV4, dn42ImportV6, dn42ExportV4, dn42ExportV6 string
	var dn42DisallowedImportV4, dn42DisallowedImportV6 string
	var dn42DisallowedExportV4, dn42DisallowedExportV6 string
	var dn42RejectInternet bool
	if hasDN42 {
		dn42ImportV4 = formatPrefixList(dn42Pol.AllowedSrcCIDRs, nil, false)
		dn42ImportV6 = formatPrefixList(dn42Pol.AllowedSrcCIDRs, nil, true)
		dn42DisallowedImportV4 = formatPrefixList(dn42Pol.DisallowedSrcCIDRs, nil, false)
		dn42DisallowedImportV6 = formatPrefixList(dn42Pol.DisallowedSrcCIDRs, nil, true)
		dn42ExportV4 = formatPrefixList(dn42Pol.AllowedDstCIDRs, nil, false)
		dn42ExportV6 = formatPrefixList(dn42Pol.AllowedDstCIDRs, nil, true)
		dn42DisallowedExportV4 = formatPrefixList(dn42Pol.DisallowedDstCIDRs, nil, false)
		dn42DisallowedExportV6 = formatPrefixList(dn42Pol.DisallowedDstCIDRs, nil, true)
		dn42RejectInternet = dn42Pol.RejectInternet
	} else {
		var prefixes []string
		if settings != nil {
			prefixes = settings.Prefixes
		}
		dn42ImportV4 = formatPrefixList(prefixes, []string{"172.20.0.0/14{21,29}"}, false)
		dn42ImportV6 = formatPrefixList(prefixes, []string{"fd00::/8{44,64}"}, true)
		dn42ExportV4 = dn42ImportV4
		dn42ExportV6 = dn42ImportV6
		dn42RejectInternet = true
	}
	ctx["ext_prefixes_v4"] = dn42ImportV4
	ctx["ext_prefixes_v6"] = dn42ImportV6
	ctx["ext_disallowed_import_prefixes_v4"] = dn42DisallowedImportV4
	ctx["ext_disallowed_import_prefixes_v6"] = dn42DisallowedImportV6
	ctx["ext_export_prefixes_v4"] = dn42ExportV4
	ctx["ext_export_prefixes_v6"] = dn42ExportV6
	ctx["ext_disallowed_export_prefixes_v4"] = dn42DisallowedExportV4
	ctx["ext_disallowed_export_prefixes_v6"] = dn42DisallowedExportV6
	ctx["has_ext_import_v4"] = dn42ImportV4 != ""
	ctx["has_ext_import_v6"] = dn42ImportV6 != ""
	ctx["has_ext_disallowed_import_v4"] = dn42DisallowedImportV4 != ""
	ctx["has_ext_disallowed_import_v6"] = dn42DisallowedImportV6 != ""
	ctx["has_ext_export_v4"] = dn42ExportV4 != ""
	ctx["has_ext_export_v6"] = dn42ExportV6 != ""
	ctx["has_ext_disallowed_export_v4"] = dn42DisallowedExportV4 != ""
	ctx["has_ext_disallowed_export_v6"] = dn42DisallowedExportV6 != ""
	ctx["ext_reject_internet"] = dn42RejectInternet

	// 3. Normalize routes and static routes
	routes := node.Routes
	var routeMaps []map[string]any
	var mainKernelPrefixesV4 []string
	var mainKernelPrefixesV6 []string
	otherRoutesV4 := make(map[int][]string)
	otherRoutesV6 := make(map[int][]string)
	var otherTableOrder []int

	for _, r := range routes {
		tbl := r.Table
		if tbl <= 0 {
			tbl = table
		}
		var cleanedV4 []string
		var cleanedV6 []string
		for _, p := range r.Prefixes {
			trimmed := strings.TrimSpace(p)
			if trimmed == "" {
				continue
			}
			if strings.Contains(trimmed, ":") {
				cleanedV6 = append(cleanedV6, trimmed)
			} else {
				cleanedV4 = append(cleanedV4, trimmed)
			}
		}

		if tbl == table {
			mainKernelPrefixesV4 = append(mainKernelPrefixesV4, cleanedV4...)
			mainKernelPrefixesV6 = append(mainKernelPrefixesV6, cleanedV6...)
		} else {
			if _, exists := otherRoutesV4[tbl]; !exists {
				otherTableOrder = append(otherTableOrder, tbl)
			}
			otherRoutesV4[tbl] = append(otherRoutesV4[tbl], cleanedV4...)
			otherRoutesV6[tbl] = append(otherRoutesV6[tbl], cleanedV6...)
		}

		routeMaps = append(routeMaps, map[string]any{
			"table":    r.Table,
			"prefixes": r.Prefixes,
		})
	}
	ctx["routes"] = routeMaps
	ctx["kernel_routes"] = routeMaps

	var otherKernelRoutes []map[string]any
	for _, tbl := range otherTableOrder {
		v4List := otherRoutesV4[tbl]
		v6List := otherRoutesV6[tbl]
		otherKernelRoutes = append(otherKernelRoutes, map[string]any{
			"table":       tbl,
			"prefixes":    v4List,
			"prefixes_v4": v4List,
			"prefixes_v6": v6List,
			"has_v4":      len(v4List) > 0,
			"has_v6":      len(v6List) > 0,
		})
	}
	ctx["other_kernel_routes"] = otherKernelRoutes
	ctx["has_main_kernel_routes"] = len(mainKernelPrefixesV4) > 0
	ctx["main_kernel_prefixes"] = mainKernelPrefixesV4
	ctx["has_main_kernel_routes_v6"] = len(mainKernelPrefixesV6) > 0
	ctx["main_kernel_prefixes_v6"] = mainKernelPrefixesV6

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
		routingPolicy    string
	}

	type policyVariantKey struct {
		cost          int
		routingPolicy string
	}

	var rawLinks []rawLinkInfo
	hasExternalLinks := false
	hasNonePolicy := false
	usedPolicyMap := make(map[string]config.NetworkPolicy)
	policyVariantsMap := make(map[string]map[policyVariantKey]bool)
	extVariantsMap := make(map[string]bool)

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

		isExternal := isRemoteExternal || node.IsExternal
		if isExternal {
			hasExternalLinks = true
		}

		isDN42 := (policyID == config.PolicyDN42)
		if policyID == config.PolicyNone {
			hasNonePolicy = true
		}

		linkCost := localEnd.EffectiveCostWithPolicy(&pol)
		routingPolicy := localEnd.EffectiveRoutingPolicy(&pol)

		if policyVariantsMap[policyID] == nil {
			policyVariantsMap[policyID] = make(map[policyVariantKey]bool)
		}
		policyVariantsMap[policyID][policyVariantKey{cost: linkCost, routingPolicy: routingPolicy}] = true
		if isRemoteExternal {
			extVariantsMap[routingPolicy] = true
		}

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
			routingPolicy:    routingPolicy,
		})
	}

	defaultCost := 100
	defaultRoutingPolicy := config.RoutingPolicyFull
	if p, ok := policyMap[config.PolicyDefault]; ok {
		defaultCost = p.EffectiveCost()
		defaultRoutingPolicy = p.EffectiveRoutingPolicy()
	}
	if variants, ok := policyVariantsMap[config.PolicyDefault]; ok && len(variants) == 1 {
		for v := range variants {
			defaultCost = v.cost
			defaultRoutingPolicy = v.routingPolicy
		}
	}

	noneCost := 100
	noneRoutingPolicy := config.RoutingPolicyFull
	if p, ok := policyMap[config.PolicyNone]; ok {
		noneCost = p.EffectiveCost()
		noneRoutingPolicy = p.EffectiveRoutingPolicy()
	}
	if variants, ok := policyVariantsMap[config.PolicyNone]; ok && len(variants) == 1 {
		for v := range variants {
			noneCost = v.cost
			noneRoutingPolicy = v.routingPolicy
		}
	}

	extRoutingPolicy := config.RoutingPolicyFull
	if hasDN42 {
		extRoutingPolicy = dn42Pol.EffectiveRoutingPolicy()
	}
	if len(extVariantsMap) == 1 {
		for rp := range extVariantsMap {
			extRoutingPolicy = rp
		}
	}

	type basePolicyConfig struct {
		cost          int
		routingPolicy string
	}
	customBaseConfigs := make(map[string]basePolicyConfig)
	for id, pol := range usedPolicyMap {
		base := basePolicyConfig{
			cost:          pol.EffectiveCost(),
			routingPolicy: pol.EffectiveRoutingPolicy(),
		}
		if variants, ok := policyVariantsMap[id]; ok && len(variants) == 1 {
			for v := range variants {
				base.cost = v.cost
				base.routingPolicy = v.routingPolicy
			}
		}
		customBaseConfigs[id] = base
	}

	var nodeLinks []map[string]any
	usedBgpProtoNames := make(map[string]int)
	for _, rl := range rawLinks {
		bgpTemplate := "easy42_peer"
		localAS := "SELF_AS"

		switch rl.policyID {
		case config.PolicyDefault:
			bgpTemplate = formatVariantTemplateName("easy42_peer", rl.routingPolicy, defaultRoutingPolicy, rl.linkCost, defaultCost)
		case config.PolicyDN42:
			if rl.isRemoteExternal {
				if rl.routingPolicy == extRoutingPolicy {
					bgpTemplate = "external_peer"
				} else {
					bgpTemplate = fmt.Sprintf("external_peer_%s", rl.routingPolicy)
				}
			} else {
				cleanID := SanitizeIdentifier(rl.policyID)
				baseCfg := customBaseConfigs[rl.policyID]
				bgpTemplate = formatVariantTemplateName("pol_peer_"+cleanID, rl.routingPolicy, baseCfg.routingPolicy, rl.linkCost, baseCfg.cost)
			}
		case config.PolicyNone:
			bgpTemplate = formatVariantTemplateName("none_peer", rl.routingPolicy, noneRoutingPolicy, rl.linkCost, noneCost)
		default:
			cleanID := SanitizeIdentifier(rl.policyID)
			baseCfg := customBaseConfigs[rl.policyID]
			bgpTemplate = formatVariantTemplateName("pol_peer_"+cleanID, rl.routingPolicy, baseCfg.routingPolicy, rl.linkCost, baseCfg.cost)
		}

		if rl.isRemoteExternal && settings != nil && settings.PublicASN > 0 {
			localAS = "CONFED_AS"
		}

		localMap := linkEndToContextMap(rl.localEnd, node, rl.remoteEnd.Name, rl.isRemoteExternal)
		localMap["policy"] = rl.policyID
		localMap["cost"] = rl.linkCost
		remoteMap := linkEndToContextMap(rl.remoteEnd, rl.remoteNode, rl.localEnd.Name, node.IsExternal)
		if remoteMap["address"] == "" && rl.localEnd.NeighborAddress != "" {
			nAddr := strings.TrimSpace(rl.localEnd.NeighborAddress)
			if idx := strings.Index(nAddr, "/"); idx != -1 {
				nAddr = nAddr[:idx]
			}
			remoteMap["address"] = nAddr
			remoteMap["is_link_local"] = strings.HasPrefix(strings.ToLower(nAddr), "fe80:")
		}

		// Determine BIRD peer protocol name suffix (e.g. "", "1", "2") so each link has a unique BGP protocol name
		suffix := ExtractInterfaceSuffix(rl.localEnd.Interface, rl.remoteEnd.Name, rl.isRemoteExternal)
		protoPeerName := rl.remoteEnd.Name
		if suffix != "" {
			protoPeerName = fmt.Sprintf("%s%s", rl.remoteEnd.Name, suffix)
		}
		if count := usedBgpProtoNames[protoPeerName]; count > 0 {
			protoPeerName = fmt.Sprintf("%s_%d", protoPeerName, count+1)
		}
		usedBgpProtoNames[protoPeerName]++
		remoteMap["name"] = protoPeerName

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
			if rl.remoteNode.Metric != nil {
				remoteNodeMap["metric"] = *rl.remoteNode.Metric
				remoteNodeMap["has_metric"] = true
			}
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

		pref := rl.localEnd.EffectivePreference(&rl.pol)
		mark := rl.localEnd.EffectiveMark(&rl.pol)
		nodeLink := map[string]any{
			"type":           rl.link.LinkType(),
			"is_manual":      rl.link.IsManual(),
			"tags":           tags,
			"local":          localMap,
			"remote":         remoteMap,
			"remote_node":    remoteNodeMap,
			"is_external":    rl.isExternal,
			"is_dn42":        rl.isDN42,
			"policy_id":      rl.policyID,
			"bgp_template":   bgpTemplate,
			"local_as":       localAS,
			"cost":           rl.linkCost,
			"has_preference": pref != nil,
			"has_mark":       mark != "",
		}
		if pref != nil {
			nodeLink["preference"] = *pref
			localMap["preference"] = *pref
			localMap["has_preference"] = true
		}
		if mark != "" {
			nodeLink["mark"] = mark
			localMap["mark"] = mark
			localMap["has_mark"] = true
		}
		nodeLinks = append(nodeLinks, nodeLink)
	}

	var customDefaultTemplates []map[string]any
	if variants, ok := policyVariantsMap[config.PolicyDefault]; ok {
		var extraVariants []policyVariantKey
		for v := range variants {
			if v.cost != defaultCost || v.routingPolicy != defaultRoutingPolicy {
				extraVariants = append(extraVariants, v)
			}
		}
		sort.Slice(extraVariants, func(i, j int) bool {
			if extraVariants[i].routingPolicy != extraVariants[j].routingPolicy {
				return extraVariants[i].routingPolicy < extraVariants[j].routingPolicy
			}
			return extraVariants[i].cost < extraVariants[j].cost
		})
		for _, v := range extraVariants {
			customDefaultTemplates = append(customDefaultTemplates, map[string]any{
				"template_name":  formatVariantTemplateName("easy42_peer", v.routingPolicy, defaultRoutingPolicy, v.cost, defaultCost),
				"cost":           v.cost,
				"routing_policy": v.routingPolicy,
			})
		}
	}

	var customNoneTemplates []map[string]any
	if variants, ok := policyVariantsMap[config.PolicyNone]; ok {
		var extraVariants []policyVariantKey
		for v := range variants {
			if v.cost != noneCost || v.routingPolicy != noneRoutingPolicy {
				extraVariants = append(extraVariants, v)
			}
		}
		sort.Slice(extraVariants, func(i, j int) bool {
			if extraVariants[i].routingPolicy != extraVariants[j].routingPolicy {
				return extraVariants[i].routingPolicy < extraVariants[j].routingPolicy
			}
			return extraVariants[i].cost < extraVariants[j].cost
		})
		for _, v := range extraVariants {
			customNoneTemplates = append(customNoneTemplates, map[string]any{
				"template_name":  formatVariantTemplateName("none_peer", v.routingPolicy, noneRoutingPolicy, v.cost, noneCost),
				"cost":           v.cost,
				"routing_policy": v.routingPolicy,
			})
		}
	}

	var customExternalTemplates []map[string]any
	var extraExtRPs []string
	for rp := range extVariantsMap {
		if rp != extRoutingPolicy {
			extraExtRPs = append(extraExtRPs, rp)
		}
	}
	sort.Strings(extraExtRPs)
	for _, rp := range extraExtRPs {
		customExternalTemplates = append(customExternalTemplates, map[string]any{
			"template_name":  fmt.Sprintf("external_peer_%s", rp),
			"routing_policy": rp,
		})
	}

	var customPolicyPrefixes []map[string]any
	var customBirdPolicies []map[string]any

	hasInternalDN42 := false
	for _, rl := range rawLinks {
		if !rl.isRemoteExternal && rl.policyID == config.PolicyDN42 {
			hasInternalDN42 = true
			break
		}
	}

	var customPolicyIDs []string
	for id := range usedPolicyMap {
		if id == config.PolicyDefault || id == config.PolicyNone {
			continue
		}
		if id == config.PolicyDN42 && !hasInternalDN42 {
			continue
		}
		customPolicyIDs = append(customPolicyIDs, id)
	}
	sort.Strings(customPolicyIDs)

	for _, id := range customPolicyIDs {
		pol := usedPolicyMap[id]
		cleanID := SanitizeIdentifier(id)

		importV4 := formatPrefixList(pol.AllowedSrcCIDRs, nil, false)
		importV6 := formatPrefixList(pol.AllowedSrcCIDRs, nil, true)
		disallowedImportV4 := formatPrefixList(pol.DisallowedSrcCIDRs, nil, false)
		disallowedImportV6 := formatPrefixList(pol.DisallowedSrcCIDRs, nil, true)
		exportV4 := formatPrefixList(pol.AllowedDstCIDRs, nil, false)
		exportV6 := formatPrefixList(pol.AllowedDstCIDRs, nil, true)
		disallowedExportV4 := formatPrefixList(pol.DisallowedDstCIDRs, nil, false)
		disallowedExportV6 := formatPrefixList(pol.DisallowedDstCIDRs, nil, true)

		hasRoa := strings.TrimSpace(pol.ROA4) != "" || strings.TrimSpace(pol.ROA6) != ""
		roaFn := cleanID + "_roa_check"

		customPolicyPrefixes = append(customPolicyPrefixes, map[string]any{
			"id":                            cleanID,
			"has_import_v4":                 importV4 != "",
			"has_import_v6":                 importV6 != "",
			"import_prefixes_v4":            importV4,
			"import_prefixes_v6":            importV6,
			"has_disallowed_import_v4":      disallowedImportV4 != "",
			"has_disallowed_import_v6":      disallowedImportV6 != "",
			"disallowed_import_prefixes_v4": disallowedImportV4,
			"disallowed_import_prefixes_v6": disallowedImportV6,
			"has_export_v4":                 exportV4 != "",
			"has_export_v6":                 exportV6 != "",
			"export_prefixes_v4":            exportV4,
			"export_prefixes_v6":            exportV6,
			"has_disallowed_export_v4":      disallowedExportV4 != "",
			"has_disallowed_export_v6":      disallowedExportV6 != "",
			"disallowed_export_prefixes_v4": disallowedExportV4,
			"disallowed_export_prefixes_v6": disallowedExportV6,
		})

		baseCfg := customBaseConfigs[id]
		baseTmplName := "pol_peer_" + cleanID

		customBirdPolicies = append(customBirdPolicies, map[string]any{
			"id":                            cleanID,
			"name":                          pol.Name,
			"template_name":                 baseTmplName,
			"cost":                          baseCfg.cost,
			"routing_policy":                baseCfg.routingPolicy,
			"reject_internet":               pol.RejectInternet,
			"has_import_v4":                 importV4 != "",
			"has_import_v6":                 importV6 != "",
			"import_prefixes_v4":            importV4,
			"import_prefixes_v6":            importV6,
			"has_disallowed_import_v4":      disallowedImportV4 != "",
			"has_disallowed_import_v6":      disallowedImportV6 != "",
			"disallowed_import_prefixes_v4": disallowedImportV4,
			"disallowed_import_prefixes_v6": disallowedImportV6,
			"has_export_v4":                 exportV4 != "",
			"has_export_v6":                 exportV6 != "",
			"export_prefixes_v4":            exportV4,
			"export_prefixes_v6":            exportV6,
			"has_disallowed_export_v4":      disallowedExportV4 != "",
			"has_disallowed_export_v6":      disallowedExportV6 != "",
			"disallowed_export_prefixes_v4": disallowedExportV4,
			"disallowed_export_prefixes_v6": disallowedExportV6,
			"has_roa":                       hasRoa,
			"roa_fn":                        roaFn,
		})

		if variants, ok := policyVariantsMap[id]; ok {
			var extraVariants []policyVariantKey
			for v := range variants {
				if v.cost != baseCfg.cost || v.routingPolicy != baseCfg.routingPolicy {
					extraVariants = append(extraVariants, v)
				}
			}
			sort.Slice(extraVariants, func(i, j int) bool {
				if extraVariants[i].routingPolicy != extraVariants[j].routingPolicy {
					return extraVariants[i].routingPolicy < extraVariants[j].routingPolicy
				}
				return extraVariants[i].cost < extraVariants[j].cost
			})
			for _, v := range extraVariants {
				vTmplName := formatVariantTemplateName("pol_peer_"+cleanID, v.routingPolicy, baseCfg.routingPolicy, v.cost, baseCfg.cost)
				customBirdPolicies = append(customBirdPolicies, map[string]any{
					"id":                            cleanID,
					"name":                          pol.Name,
					"template_name":                 vTmplName,
					"cost":                          v.cost,
					"routing_policy":                v.routingPolicy,
					"reject_internet":               pol.RejectInternet,
					"has_import_v4":                 importV4 != "",
					"has_import_v6":                 importV6 != "",
					"import_prefixes_v4":            importV4,
					"import_prefixes_v6":            importV6,
					"has_disallowed_import_v4":      disallowedImportV4 != "",
					"has_disallowed_import_v6":      disallowedImportV6 != "",
					"disallowed_import_prefixes_v4": disallowedImportV4,
					"disallowed_import_prefixes_v6": disallowedImportV6,
					"has_export_v4":                 exportV4 != "",
					"has_export_v6":                 exportV6 != "",
					"export_prefixes_v4":            exportV4,
					"export_prefixes_v6":            exportV6,
					"has_disallowed_export_v4":      disallowedExportV4 != "",
					"has_disallowed_export_v6":      disallowedExportV6 != "",
					"disallowed_export_prefixes_v4": disallowedExportV4,
					"disallowed_export_prefixes_v6": disallowedExportV6,
					"has_roa":                       hasRoa,
					"roa_fn":                        roaFn,
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
	ctx["default_routing_policy"] = defaultRoutingPolicy
	ctx["none_cost"] = noneCost
	ctx["none_routing_policy"] = noneRoutingPolicy
	ctx["ext_routing_policy"] = extRoutingPolicy
	ctx["has_external_links"] = hasExternalLinks
	ctx["has_none_policy"] = hasNonePolicy
	ctx["custom_default_templates"] = customDefaultTemplates
	ctx["custom_none_templates"] = customNoneTemplates
	ctx["custom_external_templates"] = customExternalTemplates
	ctx["custom_policy_prefixes"] = customPolicyPrefixes
	ctx["custom_bird_policies"] = customBirdPolicies
	ctx["roa_policies"] = roaPolicies
	ctx["has_dn42_roa"] = hasDN42Roa
	ctx["has_default_roa"] = hasDefaultRoa

	var birdHooksPre []string
	var birdHooksPost []string
	for _, h := range node.ConfigHooks {
		content := strings.TrimSpace(h.Content)
		if content == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(h.Type)) {
		case "bird.pre":
			birdHooksPre = append(birdHooksPre, content)
		case "bird", "bird.post", "bird.global":
			birdHooksPost = append(birdHooksPost, content)
		}
	}
	if len(birdHooksPre) > 0 {
		ctx["bird_hooks_pre"] = birdHooksPre
	}
	if len(birdHooksPost) > 0 {
		ctx["bird_hooks"] = birdHooksPost
	}

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
	fwmark := ""
	var preference *int
	mark := ""
	routingPolicy := ""

	neighborAddr := ""
	linkType := config.LinkTypeWireGuard
	isManual := false
	if end != nil {
		name = end.Name
		linkType = end.LinkType()
		isManual = end.IsManual()
		neighborAddr = end.NeighborAddress
		listenPort = end.ListenPort
		endpoint = end.Endpoint
		pubKey = end.PublicKey
		keepalive = end.PersistentKeepalive
		mtu = end.MTU
		cost = end.Cost
		fwmark = end.Fwmark
		preference = end.Preference
		mark = end.Mark
		routingPolicy = end.RoutingPolicy
	}

	isLinkLocal := strings.HasPrefix(strings.ToLower(addr), "fe80:")

	res := map[string]any{
		"name":                 name,
		"type":                 linkType,
		"is_manual":            isManual,
		"interface":            iface,
		"address":              addr,
		"neighbor_address":     neighborAddr,
		"is_link_local":        isLinkLocal,
		"listen_port":          listenPort,
		"endpoint":             endpoint,
		"public_key":           pubKey,
		"persistent_keepalive": keepalive,
		"mtu":                  mtu,
		"cost":                 cost,
		"fwmark":               fwmark,
		"mark":                 mark,
		"routing_policy":       routingPolicy,
	}
	if preference != nil {
		res["preference"] = *preference
		res["has_preference"] = true
	}
	return res
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
