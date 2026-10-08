package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
	"easy42/internal/lookingglass"
	"github.com/google/uuid"
)

// GetLookingGlassTasks returns both built-in templates and user-defined custom tasks
func (m *Manager) GetLookingGlassTasks() (lookingglass.TasksResponse, error) {
	cfg, err := m.store.Load()
	if err != nil {
		return lookingglass.TasksResponse{}, fmt.Errorf("failed to load config: %w", err)
	}

	custom := cfg.LookingGlassTasks
	if custom == nil {
		custom = []config.LookingGlassTask{}
	}

	return lookingglass.TasksResponse{
		Builtin: config.DefaultLookingGlassTasks(),
		Custom:  custom,
	}, nil
}

// SaveLookingGlassTask creates or updates a custom Looking Glass task
func (m *Manager) SaveLookingGlassTask(task config.LookingGlassTask) (config.LookingGlassTask, error) {
	if strings.TrimSpace(task.Name) == "" {
		return task, fmt.Errorf("task name cannot be empty")
	}
	if strings.TrimSpace(task.CommandTmpl) == "" {
		return task, fmt.Errorf("command template cannot be empty")
	}

	// Disallow modifying built-ins
	for _, b := range config.DefaultLookingGlassTasks() {
		if b.ID == task.ID {
			return task, fmt.Errorf("cannot overwrite built-in task '%s'", task.ID)
		}
	}

	cfg, err := m.store.Load()
	if err != nil {
		return task, fmt.Errorf("failed to load config: %w", err)
	}

	if task.ID == "" {
		task.ID = "custom_" + uuid.New().String()[:8]
	}
	if task.Category == "" {
		task.Category = "Custom"
	}
	if task.Parser == "" {
		task.Parser = "raw"
	}
	if task.TimeoutSec <= 0 {
		task.TimeoutSec = 15
	}
	task.IsBuiltin = false

	found := false
	for i, existing := range cfg.LookingGlassTasks {
		if existing.ID == task.ID {
			cfg.LookingGlassTasks[i] = task
			found = true
			break
		}
	}
	if !found {
		cfg.LookingGlassTasks = append(cfg.LookingGlassTasks, task)
	}

	if err := m.store.Save(cfg); err != nil {
		return task, fmt.Errorf("failed to save config: %w", err)
	}

	return task, nil
}

// DeleteLookingGlassTask removes a custom Looking Glass task
func (m *Manager) DeleteLookingGlassTask(taskID string) error {
	for _, b := range config.DefaultLookingGlassTasks() {
		if b.ID == taskID {
			return fmt.Errorf("cannot delete built-in task '%s'", taskID)
		}
	}

	cfg, err := m.store.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	updated := make([]config.LookingGlassTask, 0, len(cfg.LookingGlassTasks))
	deleted := false
	for _, t := range cfg.LookingGlassTasks {
		if t.ID == taskID {
			deleted = true
			continue
		}
		updated = append(updated, t)
	}

	if !deleted {
		return fmt.Errorf("task '%s' not found", taskID)
	}

	cfg.LookingGlassTasks = updated
	return m.store.Save(cfg)
}

// RunLookingGlass executes an ad-hoc or template task across requested nodes
func (m *Manager) RunLookingGlass(ctx context.Context, req lookingglass.RunRequest) (lookingglass.RunResponse, error) {
	if len(req.Nodes) == 0 {
		return lookingglass.RunResponse{}, fmt.Errorf("at least one target node must be specified")
	}

	cfg, err := m.store.Load()
	if err != nil {
		return lookingglass.RunResponse{}, fmt.Errorf("failed to load config: %w", err)
	}

	// Filter nodes
	targetMap := make(map[string]config.Node)
	for _, n := range cfg.Nodes {
		targetMap[n.Name] = n
	}

	var targetNodes []config.Node
	for _, name := range req.Nodes {
		node, exists := targetMap[name]
		if !exists {
			return lookingglass.RunResponse{}, fmt.Errorf("node '%s' not found in configuration", name)
		}
		if node.IsExternal {
			return lookingglass.RunResponse{}, fmt.Errorf("node '%s' is an external node; cannot run tasks via SSH", name)
		}
		targetNodes = append(targetNodes, node)
	}

	var cmd string
	var parser string
	var timeout time.Duration
	var targetVal string

	if req.AdHoc {
		if err := lookingglass.ValidateAdHocCommand(req.CustomCommand); err != nil {
			return lookingglass.RunResponse{}, fmt.Errorf("invalid ad-hoc command: %w", err)
		}
		cmd = req.CustomCommand
		parser = req.CustomParser
		if parser == "" {
			parser = "raw"
		}
		if req.TimeoutSec > 0 {
			timeout = time.Duration(req.TimeoutSec) * time.Second
		} else {
			timeout = 15 * time.Second
		}
	} else {
		// Look up task template
		allTasks, _ := m.GetLookingGlassTasks()
		var foundTask *config.LookingGlassTask
		for _, t := range allTasks.Builtin {
			if t.ID == req.TaskID {
				foundTask = &t
				break
			}
		}
		if foundTask == nil {
			for _, t := range allTasks.Custom {
				if t.ID == req.TaskID {
					foundTask = &t
					break
				}
			}
		}
		if foundTask == nil {
			return lookingglass.RunResponse{}, fmt.Errorf("task template '%s' not found", req.TaskID)
		}

		interpolated, err := lookingglass.BuildCommand(foundTask.CommandTmpl, foundTask.Params, req.Params)
		if err != nil {
			return lookingglass.RunResponse{}, fmt.Errorf("failed to construct command: %w", err)
		}
		cmd = interpolated
		parser = foundTask.Parser
		if req.TimeoutSec > 0 {
			timeout = time.Duration(req.TimeoutSec) * time.Second
		} else if foundTask.TimeoutSec > 0 {
			timeout = time.Duration(foundTask.TimeoutSec) * time.Second
		} else {
			timeout = 15 * time.Second
		}

		if req.Params != nil {
			targetVal = req.Params["target"]
		}
	}

	// Cap timeout between 1s and 60s
	if timeout < time.Second {
		timeout = time.Second
	}
	if timeout > 60*time.Second {
		timeout = 60 * time.Second
	}

	results := make(map[string]lookingglass.NodeResult)
	var sshNodes []config.Node
	var agentWg sync.WaitGroup
	var agentMu sync.Mutex

	for _, node := range targetNodes {
		if node.IsAgentMode() {
			agentWg.Add(1)
			go func(n config.Node) {
				defer agentWg.Done()
				start := time.Now()
				nr := lookingglass.NodeResult{
					NodeName: n.Name,
					Command:  cmd,
					Parser:   parser,
				}
				if !m.agentHub.IsConnected(n.Name) {
					nr.ExitCode = -1
					nr.Error = "Agent is not connected"
					nr.DurationMs = time.Since(start).Milliseconds()
				} else {
					cmdCtx, cancel := context.WithTimeout(ctx, timeout)
					defer cancel()
					resp, err := m.agentHub.SendCommand(cmdCtx, n.Name, &agentpb.CommandRequest{
						Command: &agentpb.CommandRequest_LookingGlass{
							LookingGlass: &agentpb.LookingGlassCmd{
								Tool:           agentpb.LookingGlassCmd_CUSTOM,
								Command:        cmd,
								TimeoutSeconds: uint32(timeout.Seconds()),
							},
						},
					})
					nr.DurationMs = time.Since(start).Milliseconds()
					if err != nil {
						nr.ExitCode = -1
						nr.Error = err.Error()
					} else {
						nr.ExitCode = int(resp.ExitCode)
						nr.RawOutput = resp.Stdout
						if resp.Stderr != "" {
							if nr.RawOutput != "" {
								nr.RawOutput += "\n"
							}
							nr.RawOutput += resp.Stderr
						}
						nr.Parsed = lookingglass.ParseOutput(parser, nr.RawOutput, targetVal)
					}
				}
				agentMu.Lock()
				results[n.Name] = nr
				agentMu.Unlock()
			}(node)
		} else {
			sshNodes = append(sshNodes, node)
		}
	}

	if len(sshNodes) > 0 {
		sshResults := lookingglass.ExecuteMultiNode(ctx, m.pool, sshNodes, cmd, parser, targetVal, timeout)
		for k, v := range sshResults {
			results[k] = v
		}
	}
	agentWg.Wait()

	return lookingglass.RunResponse{
		TaskID:  req.TaskID,
		Command: cmd,
		Results: results,
	}, nil
}
