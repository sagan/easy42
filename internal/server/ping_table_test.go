package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"easy42/internal/config"
)

func TestPingTableAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Save nodes to config store
	cfg := srv.mgr.Store().Get()
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.Nodes = []config.Node{
		{Name: "alice", IP: "192.168.100.1", IsExternal: false},
		{Name: "bob", IP: "192.168.100.2", IsExternal: false},
		{Name: "carol", IP: "192.168.100.3", IsExternal: false},
		{Name: "ext1", IP: "8.8.8.8", IsExternal: true},
	}
	cfg.Links = []config.Link{
		{
			From: config.LinkEnd{Name: "alice", Interface: "wg42bob", Address: "fe80::c0a8:6401/64"},
			To:   config.LinkEnd{Name: "bob", Interface: "wg42alice", Address: "fe80::c0a8:6402/64"},
		},
	}
	if err := srv.mgr.Store().Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Login
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]

	// 1. Run Ping Table (e2e)
	runReqBody, _ := json.Marshal(map[string]string{"latency_type": "e2e"})
	req = httptest.NewRequest("POST", "/api/looking-glass/ping-table/run", bytes.NewReader(runReqBody))
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("run ping-table failed: %d - %s", w.Code, w.Body.String())
	}
	var task config.PingTableTask
	if err := json.NewDecoder(w.Body).Decode(&task); err != nil {
		t.Fatalf("decode task failed: %v", err)
	}
	if task.ID == "" {
		t.Fatalf("expected non-empty task ID")
	}
	if len(task.Nodes) != 3 {
		t.Fatalf("expected 3 internal nodes, got %d", len(task.Nodes))
	}
	if task.LatencyType != "e2e" {
		t.Fatalf("expected e2e latency type, got %s", task.LatencyType)
	}

	// 2. Get Current Ping Table
	req = httptest.NewRequest("GET", "/api/looking-glass/ping-table/current", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get current ping-table failed: %d", w.Code)
	}

	// 3. Cancel Ping Table
	req = httptest.NewRequest("POST", "/api/looking-glass/ping-table/cancel", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel ping-table failed: %d", w.Code)
	}

	time.Sleep(100 * time.Millisecond)

	// 4. Get Ping Table History
	req = httptest.NewRequest("GET", "/api/looking-glass/ping-table/history", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get ping-table history failed: %d", w.Code)
	}
	var history []config.PingTableTaskSummary
	if err := json.NewDecoder(w.Body).Decode(&history); err != nil {
		t.Fatalf("decode history failed: %v", err)
	}
	if len(history) == 0 {
		t.Fatalf("expected history entries, got 0")
	}

	// 5. Get Ping Table Task By ID
	req = httptest.NewRequest("GET", "/api/looking-glass/ping-table/tasks/"+task.ID, nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get ping-table task failed: %d", w.Code)
	}
	var fetchedTask config.PingTableTask
	if err := json.NewDecoder(w.Body).Decode(&fetchedTask); err != nil {
		t.Fatalf("decode fetched task failed: %v", err)
	}
	if fetchedTask.ID != task.ID {
		t.Fatalf("expected ID %s, got %s", task.ID, fetchedTask.ID)
	}

	// 6. Delete Ping Table Task By ID
	req = httptest.NewRequest("DELETE", "/api/looking-glass/ping-table/tasks/"+task.ID, nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete ping-table task failed: %d", w.Code)
	}

	// 7. Verify Task is deleted
	req = httptest.NewRequest("GET", "/api/looking-glass/ping-table/tasks/"+task.ID, nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}
