package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/apps"
	"easy42/internal/config"
	"github.com/google/uuid"
)

// AppStatusRecord represents the live status of an app on a node
type AppStatusRecord struct {
	AppID        string `json:"app_id"`
	Name         string `json:"name"`
	AppType      string `json:"app_type"`
	State        string `json:"state"` // "running" | "stopped" | "failed" | "unknown"
	StatusText   string `json:"status_text"`
	LastUpdated  int64  `json:"last_updated"`
	ErrorMessage string `json:"error_message,omitempty"`
}

// setupAgentAppsListeners initializes hooks for agent registration and app status reporting
func (m *Manager) setupAgentAppsListeners() {
	if m.agentHub == nil {
		return
	}

	// 1. When agent registers, immediately push its assigned apps
	m.agentHub.OnRegister(func(conn *agent.AgentConnection) {
		_ = m.SyncAppsToNode(conn.NodeName())
	})

	// 2. When telemetry report arrives, cache reported app statuses
	m.agentHub.OnTelemetry(func(nodeName string, t *agentpb.TelemetryReport) {
		if t == nil || len(t.Apps) == 0 {
			return
		}

		m.appStatusesMu.Lock()
		defer m.appStatusesMu.Unlock()

		if m.appStatuses == nil {
			m.appStatuses = make(map[string]map[string]AppStatusRecord)
		}

		nodeMap := m.appStatuses[nodeName]
		if nodeMap == nil {
			nodeMap = make(map[string]AppStatusRecord)
			m.appStatuses[nodeName] = nodeMap
		}

		now := time.Now().Unix()
		for _, a := range t.Apps {
			if a == nil {
				continue
			}
			appType := "docker"
			if a.AppType == agentpb.AppType_SYSTEMD {
				appType = "systemd"
			}
			nodeMap[a.Name] = AppStatusRecord{
				AppID:        a.AppId,
				Name:         a.Name,
				AppType:      appType,
				State:        a.State,
				StatusText:   a.StatusText,
				LastUpdated:  now,
				ErrorMessage: a.ErrorMessage,
			}
		}
	})
}

// GetApps returns all configured apps
func (m *Manager) GetApps() ([]config.App, error) {
	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}

	if cfg.Apps == nil {
		return []config.App{}, nil
	}
	return cfg.Apps, nil
}

// SaveApp creates or updates an App definition
func (m *Manager) SaveApp(app config.App) (config.App, error) {
	if err := config.ValidateApp(app); err != nil {
		return app, err
	}

	cfg, err := m.store.Load()
	if err != nil {
		return app, fmt.Errorf("failed to load config: %w", err)
	}

	now := time.Now()
	if app.ID == "" {
		app.ID = "app_" + uuid.New().String()[:8]
		app.CreatedAt = now
		app.UpdatedAt = now
		app.Enabled = true
	} else {
		app.UpdatedAt = now
	}

	// Filter out invalid or external nodes
	nodeSet := make(map[string]bool)
	for _, n := range cfg.Nodes {
		if !n.IsExternal {
			nodeSet[n.Name] = true
		}
	}
	var validNodes []string
	for _, n := range app.Nodes {
		if nodeSet[n] {
			validNodes = append(validNodes, n)
		}
	}
	app.Nodes = validNodes

	found := false
	for i, existing := range cfg.Apps {
		if existing.ID == app.ID {
			app.CreatedAt = existing.CreatedAt
			cfg.Apps[i] = app
			found = true
			break
		}
	}
	if !found {
		cfg.Apps = append(cfg.Apps, app)
	}

	if err := m.store.Save(cfg); err != nil {
		return app, fmt.Errorf("failed to save config: %w", err)
	}

	// Synchronize apps to connected agents
	m.SyncAppsToAgents()

	return app, nil
}

// DeleteApp deletes an app by ID and updates agents
func (m *Manager) DeleteApp(appID string) error {
	cfg, err := m.store.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	updated := make([]config.App, 0, len(cfg.Apps))
	deleted := false
	for _, a := range cfg.Apps {
		if a.ID == appID {
			deleted = true
			continue
		}
		updated = append(updated, a)
	}

	if !deleted {
		return fmt.Errorf("app '%s' not found", appID)
	}

	cfg.Apps = updated
	if err := m.store.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	m.SyncAppsToAgents()
	return nil
}

// ToggleApp toggles the enabled state of an app
func (m *Manager) ToggleApp(appID string) (config.App, error) {
	cfg, err := m.store.Load()
	if err != nil {
		return config.App{}, fmt.Errorf("failed to load config: %w", err)
	}

	var toggled config.App
	found := false
	for i, a := range cfg.Apps {
		if a.ID == appID {
			cfg.Apps[i].Enabled = !cfg.Apps[i].Enabled
			cfg.Apps[i].UpdatedAt = time.Now()
			toggled = cfg.Apps[i]
			found = true
			break
		}
	}

	if !found {
		return config.App{}, fmt.Errorf("app '%s' not found", appID)
	}

	if err := m.store.Save(cfg); err != nil {
		return config.App{}, fmt.Errorf("failed to save config: %w", err)
	}

	m.SyncAppsToAgents()
	return toggled, nil
}

// GetAppVariables returns all configured global app variables
func (m *Manager) GetAppVariables() ([]config.AppVariable, error) {
	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %w", err)
		}
	}

	if cfg.AppVariables == nil {
		return []config.AppVariable{}, nil
	}
	return cfg.AppVariables, nil
}

// SaveAppVariable creates or updates a global app variable
func (m *Manager) SaveAppVariable(variable config.AppVariable) (config.AppVariable, error) {
	variable.Key = strings.TrimSpace(variable.Key)
	if err := config.ValidateVariableKey(variable.Key); err != nil {
		return variable, err
	}

	cfg, err := m.store.Load()
	if err != nil {
		return variable, fmt.Errorf("failed to load config: %w", err)
	}

	variable.UpdatedAt = time.Now()

	found := false
	for i, existing := range cfg.AppVariables {
		if existing.Key == variable.Key {
			cfg.AppVariables[i] = variable
			found = true
			break
		}
	}
	if !found {
		cfg.AppVariables = append(cfg.AppVariables, variable)
	}

	if err := m.store.Save(cfg); err != nil {
		return variable, fmt.Errorf("failed to save config: %w", err)
	}

	// Trigger sync so apps using this variable get re-rendered and updated
	m.SyncAppsToAgents()
	return variable, nil
}

// DeleteAppVariable deletes a global app variable by key
func (m *Manager) DeleteAppVariable(key string) error {
	cfg, err := m.store.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	updated := make([]config.AppVariable, 0, len(cfg.AppVariables))
	deleted := false
	for _, v := range cfg.AppVariables {
		if v.Key == key {
			deleted = true
			continue
		}
		updated = append(updated, v)
	}

	if !deleted {
		return fmt.Errorf("app variable '%s' not found", key)
	}

	cfg.AppVariables = updated
	if err := m.store.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	m.SyncAppsToAgents()
	return nil
}

// BuildAppSpecsForNode converts the apps assigned to nodeName into proto AppSpecs,
// automatically handling transpilation for non-systemd nodes (OpenWrt/Procd, Alpine/OpenRC)
// and interpolating global variables and node context variables.
func (m *Manager) BuildAppSpecsForNode(nodeName string) ([]*agentpb.AppSpec, error) {
	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return nil, err
		}
	}

	osInfo := ""
	if m.agentHub != nil {
		osInfo = strings.ToLower(m.agentHub.GetOSInfo(nodeName))
	}

	globalVars := apps.VariablesToMap(cfg.AppVariables)

	targetNode := m.FindNode(nodeName)

	var contextVars map[string]string
	if targetNode != nil {
		contextVars = apps.BuildNodeContextVars(*targetNode)
	} else {
		contextVars = map[string]string{
			"NODE_NAME": nodeName,
		}
	}

	var specs []*agentpb.AppSpec
	for _, rawApp := range cfg.Apps {
		if !config.AppTargetsNode(rawApp, targetNode, nodeName) {
			continue
		}

		// App-specific context variables
		appCtx := make(map[string]string, len(contextVars)+1)
		for k, v := range contextVars {
			appCtx[k] = v
		}
		appCtx["APP_NAME"] = rawApp.Name

		// Interpolate global & context variables for this specific node
		app := apps.InterpolateApp(rawApp, globalVars, appCtx)

		spec := &agentpb.AppSpec{
			Id:          app.ID,
			Name:        app.Name,
			Sha256Hash:  apps.ComputeAppHash(app),
			Enabled:     app.Enabled,
			EnvContent:  app.EnvContent,
			MainContent: app.MainContent,
		}

		if strings.ToLower(app.AppType) == "docker" {
			spec.AppType = agentpb.AppType_DOCKER_COMPOSE
		} else {
			spec.AppType = agentpb.AppType_SYSTEMD

			// Transpile if target node uses OpenWrt / Procd or Alpine / OpenRC
			parsedUnit, err := apps.ParseSystemdService(app.MainContent)
			if err == nil && parsedUnit != nil {
				parsedEnv := apps.ParseEnvFile(app.EnvContent)
				// Merge any unit-level Environment directives
				for k, v := range parsedUnit.Environment {
					if _, exists := parsedEnv[k]; !exists {
						parsedEnv[k] = v
					}
				}

				if strings.Contains(osInfo, "openwrt") {
					if procdScript, err := apps.TranspileToProcd(app.Name, parsedUnit, parsedEnv); err == nil {
						spec.MainContent = procdScript
					}
				} else if strings.Contains(osInfo, "alpine") || strings.Contains(osInfo, "openrc") {
					if openrcScript, err := apps.TranspileToOpenRC(app.Name, parsedUnit); err == nil {
						spec.MainContent = openrcScript
						spec.EnvContent = apps.GenerateOpenRCConfD(app.Name, parsedEnv)
					}
				}
			}
		}

		// Convert auxiliary config files map to proto repeated AppConfigFile
		for relPath, content := range app.Files {
			spec.ConfigFiles = append(spec.ConfigFiles, &agentpb.AppConfigFile{
				Path:     relPath,
				Content:  []byte(content),
				FileMode: 0644,
			})
		}

		specs = append(specs, spec)
	}

	return specs, nil
}

// SyncAppsToNode builds and sends the desired apps to a specific node agent
func (m *Manager) SyncAppsToNode(nodeName string) error {
	if m.agentHub == nil {
		return nil
	}

	specs, err := m.BuildAppSpecsForNode(nodeName)
	if err != nil {
		return fmt.Errorf("failed to build app specs for node %s: %w", nodeName, err)
	}

	return m.agentHub.SyncApps(nodeName, specs)
}

// SyncAppsToAgents pushes desired apps to all managed node agents
func (m *Manager) SyncAppsToAgents() {
	if m.agentHub == nil {
		return
	}

	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return
		}
	}

	for _, node := range cfg.Nodes {
		if node.IsExternal {
			continue
		}
		_ = m.SyncAppsToNode(node.Name)
	}
}

// GetNodeAppsStatus returns the latest reported statuses for apps on nodeName
func (m *Manager) GetNodeAppsStatus(nodeName string) []AppStatusRecord {
	m.appStatusesMu.RLock()
	defer m.appStatusesMu.RUnlock()

	var res []AppStatusRecord
	if m.appStatuses == nil {
		return res
	}

	nodeMap := m.appStatuses[nodeName]
	if nodeMap == nil {
		return res
	}

	for _, st := range nodeMap {
		res = append(res, st)
	}
	return res
}

// TriggerAppAction sends an action command (start, stop, restart, logs) to the node agent
func (m *Manager) TriggerAppAction(ctx context.Context, nodeName string, appName string, action string, appType string, logLines uint32) (*agentpb.CommandResponse, error) {
	if m.agentHub == nil {
		return nil, fmt.Errorf("agent hub not available")
	}

	conn, ok := m.agentHub.GetConnection(nodeName)
	if !ok {
		return nil, fmt.Errorf("node agent '%s' is not connected", nodeName)
	}

	protoType := agentpb.AppType_DOCKER_COMPOSE
	if strings.ToLower(appType) == "systemd" {
		protoType = agentpb.AppType_SYSTEMD
	}

	if logLines == 0 {
		logLines = 100
	}

	cmd := &agentpb.CommandRequest{
		Command: &agentpb.CommandRequest_AppAction{
			AppAction: &agentpb.AppActionCmd{
				AppName:  appName,
				Action:   action,
				AppType:  protoType,
				LogLines: logLines,
			},
		},
	}

	return conn.SendCommand(ctx, cmd)
}
