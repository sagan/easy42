package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func TestAppsAPI(t *testing.T) {
	srv, tempDir, _ := setupTestServer(t)
	defer func() {
		srv.mgr.Close()
		_ = os.RemoveAll(tempDir)
	}()

	// 0. Add node1
	cfg := srv.mgr.Store().Get()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name: "node1",
		Host: "192.168.1.50",
		IP:   "10.42.0.77/24",
	})
	_ = srv.mgr.Store().Save(cfg)

	// 1. Create App
	newApp := config.App{
		ID:          "my-nginx-app",
		Name:        "test-app",
		Description: "A test docker compose app",
		URL:         "http://${NODE_IP}:8080",
		AppType:     "docker",
		MainContent: "version: '3'\nservices:\n  web:\n    image: nginx\n",
		EnvContent:  "PORT=80\n",
		Files: map[string]string{
			"config.json": `{"hello":"world"}`,
		},
		Nodes:     []string{"node1"},
		Published: true,
		Enabled:   true,
	}

	body, _ := json.Marshal(newApp)
	req := httptest.NewRequest(http.MethodPost, "/api/apps", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleCreateApp(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var created config.App
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if created.ID == "" || created.Name != "test-app" {
		t.Fatalf("unexpected created app: %+v", created)
	}
	if created.URL != "http://${NODE_IP}:8080" {
		t.Fatalf("expected created app URL to be http://${NODE_IP}:8080, got %s", created.URL)
	}

	// 2. Get Apps
	req = httptest.NewRequest(http.MethodGet, "/api/apps", nil)
	w = httptest.NewRecorder()
	srv.handleGetApps(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var appsList []config.App
	_ = json.NewDecoder(w.Body).Decode(&appsList)
	if len(appsList) != 1 || appsList[0].ID != created.ID {
		t.Fatalf("expected 1 app with id %s, got %+v", created.ID, appsList)
	}

	// 3. Toggle App
	req = httptest.NewRequest(http.MethodPost, "/api/apps/"+created.ID+"/toggle", nil)
	rctx := chiContextWithParam("id", created.ID)
	req = req.WithContext(rctx)
	w = httptest.NewRecorder()
	srv.handleToggleApp(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 on toggle, got %d: %s", w.Code, w.Body.String())
	}

	var toggled config.App
	_ = json.NewDecoder(w.Body).Decode(&toggled)
	if toggled.Enabled != false {
		t.Fatalf("expected enabled to be false after toggle, got %v", toggled.Enabled)
	}

	// 4. Rename App
	renameBody, _ := json.Marshal(map[string]string{"new_name": "renamed-app"})
	req = httptest.NewRequest(http.MethodPost, "/api/apps/"+created.ID+"/rename", bytes.NewReader(renameBody))
	req = req.WithContext(rctx)
	w = httptest.NewRecorder()
	srv.handleRenameApp(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 on rename, got %d: %s", w.Code, w.Body.String())
	}
	var renamed config.App
	_ = json.NewDecoder(w.Body).Decode(&renamed)
	if renamed.Name != "renamed-app" {
		t.Fatalf("expected renamed name to be 'renamed-app', got %s", renamed.Name)
	}

	// Rename with invalid name should fail
	badRenameBody, _ := json.Marshal(map[string]string{"new_name": "bad name!"})
	req = httptest.NewRequest(http.MethodPost, "/api/apps/"+created.ID+"/rename", bytes.NewReader(badRenameBody))
	req = req.WithContext(rctx)
	w = httptest.NewRecorder()
	srv.handleRenameApp(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 on invalid name, got %d", w.Code)
	}

	// 5. Duplicate App (first time -> appends '1')
	req = httptest.NewRequest(http.MethodPost, "/api/apps/"+created.ID+"/duplicate", nil)
	req = req.WithContext(rctx)
	w = httptest.NewRecorder()
	srv.handleDuplicateApp(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on duplicate, got %d: %s", w.Code, w.Body.String())
	}
	var dup1 config.App
	_ = json.NewDecoder(w.Body).Decode(&dup1)
	expectedDup1ID := created.ID + "1"
	if dup1.ID != expectedDup1ID {
		t.Fatalf("expected dup1 ID to be %s, got %s", expectedDup1ID, dup1.ID)
	}
	if dup1.Published != false {
		t.Fatalf("expected duplicated app to be draft (published=false), got %v", dup1.Published)
	}
	if dup1.Enabled != false {
		t.Fatalf("expected duplicated app to be disabled (enabled=false), got %v", dup1.Enabled)
	}
	if dup1.MainContent != created.MainContent {
		t.Fatalf("expected duplicated content to match original")
	}
	if dup1.URL != "http://${NODE_IP}:8080" {
		t.Fatalf("expected duplicated app URL to match original, got %s", dup1.URL)
	}

	// Duplicate dup1 (ends in '1' -> increments to '2')
	rctxDup1 := chiContextWithParam("id", dup1.ID)
	req = httptest.NewRequest(http.MethodPost, "/api/apps/"+dup1.ID+"/duplicate", nil)
	req = req.WithContext(rctxDup1)
	w = httptest.NewRecorder()
	srv.handleDuplicateApp(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on duplicating dup1, got %d: %s", w.Code, w.Body.String())
	}
	var dup2 config.App
	_ = json.NewDecoder(w.Body).Decode(&dup2)
	expectedDup2ID := created.ID + "2"
	if dup2.ID != expectedDup2ID {
		t.Fatalf("expected dup2 ID to be %s, got %s", expectedDup2ID, dup2.ID)
	}
	if dup2.Published != false {
		t.Fatalf("expected dup2 to be draft (published=false), got %v", dup2.Published)
	}
	if dup2.Enabled != false {
		t.Fatalf("expected dup2 to be disabled (enabled=false), got %v", dup2.Enabled)
	}

	// 6. Preview Transpile
	previewReq := transpilePreviewRequest{
		AppName:     "mydaemon",
		TargetInit:  "openrc",
		MainContent: "[Unit]\nDescription=My Daemon\n[Service]\nExecStart=/usr/bin/mydaemon --flag\n",
		EnvContent:  "FOO=bar\n",
	}
	pBody, _ := json.Marshal(previewReq)
	req = httptest.NewRequest(http.MethodPost, "/api/apps/preview-transpile", bytes.NewReader(pBody))
	w = httptest.NewRecorder()
	srv.handlePreviewTranspile(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for preview transpile, got %d", w.Code)
	}
	var previewResp transpilePreviewResponse
	_ = json.NewDecoder(w.Body).Decode(&previewResp)
	if previewResp.Error != "" || previewResp.Script == "" {
		t.Fatalf("unexpected preview response: %+v", previewResp)
	}

	// Verify handleGetNodeApps resolves URL variables
	nodeAppsReq := httptest.NewRequest(http.MethodGet, "/api/nodes/node1/apps", nil)
	nodeAppsReq = nodeAppsReq.WithContext(chiContextWithParam("name", "node1"))
	nodeAppsW := httptest.NewRecorder()
	srv.handleGetNodeApps(nodeAppsW, nodeAppsReq)
	if nodeAppsW.Code != http.StatusOK {
		t.Fatalf("expected 200 on handleGetNodeApps, got %d: %s", nodeAppsW.Code, nodeAppsW.Body.String())
	}
	var nodeAppsResp NodeAppsResponse
	_ = json.NewDecoder(nodeAppsW.Body).Decode(&nodeAppsResp)
	var foundApp *config.App
	for i := range nodeAppsResp.Apps {
		if nodeAppsResp.Apps[i].ID == created.ID {
			foundApp = &nodeAppsResp.Apps[i]
			break
		}
	}
	if foundApp == nil {
		t.Fatalf("created app not found in node1 apps: %+v", nodeAppsResp.Apps)
	}
	if foundApp.URL != "http://10.42.0.77:8080" {
		t.Fatalf("expected resolved node app URL to be 'http://10.42.0.77:8080', got '%s'", foundApp.URL)
	}

	// 5. Delete App
	req = httptest.NewRequest(http.MethodDelete, "/api/apps/"+created.ID, nil)
	req = req.WithContext(rctx)
	w = httptest.NewRecorder()
	srv.handleDeleteApp(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 on delete, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAppVariablesAPI(t *testing.T) {
	srv, tempDir, _ := setupTestServer(t)
	defer func() {
		srv.mgr.Close()
		_ = os.RemoveAll(tempDir)
	}()

	// 1. Create Variable
	v := config.AppVariable{
		Key:         "CLUSTER_DOMAIN",
		Value:       "easy42.internal",
		Description: "Global cluster domain",
		IsSecret:    false,
	}
	body, _ := json.Marshal(v)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/variables", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.handleSaveAppVariable(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on save variable, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Get Variables
	req = httptest.NewRequest(http.MethodGet, "/api/apps/variables", nil)
	w = httptest.NewRecorder()
	srv.handleGetAppVariables(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on get variables, got %d", w.Code)
	}
	var list []config.AppVariable
	_ = json.NewDecoder(w.Body).Decode(&list)
	if len(list) != 1 || list[0].Key != "CLUSTER_DOMAIN" {
		t.Fatalf("expected 1 variable with key CLUSTER_DOMAIN, got %+v", list)
	}

	// 3. Preview Interpolate
	pReq := interpolatePreviewRequest{
		Content:  "Server: ${CLUSTER_DOMAIN}\nNode: ${NODE_NAME}",
		NodeName: "gateway-1",
	}
	pBody, _ := json.Marshal(pReq)
	req = httptest.NewRequest(http.MethodPost, "/api/apps/preview-interpolate", bytes.NewReader(pBody))
	w = httptest.NewRecorder()
	srv.handlePreviewInterpolate(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on preview interpolate, got %d", w.Code)
	}
	var pResp interpolatePreviewResponse
	_ = json.NewDecoder(w.Body).Decode(&pResp)
	expected := "Server: easy42.internal\nNode: gateway-1"
	if pResp.Rendered != expected {
		t.Fatalf("expected rendered '%s', got '%s'", expected, pResp.Rendered)
	}

	// 4. Delete Variable
	req = httptest.NewRequest(http.MethodDelete, "/api/apps/variables/CLUSTER_DOMAIN", nil)
	req = req.WithContext(chiContextWithParam("key", "CLUSTER_DOMAIN"))
	w = httptest.NewRecorder()
	srv.handleDeleteAppVariable(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete variable, got %d", w.Code)
	}
}

func chiContextWithParam(key, val string) context.Context {
	chiCtx := chi.NewRouteContext()
	chiCtx.URLParams.Add(key, val)
	return context.WithValue(context.Background(), chi.RouteCtxKey, chiCtx)
}

func TestNodeAppsDisabled(t *testing.T) {
	srv, tempDir, _ := setupTestServer(t)
	defer func() {
		srv.mgr.Close()
		_ = os.RemoveAll(tempDir)
	}()

	// 1. Initially node1 has apps enabled (default)
	req := httptest.NewRequest(http.MethodGet, "/api/nodes/node1/apps", nil)
	req = req.WithContext(chiContextWithParam("name", "node1"))
	w := httptest.NewRecorder()
	srv.handleGetNodeApps(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp NodeAppsResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.DisableApps {
		t.Fatalf("expected DisableApps to be false initially")
	}

	// 2. Report telemetry with DisableApps = true
	srv.mgr.AgentHub().UpdateTelemetry("node1", &agentpb.TelemetryReport{
		DisableApps: true,
	})

	if !srv.mgr.AgentHub().IsAppsDisabled("node1") {
		t.Fatalf("expected AgentHub().IsAppsDisabled(node1) to be true")
	}

	// 3. Query again, DisableApps should now be true
	req = httptest.NewRequest(http.MethodGet, "/api/nodes/node1/apps", nil)
	req = req.WithContext(chiContextWithParam("name", "node1"))
	w = httptest.NewRecorder()
	srv.handleGetNodeApps(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp2 NodeAppsResponse
	_ = json.NewDecoder(w.Body).Decode(&resp2)
	if !resp2.DisableApps {
		t.Fatalf("expected DisableApps to be true after telemetry report")
	}
}

func TestAppSyncReconciliation(t *testing.T) {
	srv, tempDir, _ := setupTestServer(t)
	defer func() {
		srv.mgr.Close()
		_ = os.RemoveAll(tempDir)
	}()

	// 1. Add target node
	cfg := srv.mgr.Store().Get()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name: "nbbug",
		Host: "192.168.142.207",
		Mode: "agent",
	})
	if err := srv.mgr.Store().Save(cfg); err != nil {
		t.Fatalf("failed to save node: %v", err)
	}

	// 2. Create rclone app targeting nbbug
	app := config.App{
		ID:          "app-rclone",
		Name:        "rclone",
		AppType:     "systemd",
		MainContent: "[Unit]\nDescription=rclone service\n\n[Service]\nExecStart=/usr/bin/rclone serve webdav\n",
		EnvContent:  "RCLONE_PASS=123456\n",
		Nodes:       []string{"nbbug"},
		Published:   true,
		Enabled:     true,
	}
	if _, err := srv.mgr.SaveApp(app); err != nil {
		t.Fatalf("failed to save app: %v", err)
	}

	// 3. Plan sync when app has never been deployed -> diff_status: "create", needs_apply: true
	actions, err := srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	var rcloneAct *config.SyncAction
	for i := range actions {
		if actions[i].Interface == "rclone" && actions[i].Type == config.ActionSyncApp {
			rcloneAct = &actions[i]
			break
		}
	}
	if rcloneAct == nil {
		t.Fatalf("expected sync action for rclone, none found in %v", actions)
	}
	if !rcloneAct.NeedsApply || rcloneAct.DiffStatus != "create" {
		t.Fatalf("expected create pending, got diff_status=%s, needs_apply=%v", rcloneAct.DiffStatus, rcloneAct.NeedsApply)
	}

	// 4. Record deployed state in StateStore matching desired hash
	specs, _ := srv.mgr.BuildAppSpecsForNode("nbbug")
	if len(specs) == 0 {
		t.Fatalf("expected 1 spec, got 0")
	}
	rcloneSpec := specs[0]
	if err := srv.mgr.StateStore().UpdateAppState("nbbug", "192.168.142.207", "rclone", rcloneSpec.Sha256Hash, time.Now()); err != nil {
		t.Fatalf("failed to update app state: %v", err)
	}

	// 5. Plan sync again -> should be synced (needs_apply = false)
	actions, err = srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	rcloneAct = nil
	for i := range actions {
		if actions[i].Interface == "rclone" && actions[i].Type == config.ActionSyncApp {
			rcloneAct = &actions[i]
			break
		}
	}
	if rcloneAct == nil {
		t.Fatalf("expected sync action for rclone, none found")
	}
	if rcloneAct.NeedsApply || rcloneAct.DiffStatus != "synced" {
		t.Fatalf("expected synced action, got diff_status=%s, needs_apply=%v", rcloneAct.DiffStatus, rcloneAct.NeedsApply)
	}

	// 6. User edits app unit file on server
	app.MainContent = "[Unit]\nDescription=rclone modified service\n\n[Service]\nExecStart=/usr/bin/rclone serve webdav --read-only\n"
	if _, err := srv.mgr.SaveApp(app); err != nil {
		t.Fatalf("failed to update app: %v", err)
	}

	// 7. Plan sync -> hash changed! diff_status: "update", needs_apply: true
	// Set live probed files on agentHub to test diff generation
	srv.mgr.AgentHub().SetAppFiles("nbbug", map[string]agent.ProbeConfigFile{
		"/etc/systemd/system/ezapp-rclone.service": {
			Path:    "/etc/systemd/system/ezapp-rclone.service",
			Content: "[Unit]\nDescription=rclone service\n\n[Service]\nExecStart=/usr/bin/rclone serve webdav\n",
			Hash:    "oldhash",
		},
		"/etc/systemd/system/ezapp-rclone.env": {
			Path:    "/etc/systemd/system/ezapp-rclone.env",
			Content: "RCLONE_PASS=123456\n",
			Hash:    "envhash",
		},
	})

	actions, err = srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	var unitAct, envAct *config.SyncAction
	for i := range actions {
		if actions[i].Interface == "rclone" && actions[i].Type == config.ActionSyncApp {
			if strings.HasSuffix(actions[i].TargetFile, ".service") {
				unitAct = &actions[i]
			} else if strings.HasSuffix(actions[i].TargetFile, ".env") {
				envAct = &actions[i]
			}
		}
	}
	if unitAct == nil {
		t.Fatalf("expected sync action for rclone .service unit")
	}
	if !unitAct.NeedsApply || unitAct.DiffStatus != "update" {
		t.Fatalf("expected update pending for unit file, got diff_status=%s, needs_apply=%v", unitAct.DiffStatus, unitAct.NeedsApply)
	}
	if unitAct.Diff == "" || !strings.Contains(unitAct.Diff, "+ Description=rclone modified service") {
		t.Fatalf("expected unit file diff containing modification, got: %s", unitAct.Diff)
	}

	if envAct == nil {
		t.Fatalf("expected sync action for rclone .env file")
	}
	// env content didn't change, so envAct should be synced
	if envAct.NeedsApply || envAct.DiffStatus != "synced" {
		t.Fatalf("expected synced status for unchanged env file, got diff_status=%s, needs_apply=%v", envAct.DiffStatus, envAct.NeedsApply)
	}

	// Now modify env content
	app.EnvContent = "RCLONE_PASS=secret999\n"
	if _, err := srv.mgr.SaveApp(app); err != nil {
		t.Fatalf("failed to update app env: %v", err)
	}
	actions, err = srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	envAct = nil
	for i := range actions {
		if actions[i].Interface == "rclone" && actions[i].Type == config.ActionSyncApp && strings.HasSuffix(actions[i].TargetFile, ".env") {
			envAct = &actions[i]
			break
		}
	}
	if envAct == nil || !envAct.NeedsApply || envAct.DiffStatus != "update" {
		t.Fatalf("expected env file update action, got %+v", envAct)
	}
	if envAct.Diff == "" || !strings.Contains(envAct.Diff, "+ RCLONE_PASS=secret999") {
		t.Fatalf("expected env diff containing secret999, got: %s", envAct.Diff)
	}

	// 8. Unassign rclone from nbbug -> diff_status: "delete", needs_apply: true
	app.Nodes = []string{}
	if _, err := srv.mgr.SaveApp(app); err != nil {
		t.Fatalf("failed to unassign app: %v", err)
	}
	actions, err = srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	var deleteAct *config.SyncAction
	for i := range actions {
		if actions[i].Interface == "rclone" && actions[i].Type == config.ActionDeleteApp {
			deleteAct = &actions[i]
			break
		}
	}
	if deleteAct == nil {
		t.Fatalf("expected delete action for orphaned rclone, none found in %v", actions)
	}
	if !deleteAct.NeedsApply || deleteAct.DiffStatus != "delete" {
		t.Fatalf("expected delete pending, got diff_status=%s, needs_apply=%v", deleteAct.DiffStatus, deleteAct.NeedsApply)
	}
}

func TestAppMutationsDoNotAutoSyncToConnectedAgent(t *testing.T) {
	srv, tempDir, _ := setupTestServer(t)
	defer func() {
		srv.mgr.Close()
		_ = os.RemoveAll(tempDir)
	}()

	// 1. Add target node "berohost"
	cfg := srv.mgr.Store().Get()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name: "berohost",
		Host: "192.168.142.208",
		Mode: "agent",
	})
	if err := srv.mgr.Store().Save(cfg); err != nil {
		t.Fatalf("failed to save node: %v", err)
	}

	// 2. Set up mock agent connection
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := agent.NewConnection("berohost", ws, srv.mgr.AgentHub())
		srv.mgr.AgentHub().Register(conn)
		go conn.ReadLoop()
	}))
	defer wsServer.Close()

	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")
	clientWs, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial mock agent server: %v", err)
	}
	defer clientWs.Close()

	time.Sleep(50 * time.Millisecond)

	if !srv.mgr.AgentHub().IsConnected("berohost") {
		t.Fatalf("expected berohost to be connected in AgentHub")
	}

	// Channel to capture agent incoming messages
	msgCh := make(chan *agentpb.AgentMessage, 20)
	go func() {
		for {
			_, msgBytes, err := clientWs.ReadMessage()
			if err != nil {
				return
			}
			var agentMsg agentpb.AgentMessage
			if err := proto.Unmarshal(msgBytes, &agentMsg); err == nil {
				msgCh <- &agentMsg
				if cmdReq := agentMsg.GetCommandReq(); cmdReq != nil {
					resp := &agentpb.AgentMessage{
						Payload: &agentpb.AgentMessage_CommandResp{
							CommandResp: &agentpb.CommandResponse{
								RequestId: cmdReq.RequestId,
								Success:   true,
								Stdout:    "{}",
							},
						},
					}
					data, _ := proto.Marshal(resp)
					_ = clientWs.WriteMessage(websocket.BinaryMessage, data)
				}
			}
		}
	}()

	// Drain any initial messages
	select {
	case <-msgCh:
	case <-time.After(50 * time.Millisecond):
	}

	// 3. Create app "rclone" with RestartSec=10
	app := config.App{
		ID:          "app-rclone-demo",
		Name:        "rclone",
		AppType:     "systemd",
		MainContent: "[Unit]\nDescription=rclone service\n\n[Service]\nExecStart=/usr/bin/rclone serve\nRestart=always\nRestartSec=10\n",
		EnvContent:  "FOO=bar\n",
		Nodes:       []string{"berohost"},
		Published:   true,
		Enabled:     true,
	}

	// Save initial app
	if _, err := srv.mgr.SaveApp(app); err != nil {
		t.Fatalf("failed to save app: %v", err)
	}

	// Crucial check: verify NO SyncApps message was sent to agent on SaveApp
	select {
	case msg := <-msgCh:
		if msg.GetSyncApps() != nil {
			t.Fatalf("unexpected SyncApps received upon SaveApp, desired state should not auto-apply!")
		}
	case <-time.After(100 * time.Millisecond):
		// Success: no auto-sync
	}

	// 4. Record deployed state as having RestartSec=10
	specs, _ := srv.mgr.BuildAppSpecsForNode("berohost")
	if len(specs) == 0 {
		t.Fatalf("expected specs to have rclone")
	}
	srv.mgr.AgentHub().SetAppFiles("berohost", map[string]agent.ProbeConfigFile{
		"/etc/systemd/system/ezapp-rclone.service": {
			Path:    "/etc/systemd/system/ezapp-rclone.service",
			Content: specs[0].MainContent,
			Hash:    specs[0].Sha256Hash,
		},
	})
	srv.mgr.AgentHub().SetAppHash("berohost", "rclone", specs[0].Sha256Hash)

	// Initially plan sync should show synced
	actions, err := srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	for _, act := range actions {
		if act.Interface == "rclone" && strings.HasSuffix(act.TargetFile, ".service") {
			if act.NeedsApply {
				t.Fatalf("expected rclone .service to be synced, got needs_apply=true")
			}
		}
	}

	// 5. User changes RestartSec=10 to RestartSec=5 in frontend and saves
	app.MainContent = "[Unit]\nDescription=rclone service\n\n[Service]\nExecStart=/usr/bin/rclone serve\nRestart=always\nRestartSec=5\n"
	if _, err := srv.mgr.SaveApp(app); err != nil {
		t.Fatalf("failed to update app: %v", err)
	}

	// Crucial check: verify NO SyncApps message was sent to agent upon modifying app
	select {
	case msg := <-msgCh:
		if msg.GetSyncApps() != nil {
			t.Fatalf("unexpected SyncApps received upon SaveApp edit!")
		}
	case <-time.After(100 * time.Millisecond):
		// Success: no auto-sync!
	}

	// 6. Plan sync: must detect diff and mark NeedsApply = true
	actions, err = srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	var rcloneServiceAct *config.SyncAction
	for i := range actions {
		if actions[i].Interface == "rclone" && strings.HasSuffix(actions[i].TargetFile, ".service") {
			rcloneServiceAct = &actions[i]
			break
		}
	}
	if rcloneServiceAct == nil {
		t.Fatalf("expected rclone service action in plan")
	}
	if !rcloneServiceAct.NeedsApply {
		t.Fatalf("expected NeedsApply=true for modified rclone app")
	}
	if rcloneServiceAct.DiffStatus != "update" {
		t.Fatalf("expected DiffStatus='update', got: %s", rcloneServiceAct.DiffStatus)
	}
	if !strings.Contains(rcloneServiceAct.Diff, "- RestartSec=10") || !strings.Contains(rcloneServiceAct.Diff, "+ RestartSec=5") {
		t.Fatalf("expected diff to show RestartSec change, got:\n%s", rcloneServiceAct.Diff)
	}

	// 7. Toggle app enabled state: should ALSO NOT auto-sync to agent
	if _, err := srv.mgr.ToggleApp(app.ID); err != nil {
		t.Fatalf("failed to toggle app: %v", err)
	}
	select {
	case msg := <-msgCh:
		if msg.GetSyncApps() != nil {
			t.Fatalf("unexpected SyncApps received upon ToggleApp!")
		}
	case <-time.After(100 * time.Millisecond):
		// Success: no auto-sync!
	}

	// Toggle it back on
	if _, err := srv.mgr.ToggleApp(app.ID); err != nil {
		t.Fatalf("failed to toggle app back: %v", err)
	}

	// 8. Now explicitly execute sync action (like clicking 'Apply Changes' in UI)
	results, err := srv.mgr.ExecuteSyncNodesWithScope(false, "", "berohost")
	if err != nil {
		t.Fatalf("ExecuteSyncNodesWithScope failed: %v", err)
	}
	if len(results) == 0 || !results[0].Success {
		t.Fatalf("expected sync success, got: %+v", results)
	}

	// 9. Verify that NOW the agent DID receive SyncApps
	var syncApps *agentpb.SyncApps
	timeout := time.After(2 * time.Second)
	for syncApps == nil {
		select {
		case msg := <-msgCh:
			if sync := msg.GetSyncApps(); sync != nil {
				syncApps = sync
			}
		case <-timeout:
			t.Fatalf("timed out waiting for SyncApps on agent after ExecuteSyncNodes")
		}
	}

	foundRclone := false
	for _, sp := range syncApps.Apps {
		if sp.Name == "rclone" {
			foundRclone = true
			if !strings.Contains(sp.MainContent, "RestartSec=5") {
				t.Fatalf("expected agent to receive updated content with RestartSec=5")
			}
		}
	}
	if !foundRclone {
		t.Fatalf("expected rclone spec in SyncApps payload")
	}

	// 10. Verify plan is now clean
	actions, err = srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	for _, act := range actions {
		if act.Interface == "rclone" && strings.HasSuffix(act.TargetFile, ".service") {
			if act.NeedsApply {
				t.Fatalf("expected rclone service to be synced after apply, got NeedsApply=true")
			}
		}
	}
}

func TestAppDraftIsNotDeployed(t *testing.T) {
	srv, tempDir, _ := setupTestServer(t)
	defer func() {
		srv.mgr.Close()
		_ = os.RemoveAll(tempDir)
	}()

	// 1. Add target node "node-draft"
	cfg := srv.mgr.Store().Get()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name: "node-draft",
		Host: "192.168.1.100",
		Mode: "agent",
	})
	if err := srv.mgr.Store().Save(cfg); err != nil {
		t.Fatalf("failed to save node: %v", err)
	}

	// 2. Create app with Published: false (draft)
	draftApp := config.App{
		ID:          "app-draft-test",
		Name:        "myapp-draft",
		AppType:     "systemd",
		MainContent: "[Unit]\nDescription=draft app\n[Service]\nExecStart=/bin/true\n",
		Nodes:       []string{"node-draft"},
		Published:   false,
		Enabled:     true,
	}
	saved, err := srv.mgr.SaveApp(draftApp)
	if err != nil {
		t.Fatalf("failed to save draft app: %v", err)
	}
	if saved.Published {
		t.Fatalf("expected saved app to have Published=false")
	}

	// 3. Verify BuildAppSpecsForNode does not include draft app
	specs, err := srv.mgr.BuildAppSpecsForNode("node-draft")
	if err != nil {
		t.Fatalf("BuildAppSpecsForNode failed: %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("expected 0 specs for draft app, got %d", len(specs))
	}

	// 4. Verify PlanSyncWithScope does not generate ActionSyncApp
	actions, err := srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	for _, act := range actions {
		if act.Interface == "myapp-draft" {
			t.Fatalf("unexpected sync action for draft app: %+v", act)
		}
	}

	// 5. Toggle published via HTTP endpoint
	rctx := chiContextWithParam("id", draftApp.ID)
	req := httptest.NewRequest(http.MethodPost, "/api/apps/"+draftApp.ID+"/toggle-published", nil)
	req = req.WithContext(rctx)
	w := httptest.NewRecorder()
	srv.handleToggleAppPublished(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on toggle-published, got %d: %s", w.Code, w.Body.String())
	}
	var toggled config.App
	_ = json.NewDecoder(w.Body).Decode(&toggled)
	if !toggled.Published {
		t.Fatalf("expected app to be published after toggle")
	}

	// 6. Now BuildAppSpecsForNode includes the app!
	specs, err = srv.mgr.BuildAppSpecsForNode("node-draft")
	if err != nil {
		t.Fatalf("BuildAppSpecsForNode failed: %v", err)
	}
	if len(specs) != 1 || specs[0].Name != "myapp-draft" {
		t.Fatalf("expected 1 spec for published app, got %+v", specs)
	}

	// 7. Plan sync now produces ActionSyncApp
	actions, err = srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	found := false
	for _, act := range actions {
		if act.Interface == "myapp-draft" && act.Type == config.ActionSyncApp {
			found = true
			if !act.NeedsApply {
				t.Fatalf("expected NeedsApply=true for newly published app")
			}
		}
	}
	if !found {
		t.Fatalf("expected ActionSyncApp for published app in plan")
	}
}

func TestAppDeleteAuxiliaryConfigFileReconciliation(t *testing.T) {
	srv, tempDir, _ := setupTestServer(t)
	defer func() {
		srv.mgr.Close()
		_ = os.RemoveAll(tempDir)
	}()

	// 1. Add target agent node
	cfg := srv.mgr.Store().Get()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name: "node-aux",
		Host: "192.168.1.101",
		Mode: "agent",
	})
	if err := srv.mgr.Store().Save(cfg); err != nil {
		t.Fatalf("failed to save node: %v", err)
	}

	// Set up mock agent connection
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := agent.NewConnection("node-aux", ws, srv.mgr.AgentHub())
		srv.mgr.AgentHub().Register(conn)
		go conn.ReadLoop()
	}))
	defer wsServer.Close()

	wsURL := "ws" + strings.TrimPrefix(wsServer.URL, "http")
	clientWs, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial mock agent server: %v", err)
	}
	defer clientWs.Close()

	// Respond to agent commands
	go func() {
		for {
			_, msgBytes, err := clientWs.ReadMessage()
			if err != nil {
				return
			}
			var agentMsg agentpb.AgentMessage
			if err := proto.Unmarshal(msgBytes, &agentMsg); err == nil {
				if cmdReq := agentMsg.GetCommandReq(); cmdReq != nil {
					resp := &agentpb.AgentMessage{
						Payload: &agentpb.AgentMessage_CommandResp{
							CommandResp: &agentpb.CommandResponse{
								RequestId: cmdReq.RequestId,
								Success:   true,
								Stdout:    "{}",
							},
						},
					}
					data, _ := proto.Marshal(resp)
					_ = clientWs.WriteMessage(websocket.BinaryMessage, data)
				}
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)

	// 2. Create app with an auxiliary config file config.json
	auxApp := config.App{
		ID:          "app-aux-test",
		Name:        "myapp-aux",
		AppType:     "systemd",
		MainContent: "[Unit]\nDescription=aux app\n[Service]\nExecStart=/bin/true\n",
		EnvContent:  "FOO=bar\n",
		Files: map[string]string{
			"config.json": `{"version": 1}`,
		},
		Nodes:     []string{"node-aux"},
		Published: true,
		Enabled:   true,
	}
	saved, err := srv.mgr.SaveApp(auxApp)
	if err != nil {
		t.Fatalf("failed to save aux app: %v", err)
	}

	// 3. Build specs to retrieve desired hash and content
	specs, err := srv.mgr.BuildAppSpecsForNode("node-aux")
	if err != nil || len(specs) != 1 {
		t.Fatalf("BuildAppSpecsForNode failed or returned wrong specs count: %v, specs: %+v", err, specs)
	}

	// 4. Simulate that the app and its config.json were already deployed on the node
	hub := srv.mgr.AgentHub()
	hub.SetAppHash("node-aux", "myapp-aux", specs[0].Sha256Hash)
	hub.SetAppFiles("node-aux", map[string]agent.ProbeConfigFile{
		"/etc/systemd/system/ezapp-myapp-aux.service": {
			Path:    "/etc/systemd/system/ezapp-myapp-aux.service",
			Content: specs[0].MainContent,
			Hash:    config.HashConfig(specs[0].MainContent),
		},
		"/etc/ezapp-data/myapp-aux/config.json": {
			Path:    "/etc/ezapp-data/myapp-aux/config.json",
			Content: `{"version": 1}`,
			Hash:    config.HashConfig(`{"version": 1}`),
		},
	})

	// 5. Verify initial PlanSyncWithScope: everything is synced (NeedsApply: false)
	actions, err := srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	for _, act := range actions {
		if act.Interface == "myapp-aux" && act.NeedsApply {
			t.Fatalf("expected all actions to be synced initially, but got: %+v", act)
		}
	}

	// 6. Delete config.json from the app
	saved.Files = nil
	if _, err := srv.mgr.SaveApp(saved); err != nil {
		t.Fatalf("failed to update app removing config.json: %v", err)
	}

	// 7. Verify PlanSyncWithScope detects the deletion of config.json
	actions, err = srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope failed: %v", err)
	}
	var deleteAuxAct *config.SyncAction
	for i := range actions {
		act := &actions[i]
		if act.Interface == "myapp-aux" && act.TargetFile == "/etc/ezapp-data/myapp-aux/config.json" {
			deleteAuxAct = act
			break
		}
	}
	if deleteAuxAct == nil {
		t.Fatalf("expected ActionDeleteApp for orphaned config.json in sync plan, got actions: %+v", actions)
	}
	if !deleteAuxAct.NeedsApply {
		t.Fatalf("expected NeedsApply=true for deleted config.json, got false")
	}
	if deleteAuxAct.DiffStatus != "delete" {
		t.Fatalf("expected DiffStatus='delete', got '%s'", deleteAuxAct.DiffStatus)
	}
	if deleteAuxAct.Type != config.ActionDeleteApp {
		t.Fatalf("expected Type=ActionDeleteApp, got '%s'", deleteAuxAct.Type)
	}
	if !strings.Contains(deleteAuxAct.Diff, `{"version": 1}`) {
		t.Fatalf("expected diff to show deletion of config.json content, got: %s", deleteAuxAct.Diff)
	}

	// 8. Execute sync actions
	results, err := srv.mgr.ExecuteSyncNodes(false, "node-aux")
	if err != nil {
		t.Fatalf("ExecuteSyncNodes failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least 1 sync result")
	}

	// 9. Verify hub cached app files no longer has config.json
	cachedFiles := hub.GetAppFiles("node-aux")
	if _, ok := cachedFiles["/etc/ezapp-data/myapp-aux/config.json"]; ok {
		t.Fatalf("expected /etc/ezapp-data/myapp-aux/config.json to be removed from agentHub appFiles, but it was still present")
	}

	// 10. Verify PlanSyncWithScope now has no pending changes for config.json
	actionsAfter, err := srv.mgr.PlanSyncWithScope("")
	if err != nil {
		t.Fatalf("PlanSyncWithScope after sync failed: %v", err)
	}
	for _, act := range actionsAfter {
		if act.TargetFile == "/etc/ezapp-data/myapp-aux/config.json" {
			t.Fatalf("orphaned config.json action still appears in plan after sync: %+v", act)
		}
	}
}



