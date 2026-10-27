package mcp

import (
	"context"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerSyncTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. preview_sync
	s.AddTool(mcp.NewTool("preview_sync",
		mcp.WithDescription("Preview planned WireGuard, BIRD BGP, and firewall configuration changes across all or target nodes before pushing"),
		mcp.WithTitleAnnotation("Preview Sync"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[PreviewSyncOutput](),
		mcp.WithArray("nodes", mcp.WithStringItems(), mcp.Description("Optional list of specific node names to inspect")),
		mcp.WithString("scope", mcp.Description("Sync scope filter: 'all', 'wireguard', 'bird', or 'auto'")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodes := req.GetStringSlice("nodes", nil)
		scope := req.GetString("scope", "")

		actions, err := mgr.PlanSyncWithScope(scope, nodes...)
		if err != nil {
			return errorResult(err)
		}
		if actions == nil {
			actions = []config.SyncAction{}
		}
		return jsonResult(PreviewSyncOutput{Actions: actions})
	})

	// 2. execute_sync
	s.AddTool(mcp.NewTool("execute_sync",
		mcp.WithDescription("Execute synchronization to deploy WireGuard, BIRD, nftables, and hosts configs to remote nodes via SSH or agent. Requires full-control token."),
		mcp.WithTitleAnnotation("Execute Sync"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithOutputSchema[ExecuteSyncOutput](),
		mcp.WithArray("nodes", mcp.WithStringItems(), mcp.Description("Optional list of node names to sync. If omitted, syncs all modified/eligible nodes")),
		mcp.WithString("scope", mcp.Description("Optional sync scope: 'all', 'wireguard', 'bird', or 'auto'")),
		mcp.WithBoolean("force", mcp.Description("Force regeneration and push even if no changes are detected")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		nodes := req.GetStringSlice("nodes", nil)
		scope := req.GetString("scope", "")
		force := req.GetBool("force", false)

		results, err := mgr.ExecuteSyncNodesWithScope(force, scope, nodes...)
		if err != nil {
			return errorResult(err)
		}
		if results == nil {
			results = []config.SyncResult{}
		}
		return jsonResult(ExecuteSyncOutput{Results: results})
	})

	// 3. get_sync_status
	s.AddTool(mcp.NewTool("get_sync_status",
		mcp.WithDescription("Get the current synchronization status of the cluster, checking whether active node states match desired configuration"),
		mcp.WithTitleAnnotation("Get Sync Status"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[SyncStatusOutput](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		lastSync, results := mgr.GetLastSyncResults()
		if results == nil {
			results = []config.SyncResult{}
		}
		return jsonResult(SyncStatusOutput{
			LastSync: lastSync,
			Results:  results,
		})
	})

	// 4. get_cluster_state
	s.AddTool(mcp.NewTool("get_cluster_state",
		mcp.WithDescription("Get the entire cluster runtime state including reported node statuses, agent sessions, and link states"),
		mcp.WithTitleAnnotation("Get Cluster State"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[config.NetworkState](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		state := mgr.GetNetworkState()
		return jsonResult(state)
	})
}
