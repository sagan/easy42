package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	agentpb "easy42/internal/agent/proto"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// AgentConnection represents an active WebSocket session with a remote agent
type AgentConnection struct {
	nodeName     string
	agentVersion string
	osInfo       string
	hostname     string
	ws           *websocket.Conn
	hub          *Hub
	sendMu       sync.Mutex
	closeOnce    sync.Once
	closed       chan struct{}

	seq uint64

	// Pending command requests: request_id -> chan *agentpb.CommandResponse
	pendingMu sync.Mutex
	pending   map[string]chan *agentpb.CommandResponse
}

// NewConnection creates a new AgentConnection
func NewConnection(nodeName string, ws *websocket.Conn, hub *Hub) *AgentConnection {
	return &AgentConnection{
		nodeName: nodeName,
		ws:       ws,
		hub:      hub,
		closed:   make(chan struct{}),
		pending:  make(map[string]chan *agentpb.CommandResponse),
	}
}

// NodeName returns the connected node name
func (c *AgentConnection) NodeName() string {
	return c.nodeName
}

// AgentVersion returns the reported agent version
func (c *AgentConnection) AgentVersion() string {
	return c.agentVersion
}

// Hostname returns the reported agent hostname
func (c *AgentConnection) Hostname() string {
	return c.hostname
}

// OSInfo returns the reported operating system info
func (c *AgentConnection) OSInfo() string {
	return c.osInfo
}

// Close closes the WebSocket connection gracefully
func (c *AgentConnection) Close() {
	c.closeOnce.Do(func() {
		close(c.closed)
		_ = c.ws.Close()
		c.hub.Unregister(c.nodeName)

		c.pendingMu.Lock()
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()
	})
}

// SendMessage serializes and sends an AgentMessage over WebSocket
func (c *AgentConnection) SendMessage(msg *agentpb.AgentMessage) error {
	msg.Seq = atomic.AddUint64(&c.seq, 1)
	msg.Timestamp = time.Now().UnixMilli()

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal agent message: %w", err)
	}

	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	c.ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.ws.WriteMessage(websocket.BinaryMessage, data)
}

// SendCommand sends a CommandRequest and waits for the matching CommandResponse
func (c *AgentConnection) SendCommand(ctx context.Context, cmd *agentpb.CommandRequest) (*agentpb.CommandResponse, error) {
	if cmd.RequestId == "" {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		cmd.RequestId = hex.EncodeToString(b)
	}

	respCh := make(chan *agentpb.CommandResponse, 1)

	c.pendingMu.Lock()
	c.pending[cmd.RequestId] = respCh
	c.pendingMu.Unlock()

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, cmd.RequestId)
		c.pendingMu.Unlock()
	}()

	msg := &agentpb.AgentMessage{
		Payload: &agentpb.AgentMessage_CommandReq{
			CommandReq: cmd,
		},
	}

	if err := c.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("failed to send command to agent: %w", err)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closed:
		return nil, fmt.Errorf("agent connection closed")
	case resp, ok := <-respCh:
		if !ok || resp == nil {
			return nil, fmt.Errorf("connection closed before response received")
		}
		return resp, nil
	}
}

// ReadLoop processes incoming messages from the agent WebSocket
func (c *AgentConnection) ReadLoop() {
	defer c.Close()

	// Configure ping/pong handling
	c.ws.SetReadLimit(16 * 1024 * 1024) // 16MB max message
	_ = c.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.ws.SetPongHandler(func(string) error {
		_ = c.ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		msgType, data, err := c.ws.ReadMessage()
		if err != nil {
			break
		}
		_ = c.ws.SetReadDeadline(time.Now().Add(60 * time.Second))

		if msgType != websocket.BinaryMessage {
			continue
		}

		var msg agentpb.AgentMessage
		if err := proto.Unmarshal(data, &msg); err != nil {
			continue
		}

		c.handleIncomingMessage(&msg)
	}
}

func (c *AgentConnection) handleIncomingMessage(msg *agentpb.AgentMessage) {
	switch payload := msg.Payload.(type) {
	case *agentpb.AgentMessage_RegisterReq:
		req := payload.RegisterReq
		c.agentVersion = req.AgentVersion
		c.osInfo = req.OsInfo
		c.hostname = req.Hostname

		// Acknowledge registration
		_ = c.SendMessage(&agentpb.AgentMessage{
			Payload: &agentpb.AgentMessage_RegisterResp{
				RegisterResp: &agentpb.RegisterResponse{
					Success:               true,
					TelemetryIntervalSecs: 10,
				},
			},
		})
		c.hub.NotifyRegister(c)

	case *agentpb.AgentMessage_Heartbeat:
		// Heartbeat response
		_ = c.SendMessage(&agentpb.AgentMessage{
			Payload: &agentpb.AgentMessage_HeartbeatAck{
				HeartbeatAck: &agentpb.HeartbeatAck{
					ServerTime: time.Now().UnixMilli(),
				},
			},
		})
		c.hub.RecordHeartbeat(c.nodeName)

	case *agentpb.AgentMessage_Telemetry:
		c.hub.UpdateTelemetry(c.nodeName, payload.Telemetry)

	case *agentpb.AgentMessage_CommandResp:
		resp := payload.CommandResp
		c.pendingMu.Lock()
		if ch, exists := c.pending[resp.RequestId]; exists {
			ch <- resp
		}
		c.pendingMu.Unlock()
	}
}
