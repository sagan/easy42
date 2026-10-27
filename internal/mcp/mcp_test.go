package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
)

func setupTestManager(t *testing.T) (*engine.Manager, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	store := config.NewStore(tmpDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to init config: %v", err)
	}

	mgr := engine.NewManager(store)
	_ = mgr.Unlock(pass)

	cleanup := func() {
		mgr.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return mgr, cleanup
}

func TestMCPServerToolsRegistration(t *testing.T) {
	mgr, cleanup := setupTestManager(t)
	defer cleanup()

	s := NewServer(mgr)
	tools := s.MCPServer().ListTools()

	expectedTools := []string{
		"get_cluster_info",
		"list_nodes",
		"get_node",
		"add_node",
		"update_node",
		"rename_node",
		"delete_node",
		"probe_node",
		"refresh_node_status",
		"restart_node_service",
		"get_node_telemetry",
		"get_fleet_live_status",
		"list_links",
		"get_link",
		"add_link",
		"create_mesh_links",
		"update_link",
		"delete_link",
		"reset_link_psk",
		"reset_link_private_key",
		"get_network_settings",
		"update_network_settings",
		"get_ip_blocks",
		"update_ip_blocks",
		"get_dns_config",
		"update_dns_config",
		"sync_dns",
		"list_network_policies",
		"get_network_policy",
		"create_network_policy",
		"update_network_policy",
		"delete_network_policy",
		"refresh_policy_roa",
		"list_templates",
		"get_template",
		"create_template",
		"update_template",
		"delete_template",
		"preview_config_apply",
		"execute_config_apply",
		"list_config_changes",
		"get_config_change",
		"save_config_change",
		"apply_saved_config_change",
		"delete_config_change",
		"preview_sync",
		"execute_sync",
		"get_sync_status",
		"get_cluster_state",
		"list_tasks",
		"get_task_sources",
		"check_task_status",
		"run_task",
		"get_online_tasks_manifest",
		"install_online_task",
		"delete_custom_task",
		"list_looking_glass_tasks",
		"run_looking_glass",
		"save_looking_glass_task",
		"delete_looking_glass_task",
		"list_cron_tasks",
		"create_cron_task",
		"update_cron_task",
		"toggle_cron_task",
		"delete_cron_task",
		"get_node_cron_task_results",
		"get_database_info",
		"update_database_settings",
		"reclaim_database",
		"get_integrations_settings",
		"update_integrations_settings",
		"reset_prometheus_token",
	}

	for _, toolName := range expectedTools {
		if _, ok := tools[toolName]; !ok {
			t.Errorf("expected tool %q to be registered, but was missing", toolName)
		}
	}

	if len(tools) < len(expectedTools) {
		t.Errorf("expected at least %d tools registered, got %d", len(expectedTools), len(tools))
	}

	// Verify annotations on every registered tool
	for name, tool := range tools {
		ann := tool.Tool.Annotations
		if ann.Title == "" {
			t.Errorf("tool %q missing Title annotation", name)
		}
		if ann.ReadOnlyHint == nil {
			t.Errorf("tool %q missing ReadOnlyHint annotation", name)
		}
		if ann.DestructiveHint == nil {
			t.Errorf("tool %q missing DestructiveHint annotation", name)
		}
		if ann.IdempotentHint == nil {
			t.Errorf("tool %q missing IdempotentHint annotation", name)
		}
		if ann.OpenWorldHint == nil {
			t.Errorf("tool %q missing OpenWorldHint annotation", name)
		}
	}

	// Specifically verify read-only tools have ReadOnlyHint = true and DestructiveHint = false
	clusterInfo := tools["get_cluster_info"].Tool
	if clusterInfo.Annotations.ReadOnlyHint == nil || !*clusterInfo.Annotations.ReadOnlyHint {
		t.Errorf("expected get_cluster_info to have ReadOnlyHint=true")
	}
	if clusterInfo.Annotations.DestructiveHint == nil || *clusterInfo.Annotations.DestructiveHint {
		t.Errorf("expected get_cluster_info to have DestructiveHint=false")
	}

	// Specifically verify destructive tool has DestructiveHint = true and ReadOnlyHint = false
	deleteNode := tools["delete_node"].Tool
	if deleteNode.Annotations.DestructiveHint == nil || !*deleteNode.Annotations.DestructiveHint {
		t.Errorf("expected delete_node to have DestructiveHint=true")
	}
	if deleteNode.Annotations.ReadOnlyHint == nil || *deleteNode.Annotations.ReadOnlyHint {
		t.Errorf("expected delete_node to have ReadOnlyHint=false")
	}
}

func TestMCPServerPermissions(t *testing.T) {
	mgr, cleanup := setupTestManager(t)
	defer cleanup()

	s := NewServer(mgr)

	// Context with ReadOnly role
	roCtx := WithRole(context.Background(), RoleReadOnly)

	// Context with FullControl role
	fcCtx := WithRole(context.Background(), RoleFullControl)

	// 1. Read tool: get_cluster_info should work with ReadOnly role
	infoTool := s.MCPServer().GetTool("get_cluster_info")
	if infoTool == nil {
		t.Fatal("get_cluster_info tool not found")
	}

	res, err := infoTool.Handler(roCtx, mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("unexpected error calling get_cluster_info: %v", err)
	}
	if res.IsError {
		t.Fatalf("get_cluster_info returned tool error with read-only role")
	}

	// 2. Mutating tool: add_node should FAIL with ReadOnly role
	addNodeTool := s.MCPServer().GetTool("add_node")
	if addNodeTool == nil {
		t.Fatal("add_node tool not found")
	}

	res, _ = addNodeTool.Handler(roCtx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{
				"name": "node-unauthorized",
			},
		},
	})
	if res == nil || !res.IsError {
		t.Fatalf("expected add_node to return error with read-only role, got success: %v", res)
	}

	// 3. Mutating tool: add_node should SUCCEED with FullControl role
	res, err = addNodeTool.Handler(fcCtx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: map[string]any{
				"name": "node-test1",
				"host": "192.168.100.10",
				"ip":   "192.168.100.10",
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error calling add_node: %v", err)
	}
	if res.IsError {
		t.Fatalf("add_node returned error with full-control role: %v", res)
	}

	// Verify node was added
	node := mgr.FindNode("node-test1")
	if node == nil {
		t.Fatalf("node-test1 was not found in manager after add_node")
	}
}

func TestMCPServerAuthMiddleware(t *testing.T) {
	mgr, cleanup := setupTestManager(t)
	defer cleanup()

	s := NewServer(mgr)
	settings := mgr.GetMCPSettings()

	handler := s.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := GetRole(r.Context())
		w.Header().Set("X-Role", role)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("authenticated"))
	}))

	// Case 1: Missing token -> 401 Unauthorized
	req1 := httptest.NewRequest("GET", "/mcp", nil)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", rr1.Code)
	}

	// Case 2: Invalid token -> 401 Unauthorized
	req2 := httptest.NewRequest("GET", "/mcp", nil)
	req2.Header.Set("Authorization", "Bearer invalid-token-1234")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid token, got %d", rr2.Code)
	}

	// Case 3: Read-only Bearer token -> 200 OK with RoleReadOnly
	req3 := httptest.NewRequest("GET", "/mcp", nil)
	req3.Header.Set("Authorization", "Bearer "+settings.ReadOnlyToken)
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Errorf("expected 200 with read-only token, got %d", rr3.Code)
	}
	if role := rr3.Header().Get("X-Role"); role != RoleReadOnly {
		t.Errorf("expected role %q, got %q", RoleReadOnly, role)
	}

	// Case 4: Full-control token in query param ?token=... -> 200 OK with RoleFullControl
	req4 := httptest.NewRequest("GET", "/mcp?token="+settings.FullControlToken, nil)
	rr4 := httptest.NewRecorder()
	handler.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Errorf("expected 200 with full-control query token, got %d", rr4.Code)
	}
	if role := rr4.Header().Get("X-Role"); role != RoleFullControl {
		t.Errorf("expected role %q, got %q", RoleFullControl, role)
	}

	// Case 5: When MCP is disabled -> 403 Forbidden
	disabledSettings := settings
	disabledSettings.Enabled = false
	if err := mgr.UpdateMCPSettings(disabledSettings); err != nil {
		t.Fatalf("failed to update MCP settings: %v", err)
	}

	req5 := httptest.NewRequest("GET", "/mcp?token="+settings.FullControlToken, nil)
	rr5 := httptest.NewRecorder()
	handler.ServeHTTP(rr5, req5)
	if rr5.Code != http.StatusForbidden {
		t.Errorf("expected 403 when MCP disabled, got %d", rr5.Code)
	}
}
