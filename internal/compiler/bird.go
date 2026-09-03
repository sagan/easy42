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
//	  "asn": 4299420001,
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
//	}
func BuildNodeContext(node *config.Node, allNodes []config.Node, links []config.Link) (map[string]any, error) {
	if node == nil {
		return nil, fmt.Errorf("node cannot be nil")
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

		} else {
			remoteNodeMap = map[string]any{
				"name": remoteEnd.Name,
				"Name": remoteEnd.Name,
				"asn":  uint64(0),
				"ASN":  uint64(0),
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
		})
	}

	ctx["links"] = nodeLinks
	ctx["Links"] = nodeLinks
	return ctx, nil
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

	return map[string]any{
		"name":                 name,
		"Name":                 name,
		"interface":            iface,
		"Interface":            iface,
		"address":              addr,
		"Address":              addr,
		"listen_port":          listenPort,
		"ListenPort":          listenPort,
		"endpoint":             endpoint,
		"Endpoint":             endpoint,
		"public_key":           pubKey,
		"PublicKey":            pubKey,
		"persistent_keepalive": keepalive,
		"PersistentKeepalive": keepalive,
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
) (string, error) {
	ctx, err := BuildNodeContext(node, allNodes, links)
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
) (string, error) {
	tmplContent, err := GetDefaultBirdTemplate()
	if err != nil {
		return "", err
	}
	return GenerateBirdConfigWithTemplate(tmplContent, node, allNodes, links)
}
