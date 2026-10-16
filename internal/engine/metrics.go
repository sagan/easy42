package engine

import (
	"sort"
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
		var primaryFound bool
		for _, iface := range t.Interfaces {
			input.Interfaces = append(input.Interfaces, config.InterfaceMetricInput{
				Name:      iface.Name,
				RxBytes:   iface.RxBytes,
				TxBytes:   iface.TxBytes,
				IsPrimary: (iface.Flags & config.IfaceFlagPrimary) != 0,
			})
			if (iface.Flags & config.IfaceFlagPrimary) != 0 {
				input.NetRxBytes = iface.RxBytes
				input.NetTxBytes = iface.TxBytes
				primaryFound = true
			}
		}
		if !primaryFound {
			for _, iface := range t.Interfaces {
				if iface.Name == "lo" {
					continue
				}
				input.NetRxBytes += iface.RxBytes
				input.NetTxBytes += iface.TxBytes
			}
		}
		_ = m.stateStore.RecordMetrics(nodeName, input)
		m.handleAgentTelemetryInterfaces(nodeName, t)
	})

	m.agentHub.OnRegister(func(conn *agent.AgentConnection) {
		_ = m.stateStore.RecordNodeInfo(conn.NodeName(), conn.AgentVersion(), conn.OSInfo(), conn.Hostname())
	})
}

// handleAgentTelemetryInterfaces updates the node interfaces and their working states in stateStore
// based on real-time agent telemetry.
func (m *Manager) handleAgentTelemetryInterfaces(nodeName string, t *agentpb.TelemetryReport) {
	if m.stateStore == nil || t == nil {
		return
	}

	m.mu.RLock()
	cfg := m.store.Get()
	var links []config.Link
	if cfg != nil {
		links = append([]config.Link(nil), cfg.Links...)
	}
	m.mu.RUnlock()

	type linkMeta struct {
		peerNode   string
		peerPubKey string
		keepalive  int
	}
	meta := make(map[string]linkMeta)
	for _, l := range links {
		ka := 0
		if l.From.PersistentKeepalive > 0 || l.To.PersistentKeepalive > 0 {
			ka = 25
		}
		if l.From.Name == nodeName {
			meta[l.From.Interface] = linkMeta{
				peerNode:   l.To.Name,
				peerPubKey: l.To.PublicKey,
				keepalive:  ka,
			}
		}
		if l.To.Name == nodeName {
			meta[l.To.Interface] = linkMeta{
				peerNode:   l.From.Name,
				peerPubKey: l.From.PublicKey,
				keepalive:  ka,
			}
		}
	}

	operUp := make(map[string]bool)
	for _, iface := range t.Interfaces {
		operUp[iface.Name] = iface.IsUp
	}

	type peerInfo struct {
		pubKey    string
		handshake time.Time
		rx        int64
		tx        int64
		keepalive int
	}
	wgMap := make(map[string][]peerInfo)
	for _, p := range t.WgPeers {
		var hs time.Time
		if p.LastHandshakeTime > 0 {
			hs = time.Unix(p.LastHandshakeTime, 0)
		}
		wgMap[p.InterfaceName] = append(wgMap[p.InterfaceName], peerInfo{
			pubKey:    p.PublicKey,
			handshake: hs,
			rx:        int64(p.RxBytes),
			tx:        int64(p.TxBytes),
			keepalive: int(p.PersistentKeepalive),
		})
	}

	allIfaces := make(map[string]bool)
	for ifaceName := range meta {
		allIfaces[ifaceName] = true
	}
	for ifaceName := range wgMap {
		if strings.HasPrefix(ifaceName, "wg42") {
			allIfaces[ifaceName] = true
		}
	}
	currState := m.stateStore.Get()
	if currState != nil && currState.Nodes != nil {
		if stNode, ok := currState.Nodes[nodeName]; ok {
			for ifaceName := range stNode.Interfaces {
				if strings.HasPrefix(ifaceName, "wg42") {
					allIfaces[ifaceName] = true
				}
			}
		}
	}

	if len(allIfaces) == 0 {
		return
	}

	now := time.Now()
	updates := make(map[string]config.StateInterface)

	for ifaceName := range allIfaces {
		if !strings.HasPrefix(ifaceName, "wg42") {
			continue
		}

		peers := wgMap[ifaceName]
		isStarted := operUp[ifaceName]
		if !isStarted && len(peers) > 0 {
			isStarted = true
		}

		status := "down"
		if isStarted {
			status = "active"
		}

		mInfo := meta[ifaceName]
		keepalive := mInfo.keepalive
		var latestHandshake time.Time
		var rxBytes, txBytes int64

		if len(peers) > 0 {
			var matched *peerInfo
			for idx := range peers {
				if mInfo.peerPubKey != "" && peers[idx].pubKey == mInfo.peerPubKey {
					matched = &peers[idx]
					break
				}
			}
			if matched == nil {
				matched = &peers[0]
			}
			latestHandshake = matched.handshake
			rxBytes = matched.rx
			txBytes = matched.tx
			if matched.keepalive > 0 {
				keepalive = matched.keepalive
			}
		}

		var handshakePtr *time.Time
		workingState := config.WorkingStateUnknown

		if !isStarted {
			workingState = config.WorkingStateNotWorking
		} else if !latestHandshake.IsZero() {
			handshakePtr = &latestHandshake
			if now.Sub(latestHandshake) <= 3*time.Minute {
				workingState = config.WorkingStateWorking
			} else {
				if keepalive > 0 {
					workingState = config.WorkingStateNotWorking
				} else {
					workingState = config.WorkingStateUnknown
				}
			}
		} else {
			if keepalive > 0 {
				workingState = config.WorkingStateNotWorking
			} else {
				workingState = config.WorkingStateUnknown
			}
		}

		updates[ifaceName] = config.StateInterface{
			Name:            ifaceName,
			TargetFile:      "/etc/wireguard/" + ifaceName + ".conf",
			PeerNode:        mInfo.peerNode,
			PeerPubKey:      mInfo.peerPubKey,
			Status:          status,
			LatestHandshake: handshakePtr,
			WorkingState:    workingState,
			TransferRxBytes: rxBytes,
			TransferTxBytes: txBytes,
		}
	}

	_ = m.stateStore.UpdateInterfacesTelemetry(nodeName, updates)
}

// startRetentionWorker starts the background task cleaning historical metrics and cron results
// older than the configured retention period (default 47 days), and periodically updating interface traffic stats
func (m *Manager) startRetentionWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	trafficTicker := time.NewTicker(30 * time.Second)
	go func() {
		// Run initial cleanup pass shortly after server startup
		select {
		case <-m.stopCh:
			ticker.Stop()
			trafficTicker.Stop()
			return
		case <-time.After(15 * time.Second):
			rawRetention := m.GetRawRetentionDuration()
			retention := m.GetRetentionDuration()
			_, _ = m.stateStore.RollupAndCleanMetrics(rawRetention, retention)
			_, _ = m.stateStore.RollupAndCleanCronResults(rawRetention, retention)
		}

		for {
			select {
			case <-m.stopCh:
				ticker.Stop()
				trafficTicker.Stop()
				return
			case <-trafficTicker.C:
				_ = m.stateStore.UpdateInterfaceTrafficStats("", "")
			case <-ticker.C:
				rawRetention := m.GetRawRetentionDuration()
				retention := m.GetRetentionDuration()
				_, _ = m.stateStore.RollupAndCleanMetrics(rawRetention, retention)
				_, _ = m.stateStore.RollupAndCleanCronResults(rawRetention, retention)
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

// GetNodeMetricsHistory retrieves recorded metric points for a node with optional interface selection
func (m *Manager) GetNodeMetricsHistory(nodeName string, rangeStr string, interfaceName string) ([]config.NodeMetricPoint, []string, string, error) {
	duration, maxPoints := ParseMetricRange(rangeStr)
	start := time.Now().Add(-duration)

	if interfaceName == "" {
		if m.agentHub != nil {
			if telem := m.agentHub.GetTelemetry(nodeName); telem != nil {
				for _, iface := range telem.Interfaces {
					if (iface.Flags & config.IfaceFlagPrimary) != 0 {
						interfaceName = iface.Name
						break
					}
				}
			}
		}
	}

	points, ifaces, selectedIface, err := m.stateStore.GetMetricsHistoryWithInterface(nodeName, interfaceName, start, maxPoints)
	if err != nil {
		return nil, nil, "", err
	}

	known := make(map[string]bool)
	for _, ifn := range ifaces {
		known[ifn] = true
	}
	if m.agentHub != nil {
		if telem := m.agentHub.GetTelemetry(nodeName); telem != nil {
			for _, iface := range telem.Interfaces {
				if iface.Name != "" && !known[iface.Name] {
					ifaces = append(ifaces, iface.Name)
					known[iface.Name] = true
				}
			}
		}
	}
	sort.Strings(ifaces)

	if selectedIface == "" && len(ifaces) > 0 {
		selectedIface = ifaces[0]
	}

	return points, ifaces, selectedIface, nil
}

// GetNodeInterfaceTraffic retrieves derived interface traffic statistics for any specified timespan
func (m *Manager) GetNodeInterfaceTraffic(nodeName, interfaceName string, start, end time.Time, maxPoints int) (*config.NodeInterfaceTrafficSummary, error) {
	if interfaceName == "" {
		if m.agentHub != nil {
			if telem := m.agentHub.GetTelemetry(nodeName); telem != nil {
				for _, iface := range telem.Interfaces {
					if (iface.Flags & config.IfaceFlagPrimary) != 0 {
						interfaceName = iface.Name
						break
					}
				}
			}
		}
	}
	return m.stateStore.GetNodeInterfaceTraffic(nodeName, interfaceName, start, end, maxPoints)
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

		if m.agentHub != nil {
			if telem := m.agentHub.GetTelemetry(n.Name); telem != nil {
				for _, iface := range telem.Interfaces {
					if (iface.Flags & config.IfaceFlagPrimary) != 0 {
						status.PrimaryInterface = iface.Name
						break
					}
				}
			}
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
