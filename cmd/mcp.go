package cmd

import (
	"context"
	"fmt"
	"os"

	"easy42/internal/config"
	"easy42/internal/engine"
	"easy42/internal/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var (
	mcpRole string
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run the Easy42 Model Context Protocol (MCP) server over standard I/O (stdio)",
	Long: `Run the Easy42 MCP server over stdio for local AI coding assistants and agents
(e.g., Claude Desktop, Cursor, Cline, Roo Code, Windsurf).

Example claude_desktop_config.json configuration:
  {
    "mcpServers": {
      "easy42": {
        "command": "easy42",
        "args": ["mcp"]
      }
    }
  }`,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := config.NewStore(GetDataDir())
		if !store.Exists() {
			return fmt.Errorf("easy42 configuration not found in %s. Please run 'easy42 serve' first to initialize the cluster", GetDataDir())
		}

		mgr := engine.NewManager(store)
		defer mgr.Close()

		s := mcp.NewServer(mgr)
		stdioServer := mcpserver.NewStdioServer(s.MCPServer())

		ctx := mcp.WithRole(context.Background(), mcpRole)
		stdioServer.SetContextFunc(func(c context.Context) context.Context {
			return ctx
		})

		return stdioServer.Listen(ctx, os.Stdin, os.Stdout)
	},
}

func init() {
	mcpCmd.Flags().StringVar(&mcpRole, "role", mcp.RoleFullControl, "Access role: 'full_control' (read-write) or 'read_only'")
	RootCmd.AddCommand(mcpCmd)
}
