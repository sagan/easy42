package engine

import (
	"strings"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
)

// setupAgentHostsListeners registers host synchronization hooks on the agent hub
func (m *Manager) setupAgentHostsListeners() {
	if m.agentHub == nil {
		return
	}

	// 1. When agent registers, immediately check and push latest hosts info if needed
	m.agentHub.OnRegister(func(conn *agent.AgentConnection) {
		m.SyncHostsToNode(conn.NodeName())
	})

	// 2. When agent reports telemetry, verify if hosts block state is identical; if not, push latest hosts info
	m.agentHub.OnTelemetry(func(nodeName string, t *agentpb.TelemetryReport) {
		if t != nil && t.HostsHash != "" {
			m.checkAndSyncHostsIfMismatched(nodeName, t.HostsHash)
		}
	})
}

// GetClusterHostRecords returns the list of active cluster node name-to-IPv4 records
func (m *Manager) GetClusterHostRecords() []config.HostRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil
	}
	var records []config.HostRecord
	for _, n := range cfg.Nodes {
		if !n.IsExternal && strings.TrimSpace(n.IP) != "" {
			records = append(records, config.HostRecord{
				Name: strings.TrimSpace(n.Name),
				IP:   strings.TrimSpace(n.IP),
			})
		}
	}
	return records
}

// buildClusterHostRecords returns protobuf HostRecord entries for agent communication
func (m *Manager) buildClusterHostRecords() []*agentpb.HostRecord {
	records := m.GetClusterHostRecords()
	res := make([]*agentpb.HostRecord, len(records))
	for i, r := range records {
		res[i] = &agentpb.HostRecord{
			Name: r.Name,
			Ip:   r.IP,
		}
	}
	return res
}

// ExpectedHostsBlock generates the full expected easy42 block and its SHA-256 hash
func (m *Manager) ExpectedHostsBlock() (string, string) {
	records := m.GetClusterHostRecords()
	block := config.BuildHostsBlock(records)
	hash := config.HashHostsBlock(block)
	return block, hash
}

// SyncHostsToNode pushes the latest hosts records to a specific node agent if its hash differs
func (m *Manager) SyncHostsToNode(nodeName string) {
	if m.agentHub == nil || !m.agentHub.IsConnected(nodeName) {
		return
	}

	node := m.FindNode(nodeName)
	if node == nil || !node.UpdateHosts || !node.IsAgentMode() {
		return
	}

	_, expectedHash := m.ExpectedHostsBlock()
	currentHash := m.agentHub.GetHostsHash(nodeName)
	if currentHash != "" && currentHash == expectedHash {
		return
	}

	records := m.buildClusterHostRecords()
	_ = m.agentHub.SyncHosts(nodeName, records)
}

// checkAndSyncHostsIfMismatched pushes latest hosts info if reported state is not identical
func (m *Manager) checkAndSyncHostsIfMismatched(nodeName string, currentHash string) {
	if m.agentHub == nil || !m.agentHub.IsConnected(nodeName) {
		return
	}

	node := m.FindNode(nodeName)
	if node == nil || !node.UpdateHosts || !node.IsAgentMode() {
		return
	}

	_, expectedHash := m.ExpectedHostsBlock()
	if currentHash != expectedHash {
		records := m.buildClusterHostRecords()
		_ = m.agentHub.SyncHosts(nodeName, records)
	}
}

// SyncHostsToAgents pushes up to date hosts info to all connected node agents with UpdateHosts enabled
func (m *Manager) SyncHostsToAgents() {
	if m.agentHub == nil {
		return
	}

	m.mu.RLock()
	cfg := m.store.Get()
	if cfg == nil {
		m.mu.RUnlock()
		return
	}
	var targetNodes []string
	for _, n := range cfg.Nodes {
		if n.UpdateHosts && n.IsAgentMode() {
			targetNodes = append(targetNodes, n.Name)
		}
	}
	m.mu.RUnlock()

	for _, name := range targetNodes {
		m.SyncHostsToNode(name)
	}
}
