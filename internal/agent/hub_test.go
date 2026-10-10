package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentpb "easy42/internal/agent/proto"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestGenerateToken(t *testing.T) {
	tok1 := GenerateToken()
	tok2 := GenerateToken()
	if !strings.HasPrefix(tok1, "e42_") {
		t.Fatalf("expected token prefix e42_, got: %s", tok1)
	}
	if tok1 == tok2 {
		t.Fatalf("tokens should be unique, got duplicate: %s", tok1)
	}
}

func TestHubRegistrationAndStatus(t *testing.T) {
	hub := NewHub()

	if hub.IsConnected("node1") {
		t.Fatalf("expected node1 not to be connected initially")
	}

	// Mock WebSocket server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := NewConnection("node1", ws, hub)
		hub.Register(conn)
		go conn.ReadLoop()
	}))
	defer server.Close()

	// Connect mock agent client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientWs, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial mock server: %v", err)
	}
	defer clientWs.Close()

	time.Sleep(50 * time.Millisecond)

	if !hub.IsConnected("node1") {
		t.Fatalf("expected node1 to be connected after registration")
	}

	// Send RegisterRequest from client
	regMsg := &agentpb.AgentMessage{
		Seq:       1,
		Timestamp: time.Now().UnixMilli(),
		Payload: &agentpb.AgentMessage_RegisterReq{
			RegisterReq: &agentpb.RegisterRequest{
				NodeName:     "node1",
				AgentVersion: "0.1.0",
				Hostname:     "node1-host",
			},
		},
	}
	data, _ := proto.Marshal(regMsg)
	_ = clientWs.WriteMessage(websocket.BinaryMessage, data)

	// Update telemetry in hub
	hub.UpdateTelemetry("node1", &agentpb.TelemetryReport{
		Interfaces: []*agentpb.InterfaceMetrics{
			{Name: "eth0", IsUp: true, Addresses: []string{"192.168.1.100/24"}},
		},
		WgPeers: []*agentpb.WgPeerMetrics{
			{
				InterfaceName:     "wg42node2",
				PublicKey:         "pubkey123",
				Endpoint:          "1.2.3.4:51820",
				LastHandshakeTime: time.Now().Unix(),
				RxBytes:           1024,
				TxBytes:           2048,
			},
		},
	})

	status := hub.NodeStatus("node1")
	if !status.Connected {
		t.Fatalf("expected status.Connected to be true")
	}
	if len(status.Interfaces) != 1 || status.Interfaces[0].Name != "eth0" {
		t.Fatalf("unexpected interfaces: %+v", status.Interfaces)
	}
	if len(status.WgInterfaces) != 1 || len(status.WgInterfaces[0].Peers) != 1 {
		t.Fatalf("unexpected wg interfaces: %+v", status.WgInterfaces)
	}
}

func TestHubCommandDispatch(t *testing.T) {
	hub := NewHub()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := NewConnection("node-cmd", ws, hub)
		hub.Register(conn)
		go conn.ReadLoop()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientWs, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial mock server: %v", err)
	}
	defer clientWs.Close()

	time.Sleep(50 * time.Millisecond)

	// Handle command in background mock agent
	go func() {
		_, msgData, err := clientWs.ReadMessage()
		if err != nil {
			return
		}
		var msg agentpb.AgentMessage
		if err := proto.Unmarshal(msgData, &msg); err != nil {
			return
		}

		cmdReq := msg.GetCommandReq()
		if cmdReq != nil {
			resp := &agentpb.AgentMessage{
				Seq:       2,
				Timestamp: time.Now().UnixMilli(),
				Payload: &agentpb.AgentMessage_CommandResp{
					CommandResp: &agentpb.CommandResponse{
						RequestId: cmdReq.RequestId,
						Success:   true,
						Stdout:    "ok-reloaded",
					},
				},
			}
			respBytes, _ := proto.Marshal(resp)
			_ = clientWs.WriteMessage(websocket.BinaryMessage, respBytes)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := hub.SendCommand(ctx, "node-cmd", &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_ReloadBird{
			ReloadBird: &agentpb.ReloadBirdCmd{},
		},
	})
	if err != nil {
		t.Fatalf("SendCommand failed: %v", err)
	}
	if !resp.Success || resp.Stdout != "ok-reloaded" {
		t.Fatalf("unexpected command response: %+v", resp)
	}
}

func TestHubReconnectRace(t *testing.T) {
	hub := NewHub()

	// Mock server that registers connections
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := NewConnection("node-race", ws, hub)
		hub.Register(conn)
		go conn.ReadLoop()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// 1. Establish first connection
	client1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial client 1: %v", err)
	}
	defer client1.Close()

	time.Sleep(50 * time.Millisecond)
	if !hub.IsConnected("node-race") {
		t.Fatalf("expected node-race to be connected on client 1")
	}

	// 2. Establish second connection (reconnect) for the same node
	client2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial client 2: %v", err)
	}
	defer client2.Close()

	// Wait for old connection to be closed and old.Close() goroutine to finish
	time.Sleep(100 * time.Millisecond)

	// 3. node-race MUST still be connected via client 2!
	if !hub.IsConnected("node-race") {
		t.Fatalf("node-race was erroneously unregistered when old connection closed!")
	}
}
