package compiler

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"easy42/internal/config"
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
//	  "routes": [...],
//	  "links": [
//	    {
//	      "tags": [...],
//	      "local": { "name": ..., "interface": ..., "address": ... },
//	      "remote": { "name": ..., "interface": ..., "address": ... },
//	      "remote_node": { "name": ..., "asn": ..., "ip": ... }
//	    }
//	  ]
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
	ctx["Table"] = table
	ctx["routing_table"] = table
	ctx["RoutingTable"] = table
	ctx["asn"] = node.ASN
	ctx["ASN"] = node.ASN

	// NetworkSettings / BGP Confederation support
	if settings != nil && settings.PublicASN > 0 {
		ctx["confed_as"] = settings.PublicASN
		ctx["ConfedAS"] = settings.PublicASN

		confedMembers := strings.TrimSpace(settings.ConfedMembers)
		if confedMembers == "" {
			confedMembers = "4224420000..4224429999"
		}
		if !strings.HasPrefix(confedMembers, "[") {
			confedMembers = "[ " + confedMembers + " ]"
		}
		ctx["confed_members"] = confedMembers
		ctx["ConfedMembers"] = confedMembers
	}

	var exportPrefixes, importPrefixes []string
	if settings != nil {
		exportPrefixes = settings.ExportPrefixes
		importPrefixes = settings.ImportPrefixes
	}
	ctx["ext_import_v4"] = formatPrefixList(importPrefixes, []string{"172.20.0.0/14{21,29}"}, false)
	ctx["ext_import_v6"] = formatPrefixList(importPrefixes, []string{"fd00::/8{44,64}"}, true)
	ctx["ext_export_v4"] = formatPrefixList(exportPrefixes, []string{"172.20.0.0/14+"}, false)
	ctx["ext_export_v6"] = formatPrefixList(exportPrefixes, []string{"fd00::/8+"}, true)

	// 3. Normalize routes and static routes
	routes := node.Routes
	var routeMaps []map[string]any
	for _, r := range routes {
		routeMaps = append(routeMaps, map[string]any{
			"table":    r.Table,
			"Table":    r.Table,
			"prefixes": r.Prefixes,
			"Prefixes": r.Prefixes,
		})
	}
	ctx["routes"] = routeMaps
	ctx["kernel_routes"] = routeMaps

	if node.StaticRoutes == nil {
		ctx["static_routes"] = []string{}
	} else {
		ctx["static_routes"] = node.StaticRoutes
	}

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

		localMap := linkEndToContextMap(localEnd, node, remoteEnd.Name)
		remoteMap := linkEndToContextMap(remoteEnd, remoteNode, localEnd.Name)

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
			remoteNodeMap["Table"] = rTable
			remoteNodeMap["asn"] = remoteNode.ASN
			remoteNodeMap["ASN"] = remoteNode.ASN
			remoteNodeMap["Name"] = remoteNode.Name
			remoteNodeMap["IP"] = remoteNode.IP
			remoteNodeMap["Interface"] = remoteNode.Interface
			remoteNodeMap["is_external"] = remoteNode.IsExternal
			remoteNodeMap["IsExternal"] = remoteNode.IsExternal
		} else {
			remoteNodeMap = map[string]any{
				"name":        remoteEnd.Name,
				"Name":        remoteEnd.Name,
				"asn":         uint64(0),
				"ASN":         uint64(0),
				"is_external": false,
				"IsExternal":  false,
			}
		}

		tags := l.Tags
		if tags == nil {
			tags = []string{}
		}

		nodeLinks = append(nodeLinks, map[string]any{
			"tags":        tags,
			"Tags":        tags,
			"local":       localMap,
			"Local":       localMap,
			"remote":      remoteMap,
			"Remote":      remoteMap,
			"remote_node": remoteNodeMap,
			"RemoteNode":  remoteNodeMap,
			"is_external": isExternal,
			"IsExternal":  isExternal,
		})
	}

	ctx["links"] = nodeLinks
	ctx["Links"] = nodeLinks
	ctx["has_external_links"] = hasExternalLinks
	ctx["HasExternalLinks"] = hasExternalLinks
	return ctx, nil
}

// formatPrefixList formats a list of IP prefixes into a BIRD set string like "[ 172.20.0.0/14{21,29} ]"
func formatPrefixList(prefixes []string, defaultList []string, isV6 bool) string {
	var list []string
	for _, p := range prefixes {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if isV6 && strings.Contains(p, ":") {
			list = append(list, p)
		} else if !isV6 && !strings.Contains(p, ":") {
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
func linkEndToContextMap(end *config.LinkEnd, node *config.Node, peerName string) map[string]any {
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
		iface = GetInterfaceName(peerName)
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
		"Name":                 name,
		"interface":            iface,
		"Interface":            iface,
		"address":              addr,
		"Address":              addr,
		"is_link_local":        isLinkLocal,
		"IsLinkLocal":          isLinkLocal,
		"listen_port":          listenPort,
		"ListenPort":           listenPort,
		"endpoint":             endpoint,
		"Endpoint":             endpoint,
		"public_key":           pubKey,
		"PublicKey":            pubKey,
		"persistent_keepalive": keepalive,
		"PersistentKeepalive":  keepalive,
		"mtu":                  mtu,
		"MTU":                  mtu,
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

	funcMap := template.FuncMap{
		"join": strings.Join,
		"stripPrefix": func(s string) string {
			if idx := strings.Index(s, "/"); idx != -1 {
				return s[:idx]
			}
			return s
		},
	}

	tmpl, err := template.New("bird_config").Funcs(funcMap).Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse bird config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("failed to execute bird config template: %w", err)
	}

	return buf.String(), nil
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
