package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/internal/engine"
)

func setupTestServerForMCP(t *testing.T) (*Server, func(), string) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "easy42-mcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := engine.NewManager(store)
	_ = mgr.Unlock(pass)

	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Manager:    mgr,
	})

	cfg := store.Get()
	token := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 24*60*60*1000*1000*1000)

	cleanup := func() {
		srv.Close()
		_ = os.RemoveAll(tempDir)
	}

	return srv, cleanup, token
}

func TestMCPSettingsEndpoints(t *testing.T) {
	srv, cleanup, sessionToken := setupTestServerForMCP(t)
	defer cleanup()

	// 1. GET /api/settings/mcp
	req := httptest.NewRequest("GET", "/api/settings/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 from GET /api/settings/mcp, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp mcpSettingsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode mcpSettingsResponse: %v", err)
	}

	if !resp.Enabled {
		t.Errorf("expected MCP enabled by default")
	}
	if resp.ReadOnlyToken == "" {
		t.Errorf("expected non-empty read-only token")
	}
	if resp.FullControlToken == "" {
		t.Errorf("expected non-empty full-control token")
	}
	if resp.ReadOnlyToken == resp.FullControlToken {
		t.Errorf("expected read-only and full-control tokens to be different")
	}

	// 2. PUT /api/settings/mcp (disable MCP)
	disabled := false
	body, _ := json.Marshal(map[string]any{
		"enabled": disabled,
	})
	putReq := httptest.NewRequest("PUT", "/api/settings/mcp", bytes.NewReader(body))
	putReq.Header.Set("Authorization", "Bearer "+sessionToken)
	putReq.Header.Set("Content-Type", "application/json")
	putRr := httptest.NewRecorder()
	srv.router.ServeHTTP(putRr, putReq)

	if putRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from PUT /api/settings/mcp, got %d: %s", putRr.Code, putRr.Body.String())
	}

	var putResp mcpSettingsResponse
	_ = json.NewDecoder(putRr.Body).Decode(&putResp)
	if putResp.Enabled {
		t.Errorf("expected MCP enabled to be false after PUT")
	}

	// 3. Re-enable and reset tokens
	enabled := true
	reEnableBody, _ := json.Marshal(map[string]any{"enabled": enabled})
	reEnableReq := httptest.NewRequest("PUT", "/api/settings/mcp", bytes.NewReader(reEnableBody))
	reEnableReq.Header.Set("Authorization", "Bearer "+sessionToken)
	reEnableReq.Header.Set("Content-Type", "application/json")
	reEnableRr := httptest.NewRecorder()
	srv.router.ServeHTTP(reEnableRr, reEnableReq)

	resetBody, _ := json.Marshal(map[string]string{"token_type": "read_only"})
	resetReq := httptest.NewRequest("POST", "/api/settings/mcp/reset-token", bytes.NewReader(resetBody))
	resetReq.Header.Set("Authorization", "Bearer "+sessionToken)
	resetReq.Header.Set("Content-Type", "application/json")
	resetRr := httptest.NewRecorder()
	srv.router.ServeHTTP(resetRr, resetReq)

	if resetRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from reset-token, got %d", resetRr.Code)
	}

	var resetResp mcpSettingsResponse
	_ = json.NewDecoder(resetRr.Body).Decode(&resetResp)
	if resetResp.ReadOnlyToken == resp.ReadOnlyToken {
		t.Errorf("expected read-only token to change after reset, got same token")
	}
	if resetResp.FullControlToken != resp.FullControlToken {
		t.Errorf("expected full-control token to remain unchanged when resetting only read_only")
	}

	// 4. Test MCP endpoint /mcp with new full-control token
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	mcpReq := httptest.NewRequest("GET", "/mcp?token="+resetResp.FullControlToken, nil).WithContext(ctx)
	mcpRr := httptest.NewRecorder()
	srv.router.ServeHTTP(mcpRr, mcpReq)
	if mcpRr.Code == http.StatusUnauthorized || mcpRr.Code == http.StatusForbidden {
		t.Errorf("expected authenticated response from /mcp, got %d: %s", mcpRr.Code, mcpRr.Body.String())
	}

	// 5. Test MCP endpoint with external Host header through reverse proxy (e.g. Cloudflare / Nginx)
	postBody := bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"test-client","version":"1.0"}}}`))
	proxyReq := httptest.NewRequest("POST", "/mcp", postBody)
	proxyReq.Host = "easy42.sagan.me"
	proxyReq.Header.Set("Authorization", "Bearer "+resetResp.FullControlToken)
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyRr := httptest.NewRecorder()
	srv.router.ServeHTTP(proxyRr, proxyReq)
	if proxyRr.Code == http.StatusForbidden && bytes.Contains(proxyRr.Body.Bytes(), []byte("invalid Host header")) {
		t.Fatalf("expected Host header 'easy42.sagan.me' to be accepted, got 403 invalid Host header")
	}
	if proxyRr.Code != http.StatusOK {
		t.Errorf("expected 200 from /mcp POST initialize, got %d: %s", proxyRr.Code, proxyRr.Body.String())
	}
}
