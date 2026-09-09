package compiler

import (
	"embed"
	"encoding/json"
	"fmt"
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
func BuildNodeContext(node *config.Node, allNodes []config.Node, links []config.Link, netSettings ...*config.NetworkSettings) (map[string]any, error) {
	if node == nil {
		return nil, fmt.Errorf("node cannot be nil")
	}

	var settings *config.NetworkSettings
	if len(netSettings) > 0 && netSettings[0] != nil {
		settings = netSettings[0]
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
	var nodeLinks []map[string]any
	hasExternalLinks := false
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

		isExternal := false
		if (remoteNode != nil && remoteNode.IsExternal) || node.IsExternal {
			isExternal = true
			hasExternalLinks = true
		}

		localMap := linkEndToContextMap(localEnd, node, remoteEnd.Name, remoteNode != nil && remoteNode.IsExternal)
		remoteMap := linkEndToContextMap(remoteEnd, remoteNode, localEnd.Name, node.IsExternal)

		var remoteNodeMap map[string]any
		if remoteNode != nil {
			nodeData, _ := json.Marshal(remoteNode)
			_ = json.Unmarshal(nodeData, &remoteNodeMap)
			// Ensure table defaults for remote_node as well
			rTable := remoteNode.Table
			if rTable <= 0 {
				rTable = 254
			}
			remoteNodeMap["table"] = rTable
			remoteNodeMap["routing_table"] = rTable
			remoteNodeMap["external_table"] = remoteNode.ExternalTable
			remoteNodeMap["use_external_table"] = remoteNode.ExternalTable > 0 && remoteNode.ExternalTable != rTable
			remoteNodeMap["asn"] = remoteNode.ASN
			remoteNodeMap["name"] = remoteNode.Name
			remoteNodeMap["ip"] = remoteNode.IP
			remoteNodeMap["external_ip"] = remoteNode.ExternalIP
			remoteNodeMap["external_ip6"] = remoteNode.ExternalIP6
			remoteNodeMap["interface"] = remoteNode.Interface
			remoteNodeMap["is_external"] = remoteNode.IsExternal
		} else {
			remoteNodeMap = map[string]any{
				"name":        remoteEnd.Name,
				"asn":         uint64(0),
				"is_external": false,
			}
		}

		tags := l.Tags
		if tags == nil {
			tags = []string{}
		}

		nodeLinks = append(nodeLinks, map[string]any{
			"tags":        tags,
			"local":       localMap,
			"remote":      remoteMap,
			"remote_node": remoteNodeMap,
			"is_external": isExternal,
		})
	}

	ctx["links"] = nodeLinks
	ctx["has_external_links"] = hasExternalLinks
	return ctx, nil
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

	if end != nil {
		name = end.Name
		listenPort = end.ListenPort
		endpoint = end.Endpoint
		pubKey = end.PublicKey
		keepalive = end.PersistentKeepalive
		mtu = end.MTU
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
	}
}

// GenerateBirdConfigWithTemplate executes a custom template with the node context
func GenerateBirdConfigWithTemplate(
	tmplContent string,
	node *config.Node,
	allNodes []config.Node,
	links []config.Link,
	netSettings ...*config.NetworkSettings,
) (string, error) {
	ctx, err := BuildNodeContext(node, allNodes, links, netSettings...)
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
	netSettings ...*config.NetworkSettings,
) (string, error) {
	tmplContent, err := GetDefaultBirdTemplate()
	if err != nil {
		return "", err
	}
	return GenerateBirdConfigWithTemplate(tmplContent, node, allNodes, links, netSettings...)
}
