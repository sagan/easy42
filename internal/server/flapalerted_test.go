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

	"easy42/internal/config"
)

func TestFlapAlertedMetricsStream(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login to get session cookie
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d", w.Code)
	}
	cookie := w.Result().Cookies()[0]

	// 1. Not configured -> expect 400 Bad Request
	req = httptest.NewRequest("GET", "/api/flaps/metrics/stream", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when unconfigured, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Mock FlapAlerted service
	samplePayload := `{"ActiveFlapCount":0,"ActiveFlapTotalPathChangeCount":0,"TotalPathChangeCount":381,"AverageRouteChanges90":"7.82","Sessions":3}`
	faServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/flaps/metrics/json" {
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte(samplePayload))
			return
		}
		http.NotFound(rw, r)
	}))
	defer faServer.Close()

	// Configure NetworkSettings with custom FlapAlerted URL
	err := srv.mgr.UpdateNetworkSettings(config.NetworkSettings{
		FlapAlertedURL: faServer.URL,
	})
	if err != nil {
		t.Fatalf("UpdateNetworkSettings failed: %v", err)
	}

	// 3. Test successful SSE stream
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req = httptest.NewRequestWithContext(ctx, "GET", "/api/flaps/metrics/stream", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for SSE stream, got %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), samplePayload) {
		t.Fatalf("expected body to contain sample metrics, got:\n%s", w.Body.String())
	}
	if !strings.HasPrefix(w.Body.String(), "data: ") {
		t.Fatalf("expected body to start with 'data: ', got:\n%s", w.Body.String())
	}

	// 4. Test one-shot JSON endpoint /api/flaps/metrics
	req = httptest.NewRequest("GET", "/api/flaps/metrics", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for one-shot metrics, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "AverageRouteChanges90") {
		t.Fatalf("expected JSON body with AverageRouteChanges90, got: %s", w.Body.String())
	}

	// 5. Test FA URL inaccessible -> returns 502 Bad Gateway directly
	err = srv.mgr.UpdateNetworkSettings(config.NetworkSettings{
		FlapAlertedURL: "http://127.0.0.1:54321", // non-existent service
	})
	if err != nil {
		t.Fatalf("UpdateNetworkSettings failed: %v", err)
	}

	req = httptest.NewRequest("GET", "/api/flaps/metrics/stream", nil)
	req.AddCookie(cookie)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 Bad Gateway when inaccessible, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "FlapAlerted is inaccessible") {
		t.Fatalf("expected error message in body, got: %s", w.Body.String())
	}
}

func TestResolveFAMetricsURL(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"http://[fd42:a159:f9f0:8000::c8]:8699", "http://[fd42:a159:f9f0:8000::c8]:8699/flaps/metrics/json"},
		{"http://[fd42:a159:f9f0:8000::c8]:8699/", "http://[fd42:a159:f9f0:8000::c8]:8699/flaps/metrics/json"},
		{"https://fa.example.com", "https://fa.example.com/flaps/metrics/json"},
		{"https://fa.example.com/flaps/metrics/json", "https://fa.example.com/flaps/metrics/json"},
	}

	for _, c := range cases {
		got := resolveFAMetricsURL(c.input)
		if got != c.expected {
			t.Errorf("resolveFAMetricsURL(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}
