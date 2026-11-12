package engine

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

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

	// When telemetry report arrives, cache reported app statuses
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
			} else if a.AppType == agentpb.AppType_CRON {
				appType = "cron"
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

	// Check if another app already uses the same name
	for _, existing := range cfg.Apps {
		if existing.ID != app.ID && strings.EqualFold(existing.Name, app.Name) {
			return app, fmt.Errorf("app with name '%s' already exists", app.Name)
		}
	}

	if strings.ToLower(app.AppType) == "systemd" {
		app.MainContent = apps.EnsureSystemdEnvironmentFile(app.Name, app.MainContent)
	}

	found := false
	for i, existing := range cfg.Apps {
		if existing.ID == app.ID {
			if existing.Name != app.Name {
				m.appStatusesMu.Lock()
				for _, nodeMap := range m.appStatuses {
					delete(nodeMap, existing.Name)
				}
				m.appStatusesMu.Unlock()
			}
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

	return app, nil
}

// RenameApp renames an existing app by ID
func (m *Manager) RenameApp(appID string, newName string) (config.App, error) {
	newName = strings.TrimSpace(newName)
	if err := config.ValidateAppName(newName); err != nil {
		return config.App{}, err
	}

	cfg, err := m.store.Load()
	if err != nil {
		return config.App{}, fmt.Errorf("failed to load config: %w", err)
	}

	appIndex := -1
	var oldName string
	for i, a := range cfg.Apps {
		if a.ID == appID {
			appIndex = i
			oldName = a.Name
			break
		}
	}

	if appIndex == -1 {
		return config.App{}, fmt.Errorf("app '%s' not found", appID)
	}

	if strings.EqualFold(oldName, newName) {
		cfg.Apps[appIndex].Name = newName
		cfg.Apps[appIndex].UpdatedAt = time.Now()
		if err := m.store.Save(cfg); err != nil {
			return config.App{}, fmt.Errorf("failed to save config: %w", err)
		}
		return cfg.Apps[appIndex], nil
	}

	for _, a := range cfg.Apps {
		if a.ID != appID && strings.EqualFold(a.Name, newName) {
			return config.App{}, fmt.Errorf("app with name '%s' already exists", newName)
		}
	}

	cfg.Apps[appIndex].Name = newName
	cfg.Apps[appIndex].UpdatedAt = time.Now()
	if strings.ToLower(cfg.Apps[appIndex].AppType) == "systemd" {
		cfg.Apps[appIndex].MainContent = apps.EnsureSystemdEnvironmentFile(newName, cfg.Apps[appIndex].MainContent)
	}

	// Clear cached status under old name
	m.appStatusesMu.Lock()
	for _, nodeMap := range m.appStatuses {
		delete(nodeMap, oldName)
	}
	m.appStatusesMu.Unlock()

	if err := m.store.Save(cfg); err != nil {
		return config.App{}, fmt.Errorf("failed to save config: %w", err)
	}

	return cfg.Apps[appIndex], nil
}

var trailingDigitsRegex = regexp.MustCompile(`^(.*?)(\d+)$`)

// computeDuplicateIdentifier calculates the next identifier by appending '1' or incrementing a trailing digit suffix.
func computeDuplicateIdentifier(id string, exists func(string) bool) string {
	var base string
	var nextNum uint64

	matches := trailingDigitsRegex.FindStringSubmatch(id)
	if len(matches) == 3 {
		base = matches[1]
		num, err := strconv.ParseUint(matches[2], 10, 64)
		if err == nil {
			nextNum = num + 1
		} else {
			base = id
			nextNum = 1
		}
	} else {
		base = id
		nextNum = 1
	}

	candidate := fmt.Sprintf("%s%d", base, nextNum)
	for exists != nil && exists(candidate) {
		nextNum++
		candidate = fmt.Sprintf("%s%d", base, nextNum)
	}
	return candidate
}

// DuplicateApp creates a duplicate app from an existing app by ID.
// The new app id is set to "<original-id>1", or if it already ends in digit(s), the suffix number is incremented.
// The duplicated app is forced to disabled state.
func (m *Manager) DuplicateApp(appID string) (config.App, error) {
	cfg, err := m.store.Load()
	if err != nil {
		return config.App{}, fmt.Errorf("failed to load config: %w", err)
	}

	var original *config.App
	for _, a := range cfg.Apps {
		if a.ID == appID {
			cpy := a
			original = &cpy
			break
		}
	}

	if original == nil {
		return config.App{}, fmt.Errorf("app '%s' not found", appID)
	}

	existingIDs := make(map[string]bool)
	existingNames := make(map[string]bool)
	for _, a := range cfg.Apps {
		existingIDs[a.ID] = true
		existingNames[strings.ToLower(a.Name)] = true
	}

	newID := computeDuplicateIdentifier(original.ID, func(candidate string) bool {
		return existingIDs[candidate]
	})

	newName := computeDuplicateIdentifier(original.Name, func(candidate string) bool {
		return existingNames[strings.ToLower(candidate)]
	})

	now := time.Now()
	duplicated := config.App{
		ID:          newID,
		Name:        newName,
		Description: original.Description,
		Note:        original.Note,
		URL:         original.URL,
		AppType:     original.AppType,
		MainContent: original.MainContent,
		EnvContent:  original.EnvContent,
		Published:   false, // Draft
		Enabled:     false, // Forced to disabled state
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if strings.ToLower(duplicated.AppType) == "systemd" {
		duplicated.MainContent = apps.EnsureSystemdEnvironmentFile(duplicated.Name, duplicated.MainContent)
	}

	if original.Files != nil {
		duplicated.Files = make(map[string]string, len(original.Files))
		for k, v := range original.Files {
			duplicated.Files[k] = v
		}
	}

	if original.Selector != nil {
		duplicated.Selector = make(map[string]string, len(original.Selector))
		for k, v := range original.Selector {
			duplicated.Selector[k] = v
		}
	}

	if original.Artifact != nil {
		artCopy := *original.Artifact
		if original.Artifact.Resolver != nil {
			resCopy := *original.Artifact.Resolver
			artCopy.Resolver = &resCopy
		}
		if original.Artifact.ArchMapping != nil {
			artCopy.ArchMapping = make(map[string]string, len(original.Artifact.ArchMapping))
			for k, v := range original.Artifact.ArchMapping {
				artCopy.ArchMapping[k] = v
			}
		}
		if original.Artifact.Checksums != nil {
			artCopy.Checksums = make(map[string]string, len(original.Artifact.Checksums))
			for k, v := range original.Artifact.Checksums {
				artCopy.Checksums[k] = v
			}
		}
		if original.Artifact.Files != nil {
			artCopy.Files = make([]config.ArtifactFile, len(original.Artifact.Files))
			copy(artCopy.Files, original.Artifact.Files)
		}
		duplicated.Artifact = &artCopy
	}

	if err := config.ValidateApp(duplicated); err != nil {
		return config.App{}, fmt.Errorf("invalid duplicated app: %w", err)
	}

	cfg.Apps = append(cfg.Apps, duplicated)
	if err := m.store.Save(cfg); err != nil {
		return config.App{}, fmt.Errorf("failed to save config: %w", err)
	}

	return duplicated, nil
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

	return toggled, nil
}

// ToggleAppPublished toggles the published state of an app
func (m *Manager) ToggleAppPublished(appID string) (config.App, error) {
	cfg, err := m.store.Load()
	if err != nil {
		return config.App{}, fmt.Errorf("failed to load config: %w", err)
	}

	var toggled config.App
	found := false
	for i, a := range cfg.Apps {
		if a.ID == appID {
			cfg.Apps[i].Published = !cfg.Apps[i].Published
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
		if !rawApp.Published {
			continue
		}
		if !config.AppTargetsNode(rawApp, targetNode, nodeName) {
			continue
		}

		nodeArch := "amd64"
		parts := strings.Fields(osInfo)
		if len(parts) >= 2 {
			nodeArch = parts[1]
		} else if len(parts) == 1 && !strings.Contains(parts[0], "linux") {
			nodeArch = parts[0]
		}
		goArch := apps.NormalizeArch(nodeArch, nil)
		gnuArch := apps.ToGNUArch(goArch)
		rustArch := apps.ToRustArch(goArch)
		debArch := apps.ToDebArch(goArch)

		var ezappDir string
		if strings.ToLower(rawApp.AppType) == "docker" {
			ezappDir = "/etc/ezappc/ezapp-" + rawApp.Name
		} else {
			ezappDir = "/etc/ezapp/" + rawApp.Name
		}

		// App-specific context variables
		appCtx := make(map[string]string, len(contextVars)+14)
		for k, v := range contextVars {
			appCtx[k] = v
		}
		appCtx["APP_NAME"] = rawApp.Name
		appCtx["EZAPP_DIR"] = ezappDir
		appCtx["ezapp_dir"] = ezappDir
		appCtx["NODE_ARCH"] = nodeArch
		appCtx["ARCH"] = goArch
		appCtx["ARCH_GNU"] = gnuArch
		appCtx["ARCH_RUST"] = rustArch
		appCtx["ARCH_DEB"] = debArch
		appCtx["arch"] = goArch
		appCtx["arch_gnu"] = gnuArch
		appCtx["arch_rust"] = rustArch
		appCtx["arch_deb"] = debArch

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
		} else if strings.ToLower(app.AppType) == "cron" {
			spec.AppType = agentpb.AppType_CRON
		} else {
			spec.AppType = agentpb.AppType_SYSTEMD
			spec.MainContent = apps.EnsureSystemdEnvironmentFile(app.Name, spec.MainContent)

			// Transpile if target node uses OpenWrt / Procd or Alpine / OpenRC
			parsedUnit, err := apps.ParseSystemdService(spec.MainContent)
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

		// Process Artifacts for native (systemd) apps
		if rawApp.Artifact != nil && strings.ToLower(rawApp.AppType) == "systemd" && m.artifactMgr != nil && m.artifactTokens != nil {
			osName := "linux"

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			extractedFiles, err := m.artifactMgr.EnsureArtifact(ctx, rawApp, nodeArch, osName, globalVars)
			cancel()
			if err != nil {
				// Failed to download or verify artifact upstream
			} else {
				binPath := "/usr/local/bin"
				if strings.Contains(osInfo, "openwrt") {
					binPath = "/usr/bin"
				}
				for _, reqFile := range rawApp.Artifact.Files {
					info, ok := extractedFiles[reqFile.Name]
					if !ok || info == nil {
						continue
					}
					token := m.artifactTokens.GenerateToken(info.LocalPath, reqFile.Name, info.SHA256, nodeName, 24*time.Hour)

					pullPolicy := reqFile.PullPolicy
					if pullPolicy == "" {
						pullPolicy = "if_changed"
					}
					fileMode := reqFile.FileMode
					if fileMode == 0 {
						fileMode = 0755
					}

					targetPath := reqFile.TargetPath
					targetPath = strings.ReplaceAll(targetPath, "${PATH}", binPath)
					targetPath = strings.ReplaceAll(targetPath, "${EZAPP_DIR}", ezappDir)

					spec.Artifacts = append(spec.Artifacts, &agentpb.AppArtifactFile{
						Name:          reqFile.Name,
						TargetPath:    targetPath,
						PullPolicy:    pullPolicy,
						FileSha256:    info.SHA256,
						DownloadToken: token,
						FileMode:      fileMode,
						FileSize:      uint64(info.Size),
					})
				}
			}
		}

		spec.Sha256Hash = apps.ComputeAppSpecFilesHash(spec)

		specs = append(specs, spec)
	}

	return specs, nil
}

// SyncAppsToNode builds and sends the desired apps to a specific node agent
func (m *Manager) SyncAppsToNode(nodeName string) error {
	if m.agentHub == nil {
		return nil
	}

	if m.agentHub.IsAppsDisabled(nodeName) {
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
		if m.agentHub.IsAppsDisabled(node.Name) {
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

	if conn.DisableApps() {
		return nil, fmt.Errorf("app management is disabled on node '%s' (disable_apps = true in agent.json)", nodeName)
	}

	protoType := agentpb.AppType_DOCKER_COMPOSE
	if strings.ToLower(appType) == "systemd" {
		protoType = agentpb.AppType_SYSTEMD
	} else if strings.ToLower(appType) == "cron" {
		protoType = agentpb.AppType_CRON
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
