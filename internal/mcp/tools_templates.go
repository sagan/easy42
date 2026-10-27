package mcp

import (
	"context"
	"fmt"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerTemplateTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. list_templates
	s.AddTool(mcp.NewTool("list_templates",
		mcp.WithDescription("List all custom and built-in configuration templates (for WireGuard, BIRD BGP, and nftables)"),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		templates := mgr.GetTemplates()
		return jsonResult(templates)
	})

	// 2. get_template
	s.AddTool(mcp.NewTool("get_template",
		mcp.WithDescription("Get the source code and metadata of a configuration template"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Template ID (e.g. 'default_bird', 'default_wg', or custom ID)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		tmpl, err := mgr.GetTemplate(id)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(tmpl)
	})

	// 3. create_template
	s.AddTool(mcp.NewTool("create_template",
		mcp.WithDescription("Create a custom configuration template. Requires full-control token."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Unique template identifier")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Template name")),
		mcp.WithString("type", mcp.Required(), mcp.Description("Template type: 'bird', 'wg', or 'nft'")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Go text/template source code")),
		mcp.WithString("description", mcp.Description("Optional template description")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		name, err := req.RequireString("name")
		if err != nil {
			return errorResult(err)
		}
		tmplType, err := req.RequireString("type")
		if err != nil {
			return errorResult(err)
		}
		content, err := req.RequireString("content")
		if err != nil {
			return errorResult(err)
		}

		tmpl := config.ConfigTemplate{
			ID:          id,
			Name:        name,
			Type:        tmplType,
			Content:     content,
			Description: req.GetString("description", ""),
		}

		created, err := mgr.CreateTemplate(tmpl)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message":  fmt.Sprintf("Template %q created successfully", id),
			"template": created,
		})
	})

	// 4. update_template
	s.AddTool(mcp.NewTool("update_template",
		mcp.WithDescription("Update an existing custom template. Requires full-control token."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Template ID")),
		mcp.WithString("name", mcp.Description("Updated template name")),
		mcp.WithString("type", mcp.Description("Updated template type: 'bird', 'wg', or 'nft'")),
		mcp.WithString("content", mcp.Description("Updated template source code")),
		mcp.WithString("description", mcp.Description("Updated description")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		existing, err := mgr.GetTemplate(id)
		if err != nil {
			return errorResult(err)
		}

		tmpl := *existing
		if name := req.GetString("name", ""); name != "" {
			tmpl.Name = name
		}
		if t := req.GetString("type", ""); t != "" {
			tmpl.Type = t
		}
		if content := req.GetString("content", ""); content != "" {
			tmpl.Content = content
		}
		if desc := req.GetString("description", ""); desc != "" {
			tmpl.Description = desc
		}

		updated, err := mgr.UpdateTemplate(id, tmpl)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message":  fmt.Sprintf("Template %q updated successfully", id),
			"template": updated,
		})
	})

	// 5. delete_template
	s.AddTool(mcp.NewTool("delete_template",
		mcp.WithDescription("Delete a custom configuration template. Requires full-control token."),
		mcp.WithString("id", mcp.Required(), mcp.Description("Template ID to delete")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		if err := mgr.DeleteTemplate(id); err != nil {
			return errorResult(err)
		}
		return successText(fmt.Sprintf("Template %q deleted successfully", id))
	})
}
