package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
)

// TelemetryListener handles telemetry reports received by the hub
type TelemetryListener func(nodeName string, t *agentpb.TelemetryReport)

// RegisterListener handles new agent registrations
type RegisterListener func(conn *AgentConnection)

// Hub manages active agent connections and cached telemetry
type Hub struct {
	mu          sync.RWMutex
	connections map[string]*AgentConnection
	telemetry   map[string]*agentpb.TelemetryReport
	lastSeen    map[string]time.Time
	onTelemetry []TelemetryListener
	onRegister  []RegisterListener
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		connections: make(map[string]*AgentConnection),
		telemetry:   make(map[string]*agentpb.TelemetryReport),
		lastSeen:    make(map[string]time.Time),
	}
}

// OnTelemetry registers a listener invoked when telemetry is received
func (h *Hub) OnTelemetry(l TelemetryListener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onTelemetry = append(h.onTelemetry, l)
}

// OnRegister registers a listener invoked when an agent registers
func (h *Hub) OnRegister(l RegisterListener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onRegister = append(h.onRegister, l)
}

// NotifyRegister triggers registered registration listeners
func (h *Hub) NotifyRegister(conn *AgentConnection) {
	h.mu.RLock()
	listeners := append([]RegisterListener(nil), h.onRegister...)
	h.mu.RUnlock()

	for _, l := range listeners {
		go l(conn)
	}
}

// Register registers an active connection for a node
func (h *Hub) Register(conn *AgentConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// If old connection exists, close it
	if old, exists := h.connections[conn.NodeName()]; exists && old != conn {
		go old.Close()
	}

	h.connections[conn.NodeName()] = conn
	h.lastSeen[conn.NodeName()] = time.Now()
}

// Unregister removes a connection for a node
func (h *Hub) Unregister(nodeName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, nodeName)
}

// IsConnected returns whether the node agent is currently connected
func (h *Hub) IsConnected(nodeName string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, exists := h.connections[nodeName]
	return exists
}

// GetConnection retrieves the active connection for a node
func (h *Hub) GetConnection(nodeName string) (*AgentConnection, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, exists := h.connections[nodeName]
	return conn, exists
}

// RecordHeartbeat records the last heartbeat time for a node
func (h *Hub) RecordHeartbeat(nodeName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastSeen[nodeName] = time.Now()
}

// UpdateTelemetry stores the latest telemetry report for a node
func (h *Hub) UpdateTelemetry(nodeName string, t *agentpb.TelemetryReport) {
	h.mu.Lock()
	h.telemetry[nodeName] = t
	h.lastSeen[nodeName] = time.Now()
	listeners := append([]TelemetryListener(nil), h.onTelemetry...)
	h.mu.Unlock()

	for _, l := range listeners {
		go l(nodeName, t)
	}
}

// GetTelemetry retrieves the latest telemetry report for a node
func (h *Hub) GetTelemetry(nodeName string) *agentpb.TelemetryReport {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.telemetry[nodeName]
}

// GetLastSeen returns the last time an agent sent telemetry or heartbeat
func (h *Hub) GetLastSeen(nodeName string) time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastSeen[nodeName]
}

// SendCommand dispatches a command to the specified node's agent
func (h *Hub) SendCommand(ctx context.Context, nodeName string, cmd *agentpb.CommandRequest) (*agentpb.CommandResponse, error) {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return nil, fmt.Errorf("node %s is not connected via agent", nodeName)
	}
	return conn.SendCommand(ctx, cmd)
}

// NodeStatus converts cached agent telemetry to config.NodeStatus
func (h *Hub) NodeStatus(nodeName string) *config.NodeStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	conn, connected := h.connections[nodeName]
	lastSeen := h.lastSeen[nodeName]
	telemetry := h.telemetry[nodeName]

	status := &config.NodeStatus{
		Name:         nodeName,
		Connected:    connected,
		LastSeen:     lastSeen,
		Mode:         "agent",
		Interfaces:   make([]config.InterfaceInfo, 0),
		WgInterfaces: make([]config.WgInterfaceStatus, 0),
	}

	if conn != nil {
		status.Hostname = conn.Hostname()
		status.AgentVersion = conn.AgentVersion()
	}

	if telemetry != nil {
		if telemetry.System != nil {
			status.Metrics = &config.SystemMetrics{
				CPUPercent:       telemetry.System.CpuPercent,
				MemoryUsedBytes:  telemetry.System.MemoryUsedBytes,
				MemoryTotalBytes: telemetry.System.MemoryTotalBytes,
				UptimeSeconds:    telemetry.System.UptimeSeconds,
				LoadAvg:          telemetry.System.LoadAvg,
			}
		}

		for _, iface := range telemetry.Interfaces {
			status.Interfaces = append(status.Interfaces, config.InterfaceInfo{
				Name:      iface.Name,
				Up:        iface.IsUp,
				Addresses: iface.Addresses,
			})
		}

		// Group WireGuard peers by interface
		wgMap := make(map[string]*config.WgInterfaceStatus)
		for _, peer := range telemetry.WgPeers {
			wg, exists := wgMap[peer.InterfaceName]
			if !exists {
				wg = &config.WgInterfaceStatus{
					Name:  peer.InterfaceName,
					Peers: make([]config.WgPeerStatus, 0),
				}
				wgMap[peer.InterfaceName] = wg
			}

			var lastHandshake time.Time
			if peer.LastHandshakeTime > 0 {
				lastHandshake = time.Unix(peer.LastHandshakeTime, 0)
			}

			wg.Peers = append(wg.Peers, config.WgPeerStatus{
				PublicKey:           peer.PublicKey,
				Endpoint:            peer.Endpoint,
				LatestHandshake:     lastHandshake,
				TransferRxBytes:     int64(peer.RxBytes),
				TransferTxBytes:     int64(peer.TxBytes),
				PersistentKeepalive: int(peer.PersistentKeepalive),
			})
		}

		for _, wg := range wgMap {
			status.WgInterfaces = append(status.WgInterfaces, *wg)
		}
	}

	return status
}

// GenerateToken creates a secure random hex token for agent authorization
func GenerateToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("e42_%d", time.Now().UnixNano())
	}
	return "e42_" + hex.EncodeToString(b)
}
