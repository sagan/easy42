package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"easy42/internal/apps"
	"easy42/internal/config"

	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetApps(w http.ResponseWriter, r *http.Request) {
	allApps, err := s.mgr.GetApps()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, allApps)
}

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var app config.App
	if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	saved, err := s.mgr.SaveApp(app)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	var app config.App
	if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	id := chi.URLParam(r, "id")
	if id != "" {
		app.ID = id
	}

	saved, err := s.mgr.SaveApp(app)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "App ID is required")
		return
	}

	if err := s.mgr.DeleteApp(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleToggleApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "App ID is required")
		return
	}

	app, err := s.mgr.ToggleApp(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, app)
}

func (s *Server) handleToggleAppPublished(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "App ID is required")
		return
	}

	app, err := s.mgr.ToggleAppPublished(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, app)
}

type renameAppRequest struct {
	NewName string `json:"new_name"`
}

func (s *Server) handleRenameApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "App ID is required")
		return
	}

	var req renameAppRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	newName := strings.TrimSpace(req.NewName)
	if newName == "" {
		writeError(w, http.StatusBadRequest, "New app name is required")
		return
	}

	renamed, err := s.mgr.RenameApp(id, newName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, renamed)
}

func (s *Server) handleDuplicateApp(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "App ID is required")
		return
	}

	duplicated, err := s.mgr.DuplicateApp(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, duplicated)
}

// NodeAppsResponse combines assigned apps configuration and live statuses on a node
type NodeAppsResponse struct {
	Apps        []config.App            `json:"apps"`
	Statuses    []engineAppStatusRecord `json:"statuses"`
	DisableApps bool                    `json:"disable_apps"`
}

type engineAppStatusRecord struct {
	AppID        string `json:"app_id"`
	Name         string `json:"name"`
	AppType      string `json:"app_type"`
	State        string `json:"state"`
	StatusText   string `json:"status_text"`
	LastUpdated  int64  `json:"last_updated"`
	ErrorMessage string `json:"error_message,omitempty"`
}

func (s *Server) handleGetNodeApps(w http.ResponseWriter, r *http.Request) {
	nodeName := chi.URLParam(r, "name")
	if nodeName == "" {
		writeError(w, http.StatusBadRequest, "Node name is required")
		return
	}

	allApps, err := s.mgr.GetApps()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	targetNode := s.mgr.FindNode(nodeName)
	var contextVars map[string]string
	var globalVars map[string]string
	if targetNode != nil {
		contextVars = apps.BuildNodeContextVars(*targetNode)
		rawVars, _ := s.mgr.GetAppVariables()
		globalVars = apps.VariablesToMap(rawVars)
	}

	var nodeApps []config.App
	for _, a := range allApps {
		if a.Published && config.AppTargetsNode(a, targetNode, nodeName) {
			appCopy := a
			if appCopy.URL != "" && targetNode != nil {
				appCopy.URL = apps.InterpolateVariables(appCopy.URL, globalVars, contextVars)
			}
			nodeApps = append(nodeApps, appCopy)
		}
	}

	rawStatuses := s.mgr.GetNodeAppsStatus(nodeName)
	var statuses []engineAppStatusRecord
	for _, st := range rawStatuses {
		statuses = append(statuses, engineAppStatusRecord{
			AppID:        st.AppID,
			Name:         st.Name,
			AppType:      st.AppType,
			State:        st.State,
			StatusText:   st.StatusText,
			LastUpdated:  st.LastUpdated,
			ErrorMessage: st.ErrorMessage,
		})
	}

	disabled := s.mgr.AgentHub() != nil && s.mgr.AgentHub().IsAppsDisabled(nodeName)

	writeJSON(w, http.StatusOK, NodeAppsResponse{
		Apps:        nodeApps,
		Statuses:    statuses,
		DisableApps: disabled,
	})
}

type appActionRequest struct {
	Action   string `json:"action"` // "start" | "stop" | "restart" | "logs"
	AppType  string `json:"app_type"`
	LogLines uint32 `json:"log_lines,omitempty"`
}

func (s *Server) handleNodeAppAction(w http.ResponseWriter, r *http.Request) {
	nodeName := chi.URLParam(r, "name")
	appName := chi.URLParam(r, "appName")

	var req appActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	if req.Action == "" {
		req.Action = "restart"
	}
	if req.LogLines == 0 {
		req.LogLines = 100
	}

	resp, err := s.mgr.TriggerAppAction(r.Context(), nodeName, appName, req.Action, req.AppType, req.LogLines)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

type appClusterActionResult struct {
	NodeName   string `json:"node_name"`
	Success    bool   `json:"success"`
	ExitCode   int32  `json:"exit_code"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	DurationMs uint64 `json:"duration_ms,omitempty"`
	StartedAt  int64  `json:"started_at,omitempty"`
	EndedAt    int64  `json:"ended_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

type appClusterActionResponse struct {
	AppID   string                   `json:"app_id"`
	AppName string                   `json:"app_name"`
	Action  string                   `json:"action"`
	Results []appClusterActionResult `json:"results"`
}

func (s *Server) handleAppClusterAction(w http.ResponseWriter, r *http.Request) {
	appID := chi.URLParam(r, "id")

	var req appActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	appsList, err := s.mgr.GetApps()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load apps: "+err.Error())
		return
	}

	var targetApp *config.App
	for _, a := range appsList {
		if a.ID == appID || a.Name == appID {
			targetApp = &a
			break
		}
	}
	if targetApp == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("App '%s' not found", appID))
		return
	}

	if req.Action == "" {
		req.Action = "start"
	}
	if req.LogLines == 0 {
		req.LogLines = 100
	}
	if req.AppType == "" {
		req.AppType = targetApp.AppType
	}

	// Find all target nodes
	cfg := s.mgr.Store().Get()
	if cfg == nil {
		cfg, _ = s.mgr.Store().Load()
	}

	var targetNodes []config.Node
	if cfg != nil {
		for _, n := range cfg.Nodes {
			if n.IsExternal {
				continue
			}
			if config.AppTargetsNode(*targetApp, &n, n.Name) {
				targetNodes = append(targetNodes, n)
			}
		}
	}

	if len(targetNodes) == 0 {
		writeJSON(w, http.StatusOK, appClusterActionResponse{
			AppID:   targetApp.ID,
			AppName: targetApp.Name,
			Action:  req.Action,
			Results: []appClusterActionResult{},
		})
		return
	}

	results := make([]appClusterActionResult, len(targetNodes))
	var wg sync.WaitGroup

	for i, node := range targetNodes {
		wg.Add(1)
		go func(idx int, n config.Node) {
			defer wg.Done()
			resp, err := s.mgr.TriggerAppAction(r.Context(), n.Name, targetApp.Name, req.Action, req.AppType, req.LogLines)
			if err != nil {
				results[idx] = appClusterActionResult{
					NodeName: n.Name,
					Success:  false,
					ExitCode: -1,
					Error:    err.Error(),
				}
				return
			}
			results[idx] = appClusterActionResult{
				NodeName:   n.Name,
				Success:    resp.Success,
				ExitCode:   resp.ExitCode,
				Stdout:     resp.Stdout,
				Stderr:     resp.Stderr,
				DurationMs: resp.DurationMs,
				StartedAt:  resp.StartedAt,
				EndedAt:    resp.EndedAt,
			}
		}(i, node)
	}
	wg.Wait()

	writeJSON(w, http.StatusOK, appClusterActionResponse{
		AppID:   targetApp.ID,
		AppName: targetApp.Name,
		Action:  req.Action,
		Results: results,
	})
}

type transpilePreviewRequest struct {
	AppName     string            `json:"app_name"`
	TargetInit  string            `json:"target_init"` // "openrc" | "procd"
	MainContent string            `json:"main_content"`
	EnvContent  string            `json:"env_content"`
	Files       map[string]string `json:"files,omitempty"`
}

type transpilePreviewResponse struct {
	Script      string `json:"script"`
	ConfD       string `json:"conf_d,omitempty"`
	Environment string `json:"environment,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (s *Server) handlePreviewTranspile(w http.ResponseWriter, r *http.Request) {
	var req transpilePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if req.AppName == "" {
		req.AppName = "sample"
	}

	parsedUnit, err := apps.ParseSystemdService(req.MainContent)
	if err != nil {
		writeJSON(w, http.StatusOK, transpilePreviewResponse{Error: "Failed to parse systemd service: " + err.Error()})
		return
	}

	parsedEnv := apps.ParseEnvFile(req.EnvContent)
	for k, v := range parsedUnit.Environment {
		if _, exists := parsedEnv[k]; !exists {
			parsedEnv[k] = v
		}
	}

	target := strings.ToLower(req.TargetInit)
	if target == "openrc" {
		script, err := apps.TranspileToOpenRC(req.AppName, parsedUnit, parsedEnv)
		if err != nil {
			writeJSON(w, http.StatusOK, transpilePreviewResponse{Error: err.Error()})
			return
		}
		confD := apps.GenerateOpenRCConfD(req.AppName, parsedEnv)
		writeJSON(w, http.StatusOK, transpilePreviewResponse{
			Script: script,
			ConfD:  confD,
		})
		return
	}

	// Default to procd
	script, err := apps.TranspileToProcd(req.AppName, parsedUnit, parsedEnv)
	if err != nil {
		writeJSON(w, http.StatusOK, transpilePreviewResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, transpilePreviewResponse{
		Script: script,
	})
}

func (s *Server) handleGetAppVariables(w http.ResponseWriter, r *http.Request) {
	vars, err := s.mgr.GetAppVariables()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, vars)
}

func (s *Server) handleSaveAppVariable(w http.ResponseWriter, r *http.Request) {
	var variable config.AppVariable
	if err := json.NewDecoder(r.Body).Decode(&variable); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	saved, err := s.mgr.SaveAppVariable(variable)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteAppVariable(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "Variable key is required")
		return
	}

	if err := s.mgr.DeleteAppVariable(key); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

type interpolatePreviewRequest struct {
	Content  string `json:"content"`
	NodeName string `json:"node_name,omitempty"`
	AppName  string `json:"app_name,omitempty"`
	AppType  string `json:"app_type,omitempty"`
}

type interpolatePreviewResponse struct {
	Rendered string `json:"rendered"`
}

func (s *Server) handlePreviewInterpolate(w http.ResponseWriter, r *http.Request) {
	var req interpolatePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	allVars, _ := s.mgr.GetAppVariables()
	globalVars := apps.VariablesToMap(allVars)

	var targetNode *config.Node
	if req.NodeName != "" {
		targetNode = s.mgr.FindNode(req.NodeName)
	}

	var contextVars map[string]string
	if targetNode != nil {
		contextVars = apps.BuildNodeContextVars(*targetNode)
	} else {
		nodeName := req.NodeName
		if nodeName == "" {
			nodeName = "sample-node"
		}
		contextVars = map[string]string{
			"NODE_NAME":         nodeName,
			"NODE_INTERFACE":    "dummy0",
			"NODE_IP":           "10.42.0.1",
			"NODE_IP6":          "fd42:a159:f9f0::1",
			"NODE_EXTERNAL_IP":  "172.20.229.1",
			"NODE_EXTERNAL_IP6": "2001:db8::1",
			"HAS_CUSTOM_SCRIPT": "0",
			"NODE_TAGS":         "edge,vps",
			"NODE_TAG_EDGE":     "1",
			"NODE_TAG_VPS":      "1",
			"NODE_IPV4":         "10.42.0.1",
			"NODE_IPV6":         "fd42:a159:f9f0::1",
		}
	}
	if req.AppName != "" {
		contextVars["APP_NAME"] = req.AppName
	} else {
		contextVars["APP_NAME"] = "my-app"
	}

	nodeArch := "amd64"
	nodeEndian := ""
	if req.NodeName != "" {
		osInfo := strings.ToLower(s.mgr.GetOSInfo(req.NodeName))
		if arch := apps.NodeArchFromOSInfo(osInfo); arch != "" {
			nodeArch = arch
		}
		nodeEndian = apps.NodeEndianFromOSInfo(osInfo)
	}
	goArch := apps.NormalizeArch(nodeArch, nil)
	contextVars["NODE_ARCH"] = nodeArch
	contextVars["NODE_ENDIAN"] = nodeEndian
	contextVars["ARCH"] = goArch
	contextVars["ARCH_GNU"] = apps.ToGNUArch(goArch)
	contextVars["ARCH_RUST"] = apps.ToRustArch(goArch)
	contextVars["ARCH_DEB"] = apps.ToDebArch(goArch)

	appName := req.AppName
	if appName == "" {
		appName = "my-app"
	}
	var ezappDir string
	if strings.ToLower(req.AppType) == "docker" {
		ezappDir = "/etc/ezappc/ezapp-" + appName
	} else {
		ezappDir = "/etc/ezapp/" + appName
	}
	contextVars["EZAPP_DIR"] = ezappDir

	rendered := apps.InterpolateVariables(req.Content, globalVars, contextVars)
	writeJSON(w, http.StatusOK, interpolatePreviewResponse{Rendered: rendered})
}

func (s *Server) handleGetAppArtifactStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "App ID is required")
		return
	}

	status, err := s.mgr.GetAppArtifactStatus(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleGetAllAppArtifactStatus(w http.ResponseWriter, r *http.Request) {
	statuses, err := s.mgr.GetAllAppArtifactStatus()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) handleValidateAppArtifact(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "App ID is required")
		return
	}

	res, err := s.mgr.ValidateAppArtifact(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleValidateAllAppArtifacts(w http.ResponseWriter, r *http.Request) {
	results, err := s.mgr.ValidateAllAppArtifacts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleClearAppArtifactCache(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "App ID is required")
		return
	}

	res, err := s.mgr.ClearAppArtifactCache(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleClearAllAppArtifactCache(w http.ResponseWriter, r *http.Request) {
	res, err := s.mgr.ClearAllAppArtifactCache()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, res)
}
