package engine

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"easy42/internal/agent"
	"easy42/internal/config"
	"easy42/internal/ssh"
	"easy42/tasks"
)

func (m *Manager) customTasksDir() string {
	if m.store != nil && m.store.DataDir() != "" {
		return filepath.Join(m.store.DataDir(), "tasks")
	}
	return tasks.GetCustomTasksDir()
}

// GetTasks returns all available helper tasks (embedded and custom)
func (m *Manager) GetTasks() ([]tasks.TaskMeta, error) {
	return tasks.GetTasks(m.customTasksDir())
}

// GetTaskSources returns all script/source files for a specific task
func (m *Manager) GetTaskSources(taskID string) ([]tasks.TaskSourceFile, error) {
	return tasks.GetTaskSources(taskID, m.customTasksDir())
}

// GetOnlineSources returns all online task repository source URLs
func (m *Manager) GetOnlineSources() []string {
	return tasks.GetOnlineSources(m.customTasksDir())
}

// GetOnlineManifest fetches manifest.json from the given source URL
func (m *Manager) GetOnlineManifest(ctx context.Context, sourceURL string) (*tasks.OnlineManifest, error) {
	return tasks.FetchOnlineManifest(ctx, sourceURL)
}

// InstallOnlineTask downloads and installs an online task into <data-dir>/tasks/<task-id>
func (m *Manager) InstallOnlineTask(ctx context.Context, sourceURL string, taskID string, files []string) error {
	return tasks.InstallOnlineTask(ctx, m.customTasksDir(), sourceURL, taskID, files)
}

// DeleteCustomTask deletes a custom task folder from <data-dir>/tasks/<task-id>
func (m *Manager) DeleteCustomTask(ctx context.Context, taskID string) error {
	return tasks.DeleteCustomTask(m.customTasksDir(), taskID)
}

// CheckTaskStatus checks the status of a task on specified nodes (or all nodes if nodeNames is empty)
func (m *Manager) CheckTaskStatus(ctx context.Context, taskID string, nodeNames []string) (map[string]tasks.TaskStatusResult, error) {
	// Verify task exists
	if _, err := tasks.GetTaskMeta(taskID, m.customTasksDir()); err != nil {
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

			var res tasks.TaskStatusResult
			if taskID == "configure_network" || taskID == "configure_ip" {
				payload := buildConfigureNetworkPayload(n)
				res = tasks.CheckStatusWithPayload(sshClient, sftpClient, taskID, n.Name, payload, m.customTasksDir())
			} else {
				payload := map[string][]byte{
					"node_ip.env": []byte(buildNodeEnv(n)),
				}
				res = tasks.CheckStatusWithPayload(sshClient, sftpClient, taskID, n.Name, payload, m.customTasksDir())
			}
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
	if _, err := tasks.GetTaskMeta(taskID, m.customTasksDir()); err != nil {
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
					"node_ip.env":  []byte(buildNodeEnv(n)),
				}
				res = tasks.RunTaskWithPayload(sshClient, sftpClient, taskID, n.Name, payload, m.customTasksDir())
				if res.Success {
					go func() {
						_, _ = m.RefreshNodeStatus(n.Name)
					}()
				}
			} else if taskID == "configure_network" || taskID == "configure_ip" {
				payload := buildConfigureNetworkPayload(n)
				res = tasks.RunTaskWithPayload(sshClient, sftpClient, taskID, n.Name, payload, m.customTasksDir())
				if res.Success {
					go func() {
						_, _ = m.RefreshNodeStatus(n.Name)
					}()
				}
			} else {
				payload := map[string][]byte{
					"node_ip.env": []byte(buildNodeEnv(n)),
				}
				res = tasks.RunTaskWithPayload(sshClient, sftpClient, taskID, n.Name, payload, m.customTasksDir())
			}
			mu.Lock()
			results[n.Name] = res
			mu.Unlock()
		}(node)
	}

	wg.Wait()
	return results, nil
}

func cleanCIDR(ip string) string {
	ip = strings.TrimSpace(ip)
	if idx := strings.Index(ip, "/"); idx != -1 {
		return strings.TrimSpace(ip[:idx])
	}
	return ip
}

func injectEnvIntoScript(baseScript []byte, envContent string) string {
	content := string(baseScript)
	if strings.HasPrefix(content, "#!/bin/sh") || strings.HasPrefix(content, "#!/bin/bash") {
		idx := strings.Index(content, "\n")
		if idx != -1 {
			return content[:idx+1] + "\n# --- Dynamically generated node configuration ---\n" + envContent + "# ------------------------------------------------\n\n" + content[idx+1:]
		}
	}
	return envContent + "\n" + content
}

func buildConfigureIPPayload(n config.Node) map[string][]byte {
	return buildConfigureNetworkPayload(n)
}

func buildNodeEnv(n config.Node) string {
	nodeIP := cleanCIDR(n.IP)
	nodeIP6 := cleanCIDR(n.IP6)
	nodeExtIP := cleanCIDR(n.ExternalIP)
	nodeExtIP6 := cleanCIDR(n.ExternalIP6)
	iface := strings.TrimSpace(n.Interface)
	name := strings.TrimSpace(n.Name)
	customScript := strings.TrimSpace(n.CustomNetworkScript)

	var envBuf strings.Builder
	fmt.Fprintf(&envBuf, "NODE_NAME=%q\n", name)
	fmt.Fprintf(&envBuf, "NODE_INTERFACE=%q\n", iface)
	fmt.Fprintf(&envBuf, "NODE_IP=%q\n", nodeIP)
	fmt.Fprintf(&envBuf, "NODE_IP6=%q\n", nodeIP6)
	fmt.Fprintf(&envBuf, "NODE_EXTERNAL_IP=%q\n", nodeExtIP)
	fmt.Fprintf(&envBuf, "NODE_EXTERNAL_IP6=%q\n", nodeExtIP6)
	if customScript != "" {
		fmt.Fprintf(&envBuf, "HAS_CUSTOM_SCRIPT=\"1\"\n")
	} else {
		fmt.Fprintf(&envBuf, "HAS_CUSTOM_SCRIPT=\"0\"\n")
	}

	injectNodeDataEnv(&envBuf, n.Note)

	return envBuf.String()
}

// sanitizeEnvKey converts a key string to an uppercase alphanumeric + underscore env name.
func sanitizeEnvKey(k string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(k) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// formatEnvValue formats primitive values (string, bool, numbers) for .env injection.
func formatEnvValue(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		if val == "" {
			return `""`, true
		}
		if strings.ContainsAny(val, " \t\n\r\"'$`\\;&|<>*?~#()[]{}=") {
			return fmt.Sprintf("%q", val), true
		}
		return val, true
	case bool:
		if val {
			return "true", true
		}
		return "false", true
	case float64:
		if val == math.Trunc(val) && !math.IsNaN(val) && !math.IsInf(val, 0) {
			return strconv.FormatInt(int64(val), 10), true
		}
		return strconv.FormatFloat(val, 'f', -1, 64), true
	case int:
		return strconv.Itoa(val), true
	case int64:
		return strconv.FormatInt(val, 10), true
	case uint64:
		return strconv.FormatUint(val, 10), true
	default:
		return "", false
	}
}

// injectNodeDataEnv extracts structured data from Note and injects primitive fields into envBuf.
// Follows at most 1 layer of object hierarchy (e.g. NODE_DATA_META_ID).
func injectNodeDataEnv(envBuf *strings.Builder, note string) {
	noteData := config.ExtractNoteData(note)
	if len(noteData) == 0 {
		return
	}

	envMap := make(map[string]string)

	// 1. Process Layer 0 primitive fields
	for k0, v0 := range noteData {
		s0 := sanitizeEnvKey(k0)
		if s0 == "" {
			continue
		}
		if formatted, ok := formatEnvValue(v0); ok {
			envMap["NODE_DATA_"+s0] = formatted
		}
	}

	// 2. Process Layer 1 nested object fields
	for k0, v0 := range noteData {
		s0 := sanitizeEnvKey(k0)
		if s0 == "" {
			continue
		}
		if subMap, ok := v0.(map[string]any); ok {
			for k1, v1 := range subMap {
				s1 := sanitizeEnvKey(k1)
				if s1 == "" {
					continue
				}
				if subFormatted, subOk := formatEnvValue(v1); subOk {
					envMap["NODE_DATA_"+s0+"_"+s1] = subFormatted
					// Also provide alias without parent prefix if not already defined at root
					aliasKey := "NODE_DATA_" + s1
					if _, exists := envMap[aliasKey]; !exists {
						envMap[aliasKey] = subFormatted
					}
				}
				// Layer 2 objects are ignored (at most 1 layer object hierarchy)
			}
		}
	}

	var keys []string
	for k := range envMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(envBuf, "%s=%s\n", k, envMap[k])
	}
}

func buildConfigureNetworkPayload(n config.Node) map[string][]byte {
	envContent := buildNodeEnv(n)
	customScript := strings.TrimSpace(n.CustomNetworkScript)

	payload := map[string][]byte{
		"node_ip.env": []byte(envContent),
	}

	if customScript != "" {
		payload["custom_network.sh"] = []byte(customScript)
	}

	taskID := "configure_network"
	if baseStatus, err := tasks.ReadTaskScript(taskID, "status.sh"); err == nil {
		dynamicStatus := injectEnvIntoScript(baseStatus, envContent)
		payload["status.sh"] = []byte(dynamicStatus)
	} else if baseStatus, err := tasks.ReadTaskScript("configure_ip", "status.sh"); err == nil {
		dynamicStatus := injectEnvIntoScript(baseStatus, envContent)
		payload["status.sh"] = []byte(dynamicStatus)
	}

	if baseRun, err := tasks.ReadTaskScript(taskID, "run.sh"); err == nil {
		dynamicRun := injectEnvIntoScript(baseRun, envContent)
		payload["run.sh"] = []byte(dynamicRun)
	} else if baseRun, err := tasks.ReadTaskScript("configure_ip", "run.sh"); err == nil {
		dynamicRun := injectEnvIntoScript(baseRun, envContent)
		payload["run.sh"] = []byte(dynamicRun)
	}

	return payload
}
