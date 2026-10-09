package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"easy42/internal/agent"
	"easy42/internal/config"
	"easy42/internal/ssh"
	"easy42/tasks"
)

// GetTasks returns all available embedded helper tasks
func (m *Manager) GetTasks() ([]tasks.TaskMeta, error) {
	return tasks.GetTasks()
}

// CheckTaskStatus checks the status of a task on specified nodes (or all nodes if nodeNames is empty)
func (m *Manager) CheckTaskStatus(ctx context.Context, taskID string, nodeNames []string) (map[string]tasks.TaskStatusResult, error) {
	// Verify task exists
	if _, err := tasks.GetTaskMeta(taskID); err != nil {
		return nil, err
	}

	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Filter target nodes
	targetMap := make(map[string]config.Node)
	if len(nodeNames) == 0 {
		for _, n := range cfg.Nodes {
			if !n.IsExternal {
				targetMap[n.Name] = n
			}
		}
	} else {
		allowed := make(map[string]bool)
		for _, name := range nodeNames {
			allowed[name] = true
		}
		for _, n := range cfg.Nodes {
			if allowed[n.Name] && !n.IsExternal {
				targetMap[n.Name] = n
			}
		}
	}

	results := make(map[string]tasks.TaskStatusResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, node := range targetMap {
		wg.Add(1)
		go func(n config.Node) {
			defer wg.Done()

			sshClient, sftpClient, err := m.pool.GetClient(n.Host)
			if err != nil {
				mu.Lock()
				results[n.Name] = tasks.TaskStatusResult{
					TaskID:   taskID,
					NodeName: n.Name,
					Status:   "error",
					Message:  fmt.Sprintf("SSH connection failed: %v", err),
					ExitCode: -1,
				}
				mu.Unlock()
				return
			}

			res := tasks.CheckStatus(sshClient, sftpClient, taskID, n.Name)
			mu.Lock()
			results[n.Name] = res
			mu.Unlock()
		}(node)
	}

	wg.Wait()
	return results, nil
}

// RunTask executes a task on specified nodes
func (m *Manager) RunTask(ctx context.Context, taskID string, nodeNames []string) (map[string]tasks.TaskRunResult, error) {
	// Verify task exists
	if _, err := tasks.GetTaskMeta(taskID); err != nil {
		return nil, err
	}

	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Filter target nodes
	targetMap := make(map[string]config.Node)
	allowed := make(map[string]bool)
	for _, name := range nodeNames {
		allowed[name] = true
	}
	for _, n := range cfg.Nodes {
		if allowed[n.Name] && !n.IsExternal {
			targetMap[n.Name] = n
		}
	}

	if len(targetMap) == 0 {
		return nil, fmt.Errorf("no valid target nodes specified")
	}

	results := make(map[string]tasks.TaskRunResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, node := range targetMap {
		wg.Add(1)
		go func(n config.Node) {
			defer wg.Done()

			sshClient, sftpClient, err := m.pool.GetClient(n.Host)
			if err != nil {
				mu.Lock()
				results[n.Name] = tasks.TaskRunResult{
					TaskID:   taskID,
					NodeName: n.Name,
					Success:  false,
					Output:   fmt.Sprintf("SSH connection failed: %v", err),
					ExitCode: -1,
				}
				mu.Unlock()
				return
			}

			var res tasks.TaskRunResult
			if taskID == "deploy_agent" {
				token := n.AgentToken
				if token == "" || !n.IsAgentMode() {
					var err error
					token, err = m.GenerateAgentToken(n.Name)
					if err != nil {
						mu.Lock()
						results[n.Name] = tasks.TaskRunResult{
							TaskID:   taskID,
							NodeName: n.Name,
							Success:  false,
							Output:   fmt.Sprintf("Failed to generate agent token: %v", err),
							ExitCode: -1,
						}
						mu.Unlock()
						return
					}
				}

				archOut, err := ssh.RunCommand(sshClient, agent.ArchDetectCommand)
				if err != nil {
					mu.Lock()
					results[n.Name] = tasks.TaskRunResult{
						TaskID:   taskID,
						NodeName: n.Name,
						Success:  false,
						Output:   fmt.Sprintf("Failed to detect architecture on %s: %v", n.Name, err),
						ExitCode: -1,
					}
					mu.Unlock()
					return
				}

				binData, err := agent.GetAgentBinary(strings.TrimSpace(archOut))
				if err != nil {
					mu.Lock()
					results[n.Name] = tasks.TaskRunResult{
						TaskID:   taskID,
						NodeName: n.Name,
						Success:  false,
						Output:   fmt.Sprintf("Agent binary not available: %v", err),
						ExitCode: -1,
					}
					mu.Unlock()
					return
				}

				serverURL := m.GetServerURL()
				if serverURL == "" {
					serverURL = "http://127.0.0.1:4242"
				}

				agentJSON := fmt.Sprintf(`{"server":%q,"token":%q,"log_level":"info","node_name":%q}`, serverURL, token, n.Name)
				payload := map[string][]byte{
					"easy42-agent": binData,
					"agent.json":   []byte(agentJSON),
				}
				res = tasks.RunTaskWithPayload(sshClient, sftpClient, taskID, n.Name, payload)
				if res.Success {
					go func() {
						_, _ = m.RefreshNodeStatus(n.Name)
					}()
				}
			} else {
				res = tasks.RunTask(sshClient, sftpClient, taskID, n.Name)
			}
			mu.Lock()
			results[n.Name] = res
			mu.Unlock()
		}(node)
	}

	wg.Wait()
	return results, nil
}
