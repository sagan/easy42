package mcp

import (
	"context"
	"fmt"

	"easy42/internal/engine"
	"easy42/tasks"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerDeviceHelperTaskTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. list_tasks
	s.AddTool(mcp.NewTool("list_tasks",
		mcp.WithDescription("List all installed device helper tasks (scripts for system tuning, service installs, and management)"),
		mcp.WithTitleAnnotation("List Tasks"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[ListTasksOutput](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tasksList, err := mgr.GetTasks()
		if err != nil {
			return errorResult(err)
		}
		if tasksList == nil {
			tasksList = []tasks.TaskMeta{}
		}
		return jsonResult(ListTasksOutput{Tasks: tasksList})
	})

	// 2. get_task_sources
	s.AddTool(mcp.NewTool("get_task_sources",
		mcp.WithDescription("Get script source code and files for a device helper task"),
		mcp.WithTitleAnnotation("Get Task Sources"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[TaskSourcesOutput](),
		mcp.WithString("id", mcp.Required(), mcp.Description("Task identifier")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		sources, err := mgr.GetTaskSources(taskID)
		if err != nil {
			return errorResult(err)
		}
		if sources == nil {
			sources = []tasks.TaskSourceFile{}
		}
		return jsonResult(TaskSourcesOutput{Sources: sources})
	})

	// 3. check_task_status
	s.AddTool(mcp.NewTool("check_task_status",
		mcp.WithDescription("Query the status/readiness of a device helper task across specific nodes"),
		mcp.WithTitleAnnotation("Check Task Status"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithOutputSchema[CheckTaskStatusOutput](),
		mcp.WithString("id", mcp.Required(), mcp.Description("Task identifier")),
		mcp.WithArray("nodes", mcp.WithStringItems(), mcp.Description("Target node names. If empty, checks all nodes")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		taskID, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		nodes := req.GetStringSlice("nodes", nil)
		results, err := mgr.CheckTaskStatus(ctx, taskID, nodes)
		if err != nil {
			return errorResult(err)
		}
		if results == nil {
			results = map[string]tasks.TaskStatusResult{}
		}
		return jsonResult(CheckTaskStatusOutput{Results: results})
	})

	// 4. run_task
	s.AddTool(mcp.NewTool("run_task",
		mcp.WithDescription("Execute a device helper task on one or more remote nodes. Requires full-control token."),
		mcp.WithTitleAnnotation("Run Task"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithOutputSchema[RunTaskOutput](),
		mcp.WithString("id", mcp.Required(), mcp.Description("Task identifier to execute")),
		mcp.WithArray("nodes", mcp.WithStringItems(), mcp.Description("Target node names")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		taskID, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		nodes := req.GetStringSlice("nodes", nil)
		results, err := mgr.RunTask(ctx, taskID, nodes)
		if err != nil {
			return errorResult(err)
		}
		if results == nil {
			results = map[string]tasks.TaskRunResult{}
		}
		return jsonResult(RunTaskOutput{Results: results})
	})

	// 5. get_online_tasks_manifest
	s.AddTool(mcp.NewTool("get_online_tasks_manifest",
		mcp.WithDescription("Fetch available tasks from the online Easy42 task repository"),
		mcp.WithTitleAnnotation("Get Online Tasks Manifest"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithOutputSchema[tasks.OnlineManifest](),
		mcp.WithString("source_url", mcp.Description("Optional custom repository URL")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sourceURL := req.GetString("source_url", "")
		manifest, err := mgr.GetOnlineManifest(ctx, sourceURL)
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(manifest)
	})

	// 6. install_online_task
	s.AddTool(mcp.NewTool("install_online_task",
		mcp.WithDescription("Download and install a task from the online task repository. Requires full-control token."),
		mcp.WithTitleAnnotation("Install Online Task"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
		mcp.WithOutputSchema[SimpleMessageOutput](),
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID from manifest")),
		mcp.WithString("source_url", mcp.Description("Optional custom repository source URL")),
		mcp.WithArray("files", mcp.WithStringItems(), mcp.Description("Files to install")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		taskID, err := req.RequireString("task_id")
		if err != nil {
			return errorResult(err)
		}
		sourceURL := req.GetString("source_url", "")
		files := req.GetStringSlice("files", nil)

		if err := mgr.InstallOnlineTask(ctx, sourceURL, taskID, files); err != nil {
			return errorResult(err)
		}
		return successText(fmt.Sprintf("Task %q installed successfully", taskID))
	})

	// 7. delete_custom_task
	s.AddTool(mcp.NewTool("delete_custom_task",
		mcp.WithDescription("Delete an installed custom task. Requires full-control token."),
		mcp.WithTitleAnnotation("Delete Custom Task"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[SimpleMessageOutput](),
		mcp.WithString("id", mcp.Required(), mcp.Description("Task ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		taskID, err := req.RequireString("id")
		if err != nil {
			return errorResult(err)
		}
		if err := mgr.DeleteCustomTask(ctx, taskID); err != nil {
			return errorResult(err)
		}
		return successText(fmt.Sprintf("Task %q deleted successfully", taskID))
	})
}
