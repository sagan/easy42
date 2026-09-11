package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"easy42/internal/config"
	"easy42/internal/lookingglass"
)

func TestLookingGlassAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]

	// 1. Get Builtin Tasks
	req = httptest.NewRequest("GET", "/api/looking-glass/tasks", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get tasks failed: %d - %s", w.Code, w.Body.String())
	}
	var tasksResp lookingglass.TasksResponse
	if err := json.NewDecoder(w.Body).Decode(&tasksResp); err != nil {
		t.Fatalf("decode tasks failed: %v", err)
	}
	if len(tasksResp.Builtin) == 0 {
		t.Fatalf("expected built-in tasks, got 0")
	}

	// 2. Create Custom Task
	newTask := config.LookingGlassTask{
		ID:          "custom_ping_google",
		Name:        "Ping Google DNS",
		Description: "Pings 8.8.8.8",
		Category:    "Diagnostics",
		CommandTmpl: "ping -c 2 8.8.8.8",
		Parser:      "ping",
		TimeoutSec:  10,
	}
	newTaskBytes, _ := json.Marshal(newTask)
	req = httptest.NewRequest("POST", "/api/looking-glass/tasks", bytes.NewReader(newTaskBytes))
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create task failed: %d - %s", w.Code, w.Body.String())
	}

	// 3. Verify Custom Task in list
	req = httptest.NewRequest("GET", "/api/looking-glass/tasks", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if err := json.NewDecoder(w.Body).Decode(&tasksResp); err != nil {
		t.Fatalf("decode tasks failed: %v", err)
	}
	found := false
	for _, ct := range tasksResp.Custom {
		if ct.ID == "custom_ping_google" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("custom task not found in list")
	}

	// 4. Delete Custom Task
	req = httptest.NewRequest("DELETE", "/api/looking-glass/tasks/custom_ping_google", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete task failed: %d - %s", w.Code, w.Body.String())
	}
}
