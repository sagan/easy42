package mcp

import (
	"context"
	"fmt"

	"easy42/internal/config"
	"easy42/internal/engine"
	"easy42/internal/lookingglass"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerLookingGlassTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. list_looking_glass_tasks
	s.AddTool(mcp.NewTool("list_looking_glass_tasks",
		mcp.WithDescription("List available Looking Glass diagnostic tasks (ping, traceroute, BIRD show route, BIRD show protocols, etc.)"),
		mcp.WithTitleAnnotation("List Looking Glass Tasks"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tasks, err := mgr.GetLookingGlassTasks()
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(tasks)
	})

	// 2. run_looking_glass
	s.AddTool(mcp.NewTool("run_looking_glass",
		mcp.WithDescription("Execute diagnostic commands (ping, traceroute, bird route show, mtr) on one or more nodes. Requires full-control token."),
		mcp.WithTitleAnnotation("Run Looking Glass"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithArray("nodes", mcp.WithStringItems(), mcp.Required(), mcp.Description("Node names to execute the diagnostic from")),
		mcp.WithString("task_id", mcp.Description("Task template ID (e.g. 'ping', 'traceroute', 'bird_route')")),
		mcp.WithString("target", mcp.Description("Target host or IP address to diagnose")),
		mcp.WithString("custom_command", mcp.Description("Direct sanitized command to run if task_id is not specified")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		nodes, err := req.RequireStringSlice("nodes")
		if err != nil {
			return errorResult(err)
		}
		taskID := req.GetString("task_id", "")
		target := req.GetString("target", "")
		customCmd := req.GetString("custom_command", "")

		runReq := lookingglass.RunRequest{
			Nodes:  nodes,
			TaskID: taskID,
			Params: map[string]string{},
		}

		if target != "" {
			runReq.Params["target"] = target
			runReq.Params["host"] = target
			runReq.Params["ip"] = target
		}

		if taskID == "" && customCmd != "" {
			runReq.AdHoc = true
			runReq.CustomCommand = customCmd
			runReq.CustomParser = "raw"
		}

		resp, err := mgr.RunLookingGlass(ctx, runReq)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(resp)
	})

	// 3. save_looking_glass_task
	s.AddTool(mcp.NewTool("save_looking_glass_task",
		mcp.WithDescription("Save or update a Looking Glass task template. Requires full-control token."),
		mcp.WithTitleAnnotation("Save Looking Glass Task"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("id", mcp.Required(), mcp.Description("Unique task ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Human-readable task name")),
		mcp.WithString("command", mcp.Required(), mcp.Description("CLI command template")),
		mcp.WithString("description", mcp.Description("Optional description")),
		mcp.WithString("parser", mcp.Description("Parser type ('raw', 'ping', 'bird_route', 'bird_protocols')")),
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
		cmd, err := req.RequireString("command")
		if err != nil {
			return errorResult(err)
		}

		task := config.LookingGlassTask{
			ID:          id,
			Name:        name,
			CommandTmpl: cmd,
			Description: req.GetString("description", ""),
			Parser:      req.GetString("parser", "raw"),
		}

		saved, err := mgr.SaveLookingGlassTask(task)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message": fmt.Sprintf("Looking Glass task %q saved successfully", id),
			"task":    saved,
		})
	})

	// 4. delete_looking_glass_task
	s.AddTool(mcp.NewTool("delete_looking_glass_task",
		mcp.WithDescription("Delete a Looking Glass task template. Requires full-control token."),
		mcp.WithTitleAnnotation("Delete Looking Glass Task"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("id", mcp.Required(), mcp.Description("Task template ID to delete")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		if err := mgr.DeleteLookingGlassTask(id); err != nil {
			return errorResult(err)
		}
		return successText(fmt.Sprintf("Looking Glass task %q deleted successfully", id))
	})
}
