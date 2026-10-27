package mcp

import (
	"context"
	"fmt"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerCronTaskTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. list_cron_tasks
	s.AddTool(mcp.NewTool("list_cron_tasks",
		mcp.WithDescription("List all scheduled automated cron tasks (e.g. periodic ping, latency monitoring) configured across nodes"),
		mcp.WithTitleAnnotation("List Cron Tasks"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tasks, err := mgr.GetCronTasks()
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(tasks)
	})

	// 2. create_cron_task
	s.AddTool(mcp.NewTool("create_cron_task",
		mcp.WithDescription("Schedule a new recurring automated task on one or more nodes. Requires full-control token."),
		mcp.WithTitleAnnotation("Create Cron Task"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("id", mcp.Required(), mcp.Description("Unique cron task ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Descriptive name")),
		mcp.WithString("task_type", mcp.Required(), mcp.Description("Task type: 'ping' or 'tcp_ping'")),
		mcp.WithString("cron_expr", mcp.Required(), mcp.Description("Standard cron expression (e.g. '*/5 * * * *')")),
		mcp.WithArray("nodes", mcp.WithStringItems(), mcp.Required(), mcp.Description("Node names that should execute this task")),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target host or IP address to test")),
		mcp.WithBoolean("enabled", mcp.Description("Enable task immediately (default true)")),
		mcp.WithString("description", mcp.Description("Optional description")),
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
		taskType, err := req.RequireString("task_type")
		if err != nil {
			return errorResult(err)
		}
		cronExpr, err := req.RequireString("cron_expr")
		if err != nil {
			return errorResult(err)
		}
		nodes, err := req.RequireStringSlice("nodes")
		if err != nil {
			return errorResult(err)
		}
		target, err := req.RequireString("target")
		if err != nil {
			return errorResult(err)
		}

		enabled := true
		if args := req.GetArguments(); args != nil {
			if _, ok := args["enabled"]; ok {
				enabled = req.GetBool("enabled", true)
			}
		}

		task := config.CronTask{
			ID:          id,
			Name:        name,
			Description: req.GetString("description", ""),
			TaskType:    taskType,
			CronExpr:    cronExpr,
			Nodes:       nodes,
			Params: map[string]string{
				"target": target,
			},
			Enabled: enabled,
		}

		saved, err := mgr.SaveCronTask(task)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message": fmt.Sprintf("Cron task %q created successfully", id),
			"task":    saved,
		})
	})

	// 3. update_cron_task
	s.AddTool(mcp.NewTool("update_cron_task",
		mcp.WithDescription("Update an existing cron task schedule, targets, or parameters. Requires full-control token."),
		mcp.WithTitleAnnotation("Update Cron Task"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("id", mcp.Required(), mcp.Description("Cron task ID")),
		mcp.WithString("name", mcp.Description("Updated name")),
		mcp.WithString("cron_expr", mcp.Description("Updated cron expression")),
		mcp.WithArray("nodes", mcp.WithStringItems(), mcp.Description("Updated target node names")),
		mcp.WithString("target", mcp.Description("Updated target host/IP")),
		mcp.WithBoolean("enabled", mcp.Description("Enable or disable")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}

		tasks, err := mgr.GetCronTasks()
		if err != nil {
			return errorResult(err)
		}

		var targetTask *config.CronTask
		for i := range tasks {
			if tasks[i].ID == id {
				targetTask = &tasks[i]
				break
			}
		}
		if targetTask == nil {
			return errorResult(fmt.Errorf("cron task %q not found", id))
		}

		task := *targetTask
		if name := req.GetString("name", ""); name != "" {
			task.Name = name
		}
		if expr := req.GetString("cron_expr", ""); expr != "" {
			task.CronExpr = expr
		}
		if nodes := req.GetStringSlice("nodes", nil); nodes != nil {
			task.Nodes = nodes
		}
		if target := req.GetString("target", ""); target != "" {
			if task.Params == nil {
				task.Params = map[string]string{}
			}
			task.Params["target"] = target
		}
		if args := req.GetArguments(); args != nil {
			if _, ok := args["enabled"]; ok {
				task.Enabled = req.GetBool("enabled", task.Enabled)
			}
		}

		saved, err := mgr.SaveCronTask(task)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message": fmt.Sprintf("Cron task %q updated successfully", id),
			"task":    saved,
		})
	})

	// 4. toggle_cron_task
	s.AddTool(mcp.NewTool("toggle_cron_task",
		mcp.WithDescription("Toggle the active/disabled status of a scheduled cron task. Requires full-control token."),
		mcp.WithTitleAnnotation("Toggle Cron Task"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("id", mcp.Required(), mcp.Description("Cron task ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		task, err := mgr.ToggleCronTask(id)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(map[string]any{
			"message": fmt.Sprintf("Cron task %q status toggled to enabled=%t", id, task.Enabled),
			"task":    task,
		})
	})

	// 5. delete_cron_task
	s.AddTool(mcp.NewTool("delete_cron_task",
		mcp.WithDescription("Delete a scheduled cron task. Requires full-control token."),
		mcp.WithTitleAnnotation("Delete Cron Task"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("id", mcp.Required(), mcp.Description("Cron task ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		id, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		if err := mgr.DeleteCronTask(id); err != nil {
			return errorResult(err)
		}
		return successText(fmt.Sprintf("Cron task %q deleted successfully", id))
	})

	// 6. get_node_cron_task_results
	s.AddTool(mcp.NewTool("get_node_cron_task_results",
		mcp.WithDescription("Retrieve execution history, metrics (latency, packet loss), and logs for cron tasks executed on a node"),
		mcp.WithTitleAnnotation("Get Node Cron Task Results"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithString("task_id", mcp.Description("Optional specific cron task ID")),
		mcp.WithString("range", mcp.Description("Time range (e.g. '1h', '24h', '7d'). Default '24h'")),
		mcp.WithInteger("limit", mcp.Description("Maximum records to return (default 100)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		node, err := req.RequireString("node")
		if err != nil {
			return errorResult(err)
		}
		taskID := req.GetString("task_id", "")
		rangeStr := req.GetString("range", "24h")
		limit := req.GetInt("limit", 100)

		results, err := mgr.GetNodeCronTasksResults(node, taskID, rangeStr, limit)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(results)
	})
}
