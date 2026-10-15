package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"easy42/internal/config"
	"easy42/internal/engine"
	"github.com/go-chi/chi/v5"
)

func withURLParam(req *http.Request, key, val string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, val)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestServerTasksAPI(t *testing.T) {
	tempDir := t.TempDir()
	store := config.NewStore(tempDir)
	_, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	mgr := engine.NewManager(store)
	defer mgr.Close()

	srv := New(Config{
		Manager: mgr,
	})

	// 1. Test GET /api/settings/tasks
	req := httptest.NewRequest("GET", "/api/settings/tasks", nil)
	w := httptest.NewRecorder()
	srv.handleGetServerTasks(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var tasks []engine.ServerTask
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("failed to parse tasks response: %v", err)
	}

	if len(tasks) != 3 {
		t.Fatalf("expected 3 predefined tasks, got %d", len(tasks))
	}

	expectedIDs := []string{config.ServerTaskUpdateROA, config.ServerTaskSync, config.ServerTaskSyncDNS}
	for i, expID := range expectedIDs {
		if tasks[i].ID != expID {
			t.Errorf("task %d: expected id %s, got %s", i, expID, tasks[i].ID)
		}
		expectedCron := config.DefaultCronForTask(expID)
		if tasks[i].CronExpr != expectedCron {
			t.Errorf("task %s: expected default cron '%s', got '%s'", expID, expectedCron, tasks[i].CronExpr)
		}
		if !tasks[i].Enabled {
			t.Errorf("task %s: expected default enabled=true, got false", expID)
		}
	}

	// 2. Test PUT /api/settings/tasks/update_roa (Valid update)
	updatePayload := map[string]interface{}{
		"cron_expr": "0 */6 * * *",
		"enabled":   false,
	}
	bodyBytes, _ := json.Marshal(updatePayload)
	req = httptest.NewRequest("PUT", "/api/settings/tasks/update_roa", bytes.NewReader(bodyBytes))
	req = withURLParam(req, "id", config.ServerTaskUpdateROA)
	w = httptest.NewRecorder()
	srv.handleUpdateServerTask(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 on update, got %d: %s", w.Code, w.Body.String())
	}

	var updated engine.ServerTask
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("failed to decode updated task: %v", err)
	}
	if updated.CronExpr != "0 */6 * * *" || updated.Enabled != false {
		t.Errorf("expected updated task cron '0 */6 * * *' and enabled false, got %s, %v", updated.CronExpr, updated.Enabled)
	}

	// 3. Test PUT with invalid cron expression
	invalidPayload := map[string]interface{}{
		"cron_expr": "invalid cron",
		"enabled":   true,
	}
	bodyBytes, _ = json.Marshal(invalidPayload)
	req = httptest.NewRequest("PUT", "/api/settings/tasks/update_roa", bytes.NewReader(bodyBytes))
	req = withURLParam(req, "id", config.ServerTaskUpdateROA)
	w = httptest.NewRecorder()
	srv.handleUpdateServerTask(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid cron, got %d", w.Code)
	}

	// 4. Test PUT with unknown task ID
	req = httptest.NewRequest("PUT", "/api/settings/tasks/unknown_task", bytes.NewReader(bodyBytes))
	req = withURLParam(req, "id", "unknown_task")
	w = httptest.NewRecorder()
	srv.handleUpdateServerTask(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status 404 for unknown task ID, got %d", w.Code)
	}

	// 5. Test POST /api/settings/tasks/sync/run (Execute immediately)
	req = httptest.NewRequest("POST", "/api/settings/tasks/sync/run", nil)
	req = withURLParam(req, "id", config.ServerTaskSync)
	w = httptest.NewRecorder()
	srv.handleRunServerTask(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 on run, got %d: %s", w.Code, w.Body.String())
	}

	var ranTask engine.ServerTask
	if err := json.Unmarshal(w.Body.Bytes(), &ranTask); err != nil {
		t.Fatalf("failed to decode ran task: %v", err)
	}
	if ranTask.LastRun == nil {
		t.Errorf("expected non-nil LastRun after execution")
	}
	if ranTask.LastStatus != "success" {
		t.Errorf("expected LastStatus 'success', got '%s'", ranTask.LastStatus)
	}
	if ranTask.LastMessage != "Successfully synced 0 node(s) (0 actions executed)" {
		t.Errorf("unexpected LastMessage: %s", ranTask.LastMessage)
	}
}
