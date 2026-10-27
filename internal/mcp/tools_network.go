package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerNetworkTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. get_network_settings
	s.AddTool(mcp.NewTool("get_network_settings",
		mcp.WithDescription("Get global cluster network settings (Network Name, ASN, permitted prefixes, disallowed networks, FlapAlerted configuration)"),
		mcp.WithTitleAnnotation("Get Network Settings"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		settings := mgr.GetNetworkSettings()
		return jsonResult(settings)
	})

	// 2. update_network_settings
	s.AddTool(mcp.NewTool("update_network_settings",
		mcp.WithDescription("Update global cluster network settings. Requires full-control token."),
		mcp.WithTitleAnnotation("Update Network Settings"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("name", mcp.Description("Network name (e.g. 'DN42 Mesh')")),
		mcp.WithInteger("public_asn", mcp.Description("Public / Network Autonomous System Number (ASN)")),
		mcp.WithArray("prefixes", mcp.WithStringItems(), mcp.Description("IP prefixes assigned to the mesh")),
		mcp.WithArray("disallowed_dn42_networks", mcp.WithStringItems(), mcp.Description("Disallowed CIDRs for dn42")),
		mcp.WithString("flapalerted_address", mcp.Description("FlapAlerted server IP address")),
		mcp.WithInteger("flapalerted_port", mcp.Description("FlapAlerted BGP listening port (default 1790)")),
		mcp.WithInteger("flapalerted_asn", mcp.Description("FlapAlerted BGP ASN (default 4224420000)")),
		mcp.WithString("flapalerted_url", mcp.Description("FlapAlerted dashboard base URL")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		current := mgr.GetNetworkSettings()

		if name := req.GetString("name", ""); name != "" {
			current.Name = name
		}
		if asn := req.GetInt("public_asn", 0); asn > 0 {
			current.PublicASN = uint64(asn)
		}
		if prefixes := req.GetStringSlice("prefixes", nil); prefixes != nil {
			current.Prefixes = prefixes
		}
		if disallowed := req.GetStringSlice("disallowed_dn42_networks", nil); disallowed != nil {
			current.DisallowedDN42Networks = disallowed
		}
		if faAddr := req.GetString("flapalerted_address", ""); faAddr != "" {
			current.FlapAlertedAddress = faAddr
		}
		if faPort := req.GetInt("flapalerted_port", 0); faPort > 0 {
			current.FlapAlertedPort = faPort
		}
		if faASN := req.GetInt("flapalerted_asn", 0); faASN > 0 {
			current.FlapAlertedASN = uint64(faASN)
		}
		if faURL := req.GetString("flapalerted_url", ""); faURL != "" {
			current.FlapAlertedURL = faURL
		}

		if err := mgr.UpdateNetworkSettings(current); err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message":  "Network settings updated successfully",
			"settings": mgr.GetNetworkSettings(),
		})
	})

	// 3. get_ip_blocks
	s.AddTool(mcp.NewTool("get_ip_blocks",
		mcp.WithDescription("Get visual grouping blocks on the network topology diagram"),
		mcp.WithTitleAnnotation("Get IP Blocks"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		blocks := mgr.GetBlocks()
		return jsonResult(blocks)
	})

	// 4. update_ip_blocks
	s.AddTool(mcp.NewTool("update_ip_blocks",
		mcp.WithDescription("Update visual topology grouping blocks. Requires full-control token."),
		mcp.WithTitleAnnotation("Update IP Blocks"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("blocks_json", mcp.Required(), mcp.Description("JSON array of Block objects")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		raw, err := req.RequireString("blocks_json")
		if err != nil {
			return errorResult(err)
		}

		var blocks []config.Block
		if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &blocks); err != nil {
			return errorResult(fmt.Errorf("invalid blocks JSON: %w", err))
		}

		if err := mgr.UpdateBlocks(blocks); err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message": "Topology blocks updated successfully",
			"blocks":  mgr.GetBlocks(),
		})
	})

	// 5. get_dns_config
	s.AddTool(mcp.NewTool("get_dns_config",
		mcp.WithDescription("Get Cloudflare DNS integration settings (Zone ID, Base Domain, automatic sync preferences)"),
		mcp.WithTitleAnnotation("Get DNS Config"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cfg := mgr.GetDNSConfig()
		display := cfg
		if display.APIToken != "" {
			display.APIToken = "******"
		}
		return jsonResult(display)
	})

	// 6. update_dns_config
	s.AddTool(mcp.NewTool("update_dns_config",
		mcp.WithDescription("Update Cloudflare DNS integration settings. Requires full-control token."),
		mcp.WithTitleAnnotation("Update DNS Config"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("zone_id", mcp.Description("Cloudflare DNS Zone ID")),
		mcp.WithString("api_token", mcp.Description("Cloudflare API Token")),
		mcp.WithString("base_domain", mcp.Description("Base domain (e.g. dn42.example.com)")),
		mcp.WithBoolean("publish_ipv6_own_name", mcp.Description("Publish IPv6 records under each node's own name")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		current := mgr.GetDNSConfig()

		if zoneID := req.GetString("zone_id", ""); zoneID != "" {
			current.ZoneID = zoneID
		}
		if token := req.GetString("api_token", ""); token != "" {
			current.APIToken = token
		}
		if domain := req.GetString("base_domain", ""); domain != "" {
			current.BaseDomain = domain
		}
		if args := req.GetArguments(); args != nil {
			if _, ok := args["publish_ipv6_own_name"]; ok {
				current.PublishIPv6OwnName = req.GetBool("publish_ipv6_own_name", current.PublishIPv6OwnName)
			}
		}

		if err := mgr.UpdateDNSConfig(current); err != nil {
			return errorResult(err)
		}
		return successText("DNS configuration updated successfully")
	})

	// 7. sync_dns
	s.AddTool(mcp.NewTool("sync_dns",
		mcp.WithDescription("Synchronize all cluster node IP addresses to Cloudflare DNS records. Requires full-control token."),
		mcp.WithTitleAnnotation("Sync DNS"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithBoolean("force", mcp.Description("Force overwrite existing remote DNS records")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		force := req.GetBool("force", false)
		res, err := mgr.SyncDNS(ctx, force)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(res)
	})
}
