package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"easy42/internal/config"
	agentpb "easy42/internal/agent/proto"
	"github.com/go-chi/chi/v5"
)

func TestAppsAPI(t *testing.T) {
	srv, tempDir, _ := setupTestServer(t)
	defer func() {
		srv.mgr.Close()
		_ = os.RemoveAll(tempDir)
	}()

	// 1. Create App
	newApp := config.App{
		ID:          "my-nginx-app",
		Name:        "test-app",
		Description: "A test docker compose app",
		AppType:     "docker",
		MainContent: "version: '3'\nservices:\n  web:\n    image: nginx\n",
		EnvContent:  "PORT=80\n",
		Files: map[string]string{
			"config.json": `{"hello":"world"}`,
		},
		Nodes:   []string{"node1"},
		Enabled: true,
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
	if dup1.Enabled != false {
		t.Fatalf("expected duplicated app to be disabled, got %v", dup1.Enabled)
	}
	if dup1.MainContent != created.MainContent {
		t.Fatalf("expected duplicated content to match original")
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
	if dup2.Enabled != false {
		t.Fatalf("expected dup2 to be disabled, got %v", dup2.Enabled)
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
	if !rcloneAct.NeedsApply || rcloneAct.DiffStatus != "update" {
		t.Fatalf("expected update pending after unit file modified, got diff_status=%s, needs_apply=%v", rcloneAct.DiffStatus, rcloneAct.NeedsApply)
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


