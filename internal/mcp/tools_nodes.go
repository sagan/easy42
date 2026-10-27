package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
	"easy42/internal/engine"
	"easy42/internal/ssh"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerNodeTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. list_nodes
	s.AddTool(mcp.NewTool("list_nodes",
		mcp.WithDescription("List all nodes in the Easy42 mesh network with their configuration, IP addresses, tags, ASN, and status"),
		mcp.WithTitleAnnotation("List Nodes"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[ListNodesOutput](),
		mcp.WithString("tag", mcp.Description("Optional tag filter (e.g. 'edge', 'gateway')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodes := mgr.GetNodes()
		tagFilter := strings.TrimSpace(req.GetString("tag", ""))
		if tagFilter != "" {
			var filtered []config.Node
			for _, n := range nodes {
				for _, t := range n.Tags {
					if strings.EqualFold(t, tagFilter) {
						filtered = append(filtered, n)
						break
					}
				}
			}
			nodes = filtered
		}
		return jsonResult(ListNodesOutput{Nodes: nodes})
	})

	// 2. get_node
	s.AddTool(mcp.NewTool("get_node",
		mcp.WithDescription("Get detailed configuration, generated BIRD BGP config, nftables firewall rules, and status for a specific node"),
		mcp.WithTitleAnnotation("Get Node"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[GetNodeOutput](),
		mcp.WithString("name", mcp.Required(), mcp.Description("The name of the node")),
		mcp.WithBoolean("include_bird", mcp.Description("Include rendered BIRD BGP routing daemon configuration")),
		mcp.WithBoolean("include_nftables", mcp.Description("Include rendered nftables firewall configuration")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return errorResult(err)
		}
		node := mgr.FindNode(name)
		if node == nil {
			return errorResult(fmt.Errorf("node %q not found", name))
		}

		result := GetNodeOutput{
			Node: node,
		}

		if req.GetBool("include_bird", false) {
			birdCfg, err := mgr.GenerateBirdConfig(name)
			if err != nil {
				result.BirdError = err.Error()
			} else {
				result.BirdConfig = birdCfg
			}
		}

		if req.GetBool("include_nftables", false) {
			nftCfg, err := mgr.GenerateNftablesConfig(name)
			if err != nil {
				result.NftablesError = err.Error()
			} else {
				result.NftablesConfig = nftCfg
			}
		}

		return jsonResult(result)
	})

	// 3. add_node
	s.AddTool(mcp.NewTool("add_node",
		mcp.WithDescription("Add a new node to the Easy42 cluster control plane. Requires full-control token."),
		mcp.WithTitleAnnotation("Add Node"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[NodeMutationOutput](),
		mcp.WithString("name", mcp.Required(), mcp.Description("Unique node identifier/hostname")),
		mcp.WithString("host", mcp.Description("SSH host / alias / IP address for remote management")),
		mcp.WithString("ip", mcp.Description("Mesh IPv4 address (e.g. 192.168.100.1)")),
		mcp.WithString("ip6", mcp.Description("Mesh IPv6 address (e.g. fd42:a159:f9f0::1)")),
		mcp.WithInteger("asn", mcp.Description("Autonomous System Number (BGP ASN). Defaults to cluster default")),
		mcp.WithArray("tags", mcp.WithStringItems(), mcp.Description("List of node tags (e.g. ['core', 'us-east'])")),
		mcp.WithString("caption", mcp.Description("Short description or location of the node")),
		mcp.WithString("mode", mcp.Description("Management mode: 'ssh' (default) or 'agent'")),
		mcp.WithBoolean("route_reflector", mcp.Description("Set to true if node acts as BGP Route Reflector for internal iBGP peers")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		name, err := req.RequireString("name")
		if err != nil {
			return errorResult(err)
		}

		node := config.Node{
			Name:           name,
			Host:           req.GetString("host", ""),
			IP:             req.GetString("ip", ""),
			IP6:            req.GetString("ip6", ""),
			ASN:            uint64(req.GetInt("asn", 0)),
			Tags:           req.GetStringSlice("tags", nil),
			Caption:        req.GetString("caption", ""),
			Mode:           req.GetString("mode", "ssh"),
			RouteReflector: req.GetBool("route_reflector", false),
		}

		if err := mgr.AddNode(node); err != nil {
			return errorResult(err)
		}
		saved := mgr.FindNode(name)
		if saved == nil {
			saved = &node
		}
		return jsonResult(NodeMutationOutput{
			Message: fmt.Sprintf("Node %q successfully created", name),
			Node:    *saved,
		})
	})

	// 4. update_node
	s.AddTool(mcp.NewTool("update_node",
		mcp.WithDescription("Update an existing node configuration. Requires full-control token."),
		mcp.WithTitleAnnotation("Update Node"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[NodeMutationOutput](),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name of the node to update")),
		mcp.WithString("host", mcp.Description("SSH host / alias / IP address")),
		mcp.WithString("ip", mcp.Description("Mesh IPv4 address")),
		mcp.WithString("ip6", mcp.Description("Mesh IPv6 address")),
		mcp.WithInteger("asn", mcp.Description("BGP ASN")),
		mcp.WithArray("tags", mcp.WithStringItems(), mcp.Description("List of node tags")),
		mcp.WithString("caption", mcp.Description("Short description")),
		mcp.WithString("mode", mcp.Description("Management mode: 'ssh' or 'agent'")),
		mcp.WithBoolean("route_reflector", mcp.Description("Act as BGP Route Reflector")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		name, err := req.RequireString("name")
		if err != nil {
			return errorResult(err)
		}
		existing := mgr.FindNode(name)
		if existing == nil {
			return errorResult(fmt.Errorf("node %q not found", name))
		}

		node := *existing
		if host := req.GetString("host", ""); host != "" {
			node.Host = host
		}
		if ip := req.GetString("ip", ""); ip != "" {
			node.IP = ip
		}
		if ip6 := req.GetString("ip6", ""); ip6 != "" {
			node.IP6 = ip6
		}
		if asn := req.GetInt("asn", 0); asn > 0 {
			node.ASN = uint64(asn)
		}
		if tags := req.GetStringSlice("tags", nil); tags != nil {
			node.Tags = tags
		}
		if caption := req.GetString("caption", ""); caption != "" {
			node.Caption = caption
		}
		if mode := req.GetString("mode", ""); mode != "" {
			node.Mode = mode
		}
		if args := req.GetArguments(); args != nil {
			if _, ok := args["route_reflector"]; ok {
				node.RouteReflector = req.GetBool("route_reflector", node.RouteReflector)
			}
		}

		if err := mgr.UpdateNode(name, node); err != nil {
			return errorResult(err)
		}
		updated := mgr.FindNode(name)
		if updated == nil {
			updated = &node
		}
		return jsonResult(NodeMutationOutput{
			Message: fmt.Sprintf("Node %q successfully updated", name),
			Node:    *updated,
		})
	})

	// 5. rename_node
	s.AddTool(mcp.NewTool("rename_node",
		mcp.WithDescription("Rename a node across the entire cluster, updating all link connections, policies, and tasks. Requires full-control token."),
		mcp.WithTitleAnnotation("Rename Node"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[NodeMutationOutput](),
		mcp.WithString("old_name", mcp.Required(), mcp.Description("Current name of the node")),
		mcp.WithString("new_name", mcp.Required(), mcp.Description("New unique name for the node")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		oldName, err := req.RequireString("old_name")
		if err != nil {
			return errorResult(err)
		}
		newName, err := req.RequireString("new_name")
		if err != nil {
			return errorResult(err)
		}

		renamed, err := mgr.RenameNode(oldName, newName)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(NodeMutationOutput{
			Message: fmt.Sprintf("Node renamed from %q to %q successfully", oldName, newName),
			Node:    *renamed,
		})
	})

	// 6. delete_node
	s.AddTool(mcp.NewTool("delete_node",
		mcp.WithDescription("Delete a node from the cluster and tear down all associated peer links. Requires full-control token."),
		mcp.WithTitleAnnotation("Delete Node"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[SimpleMessageOutput](),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name of the node to remove")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		name, err := req.RequireString("name")
		if err != nil {
			return errorResult(err)
		}
		if err := mgr.DeleteNode(name); err != nil {
			return errorResult(err)
		}
		return successText(fmt.Sprintf("Node %q and its links were successfully removed from cluster", name))
	})

	// 7. probe_node
	s.AddTool(mcp.NewTool("probe_node",
		mcp.WithDescription("Probe an endpoint or host for reachability, ping round-trip time, and SSH connectivity"),
		mcp.WithTitleAnnotation("Probe Node"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithOutputSchema[ssh.ProbeResult](),
		mcp.WithString("host", mcp.Required(), mcp.Description("Host, domain, or IP address to test")),
		mcp.WithString("node_name", mcp.Description("Optional node name to test SSH key alias")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		host, err := req.RequireString("host")
		if err != nil {
			return errorResult(err)
		}
		nodeName := req.GetString("node_name", "")
		res, err := mgr.ProbeHost(host, nodeName)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(res)
	})

	// 8. refresh_node_status
	s.AddTool(mcp.NewTool("refresh_node_status",
		mcp.WithDescription("Trigger an active ping/reachability check for a node and update cached status. Requires full-control token."),
		mcp.WithTitleAnnotation("Refresh Node Status"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithOutputSchema[config.NodeStatus](),
		mcp.WithString("name", mcp.Required(), mcp.Description("Node name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		name, err := req.RequireString("name")
		if err != nil {
			return errorResult(err)
		}
		status, err := mgr.RefreshNodeStatus(name)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(status)
	})

	// 9. restart_node_service
	s.AddTool(mcp.NewTool("restart_node_service",
		mcp.WithDescription("Restart WireGuard interfaces, BIRD routing daemon, or a specific interface on a node. Requires full-control token."),
		mcp.WithTitleAnnotation("Restart Node Service"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithOutputSchema[CommandOutput](),
		mcp.WithString("name", mcp.Required(), mcp.Description("Node name")),
		mcp.WithString("service", mcp.Required(), mcp.Description("Service to restart: 'wireguard', 'bird', or 'interface'")),
		mcp.WithString("interface", mcp.Description("Interface name (required when service is 'interface')")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		name, err := req.RequireString("name")
		if err != nil {
			return errorResult(err)
		}
		service, err := req.RequireString("service")
		if err != nil {
			return errorResult(err)
		}

		var output string
		switch strings.ToLower(service) {
		case "wireguard", "wg":
			output, err = mgr.RestartNodeWireGuardInterfaces(name)
		case "bird", "bgp":
			output, err = mgr.RestartNodeBird(name)
		case "interface", "iface":
			iface := req.GetString("interface", "")
			if iface == "" {
				return errorResult(fmt.Errorf("parameter 'interface' is required when service is 'interface'"))
			}
			output, err = mgr.RestartNodeInterface(name, iface)
		default:
			return errorResult(fmt.Errorf("unknown service %q. Must be 'wireguard', 'bird', or 'interface'", service))
		}

		if err != nil {
			return errorResult(err)
		}
		return jsonResult(CommandOutput{
			Message: fmt.Sprintf("Service %s restarted on node %s", service, name),
			Output:  output,
		})
	})

	// 10. get_node_telemetry
	s.AddTool(mcp.NewTool("get_node_telemetry",
		mcp.WithDescription("Get recent CPU, memory, load, and interface traffic telemetry collected by Easy42 agent for a node"),
		mcp.WithTitleAnnotation("Get Node Telemetry"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[NodeTelemetryOutput](),
		mcp.WithString("name", mcp.Required(), mcp.Description("Node name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return errorResult(err)
		}
		node := mgr.FindNode(name)
		if node == nil {
			return errorResult(fmt.Errorf("node %q not found", name))
		}

		var telemetry *agentpb.TelemetryReport
		var connected bool
		var lastSeenPtr *time.Time
		if hub := mgr.AgentHub(); hub != nil {
			telemetry = hub.GetTelemetry(name)
			connected = hub.IsConnected(name)
			lastSeen := hub.GetLastSeen(name)
			if !lastSeen.IsZero() {
				lastSeenPtr = &lastSeen
			}
		}

		return jsonResult(NodeTelemetryOutput{
			Node:      name,
			Connected: connected,
			LastSeen:  lastSeenPtr,
			Telemetry: telemetry,
		})
	})

	// 11. get_fleet_live_status
	s.AddTool(mcp.NewTool("get_fleet_live_status",
		mcp.WithDescription("Get real-time fleet metrics summary and online/reachability status for all nodes in the mesh"),
		mcp.WithTitleAnnotation("Get Fleet Live Status"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[FleetLiveStatusOutput](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		summary, nodes, err := mgr.GetFleetLiveStatus()
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(FleetLiveStatusOutput{
			Summary: summary,
			Nodes:   nodes,
		})
	})
}
