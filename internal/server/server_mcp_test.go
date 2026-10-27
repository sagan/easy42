package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
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

	// 6. Test OAuth 2.0 / 2.1 RFC 9728 and RFC 8414 metadata discovery
	protReq := httptest.NewRequest("GET", "/.well-known/oauth-protected-resource", nil)
	protRr := httptest.NewRecorder()
	srv.router.ServeHTTP(protRr, protReq)
	if protRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from /.well-known/oauth-protected-resource, got %d", protRr.Code)
	}

	asReq := httptest.NewRequest("GET", "/.well-known/oauth-authorization-server", nil)
	asRr := httptest.NewRecorder()
	srv.router.ServeHTTP(asRr, asReq)
	if asRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from /.well-known/oauth-authorization-server, got %d", asRr.Code)
	}

	// 7. Test OAuth Client Credentials Flow (Gemini Web App)
	tokenReqBody := "grant_type=client_credentials&client_id=" + resetResp.OAuthClientID + "&client_secret=" + resetResp.OAuthClientSecret
	tokenReq := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(tokenReqBody))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRr := httptest.NewRecorder()
	srv.router.ServeHTTP(tokenRr, tokenReq)
	if tokenRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from /oauth/token client_credentials, got %d: %s", tokenRr.Code, tokenRr.Body.String())
	}

	var oauthTokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	_ = json.NewDecoder(tokenRr.Body).Decode(&oauthTokenResp)
	if oauthTokenResp.AccessToken == "" {
		t.Fatalf("expected non-empty access_token from /oauth/token")
	}

	// 8. Test calling /mcp using OAuth Bearer Token
	oauthMcpReq := httptest.NewRequest("POST", "/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"gemini-test","version":"1.0"}}}`)))
	oauthMcpReq.Header.Set("Authorization", "Bearer "+oauthTokenResp.AccessToken)
	oauthMcpReq.Header.Set("Content-Type", "application/json")
	oauthMcpRr := httptest.NewRecorder()
	srv.router.ServeHTTP(oauthMcpRr, oauthMcpReq)
	if oauthMcpRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from /mcp with OAuth access_token, got %d: %s", oauthMcpRr.Code, oauthMcpRr.Body.String())
	}

	// 9. Test Gemini calling tools/call directly without Mcp-Session-Id header (stateless HTTP tool call)
	toolCallReq := httptest.NewRequest("POST", "/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_cluster_info","arguments":{}}}`)))
	toolCallReq.Header.Set("Authorization", "Bearer "+oauthTokenResp.AccessToken)
	toolCallReq.Header.Set("Content-Type", "application/json")
	toolCallRr := httptest.NewRecorder()
	srv.router.ServeHTTP(toolCallRr, toolCallReq)
	if toolCallRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from /mcp stateless tools/call, got %d: %s", toolCallRr.Code, toolCallRr.Body.String())
	}
	if !bytes.Contains(toolCallRr.Body.Bytes(), []byte("network_name")) {
		t.Fatalf("expected tool call result to contain 'network_name', got %s", toolCallRr.Body.String())
	}
}


func TestMCPOAuthAuthorizationCodeFlow(t *testing.T) {
	srv, cleanup, sessionToken := setupTestServerForMCP(t)
	defer cleanup()

	// 1. GET /oauth/authorize (consent page)
	authReq := httptest.NewRequest("GET", "/oauth/authorize?response_type=code&client_id=easy42-mcp&redirect_uri=https://example.com/callback&state=xyz123&code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&code_challenge_method=S256", nil)
	authReq.AddCookie(&http.Cookie{Name: "easy42_session", Value: sessionToken})
	authRr := httptest.NewRecorder()
	srv.router.ServeHTTP(authRr, authReq)

	if authRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from /oauth/authorize GET, got %d", authRr.Code)
	}

	// 2. POST /oauth/authorize (approve)
	form := "action=approve&response_type=code&client_id=easy42-mcp&redirect_uri=https://example.com/callback&state=xyz123&code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&code_challenge_method=S256&role=full_control"
	postReq := httptest.NewRequest("POST", "/oauth/authorize", strings.NewReader(form))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: "easy42_session", Value: sessionToken})
	postRr := httptest.NewRecorder()
	srv.router.ServeHTTP(postRr, postReq)

	if postRr.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect from /oauth/authorize POST, got %d: %s", postRr.Code, postRr.Body.String())
	}

	location := postRr.Header().Get("Location")
	locURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("failed to parse redirect Location: %v", err)
	}
	code := locURL.Query().Get("code")
	if code == "" {
		t.Fatalf("expected code parameter in redirect URL: %s", location)
	}
	if locURL.Query().Get("state") != "xyz123" {
		t.Errorf("expected state=xyz123 in redirect, got %s", locURL.Query().Get("state"))
	}

	// 3. POST /oauth/token to exchange authorization code using PKCE
	// RFC 7636: challenge "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", verifier "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	tokenForm := "grant_type=authorization_code&client_id=easy42-mcp&code=" + code + "&code_verifier=dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk&redirect_uri=https://example.com/callback"
	tokenReq := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(tokenForm))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenRr := httptest.NewRecorder()
	srv.router.ServeHTTP(tokenRr, tokenReq)

	if tokenRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from /oauth/token exchange, got %d: %s", tokenRr.Code, tokenRr.Body.String())
	}

	var resp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	_ = json.NewDecoder(tokenRr.Body).Decode(&resp)
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatalf("expected access_token and refresh_token, got: %+v", resp)
	}

	// 4. Test calling /mcp using the access_token
	mcpReq := httptest.NewRequest("POST", "/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":3,"method":"initialize","params":{"protocolVersion":"2025-03-26","clientInfo":{"name":"gemini-test","version":"1.0"}}}`)))
	mcpReq.Header.Set("Authorization", "Bearer "+resp.AccessToken)
	mcpReq.Header.Set("Content-Type", "application/json")
	mcpRr := httptest.NewRecorder()
	srv.router.ServeHTTP(mcpRr, mcpReq)
	if mcpRr.Code != http.StatusOK {
		t.Fatalf("expected 200 from /mcp using authorization_code token, got %d: %s", mcpRr.Code, mcpRr.Body.String())
	}
}
