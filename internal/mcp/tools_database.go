package mcp

import (
	"context"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func registerDatabaseAndIntegrationsTools(s *server.MCPServer, mgr *engine.Manager) {
	// 1. get_database_info
	s.AddTool(mcp.NewTool("get_database_info",
		mcp.WithDescription("Get SQLite database status, file size, table row counts, and data retention settings"),
		mcp.WithTitleAnnotation("Get Database Info"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[engine.DatabaseInfo](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		info, err := mgr.GetDatabaseInfo()
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(info)
	})

	// 2. update_database_settings
	s.AddTool(mcp.NewTool("update_database_settings",
		mcp.WithDescription("Update database data retention days for historical metrics. Requires full-control token."),
		mcp.WithTitleAnnotation("Update Database Settings"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[engine.DatabaseInfo](),
		mcp.WithInteger("retention_days", mcp.Description("Retention days for summarized metrics (default 47)")),
		mcp.WithInteger("raw_retention_days", mcp.Description("Retention days for raw high-resolution metrics (default 2)")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		ret := req.GetInt("retention_days", config.DefaultRetentionDays)
		rawRet := req.GetInt("raw_retention_days", config.DefaultRawRetentionDays)

		info, err := mgr.UpdateDatabaseSettings(config.DatabaseSettings{
			RetentionDays:    ret,
			RawRetentionDays: rawRet,
		})
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(info)
	})

	// 3. reclaim_database
	s.AddTool(mcp.NewTool("reclaim_database",
		mcp.WithDescription("Run SQLite VACUUM operation to reclaim unused disk space and optimize database performance. Requires full-control token."),
		mcp.WithTitleAnnotation("Reclaim Database"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[ReclaimDatabaseOutput](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		info, err := mgr.VacuumDatabase()
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(ReclaimDatabaseOutput{
			Message:  "Database vacuum completed successfully",
			Database: info,
		})
	})

	// 4. get_integrations_settings
	s.AddTool(mcp.NewTool("get_integrations_settings",
		mcp.WithDescription("Get configuration for external integrations (Prometheus / VictoriaMetrics scraping, Telegram notifications)"),
		mcp.WithTitleAnnotation("Get Integrations Settings"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[config.IntegrationsSettings](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		settings := mgr.GetIntegrationsSettings()
		display := settings
		if display.TelegramBotToken != "" {
			display.TelegramBotToken = "******"
		}
		if display.TelegramSecretToken != "" {
			display.TelegramSecretToken = "******"
		}
		return jsonResult(display)
	})

	// 5. update_integrations_settings
	s.AddTool(mcp.NewTool("update_integrations_settings",
		mcp.WithDescription("Update external integrations settings. Requires full-control token."),
		mcp.WithTitleAnnotation("Update Integrations Settings"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[SimpleMessageOutput](),
		mcp.WithBoolean("prometheus_enabled", mcp.Description("Enable Prometheus metrics export at /metrics")),
		mcp.WithBoolean("telegram_enabled", mcp.Description("Enable Telegram Bot notifications")),
		mcp.WithString("telegram_bot_token", mcp.Description("Telegram Bot API token")),
		mcp.WithInteger("telegram_admin_id", mcp.Description("Telegram user ID for admin notifications")),
		mcp.WithString("server_url", mcp.Description("External URL where easy42 is accessible for webhooks")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		current := mgr.GetIntegrationsSettings()

		if args := req.GetArguments(); args != nil {
			if _, ok := args["prometheus_enabled"]; ok {
				current.PrometheusEnabled = req.GetBool("prometheus_enabled", current.PrometheusEnabled)
			}
			if _, ok := args["telegram_enabled"]; ok {
				current.TelegramEnabled = req.GetBool("telegram_enabled", current.TelegramEnabled)
			}
		}
		if tok := req.GetString("telegram_bot_token", ""); tok != "" {
			current.TelegramBotToken = tok
		}
		if adminID := req.GetInt("telegram_admin_id", 0); adminID > 0 {
			current.TelegramAdminID = int64(adminID)
		}
		if sURL := req.GetString("server_url", ""); sURL != "" {
			current.ServerURL = sURL
		}

		if err := mgr.UpdateIntegrationsSettings(current); err != nil {
			return errorResult(err)
		}
		return successText("Integrations settings updated successfully")
	})

	// 6. reset_prometheus_token
	s.AddTool(mcp.NewTool("reset_prometheus_token",
		mcp.WithDescription("Regenerate the Bearer authentication token for Prometheus scraping. Requires full-control token."),
		mcp.WithTitleAnnotation("Reset Prometheus Token"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[ResetPrometheusTokenOutput](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if err := RequireFullControl(ctx); err != nil {
			return errorResult(err)
		}
		updated, err := mgr.ResetPrometheusToken()
		if err != nil {
			return errorResult(err)
		}
		return jsonResult(ResetPrometheusTokenOutput{
			Message:               "Prometheus auth token rotated successfully",
			PrometheusAuthToken: updated.PrometheusAuthToken,
		})
	})
}
