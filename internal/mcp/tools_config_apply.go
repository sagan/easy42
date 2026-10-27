package mcp

import (
	"context"
	"fmt"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerConfigApplyTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. preview_config_apply
	s.AddTool(mcp.NewTool("preview_config_apply",
		mcp.WithDescription("Validate and dry-run declarative configuration changes (YAML or JSON) against the active cluster, computing an exact diff without modifying state"),
		mcp.WithString("content", mcp.Required(), mcp.Description("Declarative configuration patch snippet in YAML or JSON format")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		content, err := req.RequireString("content")
		if err != nil {
			return errorResult(err)
		}

		preview, err := mgr.PreviewApply(content)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(preview)
	})

	// 2. execute_config_apply
	s.AddTool(mcp.NewTool("execute_config_apply",
		mcp.WithDescription("Atomically apply declarative configuration changes (YAML or JSON) to the cluster. Requires full-control token."),
		mcp.WithString("content", mcp.Required(), mcp.Description("Declarative configuration patch in YAML or JSON")),
		mcp.WithBoolean("auto_sync", mcp.Description("Automatically synchronize generated WireGuard and BIRD configs to affected nodes")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		content, err := req.RequireString("content")
		if err != nil {
			return errorResult(err)
		}

		result, err := mgr.ApplyConfigChange(content)
		if err != nil {
			return errorResult(err)
		}

		if req.GetBool("auto_sync", false) && len(result.AffectedNodes) > 0 {
			go func(nodes []string) {
				_, _ = mgr.ExecuteSyncNodesWithScope(false, "auto", nodes...)
			}(result.AffectedNodes)
		}

		return jsonResult(result)
	})

	// 3. list_config_changes
	s.AddTool(mcp.NewTool("list_config_changes",
		mcp.WithDescription("List saved configuration patches and change templates"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		changes := mgr.ListConfigChanges()
		return jsonResult(changes)
	})

	// 4. get_config_change
	s.AddTool(mcp.NewTool("get_config_change",
		mcp.WithDescription("Get a saved configuration change patch by its ID"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Saved change ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		change, err := mgr.GetConfigChange(id)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(change)
	})

	// 5. save_config_change
	s.AddTool(mcp.NewTool("save_config_change",
		mcp.WithDescription("Save or update a named configuration change snippet. Requires full-control token."),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name of the configuration snippet")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Patch content in YAML or JSON")),
		mcp.WithString("description", mcp.Description("Optional description of the changes")),
		mcp.WithString("id", mcp.Description("Optional existing ID to update")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		name, err := req.RequireString("name")
		if err != nil {
			return errorResult(err)
		}
		content, err := req.RequireString("content")
		if err != nil {
			return errorResult(err)
		}

		cc := config.ConfigChange{
			ID:          req.GetString("id", ""),
			Name:        name,
			Content:     content,
			Description: req.GetString("description", ""),
		}

		saved, err := mgr.SaveConfigChange(cc)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message": "Configuration change saved successfully",
			"change":  saved,
		})
	})

	// 6. apply_saved_config_change
	s.AddTool(mcp.NewTool("apply_saved_config_change",
		mcp.WithDescription("Apply a previously saved configuration change snippet by ID. Requires full-control token."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Saved change ID")),
		mcp.WithBoolean("auto_sync", mcp.Description("Automatically synchronize configurations to affected nodes")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}

		result, err := mgr.ApplySavedConfigChange(id)
		if err != nil {
			return errorResult(err)
		}

		if req.GetBool("auto_sync", false) && len(result.AffectedNodes) > 0 {
			go func(nodes []string) {
				_, _ = mgr.ExecuteSyncNodesWithScope(false, "auto", nodes...)
			}(result.AffectedNodes)
		}

		return jsonResult(result)
	})

	// 7. delete_config_change
	s.AddTool(mcp.NewTool("delete_config_change",
		mcp.WithDescription("Delete a saved configuration change snippet. Requires full-control token."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Saved change ID to delete")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		if err := mgr.DeleteConfigChange(id); err != nil {
			return errorResult(err)
		}
		return successText(fmt.Sprintf("Saved configuration change %q deleted successfully", id))
	})
}
