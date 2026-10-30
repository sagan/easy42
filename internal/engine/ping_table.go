package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	agentpb "easy42/internal/agent/proto"
	"easy42/internal/compiler"
	"easy42/internal/config"
	"easy42/internal/lookingglass"
)

type directedPingTask struct {
	from string
	to   string
}

// RunPingTable starts an asynchronous background ping table task
func (m *Manager) RunPingTable(latencyType string) (*config.PingTableTask, error) {
	m.pingTableMu.Lock()

	// Check if a ping table task is already running
	if m.currentPingTable != nil && m.currentPingTable.Status == "running" {
		taskCopy := *m.currentPingTable
		m.pingTableMu.Unlock()
		return &taskCopy, nil
	}

	normType := strings.ToLower(strings.TrimSpace(latencyType))
	if normType != "link" {
		normType = "e2e"
	}

	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			m.pingTableMu.Unlock()
			return nil, fmt.Errorf("failed to load configuration: %w", err)
		}
	}

	// Filter and sort internal nodes (!n.IsExternal)
	var internalNodes []*config.Node
	for _, n := range cfg.Nodes {
		if !n.IsExternal {
			nodeCopy := n
			internalNodes = append(internalNodes, &nodeCopy)
		}
	}

	sort.Slice(internalNodes, func(i, j int) bool {
		return internalNodes[i].Name < internalNodes[j].Name
	})

	nodeNames := make([]string, len(internalNodes))
	nodeMap := make(map[string]*config.Node, len(internalNodes))
	for i, n := range internalNodes {
		nodeNames[i] = n.Name
		nodeMap[n.Name] = n
	}

	taskID := fmt.Sprintf("pt-%d", time.Now().UnixMilli())
	totalPairs := 0
	if len(nodeNames) >= 2 {
		totalPairs = len(nodeNames) * (len(nodeNames) - 1)
	}

	results := make(map[string]map[string]*config.PingTableItem)
	for _, u := range nodeNames {
		results[u] = make(map[string]*config.PingTableItem)
		for _, v := range nodeNames {
			if u == v {
				results[u][v] = &config.PingTableItem{
					FromNode: u,
					ToNode:   v,
					Status:   "self",
				}
			} else {
				results[u][v] = &config.PingTableItem{
					FromNode: u,
					ToNode:   v,
					Status:   "pending",
				}
			}
		}
	}

	task := &config.PingTableTask{
		ID:             taskID,
		CreatedAt:      time.Now().Unix(),
		LatencyType:    normType,
		Status:         "running",
		TotalPairs:     totalPairs,
		CompletedPairs: 0,
		Nodes:          nodeNames,
		Results:        results,
	}

	m.currentPingTable = task
	taskCtx, cancel := context.WithCancel(context.Background())
	m.pingTableCancel = cancel

	_ = m.stateStore.RecordPingTableTask(*task)
	m.broadcastPingTableUpdateLocked(task)
	m.pingTableMu.Unlock()

	if len(nodeNames) < 2 {
		m.pingTableMu.Lock()
		task.Status = "completed"
		task.FinishedAt = time.Now().Unix()
		_ = m.stateStore.RecordPingTableTask(*task)
		m.broadcastPingTableUpdateLocked(task)
		m.pingTableMu.Unlock()
		return task, nil
	}

	go m.executePingTable(taskCtx, task, cfg, nodeMap)

	return task, nil
}

// CancelPingTable terminates the ongoing ping table task early
func (m *Manager) CancelPingTable() error {
	m.pingTableMu.Lock()
	defer m.pingTableMu.Unlock()

	if m.currentPingTable == nil || m.currentPingTable.Status != "running" {
		return nil
	}

	if m.pingTableCancel != nil {
		m.pingTableCancel()
	}

	m.currentPingTable.Status = "cancelled"
	m.currentPingTable.FinishedAt = time.Now().Unix()
	_ = m.stateStore.RecordPingTableTask(*m.currentPingTable)
	m.broadcastPingTableUpdateLocked(m.currentPingTable)

	return nil
}

// GetActivePingTable returns the current active or most recent ping table task
func (m *Manager) GetActivePingTable() *config.PingTableTask {
	m.pingTableMu.RLock()
	defer m.pingTableMu.RUnlock()

	if m.currentPingTable != nil {
		copyTask := *m.currentPingTable
		return &copyTask
	}
	return nil
}

// GetPingTableHistory retrieves recorded ping table task summaries
func (m *Manager) GetPingTableHistory(limit int) ([]config.PingTableTaskSummary, error) {
	return m.stateStore.ListPingTableTasks(limit)
}

// GetPingTableTask retrieves a specific ping table task by ID
func (m *Manager) GetPingTableTask(id string) (*config.PingTableTask, error) {
	m.pingTableMu.RLock()
	if m.currentPingTable != nil && m.currentPingTable.ID == id {
		copyTask := *m.currentPingTable
		m.pingTableMu.RUnlock()
		return &copyTask, nil
	}
	m.pingTableMu.RUnlock()

	return m.stateStore.GetPingTableTask(id)
}

// DeletePingTableTask deletes a ping table task from database
func (m *Manager) DeletePingTableTask(id string) error {
	m.pingTableMu.Lock()
	if m.currentPingTable != nil && m.currentPingTable.ID == id {
		m.currentPingTable = nil
	}
	m.pingTableMu.Unlock()

	return m.stateStore.DeletePingTableTask(id)
}

// SubscribePingTable subscribes to ping table task updates
func (m *Manager) SubscribePingTable() (chan *config.PingTableTask, func()) {
	ch := make(chan *config.PingTableTask, 50)

	m.pingTableMu.Lock()
	if m.pingTableSubscribers == nil {
		m.pingTableSubscribers = make(map[chan *config.PingTableTask]struct{})
	}
	m.pingTableSubscribers[ch] = struct{}{}
	var currentCopy *config.PingTableTask
	if m.currentPingTable != nil {
		c := *m.currentPingTable
		currentCopy = &c
	}
	m.pingTableMu.Unlock()

	if currentCopy != nil {
		select {
		case ch <- currentCopy:
		default:
		}
	}

	unsubscribe := func() {
		m.pingTableMu.Lock()
		delete(m.pingTableSubscribers, ch)
		m.pingTableMu.Unlock()
		close(ch)
	}

	return ch, unsubscribe
}

func (m *Manager) broadcastPingTableUpdateLocked(task *config.PingTableTask) {
	if task == nil {
		return
	}
	taskCopy := *task
	for ch := range m.pingTableSubscribers {
		select {
		case ch <- &taskCopy:
		default:
		}
	}
}

func (m *Manager) updatePairResult(task *config.PingTableTask, u, v string, item *config.PingTableItem) {
	m.pingTableMu.Lock()
	defer m.pingTableMu.Unlock()

	if task.Results == nil {
		task.Results = make(map[string]map[string]*config.PingTableItem)
	}
	if task.Results[u] == nil {
		task.Results[u] = make(map[string]*config.PingTableItem)
	}

	task.Results[u][v] = item
	task.CompletedPairs++
	_ = m.stateStore.RecordPingTableTask(*task)
	m.broadcastPingTableUpdateLocked(task)
}

func (m *Manager) finalizePingTableTask(task *config.PingTableTask, ctx context.Context) {
	m.pingTableMu.Lock()
	if task.Status == "running" {
		if ctx.Err() != nil {
			task.Status = "cancelled"
		} else {
			task.Status = "completed"
		}
		task.FinishedAt = time.Now().Unix()
		_ = m.stateStore.RecordPingTableTask(*task)
		m.broadcastPingTableUpdateLocked(task)
	}
	m.pingTableMu.Unlock()
}

func preparePingArgs(
	taskType string,
	fromNode, toNode string,
	cfg *config.Config,
	nodeMap map[string]*config.Node,
) (targetIP string, iface string, err error) {
	if taskType == "link" {
		var foundLink *config.Link
		var fromIsFrom bool
		for i := range cfg.Links {
			l := &cfg.Links[i]
			if l.From.Name == fromNode && l.To.Name == toNode {
				foundLink = l
				fromIsFrom = true
				break
			} else if l.From.Name == toNode && l.To.Name == fromNode {
				foundLink = l
				fromIsFrom = false
				break
			}
		}

		if foundLink == nil {
			return "", "", fmt.Errorf("no direct link between %s and %s", fromNode, toNode)
		}

		if fromIsFrom {
			iface = foundLink.From.Interface
			targetIP = foundLink.To.Address
		} else {
			iface = foundLink.To.Interface
			targetIP = foundLink.From.Address
		}

		if iface == "" {
			iface = compiler.GetInterfaceName(toNode, false)
		}

		targetIP = strings.TrimSpace(targetIP)
		if idx := strings.Index(targetIP, "/"); idx != -1 {
			targetIP = targetIP[:idx]
		}
		if targetIP == "" {
			if targetNode := nodeMap[toNode]; targetNode != nil && targetNode.IP != "" {
				if derived, err := compiler.DeriveIPv6LinkLocalAddressOnly(targetNode.IP); err == nil {
					targetIP = derived
				}
			}
		}

		if targetIP == "" {
			return "", "", fmt.Errorf("link address not configured for %s", toNode)
		}

		return targetIP, iface, nil
	}

	// End-to-end latency
	targetNode := nodeMap[toNode]
	if targetNode == nil || strings.TrimSpace(targetNode.IP) == "" {
		return "", "", fmt.Errorf("main IPv4 not configured for %s", toNode)
	}

	return strings.TrimSpace(targetNode.IP), "", nil
}

func (m *Manager) executeDirectedPing(
	ctx context.Context,
	task *config.PingTableTask,
	fromNode, toNode string,
	cfg *config.Config,
	nodeMap map[string]*config.Node,
) *config.PingTableItem {
	if ctx.Err() != nil {
		return &config.PingTableItem{
			FromNode: fromNode,
			ToNode:   toNode,
			Status:   "cancelled",
			Error:    "task cancelled",
		}
	}

	targetIP, iface, err := preparePingArgs(task.LatencyType, fromNode, toNode, cfg, nodeMap)
	if err != nil {
		return &config.PingTableItem{
			FromNode: fromNode,
			ToNode:   toNode,
			Status:   "failed",
			Error:    err.Error(),
		}
	}

	lgCmd := &agentpb.LookingGlassCmd{
		Tool:           agentpb.LookingGlassCmd_PING,
		Target:         targetIP,
		Args:           []string{"3"},
		Interface:      iface,
		TimeoutSeconds: 4,
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	resp, err := m.agentHub.SendCommand(pingCtx, fromNode, &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_LookingGlass{
			LookingGlass: lgCmd,
		},
	})
	cancel()

	if err != nil {
		return &config.PingTableItem{
			FromNode:  fromNode,
			ToNode:    toNode,
			TargetIP:  targetIP,
			Interface: iface,
			Status:    "failed",
			Error:     err.Error(),
		}
	}

	parsed := lookingglass.ParsePing(resp.Stdout)
	if resp.ExitCode == 0 && parsed.PacketsReceived > 0 && (parsed.AvgRTT > 0 || parsed.MinRTT > 0) {
		lat := parsed.AvgRTT
		if lat <= 0 {
			lat = parsed.MinRTT
		}
		return &config.PingTableItem{
			FromNode:      fromNode,
			ToNode:        toNode,
			TargetIP:      targetIP,
			Interface:     iface,
			LatencyMs:     &lat,
			PacketLossPct: parsed.PacketLossPct,
			Status:        "success",
		}
	}

	// Ping timed out / unreachable / 100% loss
	loss := parsed.PacketLossPct
	if loss == 0 && parsed.PacketsSent == 0 {
		loss = 100.0
	}
	errMsg := strings.TrimSpace(resp.Stderr)
	if errMsg == "" {
		errMsg = "Ping timeout (100% packet loss)"
	}

	return &config.PingTableItem{
		FromNode:      fromNode,
		ToNode:        toNode,
		TargetIP:      targetIP,
		Interface:     iface,
		PacketLossPct: loss,
		Status:        "unreachable",
		Error:         errMsg,
	}
}

func (m *Manager) executePingTable(ctx context.Context, task *config.PingTableTask, cfg *config.Config, nodeMap map[string]*config.Node) {
	// Allow up to 25 parallel dispatched ping tasks globally
	const maxConcurrentPings = 25

	var pendingQueue []*directedPingTask

	for _, u := range task.Nodes {
		for _, v := range task.Nodes {
			if u == v {
				continue
			}

			// Check link latency constraint upfront
			if task.LatencyType == "link" {
				hasLink := false
				for _, l := range cfg.Links {
					if (l.From.Name == u && l.To.Name == v) || (l.From.Name == v && l.To.Name == u) {
						hasLink = true
						break
					}
				}
				if !hasLink {
					m.updatePairResult(task, u, v, &config.PingTableItem{
						FromNode: u,
						ToNode:   v,
						Status:   "no_link",
					})
					continue
				}
			}

			// Check agent connectivity of source node upfront
			if m.agentHub == nil || !m.agentHub.IsConnected(u) {
				m.updatePairResult(task, u, v, &config.PingTableItem{
					FromNode: u,
					ToNode:   v,
					Status:   "no_agent",
					Error:    fmt.Sprintf("Node %s has no active agent connected", u),
				})
				continue
			}

			pendingQueue = append(pendingQueue, &directedPingTask{
				from: u,
				to:   v,
			})
		}
	}

	if len(pendingQueue) == 0 {
		m.finalizePingTableTask(task, ctx)
		return
	}

	schedMu := sync.Mutex{}
	cond := sync.NewCond(&schedMu)
	busyNodes := make(map[string]bool)
	activeTasks := 0

	for {
		if ctx.Err() != nil {
			break
		}

		schedMu.Lock()
		if len(pendingQueue) == 0 && activeTasks == 0 {
			schedMu.Unlock()
			break
		}

		// Look for an item in pendingQueue whose from node is not currently busy
		pickedIdx := -1
		if activeTasks < maxConcurrentPings {
			for idx, item := range pendingQueue {
				if !busyNodes[item.from] {
					pickedIdx = idx
					break
				}
			}
		}

		if pickedIdx == -1 {
			if activeTasks == 0 && len(pendingQueue) > 0 {
				// No active tasks and remaining items cannot be scheduled
				for _, item := range pendingQueue {
					m.updatePairResult(task, item.from, item.to, &config.PingTableItem{
						FromNode: item.from,
						ToNode:   item.to,
						Status:   "failed",
						Error:    "Could not schedule: agent disconnected",
					})
				}
				pendingQueue = nil
				schedMu.Unlock()
				break
			}

			// Wait for a running task to complete and wake us up
			cond.Wait()
			schedMu.Unlock()
			continue
		}

		item := pendingQueue[pickedIdx]
		pendingQueue = append(pendingQueue[:pickedIdx], pendingQueue[pickedIdx+1:]...)

		busyNodes[item.from] = true
		activeTasks++
		schedMu.Unlock()

		go func(from, to string) {
			resultItem := m.executeDirectedPing(ctx, task, from, to, cfg, nodeMap)

			schedMu.Lock()
			delete(busyNodes, from)
			activeTasks--

			m.updatePairResult(task, from, to, resultItem)

			cond.Broadcast()
			schedMu.Unlock()
		}(item.from, item.to)
	}

	// Wait for any remaining active tasks if context was cancelled
	schedMu.Lock()
	for activeTasks > 0 {
		cond.Wait()
	}
	schedMu.Unlock()

	m.finalizePingTableTask(task, ctx)
}
