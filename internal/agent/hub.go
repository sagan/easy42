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

// CronTaskReportListener handles cron task reports from agents
type CronTaskReportListener func(nodeName string, r *agentpb.CronTaskReport)

// ProbeConfigFile represents a probed remote configuration file with path, content, and hash
type ProbeConfigFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Hash    string `json:"hash"`
}

// Hub manages active agent connections and cached telemetry
type Hub struct {
	mu               sync.RWMutex
	connections      map[string]*AgentConnection
	telemetry        map[string]*agentpb.TelemetryReport
	lastSeen         map[string]time.Time
	appHashes        map[string]map[string]string          // nodeName -> appName -> sha256
	appFiles         map[string]map[string]ProbeConfigFile // nodeName -> filePath -> ProbeConfigFile
	appEnabled       map[string]map[string]bool            // nodeName -> appName -> last applied enabled state
	onTelemetry      []TelemetryListener
	onRegister       []RegisterListener
	onCronTaskReport []CronTaskReportListener
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		connections: make(map[string]*AgentConnection),
		telemetry:   make(map[string]*agentpb.TelemetryReport),
		lastSeen:    make(map[string]time.Time),
		appHashes:   make(map[string]map[string]string),
		appFiles:    make(map[string]map[string]ProbeConfigFile),
		appEnabled:  make(map[string]map[string]bool),
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

// OnCronTaskReport registers a listener invoked when a cron task result report is received
func (h *Hub) OnCronTaskReport(l CronTaskReportListener) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onCronTaskReport = append(h.onCronTaskReport, l)
}

// RecordCronTaskReport notifies listeners of an incoming cron task result
func (h *Hub) RecordCronTaskReport(nodeName string, r *agentpb.CronTaskReport) {
	h.mu.RLock()
	listeners := append([]CronTaskReportListener(nil), h.onCronTaskReport...)
	h.mu.RUnlock()

	for _, l := range listeners {
		go l(nodeName, r)
	}
}

// SyncCronTasks sends the latest cron tasks to a specific node agent if connected
func (h *Hub) SyncCronTasks(nodeName string, tasks []*agentpb.CronTaskSpec) error {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return nil // Agent not connected; will receive on next connect
	}
	return conn.SyncCronTasks(tasks)
}

// SyncHosts sends the latest cluster hosts to a specific node agent if connected
func (h *Hub) SyncHosts(nodeName string, hosts []*agentpb.HostRecord) error {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return nil // Agent not connected; will receive on next connect
	}
	return conn.SyncHosts(hosts)
}

// SyncApps sends the latest apps to a specific node agent if connected
func (h *Hub) SyncApps(nodeName string, apps []*agentpb.AppSpec) error {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return nil // Agent not connected; will receive on next connect
	}
	return conn.SyncApps(apps)
}

// SyncObfuscators sends the latest WireGuard obfuscator desired state to a specific node agent if connected
func (h *Hub) SyncObfuscators(nodeName string, specs []*agentpb.ObfuscatorSpec) error {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return nil // Agent not connected; will receive on next connect
	}
	return conn.SyncObfuscators(specs)
}

// GetHostsHash returns the latest reported hosts hash for a connected node agent
func (h *Hub) GetHostsHash(nodeName string) string {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return ""
	}
	return conn.HostsHash()
}

// GetOSInfo returns the reported OS info for a connected node agent
func (h *Hub) GetOSInfo(nodeName string) string {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return ""
	}
	return conn.OSInfo()
}

// GetInitSystem returns the init system reported by a connected node agent
// ("systemd", "procd" or "openrc"). It returns "" for offline nodes and for
// agents that predate the field.
func (h *Hub) GetInitSystem(nodeName string) string {
	conn, ok := h.GetConnection(nodeName)
	if !ok {
		return ""
	}
	return conn.InitSystem()
}

// IsAppsDisabled returns true if the node agent has disabled apps in its configuration
func (h *Hub) IsAppsDisabled(nodeName string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if conn, ok := h.connections[nodeName]; ok {
		return conn.DisableApps()
	}
	if t, ok := h.telemetry[nodeName]; ok {
		return t.DisableApps
	}
	return false
}

// SetAppHashes updates the cached deployed app hashes for a node
func (h *Hub) SetAppHashes(nodeName string, hashes map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appHashes == nil {
		h.appHashes = make(map[string]map[string]string)
	}
	cp := make(map[string]string, len(hashes))
	for k, v := range hashes {
		cp[k] = v
	}
	h.appHashes[nodeName] = cp
}

// GetAppHashes returns a copy of the cached deployed app hashes for a node
func (h *Hub) GetAppHashes(nodeName string) map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.appHashes == nil {
		return nil
	}
	res := make(map[string]string)
	for k, v := range h.appHashes[nodeName] {
		res[k] = v
	}
	return res
}

// SetAppHash updates the cached deployed hash of a specific app on a node
func (h *Hub) SetAppHash(nodeName, appName, hash string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appHashes == nil {
		h.appHashes = make(map[string]map[string]string)
	}
	if h.appHashes[nodeName] == nil {
		h.appHashes[nodeName] = make(map[string]string)
	}
	h.appHashes[nodeName][appName] = hash
}

// RemoveAppHash removes the cached deployed hash of a specific app on a node
func (h *Hub) RemoveAppHash(nodeName, appName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appHashes == nil || h.appHashes[nodeName] == nil {
		return
	}
	delete(h.appHashes[nodeName], appName)
}

// GetAppHash returns the cached deployed hash of a specific app on a node
func (h *Hub) GetAppHash(nodeName, appName string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.appHashes == nil {
		return ""
	}
	return h.appHashes[nodeName][appName]
}

// SetAppEnabled records the enabled state that was last applied for an app on a node.
// The agent probes cannot infer this from files alone, so it is tracked separately to
// detect enable/disable drift during sync planning.
func (h *Hub) SetAppEnabled(nodeName, appName string, enabled bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appEnabled == nil {
		h.appEnabled = make(map[string]map[string]bool)
	}
	if h.appEnabled[nodeName] == nil {
		h.appEnabled[nodeName] = make(map[string]bool)
	}
	h.appEnabled[nodeName][appName] = enabled
}

// GetAppEnabled returns the last applied enabled state and whether it is known.
func (h *Hub) GetAppEnabled(nodeName, appName string) (bool, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.appEnabled == nil || h.appEnabled[nodeName] == nil {
		return false, false
	}
	enabled, ok := h.appEnabled[nodeName][appName]
	return enabled, ok
}

// RemoveAppEnabled clears the recorded enabled state for an app on a node.
func (h *Hub) RemoveAppEnabled(nodeName, appName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appEnabled == nil || h.appEnabled[nodeName] == nil {
		return
	}
	delete(h.appEnabled[nodeName], appName)
}

// SetAppFiles updates the cached deployed app files for a node
func (h *Hub) SetAppFiles(nodeName string, files map[string]ProbeConfigFile) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appFiles == nil {
		h.appFiles = make(map[string]map[string]ProbeConfigFile)
	}
	cp := make(map[string]ProbeConfigFile, len(files))
	for k, v := range files {
		cp[k] = v
	}
	h.appFiles[nodeName] = cp
}

// SetAppFile updates or adds a single cached deployed app file for a node
func (h *Hub) SetAppFile(nodeName string, filePath string, file ProbeConfigFile) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appFiles == nil {
		h.appFiles = make(map[string]map[string]ProbeConfigFile)
	}
	if h.appFiles[nodeName] == nil {
		h.appFiles[nodeName] = make(map[string]ProbeConfigFile)
	}
	h.appFiles[nodeName][filePath] = file
}

// RemoveAppFile removes a cached deployed app file for a node
func (h *Hub) RemoveAppFile(nodeName string, filePath string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.appFiles == nil || h.appFiles[nodeName] == nil {
		return
	}
	delete(h.appFiles[nodeName], filePath)
}

// GetAppFiles returns a copy of the cached deployed app files for a node
func (h *Hub) GetAppFiles(nodeName string) map[string]ProbeConfigFile {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.appFiles == nil {
		return nil
	}
	orig := h.appFiles[nodeName]
	if orig == nil {
		return nil
	}
	cp := make(map[string]ProbeConfigFile, len(orig))
	for k, v := range orig {
		cp[k] = v
	}
	return cp
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

// Unregister removes a connection for a node if it matches the active connection
func (h *Hub) Unregister(conn *AgentConnection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if current, exists := h.connections[conn.NodeName()]; exists && current == conn {
		delete(h.connections, conn.NodeName())
	}
}

// UnregisterNode removes the active connection for a node by name
func (h *Hub) UnregisterNode(nodeName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.connections, nodeName)
}

// RemoveNode closes any active connection, removes the node from connections, telemetry, and lastSeen
func (h *Hub) RemoveNode(nodeName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if conn, exists := h.connections[nodeName]; exists {
		go conn.Close()
		delete(h.connections, nodeName)
	}
	delete(h.telemetry, nodeName)
	delete(h.lastSeen, nodeName)
}

// IsConnected returns whether the node agent is currently connected and active
func (h *Hub) IsConnected(nodeName string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, exists := h.connections[nodeName]
	if !exists {
		return false
	}
	if lastSeen, ok := h.lastSeen[nodeName]; ok && !lastSeen.IsZero() {
		if time.Since(lastSeen) > 45*time.Second {
			return false
		}
	}
	return true
}

// GetConnection retrieves the active connection for a node
func (h *Hub) GetConnection(nodeName string) (*AgentConnection, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conn, exists := h.connections[nodeName]
	if !exists {
		return nil, false
	}
	if lastSeen, ok := h.lastSeen[nodeName]; ok && !lastSeen.IsZero() {
		if time.Since(lastSeen) > 45*time.Second {
			return nil, false
		}
	}
	return conn, true
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

// GetAllTelemetry returns a snapshot copy of all cached node telemetry reports
func (h *Hub) GetAllTelemetry() map[string]*agentpb.TelemetryReport {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make(map[string]*agentpb.TelemetryReport, len(h.telemetry))
	for k, v := range h.telemetry {
		res[k] = v
	}
	return res
}

// GetAllConnections returns a snapshot copy of all active agent connections
func (h *Hub) GetAllConnections() map[string]*AgentConnection {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res := make(map[string]*AgentConnection, len(h.connections))
	for k, v := range h.connections {
		res[k] = v
	}
	return res
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
			info := config.InterfaceInfo{
				Name:      iface.Name,
				Up:        iface.IsUp,
				Addresses: iface.Addresses,
				Flags:     iface.Flags,
				RxBytes:   iface.RxBytes,
				TxBytes:   iface.TxBytes,
			}
			if (iface.Flags & config.IfaceFlagPrimary) != 0 {
				status.PrimaryInterface = iface.Name
			}
			status.Interfaces = append(status.Interfaces, info)
		}

		for _, disk := range telemetry.Disks {
			dm := config.DiskMetrics{
				Path:       disk.Path,
				TotalBytes: disk.TotalBytes,
				UsedBytes:  disk.UsedBytes,
				FreeBytes:  disk.FreeBytes,
			}
			status.Disks = append(status.Disks, dm)
			if status.Metrics == nil {
				status.Metrics = &config.SystemMetrics{}
			}
			status.Metrics.Disks = append(status.Metrics.Disks, dm)
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
