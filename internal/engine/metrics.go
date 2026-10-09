package engine

import (
	"strings"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
)

// setupAgentMetricsListeners registers telemetry and registration event handlers
func (m *Manager) setupAgentMetricsListeners() {
	if m.agentHub == nil || m.stateStore == nil {
		return
	}

	m.agentHub.OnTelemetry(func(nodeName string, t *agentpb.TelemetryReport) {
		if t == nil {
			return
		}
		input := config.NodeMetricInput{
			Timestamp: time.Now(),
		}
		if t.System != nil {
			input.CPUPercent = t.System.CpuPercent
			input.MemoryUsedBytes = t.System.MemoryUsedBytes
			input.MemoryTotalBytes = t.System.MemoryTotalBytes
			input.UptimeSeconds = t.System.UptimeSeconds
			if len(t.System.LoadAvg) > 0 {
				input.Load1m = t.System.LoadAvg[0]
			}
			if len(t.System.LoadAvg) > 1 {
				input.Load5m = t.System.LoadAvg[1]
			}
			if len(t.System.LoadAvg) > 2 {
				input.Load15m = t.System.LoadAvg[2]
			}
		}
		for _, iface := range t.Interfaces {
			if iface.Name == "lo" {
				continue
			}
			input.NetRxBytes += iface.RxBytes
			input.NetTxBytes += iface.TxBytes
		}
		_ = m.stateStore.RecordMetrics(nodeName, input)
	})

	m.agentHub.OnRegister(func(conn *agent.AgentConnection) {
		_ = m.stateStore.RecordNodeInfo(conn.NodeName(), conn.AgentVersion(), conn.OSInfo(), conn.Hostname())
	})
}

// startRetentionWorker starts the background task cleaning metrics older than 7 days
func (m *Manager) startRetentionWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for {
			select {
			case <-m.stopCh:
				ticker.Stop()
				return
			case <-ticker.C:
				_, _ = m.stateStore.CleanMetricsOlderThan(7 * 24 * time.Hour)
			}
		}
	}()
}

// ParseMetricRange parses range string into duration and maxPoints
func ParseMetricRange(rangeStr string) (time.Duration, int) {
	switch strings.ToLower(strings.TrimSpace(rangeStr)) {
	case "1h":
		return 1 * time.Hour, 120
	case "6h":
		return 6 * time.Hour, 200
	case "7d":
		return 7 * 24 * time.Hour, 300
	case "30d":
		return 30 * 24 * time.Hour, 300
	case "24h":
		fallthrough
	default:
		return 24 * time.Hour, 250
	}
}

// GetNodeMetricsHistory retrieves recorded metric points for a node
func (m *Manager) GetNodeMetricsHistory(nodeName string, rangeStr string) ([]config.NodeMetricPoint, error) {
	duration, maxPoints := ParseMetricRange(rangeStr)
	start := time.Now().Add(-duration)
	return m.stateStore.GetMetricsHistory(nodeName, start, maxPoints)
}

// GetFleetLiveStatus compiles a fleet summary and list of all nodes with live status
func (m *Manager) GetFleetLiveStatus() (config.FleetMetricsSummary, []config.NodeLiveStatus, error) {
	m.mu.RLock()
	cfg := m.store.Get()
	var nodes []config.Node
	if cfg != nil {
		nodes = cfg.Nodes
	}
	m.mu.RUnlock()

	allInfo, _ := m.stateStore.GetAllNodeInfo()
	latestMetrics, _ := m.stateStore.GetLatestMetrics()

	var (
		onlineCount  int
		offlineCount int
		noAgentCount int
		totalRxRate  float64
		totalTxRate  float64
		cpuSum       float64
		memPercSum   float64
		activeCount  int
	)

	list := make([]config.NodeLiveStatus, 0, len(nodes))

	for _, n := range nodes {
		// External nodes are completely excluded from fleet server monitoring
		if n.IsExternal {
			continue
		}

		info := allInfo[n.Name]
		connected := m.agentHub.IsConnected(n.Name)
		lastSeen := m.agentHub.GetLastSeen(n.Name)
		if lastSeen.IsZero() {
			lastSeen = info.LastSeen
		}

		status := config.NodeLiveStatus{
			Name:          n.Name,
			Hostname:      info.Hostname,
			OSInfo:        info.OSInfo,
			AgentVersion:  info.AgentVersion,
			UptimeSeconds: info.UptimeSeconds,
			Connected:     connected,
			LastSeen:      lastSeen,
		}

		if status.Hostname == "" {
			status.Hostname = n.Host
		}

		pt, hasMetrics := latestMetrics[n.Name]
		if hasMetrics {
			status.Metrics = &pt
			if pt.UptimeSeconds > status.UptimeSeconds {
				status.UptimeSeconds = pt.UptimeSeconds
			}
			totalRxRate += pt.NetRxRate
			totalTxRate += pt.NetTxRate
			if connected {
				cpuSum += float64(pt.CPUPercent)
				if pt.MemoryTotalBytes > 0 {
					memPercSum += float64(pt.MemoryUsedBytes) / float64(pt.MemoryTotalBytes) * 100.0
				}
				activeCount++
			}
		}

		// Check if agent was ever installed or connected
		agentInstalled := connected || info.AgentVersion != "" || !info.LastSeen.IsZero() || hasMetrics
		status.AgentInstalled = agentInstalled

		if connected {
			onlineCount++
		} else if agentInstalled {
			offlineCount++
		} else {
			noAgentCount++
		}

		list = append(list, status)
	}

	var avgCPU, avgMem float32
	if activeCount > 0 {
		avgCPU = float32(cpuSum / float64(activeCount))
		avgMem = float32(memPercSum / float64(activeCount))
	}

	summary := config.FleetMetricsSummary{
		TotalNodes:     len(list),
		OnlineNodes:    onlineCount,
		OfflineNodes:   offlineCount,
		NoAgentNodes:   noAgentCount,
		TotalRxRate:    totalRxRate,
		TotalTxRate:    totalTxRate,
		AverageCPU:     avgCPU,
		AverageMemPerc: avgMem,
	}

	return summary, list, nil
}
