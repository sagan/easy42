package mcp

import (
	"context"
	"fmt"
	"strings"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerLinkTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. list_links
	s.AddTool(mcp.NewTool("list_links",
		mcp.WithDescription("List all peer connection links (WireGuard / manual) between nodes in the mesh"),
		mcp.WithTitleAnnotation("List Links"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[ListLinksOutput](),
		mcp.WithString("node", mcp.Description("Optional filter: return only links connected to this node name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		links := mgr.GetLinks()
		nodeFilter := strings.TrimSpace(req.GetString("node", ""))
		if nodeFilter != "" {
			var filtered []config.Link
			for _, l := range links {
				if strings.EqualFold(l.From.Name, nodeFilter) || strings.EqualFold(l.To.Name, nodeFilter) {
					filtered = append(filtered, l)
				}
			}
			links = filtered
		}
		if links == nil {
			links = []config.Link{}
		}
		return jsonResult(ListLinksOutput{Links: links})
	})

	// 2. get_link
	s.AddTool(mcp.NewTool("get_link",
		mcp.WithDescription("Get connection details for a link between two specific nodes"),
		mcp.WithTitleAnnotation("Get Link"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[config.Link](),
		mcp.WithString("from", mcp.Required(), mcp.Description("First node name")),
		mcp.WithString("to", mcp.Required(), mcp.Description("Second node name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		from, err := req.RequireString("from")
		if err != nil {
			return errorResult(err)
		}
		to, err := req.RequireString("to")
		if err != nil {
			return errorResult(err)
		}

		links := mgr.GetLinks()
		for _, l := range links {
			if (strings.EqualFold(l.From.Name, from) && strings.EqualFold(l.To.Name, to)) ||
				(strings.EqualFold(l.From.Name, to) && strings.EqualFold(l.To.Name, from)) {
				return jsonResult(l)
			}
		}
		return errorResult(fmt.Errorf("link between %q and %q not found", from, to))
	})

	// 3. add_link
	s.AddTool(mcp.NewTool("add_link",
		mcp.WithDescription("Create a point-to-point WireGuard link between two nodes. Generates cryptographic keys and IP addresses. Requires full-control token."),
		mcp.WithTitleAnnotation("Add Link"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[LinkMutationOutput](),
		mcp.WithString("from", mcp.Required(), mcp.Description("First node name")),
		mcp.WithString("to", mcp.Required(), mcp.Description("Second node name")),
		mcp.WithInteger("from_port", mcp.Description("Listen port on the first node")),
		mcp.WithInteger("to_port", mcp.Description("Listen port on the second node")),
		mcp.WithInteger("mtu", mcp.Description("WireGuard interface MTU (default 1420)")),
		mcp.WithInteger("cost", mcp.Description("Routing cost metric for BIRD routing calculations")),
		mcp.WithBoolean("assign_ipv4", mcp.Description("Assign IPv4 link-local address (169.254.x.x)")),
		mcp.WithBoolean("disable_bfd", mcp.Description("Disable BFD session for this link (default false)")),
		mcp.WithString("note", mcp.Description("Optional link-level markdown note")),
		mcp.WithArray("tags", mcp.WithStringItems(), mcp.Description("Optional link tags")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		from, err := req.RequireString("from")
		if err != nil {
			return errorResult(err)
		}
		to, err := req.RequireString("to")
		if err != nil {
			return errorResult(err)
		}

		fromPort := req.GetInt("from_port", 0)
		toPort := req.GetInt("to_port", 0)
		mtu := req.GetInt("mtu", 0)
		cost := req.GetInt("cost", 0)
		assignIPv4 := req.GetBool("assign_ipv4", false)
		disableBFD := req.GetBool("disable_bfd", false)
		note := req.GetString("note", "")
		tags := req.GetStringSlice("tags", nil)

		var fromEnd, toEnd *config.LinkEnd
		if cost > 0 {
			fromEnd = &config.LinkEnd{Cost: cost}
			toEnd = &config.LinkEnd{Cost: cost}
		}

		link, err := mgr.AddLinkWithAllOptionsAndNote(from, to, fromEnd, toEnd, tags, assignIPv4, disableBFD, note, mtu, mtu)
		if err != nil {
			// Fallback to basic AddLink if AddLinkWithAllOptions failed
			link, err = mgr.AddLink(from, to, fromPort, toPort, tags, mtu, mtu)
		}
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(LinkMutationOutput{
			Message: fmt.Sprintf("Link between %q and %q created successfully", from, to),
			Link:    *link,
		})
	})

	// 4. create_mesh_links
	s.AddTool(mcp.NewTool("create_mesh_links",
		mcp.WithDescription("Create a full-mesh topology interconnecting all specified nodes (or all cluster nodes if empty). Requires full-control token."),
		mcp.WithTitleAnnotation("Create Mesh Links"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[CreateMeshLinksOutput](),
		mcp.WithArray("nodes", mcp.WithStringItems(), mcp.Description("List of node names to interconnect. If empty, meshes all cluster nodes")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		nodes := req.GetStringSlice("nodes", nil)
		links, err := mgr.CreateFullMesh(nodes)
		if err != nil {
			return errorResult(err)
		}
		if links == nil {
			links = []*config.Link{}
		}
		return jsonResult(CreateMeshLinksOutput{
			Message: fmt.Sprintf("Generated %d mesh links", len(links)),
			Links:   links,
		})
	})

	// 5. update_link
	s.AddTool(mcp.NewTool("update_link",
		mcp.WithDescription("Update link parameters (MTU, routing cost, tags, policy) between two nodes. Requires full-control token."),
		mcp.WithTitleAnnotation("Update Link"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[LinkMutationOutput](),
		mcp.WithString("from", mcp.Required(), mcp.Description("First node name")),
		mcp.WithString("to", mcp.Required(), mcp.Description("Second node name")),
		mcp.WithInteger("mtu", mcp.Description("MTU override")),
		mcp.WithInteger("cost", mcp.Description("Routing cost metric")),
		mcp.WithString("policy", mcp.Description("Network policy ID for routing")),
		mcp.WithBoolean("disable_bfd", mcp.Description("Disable BFD session for this link")),
		mcp.WithString("note", mcp.Description("Optional link-level markdown note (use __CLEAR__ to unset)")),
		mcp.WithArray("tags", mcp.WithStringItems(), mcp.Description("Link tags")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		from, err := req.RequireString("from")
		if err != nil {
			return errorResult(err)
		}
		to, err := req.RequireString("to")
		if err != nil {
			return errorResult(err)
		}

		mtu := req.GetInt("mtu", 0)
		cost := req.GetInt("cost", 0)
		policy := req.GetString("policy", "")
		tags := req.GetStringSlice("tags", nil)
		var disableBFD *bool
		var notePtr *string
		if args := req.GetArguments(); args != nil {
			if val, ok := args["disable_bfd"].(bool); ok {
				disableBFD = &val
			}
			if val, ok := args["note"].(string); ok {
				notePtr = &val
			}
		}

		var fromEnd, toEnd *config.LinkEnd
		if cost > 0 || policy != "" || mtu > 0 {
			fromEnd = &config.LinkEnd{Cost: cost, Policy: policy, MTU: mtu}
			toEnd = &config.LinkEnd{Cost: cost, Policy: policy, MTU: mtu}
		}

		link, err := mgr.UpdateLinkWithAllOptions(from, to, fromEnd, toEnd, tags, nil, disableBFD, notePtr)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(LinkMutationOutput{
			Message: fmt.Sprintf("Link between %q and %q updated successfully", from, to),
			Link:    *link,
		})
	})

	// 6. delete_link
	s.AddTool(mcp.NewTool("delete_link",
		mcp.WithDescription("Delete the link between two nodes. Requires full-control token."),
		mcp.WithTitleAnnotation("Delete Link"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[SimpleMessageOutput](),
		mcp.WithString("from", mcp.Required(), mcp.Description("First node name")),
		mcp.WithString("to", mcp.Required(), mcp.Description("Second node name")),
		mcp.WithString("interface", mcp.Description("Optional specific interface name if multiple links exist")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		from, err := req.RequireString("from")
		if err != nil {
			return errorResult(err)
		}
		to, err := req.RequireString("to")
		if err != nil {
			return errorResult(err)
		}
		iface := req.GetString("interface", "")

		if err := mgr.DeleteLink(from, to, iface); err != nil {
			return errorResult(err)
		}
		return successText(fmt.Sprintf("Link between %q and %q successfully removed", from, to))
	})

	// 7. reset_link_psk
	s.AddTool(mcp.NewTool("reset_link_psk",
		mcp.WithDescription("Rotate or set the WireGuard Pre-Shared Key (PSK) for a link. Requires full-control token."),
		mcp.WithTitleAnnotation("Reset Link PSK"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[ResetLinkPskOutput](),
		mcp.WithString("from", mcp.Required(), mcp.Description("First node name")),
		mcp.WithString("to", mcp.Required(), mcp.Description("Second node name")),
		mcp.WithString("psk", mcp.Description("Custom base64 PSK. If omitted, a fresh secure random key is generated")),
		mcp.WithString("interface", mcp.Description("Optional interface name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		from, err := req.RequireString("from")
		if err != nil {
			return errorResult(err)
		}
		to, err := req.RequireString("to")
		if err != nil {
			return errorResult(err)
		}
		psk := req.GetString("psk", "")
		iface := req.GetString("interface", "")

		newPsk, err := mgr.ResetLinkPsk(from, to, iface, psk)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(ResetLinkPskOutput{
			Message: "WireGuard PSK reset successfully",
			From:    from,
			To:      to,
			Psk:     newPsk,
		})
	})

	// 8. reset_link_private_key
	s.AddTool(mcp.NewTool("reset_link_private_key",
		mcp.WithDescription("Rotate or regenerate the WireGuard private key for a link endpoint. Requires full-control token."),
		mcp.WithTitleAnnotation("Reset Link Private Key"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[ResetLinkPrivateKeyOutput](),
		mcp.WithString("from", mcp.Required(), mcp.Description("First node name")),
		mcp.WithString("to", mcp.Required(), mcp.Description("Second node name")),
		mcp.WithString("node", mcp.Required(), mcp.Description("Which node endpoint's key to reset")),
		mcp.WithString("interface", mcp.Description("Optional interface name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		from, err := req.RequireString("from")
		if err != nil {
			return errorResult(err)
		}
		to, err := req.RequireString("to")
		if err != nil {
			return errorResult(err)
		}
		node, err := req.RequireString("node")
		if err != nil {
			return errorResult(err)
		}
		iface := req.GetString("interface", "")

		newPriv, newPub, err := mgr.ResetLinkPrivateKey(from, to, node, iface, "")
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(ResetLinkPrivateKeyOutput{
			Message:   "WireGuard private key regenerated successfully",
			Node:      node,
			PublicKey: newPub,
			Note:      "Private key safely encrypted and updated in configuration",
			Private:   newPriv,
		})
	})
}
