package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"easy42/internal/config"
)

func TestEventsEndpoints(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login to get session cookie
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Login failed: code %d, body: %s", w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]

	// 1. Initial unread count should be 0
	reqUnread := httptest.NewRequest("GET", "/api/events/unread-count", nil)
	reqUnread.AddCookie(cookie)
	wUnread := httptest.NewRecorder()
	srv.router.ServeHTTP(wUnread, reqUnread)
	if wUnread.Code != http.StatusOK {
		t.Fatalf("GET unread-count failed: %d", wUnread.Code)
	}
	var countResp map[string]int
	_ = json.Unmarshal(wUnread.Body.Bytes(), &countResp)
	if countResp["unread_count"] != 0 {
		t.Fatalf("expected 0 unread, got %d", countResp["unread_count"])
	}

	// 2. Record 2 events via mgr
	e1, err := srv.mgr.RecordEvent(&config.Event{
		Type:      config.EventTypeNodeOffline,
		NodeName:  "node-london",
		Title:     "Node node-london is offline",
		Message:   "Node is disconnected",
		Metadata:  map[string]any{"hostname": "lon1.net"},
		CreatedAt: time.Now().Add(-5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("RecordEvent e1 failed: %v", err)
	}

	e2, err := srv.mgr.RecordEvent(&config.Event{
		Type:      config.EventTypeCronTaskFailure,
		NodeName:  "node-london",
		TaskID:    "cron-ping-cf",
		Title:     "Cron task failed",
		Message:   "timeout",
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("RecordEvent e2 failed: %v", err)
	}

	// 3. Check unread count should now be 2
	reqUnread2 := httptest.NewRequest("GET", "/api/events/unread-count", nil)
	reqUnread2.AddCookie(cookie)
	wUnread2 := httptest.NewRecorder()
	srv.router.ServeHTTP(wUnread2, reqUnread2)
	_ = json.Unmarshal(wUnread2.Body.Bytes(), &countResp)
	if countResp["unread_count"] != 2 {
		t.Fatalf("expected 2 unread, got %d", countResp["unread_count"])
	}

	// 4. Query events list
	reqList := httptest.NewRequest("GET", "/api/events?limit=10&offset=0", nil)
	reqList.AddCookie(cookie)
	wList := httptest.NewRecorder()
	srv.router.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("GET /api/events failed: %d", wList.Code)
	}
	var listResp config.EventsListResponse
	_ = json.Unmarshal(wList.Body.Bytes(), &listResp)
	if listResp.Total != 2 || len(listResp.Events) != 2 || listResp.UnreadCount != 2 {
		t.Fatalf("unexpected list response: %+v", listResp)
	}

	// 5. Mark e1 as read
	reqRead1 := httptest.NewRequest("POST", fmt.Sprintf("/api/events/%d/read", e1.ID), nil)
	reqRead1.AddCookie(cookie)
	wRead1 := httptest.NewRecorder()
	srv.router.ServeHTTP(wRead1, reqRead1)
	if wRead1.Code != http.StatusOK {
		t.Fatalf("mark read failed: %d", wRead1.Code)
	}

	// Check unread count is 1
	reqUnread3 := httptest.NewRequest("GET", "/api/events/unread-count", nil)
	reqUnread3.AddCookie(cookie)
	wUnread3 := httptest.NewRecorder()
	srv.router.ServeHTTP(wUnread3, reqUnread3)
	_ = json.Unmarshal(wUnread3.Body.Bytes(), &countResp)
	if countResp["unread_count"] != 1 {
		t.Fatalf("expected 1 unread, got %d", countResp["unread_count"])
	}

	// 6. Mark all as read
	reqReadAll := httptest.NewRequest("POST", "/api/events/read-all", nil)
	reqReadAll.AddCookie(cookie)
	wReadAll := httptest.NewRecorder()
	srv.router.ServeHTTP(wReadAll, reqReadAll)
	if wReadAll.Code != http.StatusOK {
		t.Fatalf("mark all read failed: %d", wReadAll.Code)
	}

	reqUnread4 := httptest.NewRequest("GET", "/api/events/unread-count", nil)
	reqUnread4.AddCookie(cookie)
	wUnread4 := httptest.NewRecorder()
	srv.router.ServeHTTP(wUnread4, reqUnread4)
	_ = json.Unmarshal(wUnread4.Body.Bytes(), &countResp)
	if countResp["unread_count"] != 0 {
		t.Fatalf("expected 0 unread, got %d", countResp["unread_count"])
	}

	// 7. Delete event e2
	reqDel := httptest.NewRequest("DELETE", fmt.Sprintf("/api/events/%d", e2.ID), nil)
	reqDel.AddCookie(cookie)
	wDel := httptest.NewRecorder()
	srv.router.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("delete event failed: %d", wDel.Code)
	}

	// 8. Clear all events
	reqClear := httptest.NewRequest("DELETE", "/api/events", nil)
	reqClear.AddCookie(cookie)
	wClear := httptest.NewRecorder()
	srv.router.ServeHTTP(wClear, reqClear)
	if wClear.Code != http.StatusOK {
		t.Fatalf("clear events failed: %d", wClear.Code)
	}

	// Verify empty list
	reqEmpty := httptest.NewRequest("GET", "/api/events", nil)
	reqEmpty.AddCookie(cookie)
	wEmpty := httptest.NewRecorder()
	srv.router.ServeHTTP(wEmpty, reqEmpty)
	_ = json.Unmarshal(wEmpty.Body.Bytes(), &listResp)
	if listResp.Total != 0 || len(listResp.Events) != 0 {
		t.Fatalf("expected empty events list after clear, got: %+v", listResp)
	}
}
