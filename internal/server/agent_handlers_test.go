package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

func setupAgentTestServer(t *testing.T) (*Server, *engine.Manager, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	store := config.NewStore(tmpDir)

	cfg := &config.Config{
		PasswordHash:  "test-pass",
		SessionSecret: "test-secret-12345678901234567890",
		Nodes: []config.Node{
			{
				Name: "node-test-1",
				Host: "192.168.1.10",
				IP:   "10.0.0.1",
				ASN:  4224420001,
			},
		},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	mgr := engine.NewManager(store)
	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Manager:    mgr,
	})

	return srv, mgr, func() {}
}

func TestAgentInstallScript(t *testing.T) {
	srv, _, cleanup := setupAgentTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/agent/install.sh", nil)
	rr := httptest.NewRecorder()

	srv.router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got: %d", rr.Code)
	}
	body := rr.Body.String()
	if (!strings.Contains(body, "#!/bin/sh") && !strings.Contains(body, "#!/bin/bash")) || !strings.Contains(body, "easy42-agent") {
		t.Fatalf("unexpected install script content: %s", body)
	}
}

func TestAgentBinaryDownload(t *testing.T) {
	srv, _, cleanup := setupAgentTestServer(t)
	defer cleanup()

	archs := []string{"x86_64", "aarch64", "armv7", "mipsel"}
	for _, arch := range archs {
		req := httptest.NewRequest(http.MethodGet, "/api/agent/bin/"+arch, nil)
		rr := httptest.NewRecorder()
		srv.router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for arch %s, got: %d", arch, rr.Code)
		}
		if rr.Body.Len() == 0 {
			t.Fatalf("downloaded binary for arch %s is empty", arch)
		}
		if !strings.HasPrefix(rr.Body.String(), "\x7fELF") {
			t.Fatalf("downloaded binary for arch %s is not an ELF binary", arch)
		}
	}

	// Test 404 on invalid arch
	req := httptest.NewRequest(http.MethodGet, "/api/agent/bin/nonexistent", nil)
	rr := httptest.NewRecorder()
	srv.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent arch, got: %d", rr.Code)
	}
}

func TestGenerateAgentTokenAndTelemetry(t *testing.T) {
	srv, mgr, cleanup := setupAgentTestServer(t)
	defer cleanup()

	// 1. Generate agent token
	token, err := mgr.GenerateAgentToken("node-test-1")
	if err != nil {
		t.Fatalf("GenerateAgentToken failed: %v", err)
	}
	if !strings.HasPrefix(token, "e42_") {
		t.Fatalf("unexpected token format: %s", token)
	}

	node := mgr.FindNode("node-test-1")
	if node == nil || node.Mode != "agent" || node.AgentToken != token {
		t.Fatalf("expected node to have agent mode and token set: %+v", node)
	}

	// 2. Query telemetry endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/nodes/node-test-1/telemetry", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("name", "node-test-1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()
	srv.handleGetNodeTelemetry(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got: %d, body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode json response: %v", err)
	}
	if resp["node"] != "node-test-1" {
		t.Fatalf("expected node-test-1, got: %v", resp["node"])
	}
}

func TestAgentWebSocketAuthentication(t *testing.T) {
	srv, mgr, cleanup := setupAgentTestServer(t)
	defer cleanup()

	token, _ := mgr.GenerateAgentToken("node-test-1")

	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/agent/ws"

	// 1. Dial without token should fail with 401
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatalf("expected connection to fail without token")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got: %d", resp.StatusCode)
	}

	// 2. Dial with valid token in query param
	conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?token="+token, nil)
	if err != nil {
		t.Fatalf("expected successful WebSocket connection with token: %v", err)
	}
	defer conn.Close()

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	if !mgr.AgentHub().IsConnected("node-test-1") {
		t.Fatalf("expected node-test-1 to be marked as connected in AgentHub")
	}

	// 3. Status should show connected
	st, err := mgr.RefreshNodeStatus("node-test-1")
	if err != nil {
		t.Fatalf("RefreshNodeStatus failed: %v", err)
	}
	if !st.Connected {
		t.Fatalf("expected status.Connected to be true")
	}
}
