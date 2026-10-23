package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"easy42/tasks"
)

func TestOnlineTasksAPI(t *testing.T) {
	tasks.SetAllowPrivateSourcesForTest(true)
	defer tasks.SetAllowPrivateSourcesForTest(false)

	srv, tempDir, pass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login to get session cookie
	loginBody, _ := json.Marshal(map[string]string{"password": pass})
	reqLogin := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	reqLogin.Header.Set("Content-Type", "application/json")
	wLogin := httptest.NewRecorder()
	srv.router.ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Fatalf("login failed: %d: %s", wLogin.Code, wLogin.Body.String())
	}
	cookie := wLogin.Result().Cookies()[0]

	// Mock remote online tasks repo
	manifestContent := `{"tasks":[{"id":"mock_task","title":"Mock Task","description":"A mock task","category":"Custom","weight":10,"files":["run.sh"]}]}`
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(manifestContent))
		case "/mock_task/run.sh":
			_, _ = w.Write([]byte("#!/bin/sh\necho 'running mock task'\nexit 0\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	// 1. Test GET /api/tasks/online/sources
	req := httptest.NewRequest("GET", "/api/tasks/online/sources", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Test GET /api/tasks/online/manifest with custom source
	manifestReq := httptest.NewRequest("GET", fmt.Sprintf("/api/tasks/online/manifest?source=%s", mockServer.URL), nil)
	manifestReq.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, manifestReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var manifestResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &manifestResp); err != nil {
		t.Fatalf("failed to decode manifest response: %v", err)
	}
	tasksArr, ok := manifestResp["tasks"].([]interface{})
	if !ok || len(tasksArr) != 1 {
		t.Fatalf("unexpected tasks in response: %v", manifestResp)
	}

	// 3. Test POST /api/tasks/online/install
	installBody := map[string]interface{}{
		"source":  mockServer.URL,
		"task_id": "mock_task",
		"files":   []string{"run.sh"},
	}
	installBytes, _ := json.Marshal(installBody)
	installReq := httptest.NewRequest("POST", "/api/tasks/online/install", bytes.NewReader(installBytes))
	installReq.Header.Set("Content-Type", "application/json")
	installReq.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, installReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on install, got %d: %s", w.Code, w.Body.String())
	}

	// Verify task file exists on disk
	installedRunSh := filepath.Join(tempDir, "tasks", "mock_task", "run.sh")
	if _, err := os.Stat(installedRunSh); err != nil {
		t.Fatalf("expected installed run.sh at %s, got err: %v", installedRunSh, err)
	}

	// Verify it shows up in GET /api/tasks
	getTasksReq := httptest.NewRequest("GET", "/api/tasks", nil)
	getTasksReq.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, getTasksReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on get tasks, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("mock_task")) {
		t.Fatalf("expected mock_task in GET /api/tasks, got: %s", w.Body.String())
	}

	// 4. Test DELETE /api/tasks/custom/{id}
	delReq := httptest.NewRequest("DELETE", "/api/tasks/custom/mock_task", nil)
	delReq.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, delReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on delete, got %d: %s", w.Code, w.Body.String())
	}

	// Verify task was deleted
	if _, err := os.Stat(filepath.Join(tempDir, "tasks", "mock_task")); !os.IsNotExist(err) {
		t.Fatalf("expected mock_task to be deleted from disk")
	}
}
