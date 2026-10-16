package engine

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
	"easy42/internal/lookingglass"
	"github.com/google/uuid"
)

// setupAgentCronTasksListeners registers cron task reporting and registration hooks
func (m *Manager) setupAgentCronTasksListeners() {
	if m.agentHub == nil {
		return
	}

	// 1. When agent registers, immediately push its assigned cron tasks
	m.agentHub.OnRegister(func(conn *agent.AgentConnection) {
		m.SyncCronTasksToNode(conn.NodeName())
	})

	// 2. When agent reports a cron task execution result, parse and store into SQLite
	m.agentHub.OnCronTaskReport(func(nodeName string, r *agentpb.CronTaskReport) {
		if r == nil || m.stateStore == nil {
			return
		}

		// Look up task configuration for metadata
		cfg := m.store.Get()
		var foundTask *config.CronTask
		if cfg != nil {
			for _, t := range cfg.CronTasks {
				if t.ID == r.TaskId {
					foundTask = &t
					break
				}
			}
		}

		taskName := r.TaskId
		taskType := "ping"
		target := ""
		if foundTask != nil {
			taskName = foundTask.Name
			taskType = foundTask.TaskType
			if foundTask.Params != nil {
				target = foundTask.Params["target"]
			}
		}

		record := config.CronTaskResultRecord{
			TaskID:     r.TaskId,
			TaskName:   taskName,
			TaskType:   taskType,
			NodeName:   nodeName,
			ExecutedAt: r.ExecutedAt,
			DurationMs: int64(r.DurationMs),
			ExitCode:   int(r.ExitCode),
			Error:      r.ErrorMessage,
			Target:     target,
			RawOutput:  r.RawOutput,
		}
		if record.ExecutedAt == 0 {
			record.ExecutedAt = time.Now().Unix()
		}

		PopulateCronTaskParsedMetrics(&record)

		_ = m.stateStore.RecordCronTaskResult(record)
	})
}

// PopulateCronTaskParsedMetrics parses raw task output and fills in metrics fields if not already populated
func PopulateCronTaskParsedMetrics(record *config.CronTaskResultRecord) {
	if record == nil || record.RawOutput == "" {
		return
	}

	if record.TaskType == "tcp_ping" {
		parsed := lookingglass.ParseOutput("tcp_ping", record.RawOutput, record.Target)
		var tcpRes *lookingglass.TCPPingResult
		if tp, ok := parsed.(*lookingglass.TCPPingResult); ok && tp != nil {
			tcpRes = tp
		} else if tp, ok := parsed.(lookingglass.TCPPingResult); ok {
			tcpRes = &tp
		}
		if tcpRes != nil {
			record.PacketsSent = tcpRes.PacketsSent
			record.PacketsReceived = tcpRes.PacketsReceived
			record.PacketLossPct = tcpRes.PacketLossPct
			record.MinRTTMs = tcpRes.MinRTT
			record.AvgRTTMs = tcpRes.AvgRTT
			record.MaxRTTMs = tcpRes.MaxRTT
			record.MdevRTTMs = tcpRes.MdevRTT
			if record.Target == "" {
				record.Target = tcpRes.Target
			}
			if record.ParsedJSON == "" {
				if jsonBytes, err := json.Marshal(tcpRes); err == nil {
					record.ParsedJSON = string(jsonBytes)
				}
			}
		}
	} else {
		// default: ping
		parsed := lookingglass.ParseOutput("ping", record.RawOutput, record.Target)
		var pingRes *lookingglass.PingResult
		if p, ok := parsed.(*lookingglass.PingResult); ok && p != nil {
			pingRes = p
		} else if p, ok := parsed.(lookingglass.PingResult); ok {
			pingRes = &p
		}
		if pingRes != nil {
			record.PacketsSent = pingRes.PacketsSent
			record.PacketsReceived = pingRes.PacketsReceived
			record.PacketLossPct = pingRes.PacketLossPct
			record.MinRTTMs = pingRes.MinRTT
			record.AvgRTTMs = pingRes.AvgRTT
			record.MaxRTTMs = pingRes.MaxRTT
			record.MdevRTTMs = pingRes.MdevRTT
			if record.Target == "" {
				record.Target = pingRes.Host
			}
			if record.ParsedJSON == "" {
				if jsonBytes, err := json.Marshal(pingRes); err == nil {
					record.ParsedJSON = string(jsonBytes)
				}
			}
		}
	}

	// If execution failed with non-zero exit code and no packets received, ensure packet loss is 100%
	if record.ExitCode != 0 && record.PacketsReceived == 0 {
		record.PacketLossPct = 100.0
	}
}

// GetCronTasks returns all configured cron tasks
func (m *Manager) GetCronTasks() ([]config.CronTask, error) {
	cfg, err := m.store.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	if cfg.CronTasks == nil {
		return []config.CronTask{}, nil
	}
	return cfg.CronTasks, nil
}

// SaveCronTask creates or updates a cron task
func (m *Manager) SaveCronTask(task config.CronTask) (config.CronTask, error) {
	task.Name = strings.TrimSpace(task.Name)
	if task.Name == "" {
		return task, fmt.Errorf("cron task name cannot be empty")
	}

	task.TaskType = strings.TrimSpace(task.TaskType)
	if task.TaskType != "ping" && task.TaskType != "tcp_ping" {
		return task, fmt.Errorf("only 'ping' and 'tcp_ping' task types are supported for cron tasks")
	}

	task.CronExpr = strings.TrimSpace(task.CronExpr)
	if err := config.ValidateCronExpr(task.CronExpr); err != nil {
		return task, fmt.Errorf("invalid cron expression: %w", err)
	}

	if len(task.Nodes) == 0 {
		return task, fmt.Errorf("at least one target node must be specified")
	}

	if task.Params == nil || strings.TrimSpace(task.Params["target"]) == "" {
		return task, fmt.Errorf("task parameter 'target' is required")
	}

	// Validate count
	if countStr, ok := task.Params["count"]; ok && countStr != "" {
		c, err := strconv.Atoi(countStr)
		if err != nil || c < 1 || c > 20 {
			task.Params["count"] = "4"
		}
	} else {
		if task.Params == nil {
			task.Params = make(map[string]string)
		}
		task.Params["count"] = "4"
	}

	if task.TimeoutSec <= 0 {
		task.TimeoutSec = 15
	}
	if task.TimeoutSec > 60 {
		task.TimeoutSec = 60
	}

	cfg, err := m.store.Load()
	if err != nil {
		return task, fmt.Errorf("failed to load config: %w", err)
	}

	now := time.Now()
	if task.ID == "" {
		task.ID = "cron_" + uuid.New().String()[:8]
		task.CreatedAt = now
		task.UpdatedAt = now
		task.Enabled = true
	} else {
		task.UpdatedAt = now
	}

	// Filter out invalid or external node names
	nodeSet := make(map[string]bool)
	for _, n := range cfg.Nodes {
		if !n.IsExternal {
			nodeSet[n.Name] = true
		}
	}
	var validNodes []string
	for _, n := range task.Nodes {
		if nodeSet[n] {
			validNodes = append(validNodes, n)
		}
	}
	if len(validNodes) == 0 {
		return task, fmt.Errorf("none of the specified nodes are managed nodes eligible for cron tasks")
	}
	task.Nodes = validNodes

	found := false
	for i, existing := range cfg.CronTasks {
		if existing.ID == task.ID {
			task.CreatedAt = existing.CreatedAt
			cfg.CronTasks[i] = task
			found = true
			break
		}
	}
	if !found {
		cfg.CronTasks = append(cfg.CronTasks, task)
	}

	if err := m.store.Save(cfg); err != nil {
		return task, fmt.Errorf("failed to save config: %w", err)
	}

	// Push updated task specs to all node agents
	m.SyncCronTasksToAgents()

	return task, nil
}

// DeleteCronTask removes a cron task and updates node agents
func (m *Manager) DeleteCronTask(taskID string) error {
	cfg, err := m.store.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	updated := make([]config.CronTask, 0, len(cfg.CronTasks))
	deleted := false
	for _, t := range cfg.CronTasks {
		if t.ID == taskID {
			deleted = true
			continue
		}
		updated = append(updated, t)
	}

	if !deleted {
		return fmt.Errorf("cron task '%s' not found", taskID)
	}

	cfg.CronTasks = updated
	if err := m.store.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Push updated task specs to agents so they remove deleted task
	m.SyncCronTasksToAgents()
	return nil
}

// ToggleCronTask enables or disables a cron task
func (m *Manager) ToggleCronTask(taskID string) (config.CronTask, error) {
	cfg, err := m.store.Load()
	if err != nil {
		return config.CronTask{}, fmt.Errorf("failed to load config: %w", err)
	}

	var updatedTask config.CronTask
	found := false
	for i, t := range cfg.CronTasks {
		if t.ID == taskID {
			cfg.CronTasks[i].Enabled = !cfg.CronTasks[i].Enabled
			cfg.CronTasks[i].UpdatedAt = time.Now()
			updatedTask = cfg.CronTasks[i]
			found = true
			break
		}
	}

	if !found {
		return config.CronTask{}, fmt.Errorf("cron task '%s' not found", taskID)
	}

	if err := m.store.Save(cfg); err != nil {
		return updatedTask, fmt.Errorf("failed to save config: %w", err)
	}

	m.SyncCronTasksToAgents()
	return updatedTask, nil
}

// SyncCronTasksToAgents pushes full cron task configurations to all connected node agents
func (m *Manager) SyncCronTasksToAgents() {
	if m.agentHub == nil {
		return
	}

	cfg, err := m.store.Load()
	if err != nil {
		return
	}

	for _, node := range cfg.Nodes {
		if node.IsExternal {
			continue
		}
		m.SyncCronTasksToNode(node.Name)
	}
}

// SyncCronTasksToNode compiles and pushes all active cron tasks for a single node
func (m *Manager) SyncCronTasksToNode(nodeName string) {
	if m.agentHub == nil {
		return
	}

	cfg, err := m.store.Load()
	if err != nil {
		return
	}

	for _, n := range cfg.Nodes {
		if n.Name == nodeName && n.IsExternal {
			return
		}
	}

	var specs []*agentpb.CronTaskSpec
	for _, t := range cfg.CronTasks {
		if !t.Enabled {
			continue
		}
		// Check if task is assigned to this node
		assigned := false
		for _, assignedNode := range t.Nodes {
			if assignedNode == nodeName {
				assigned = true
				break
			}
		}
		if !assigned {
			continue
		}

		target := ""
		count := uint32(4)
		if t.Params != nil {
			target = t.Params["target"]
			if cStr, ok := t.Params["count"]; ok && cStr != "" {
				if parsed, err := strconv.ParseUint(cStr, 10, 32); err == nil && parsed > 0 {
					count = uint32(parsed)
				}
			}
		}

		timeoutSec := uint32(t.TimeoutSec)
		if timeoutSec == 0 {
			timeoutSec = 15
		}

		spec := &agentpb.CronTaskSpec{
			TaskId:         t.ID,
			Name:           t.Name,
			TaskType:       t.TaskType,
			CronExpr:       t.CronExpr,
			Target:         target,
			Count:          count,
			TimeoutSeconds: timeoutSec,
			Params:         t.Params,
		}
		specs = append(specs, spec)
	}

	_ = m.agentHub.SyncCronTasks(nodeName, specs)
}

// GetNodeCronTasksResults retrieves historical cron task results and summaries for a node
func (m *Manager) GetNodeCronTasksResults(nodeName string, taskID string, rangeStr string, limit int) (config.NodeCronTasksResultsResponse, error) {
	duration, _ := ParseMetricRange(rangeStr)
	startTime := time.Now().Add(-duration)

	results, err := m.stateStore.GetNodeCronTaskResults(nodeName, taskID, startTime, limit)
	if err != nil {
		return config.NodeCronTasksResultsResponse{}, fmt.Errorf("failed to load cron task results: %w", err)
	}

	// Load all cron tasks configured for this node
	cfg, err := m.store.Load()
	var assignedTasks []config.CronTask
	if err == nil {
		for _, t := range cfg.CronTasks {
			for _, n := range t.Nodes {
				if n == nodeName {
					assignedTasks = append(assignedTasks, t)
					break
				}
			}
		}
	}

	// Compute summaries per task
	taskMap := make(map[string]*config.CronTaskStatsSummary)
	// Initialize with assigned tasks
	for _, t := range assignedTasks {
		target := ""
		if t.Params != nil {
			target = t.Params["target"]
		}
		taskMap[t.ID] = &config.CronTaskStatsSummary{
			TaskID:     t.ID,
			TaskName:   t.Name,
			TaskType:   t.TaskType,
			Target:     target,
			LastStatus: "unknown",
		}
	}

	type accum struct {
		totalRuns int
		sumRTT    float64
		sumLoss   float64
		latest    *config.CronTaskResultRecord
	}
	accumMap := make(map[string]*accum)

	// Results are ordered by executed_at DESC
	for i := range results {
		r := &results[i]
		if r.PacketsSent == 0 && r.RawOutput != "" {
			PopulateCronTaskParsedMetrics(r)
		}
		ac, exists := accumMap[r.TaskID]
		if !exists {
			ac = &accum{latest: r}
			accumMap[r.TaskID] = ac
		}
		runs := 1
		if r.RunsCount > 0 {
			runs = r.RunsCount
		}
		ac.totalRuns += runs
		ac.sumRTT += r.AvgRTTMs * float64(runs)
		ac.sumLoss += r.PacketLossPct * float64(runs)
	}

	for tid, ac := range accumMap {
		s, exists := taskMap[tid]
		if !exists {
			s = &config.CronTaskStatsSummary{
				TaskID:   tid,
				TaskName: ac.latest.TaskName,
				TaskType: ac.latest.TaskType,
				Target:   ac.latest.Target,
			}
			taskMap[tid] = s
		}
		s.TotalRuns = ac.totalRuns
		s.LatestRunTime = ac.latest.ExecutedAt
		s.LatestAvgRTT = ac.latest.AvgRTTMs
		s.LatestLossPct = ac.latest.PacketLossPct
		if ac.totalRuns > 0 {
			s.OverallAvgRTT = ac.sumRTT / float64(ac.totalRuns)
			s.OverallLossPct = ac.sumLoss / float64(ac.totalRuns)
		}
		if ac.latest.ExitCode != 0 || ac.latest.PacketLossPct >= 100.0 {
			s.LastStatus = "failed"
		} else if ac.latest.PacketLossPct > 0 {
			s.LastStatus = "degraded"
		} else {
			s.LastStatus = "ok"
		}
	}

	summaries := make([]config.CronTaskStatsSummary, 0, len(taskMap))
	for _, s := range taskMap {
		summaries = append(summaries, *s)
	}

	return config.NodeCronTasksResultsResponse{
		Results: results,
		Summary: summaries,
		Tasks:   assignedTasks,
	}, nil
}
