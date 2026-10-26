package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/internal/engine"
	"github.com/gorilla/websocket"
)

func setupTestServerForWS(t *testing.T) (*Server, *httptest.Server, *config.Config, string) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "easy42-ws-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	store := config.NewStore(tempDir)
	password, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	cfg, err := store.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	mgr := engine.NewManager(store)
	srv := New(Config{
		Manager: mgr,
	})

	ts := httptest.NewServer(srv.router)
	t.Cleanup(func() {
		ts.Close()
		_ = srv.Close()
		_ = os.RemoveAll(tempDir)
	})

	return srv, ts, cfg, password
}

func TestClientWebSocketAuth(t *testing.T) {
	srv, ts, cfg, _ := setupTestServerForWS(t)
	_ = srv

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"

	// 1. Unauthenticated connection attempt should fail
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected connection to fail without authentication")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got: %v", resp.StatusCode)
	}

	// 2. Invalid session cookie should fail
	badHeaders := http.Header{}
	badHeaders.Add("Cookie", "easy42_session=invalid-token")
	_, resp2, err := websocket.DefaultDialer.Dial(wsURL, badHeaders)
	if err == nil {
		t.Fatal("expected connection to fail with invalid session cookie")
	}
	if resp2 != nil && resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got: %v", resp2.StatusCode)
	}

	// 3. Valid session cookie should succeed
	validToken := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 1*time.Hour)
	validHeaders := http.Header{}
	validHeaders.Add("Cookie", "easy42_session="+validToken)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, validHeaders)
	if err != nil {
		t.Fatalf("failed to connect with valid cookie: %v", err)
	}
	defer conn.Close()

	// Should receive initial connected greeting
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read greeting: %v", err)
	}
	var greeting map[string]any
	if err := json.Unmarshal(msg, &greeting); err != nil {
		t.Fatalf("failed to parse greeting JSON: %v", err)
	}
	if greeting["type"] != "connected" {
		t.Fatalf("expected greeting type 'connected', got: %v", greeting["type"])
	}

	// 4. Valid token via query param (?token=...) should also succeed
	queryURL := wsURL + "?token=" + validToken
	connQuery, _, err := websocket.DefaultDialer.Dial(queryURL, nil)
	if err != nil {
		t.Fatalf("failed to connect with ?token= query param: %v", err)
	}
	defer connQuery.Close()

	_ = connQuery.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg2, err := connQuery.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read greeting from query connection: %v", err)
	}
	var greeting2 map[string]any
	_ = json.Unmarshal(msg2, &greeting2)
	if greeting2["type"] != "connected" {
		t.Fatalf("expected greeting type 'connected', got: %v", greeting2["type"])
	}
}

func TestClientWebSocketClusterConfigBroadcast(t *testing.T) {
	srv, ts, cfg, _ := setupTestServerForWS(t)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	validToken := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 1*time.Hour)

	headers := http.Header{}
	headers.Add("Cookie", "easy42_session="+validToken)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Read initial greeting
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read greeting: %v", err)
	}

	// Trigger a config change (e.g. adding a node)
	err = srv.mgr.AddNode(config.Node{
		Name:      "alpha-node",
		Host:      "10.0.0.1",
		IP:        "172.20.0.1",
		Interface: "lo",
		ASN:       4242420001,
	})
	if err != nil {
		t.Fatalf("failed to add node: %v", err)
	}

	// Read broadcast event
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to receive broadcast message: %v", err)
	}

	var event map[string]any
	if err := json.Unmarshal(msg, &event); err != nil {
		t.Fatalf("failed to parse event JSON: %v", err)
	}

	if event["type"] != "cluster_updated" {
		t.Fatalf("expected event type 'cluster_updated', got: %v", event["type"])
	}
	if event["event"] != "config_changed" {
		t.Fatalf("expected event detail 'config_changed', got: %v", event["event"])
	}
}

func TestClientWebSocketExternalConfigChangeWatcher(t *testing.T) {
	srv, ts, cfg, _ := setupTestServerForWS(t)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	validToken := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 1*time.Hour)

	headers := http.Header{}
	headers.Add("Cookie", "easy42_session="+validToken)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Read initial greeting
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	// Simulate external modification to config.json directly on disk (e.g. from CLI or manual edit)
	configFile := filepath.Join(srv.mgr.Store().DataDir(), "config.json")
	cfgData, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("failed to read config.json: %v", err)
	}

	var diskCfg config.Config
	if err := json.Unmarshal(cfgData, &diskCfg); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	diskCfg.NetworkSettings.Name = "ExternallyModifiedMesh"
	newBytes, _ := json.MarshalIndent(&diskCfg, "", "  ")

	// Small pause so mtime is strictly greater
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(configFile, newBytes, 0600); err != nil {
		t.Fatalf("failed to write modified config.json: %v", err)
	}

	// Trigger CheckFileModified or wait for watcher check
	changed, err := srv.mgr.Store().CheckFileModified()
	if err != nil {
		t.Fatalf("check file modified failed: %v", err)
	}
	if !changed {
		t.Fatal("expected CheckFileModified to detect change")
	}

	// Verify client receives broadcast event
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to receive broadcast message from external file change: %v", err)
	}

	var event map[string]any
	_ = json.Unmarshal(msg, &event)
	if event["type"] != "cluster_updated" {
		t.Fatalf("expected 'cluster_updated', got: %v", event["type"])
	}

	// Verify server in-memory config now matches the external change
	if srv.mgr.Store().Get().NetworkSettings.Name != "ExternallyModifiedMesh" {
		t.Fatalf("expected in-memory name to be 'ExternallyModifiedMesh', got: %s", srv.mgr.Store().Get().NetworkSettings.Name)
	}
}

func TestMultipleFrontendClientsSynchronized(t *testing.T) {
	srv, ts, cfg, _ := setupTestServerForWS(t)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	validToken := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 1*time.Hour)

	headers := http.Header{}
	headers.Add("Cookie", "easy42_session="+validToken)

	// Open 3 client connections representing 3 frontend instances
	clientCount := 3
	conns := make([]*websocket.Conn, clientCount)
	for i := 0; i < clientCount; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
		if err != nil {
			t.Fatalf("client %d failed to connect: %v", i, err)
		}
		conns[i] = c
		defer c.Close()

		// Read greeting
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, _, _ = c.ReadMessage()
	}

	// Wait for registration
	time.Sleep(50 * time.Millisecond)
	if srv.wsHub.ClientCount() != clientCount {
		t.Fatalf("expected %d active clients, got %d", clientCount, srv.wsHub.ClientCount())
	}

	// Trigger config change
	err := srv.mgr.AddNode(config.Node{
		Name:      "shared-node",
		Host:      "10.10.10.10",
		IP:        "172.20.10.1",
		Interface: "lo",
		ASN:       4242421111,
	})
	if err != nil {
		t.Fatalf("failed to add node: %v", err)
	}

	// All 3 clients must receive the notification
	for i, c := range conns {
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, msg, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("client %d failed to receive broadcast: %v", i, err)
		}
		var ev map[string]any
		_ = json.Unmarshal(msg, &ev)
		if ev["type"] != "cluster_updated" {
			t.Fatalf("client %d got unexpected message type: %v", i, ev["type"])
		}
	}
}

func TestClientWebSocketPingPong(t *testing.T) {
	_, ts, cfg, _ := setupTestServerForWS(t)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	validToken := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 1*time.Hour)

	headers := http.Header{}
	headers.Add("Cookie", "easy42_session="+validToken)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Read greeting
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = conn.ReadMessage()

	// Send text ping
	pingPayload, _ := json.Marshal(map[string]string{"type": "ping"})
	if err := conn.WriteMessage(websocket.TextMessage, pingPayload); err != nil {
		t.Fatalf("failed to write ping: %v", err)
	}

	// Read pong
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, resp, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read pong: %v", err)
	}

	var m map[string]any
	_ = json.Unmarshal(resp, &m)
	if m["type"] != "pong" {
		t.Fatalf("expected 'pong', got: %v", m["type"])
	}
}
