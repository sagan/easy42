package mcp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"easy42/internal/engine"
	"easy42/internal/version"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Server wraps the mark3labs MCP Server instance and transports
type Server struct {
	mgr              *engine.Manager
	mcpServer        *server.MCPServer
	streamableServer *server.StreamableHTTPServer
	sseServer        *server.SSEServer
	oauthServer      *OAuthServer
}

// NewServer creates and initializes the Easy42 MCP server with all tools, resources, and prompts
func NewServer(mgr *engine.Manager) *Server {
	ver := version.Version
	if ver == "" {
		ver = "0.1.0"
	}

	mcpServer := server.NewMCPServer("easy42", ver,
		server.WithInstructions("Easy42 Cluster Control Plane MCP Server. Allows AI agents to query, configure, monitor, and synchronize the Easy42 overlay mesh network. Read-only tokens permit inspection; full-control tokens permit modifications."),
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
		server.WithOutputSchemaValidation(),
	)

	// Top-level cluster info tool
	mcpServer.AddTool(mcp.NewTool("get_cluster_info",
		mcp.WithDescription("Get high-level Easy42 cluster information (network name, version, total nodes, total links, policies count, templates count)"),
		mcp.WithTitleAnnotation("Get Cluster Info"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithOutputSchema[ClusterInfoOutput](),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cfg := mgr.Store().Get()
		netName := "Easy42"
		asn := uint64(0)
		if cfg != nil {
			netName = cfg.NetworkSettings.GetName()
			asn = cfg.NetworkSettings.PublicASN
		}
		nodes := mgr.GetNodes()
		links := mgr.GetLinks()
		policies := mgr.GetNetworkPolicies()
		templates := mgr.GetTemplates()

		summaries := make([]ClusterNodeSummary, 0, len(nodes))
		for _, n := range nodes {
			summaries = append(summaries, ClusterNodeSummary{
				Name: n.Name,
				IP:   n.IP,
				IP6:  n.IP6,
				ASN:  n.ASN,
			})
		}

		info := ClusterInfoOutput{
			NetworkName:   netName,
			PublicASN:     asn,
			Version:       version.Version,
			NodeCount:     len(nodes),
			LinkCount:     len(links),
			PolicyCount:   len(policies),
			TemplateCount: len(templates),
			Role:          GetRole(ctx),
			Nodes:         summaries,
			Hint:          "To inspect full runtime health or BGP WireGuard status, call 'get_cluster_state', 'get_fleet_live_status', or 'list_links'.",
		}
		return jsonResult(info)
	})

	// Register all functional tool categories
	registerNodeTools(mcpServer, mgr)
	registerLinkTools(mcpServer, mgr)
	registerNetworkTools(mcpServer, mgr)
	registerPolicyTools(mcpServer, mgr)
	registerTemplateTools(mcpServer, mgr)
	registerConfigApplyTools(mcpServer, mgr)
	registerSyncTools(mcpServer, mgr)
	registerDeviceHelperTaskTools(mcpServer, mgr)
	registerLookingGlassTools(mcpServer, mgr)
	registerCronTaskTools(mcpServer, mgr)
	registerDatabaseAndIntegrationsTools(mcpServer, mgr)

	// Register helpful MCP Resources
	mcpServer.AddResource(mcp.NewResource("easy42://cluster/summary", "Easy42 Cluster Summary",
		mcp.WithResourceDescription("High-level summary of the cluster nodes, links, and configuration"),
		mcp.WithMIMEType("application/json"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		nodes := mgr.GetNodes()
		links := mgr.GetLinks()
		data, _ := json.MarshalIndent(map[string]any{
			"nodes_count": len(nodes),
			"links_count": len(links),
			"version":     version.Version,
		}, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "easy42://cluster/summary",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})

	mcpServer.AddResource(mcp.NewResource("easy42://cluster/nodes", "Easy42 Mesh Nodes",
		mcp.WithResourceDescription("Current list of all configured nodes with IPs and tags"),
		mcp.WithMIMEType("application/json"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		nodes := mgr.GetNodes()
		data, _ := json.MarshalIndent(nodes, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "easy42://cluster/nodes",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})

	mcpServer.AddResource(mcp.NewResource("easy42://cluster/links", "Easy42 Peer Links",
		mcp.WithResourceDescription("Current list of all WireGuard peer links"),
		mcp.WithMIMEType("application/json"),
	), func(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		links := mgr.GetLinks()
		data, _ := json.MarshalIndent(links, "", "  ")
		return []mcp.ResourceContents{
			mcp.TextResourceContents{
				URI:      "easy42://cluster/links",
				MIMEType: "application/json",
				Text:     string(data),
			},
		}, nil
	})

	// Register helpful Prompts
	mcpServer.AddPrompt(mcp.Prompt{
		Name:        "cluster_health_audit",
		Description: "Perform a comprehensive health and connectivity audit of all nodes and WireGuard links in the cluster",
	}, func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Easy42 Cluster Health Audit Prompt",
			Messages: []mcp.PromptMessage{
				{
					Role: mcp.RoleUser,
					Content: mcp.TextContent{
						Type: "text",
						Text: "Please audit the Easy42 cluster: 1) Call get_cluster_info and get_fleet_live_status to check node health and reachability. 2) Call list_links to verify all peer connections. 3) Call preview_sync to check for pending unapplied configuration differences. 4) Report any offline nodes, degraded links, or synchronization drift.",
					},
				},
			},
		}, nil
	})

	mcpServer.AddPrompt(mcp.Prompt{
		Name:        "add_node_wizard",
		Description: "Interactive guide to safely onboard and configure a new node in the Easy42 mesh",
		Arguments: []mcp.PromptArgument{
			{Name: "node_name", Description: "Name of the node to onboard", Required: true},
		},
	}, func(ctx context.Context, request mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		nodeName := request.Params.Arguments["node_name"]
		return &mcp.GetPromptResult{
			Description: "Add Node Wizard",
			Messages: []mcp.PromptMessage{
				{
					Role: mcp.RoleUser,
					Content: mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf("I want to onboard a new node named '%s'. First call get_network_settings to check allocated network IP blocks and ASN. Then suggest suitable IP addresses and call add_node, followed by adding links to mesh it with existing nodes.", nodeName),
					},
				},
			},
		}, nil
	})

	s := &Server{
		mgr:         mgr,
		mcpServer:   mcpServer,
		oauthServer: NewOAuthServer(mgr),
	}

	// Setup Streamable HTTP transport
	s.streamableServer = server.NewStreamableHTTPServer(mcpServer,
		server.WithEndpointPath("/mcp"),
		server.WithDisableLocalhostProtection(true),
		server.WithStateLess(true),
		server.WithHTTPContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return s.authenticateContext(ctx, r)
		}),
	)

	// Setup SSE transport
	s.sseServer = server.NewSSEServer(mcpServer,
		server.WithSSEEndpoint("/mcp/sse"),
		server.WithMessageEndpoint("/mcp/message"),
		server.WithSSEDisableLocalhostProtection(true),
		server.WithSSEContextFunc(func(ctx context.Context, r *http.Request) context.Context {
			return s.authenticateContext(ctx, r)
		}),
	)

	return s
}

// authenticateContext inspects request tokens and populates the role on context
func (s *Server) authenticateContext(ctx context.Context, r *http.Request) context.Context {
	role := s.extractRole(r)
	if role != "" {
		return WithRole(ctx, role)
	}
	return ctx
}

// extractRole verifies credentials from Authorization header or ?token= query parameter
func (s *Server) extractRole(r *http.Request) string {
	settings := s.mgr.GetMCPSettings()
	if !settings.Enabled {
		return ""
	}

	var token string
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token = strings.TrimSpace(parts[1])
		} else {
			token = strings.TrimSpace(authHeader)
		}
	}
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	if token == "" {
		return ""
	}

	// 1. Check OAuth access token
	if s.oauthServer != nil {
		if role, ok := s.oauthServer.ValidateAccessToken(token); ok {
			return role
		}
	}

	// 2. Constant-time comparison for security
	if settings.FullControlToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(settings.FullControlToken)) == 1 {
		return RoleFullControl
	}
	if settings.ReadOnlyToken != "" && subtle.ConstantTimeCompare([]byte(token), []byte(settings.ReadOnlyToken)) == 1 {
		return RoleReadOnly
	}

	return ""
}

// OAuthServer returns the associated OAuth 2.0 authorization server
func (s *Server) OAuthServer() *OAuthServer {
	return s.oauthServer
}

// AuthMiddleware wraps an HTTP handler, ensuring the request has valid MCP credentials
func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow CORS preflight and headers for web-based AI clients (e.g. Gemini Web)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-MCP-Token, X-Requested-With, Mcp-Session-Id, Mcp-Protocol-Version")
		w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate, Authorization, Mcp-Session-Id, Mcp-Protocol-Version")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		settings := s.mgr.GetMCPSettings()
		if !settings.Enabled {
			http.Error(w, "MCP server is disabled in Easy42 settings", http.StatusForbidden)
			return
		}

		role := s.extractRole(r)
		if role == "" {
			scheme := "http"
			if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
				scheme = "https"
			}
			metaURL := fmt.Sprintf("%s://%s/.well-known/oauth-protected-resource", scheme, r.Host)
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s"`, metaURL))
			http.Error(w, "Unauthorized: valid Easy42 MCP token required", http.StatusUnauthorized)
			return
		}

		ctx := WithRole(r.Context(), role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// StreamableHandler returns http.Handler for streamable-http MCP requests
func (s *Server) StreamableHandler() http.Handler {
	return s.AuthMiddleware(s.streamableServer)
}

// SSEHandler returns http.Handler for the SSE event stream
func (s *Server) SSEHandler() http.Handler {
	return s.AuthMiddleware(s.sseServer.SSEHandler())
}

// MessageHandler returns http.Handler for posting messages to the SSE session
func (s *Server) MessageHandler() http.Handler {
	return s.AuthMiddleware(s.sseServer.MessageHandler())
}

// MCPServer returns the underlying mark3labs MCPServer instance (e.g. for Stdio)
func (s *Server) MCPServer() *server.MCPServer {
	return s.mcpServer
}
