package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/internal/engine"
)

func setupTestServerForCustomAPI(t *testing.T) (*Server, func(), string) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "easy42-customapi-srv-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	cfg := store.Get()
	cfg.Nodes = []config.Node{
		{Name: "alpha", Host: "10.42.0.1", ASN: 4200000001},
		{Name: "beta", Host: "10.42.0.2", ASN: 4200000002},
	}
	_ = store.Save(cfg)

	mgr := engine.NewManager(store)
	_ = mgr.Unlock(pass)

	srv := New(Config{
		ListenAddr: "127.0.0.1:0",
		Manager:    mgr,
	})

	cfg = store.Get()
	token := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 24*time.Hour)

	cleanup := func() {
		srv.Close()
		_ = os.RemoveAll(tempDir)
	}

	return srv, cleanup, token
}

func TestCustomAPIServerEndpoints(t *testing.T) {
	srv, cleanup, adminToken := setupTestServerForCustomAPI(t)
	defer cleanup()

	// 1. Create a custom API via admin endpoint
	newAPI := config.CustomAPI{
		ID:          "fleet-info",
		Name:        "Fleet Info",
		Description: "Exposes cluster fleet info in JSON format",
		ContentType: "application/json",
		Token:       "secret-bearer-xyz",
		ExtraHeaders: map[string]string{
			"X-Easy42-Cluster": "production",
			"Access-Control-Allow-Origin": "*",
		},
		Template: `{"count": {{ len .config.nodes }}, "nodes": [{{ range $i, $n := .config.nodes }}{{ if $i }},{{ end }}"{{ $n.name }}"{{ end }}], "query_agent": "{{ .request.query.agent }}"}`,
	}

	body, _ := json.Marshal(newAPI)
	req := httptest.NewRequest("POST", "/api/custom-apis", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var created config.CustomAPI
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to decode created API: %v", err)
	}
	if created.ID != "fleet-info" {
		t.Fatalf("expected ID 'fleet-info', got %s", created.ID)
	}

	// 2. Test Public Execution without Token (Should fail with 401 Unauthorized)
	reqPubNoAuth := httptest.NewRequest("GET", "/api/custom/fleet-info", nil)
	wPubNoAuth := httptest.NewRecorder()
	srv.router.ServeHTTP(wPubNoAuth, reqPubNoAuth)
	if wPubNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", wPubNoAuth.Code)
	}
	if wPubNoAuth.Header().Get("WWW-Authenticate") == "" {
		t.Errorf("expected WWW-Authenticate header")
	}

	// 3. Test Public Execution with Invalid Token (Should fail with 401)
	reqPubBadAuth := httptest.NewRequest("GET", "/api/custom/fleet-info", nil)
	reqPubBadAuth.Header.Set("Authorization", "Bearer invalid-token")
	wPubBadAuth := httptest.NewRecorder()
	srv.router.ServeHTTP(wPubBadAuth, reqPubBadAuth)
	if wPubBadAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for bad token, got %d", wPubBadAuth.Code)
	}

	// 4. Test Public Execution with Valid Bearer Token in Header
	reqPubValid := httptest.NewRequest("GET", "/api/custom/fleet-info?agent=prometheus", nil)
	reqPubValid.Header.Set("Authorization", "Bearer secret-bearer-xyz")
	wPubValid := httptest.NewRecorder()
	srv.router.ServeHTTP(wPubValid, reqPubValid)
	if wPubValid.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", wPubValid.Code, wPubValid.Body.String())
	}

	if wPubValid.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", wPubValid.Header().Get("Content-Type"))
	}
	if wPubValid.Header().Get("X-Easy42-Cluster") != "production" {
		t.Errorf("expected extra header X-Easy42-Cluster, got %s", wPubValid.Header().Get("X-Easy42-Cluster"))
	}
	if wPubValid.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected Access-Control-Allow-Origin header, got %s", wPubValid.Header().Get("Access-Control-Allow-Origin"))
	}

	var pubResp map[string]any
	if err := json.Unmarshal(wPubValid.Body.Bytes(), &pubResp); err != nil {
		t.Fatalf("failed to decode response JSON: %v, raw: %s", err, wPubValid.Body.String())
	}
	if pubResp["count"] != float64(2) {
		t.Errorf("expected count 2, got %v", pubResp["count"])
	}
	if pubResp["query_agent"] != "prometheus" {
		t.Errorf("expected query_agent 'prometheus', got %v", pubResp["query_agent"])
	}

	// 5. Test Public Execution with Token in Query Param ?token=
	reqPubTokenParam := httptest.NewRequest("GET", "/api/custom/fleet-info?token=secret-bearer-xyz&agent=grafana", nil)
	wPubTokenParam := httptest.NewRecorder()
	srv.router.ServeHTTP(wPubTokenParam, reqPubTokenParam)
	if wPubTokenParam.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with query token, got %d: %s", wPubTokenParam.Code, wPubTokenParam.Body.String())
	}

	// 6. Test Public Execution on Public API (No token required)
	noTokenAPI := config.CustomAPI{
		ID:          "public-ping",
		ContentType: "text/plain",
		Template:    "PONG from {{ .request.host }}",
	}
	bodyNT, _ := json.Marshal(noTokenAPI)
	reqNT := httptest.NewRequest("POST", "/api/custom-apis", bytes.NewReader(bodyNT))
	reqNT.Header.Set("Authorization", "Bearer "+adminToken)
	reqNT.Header.Set("Content-Type", "application/json")
	wNT := httptest.NewRecorder()
	srv.router.ServeHTTP(wNT, reqNT)
	if wNT.Code != http.StatusCreated {
		t.Fatalf("failed to create public API: %d %s", wNT.Code, wNT.Body.String())
	}

	reqPing := httptest.NewRequest("GET", "/api/custom/public-ping", nil)
	reqPing.Host = "cluster.easy42.internal"
	wPing := httptest.NewRecorder()
	srv.router.ServeHTTP(wPing, reqPing)
	if wPing.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for public API, got %d", wPing.Code)
	}
	if strings.TrimSpace(wPing.Body.String()) != "PONG from cluster.easy42.internal" {
		t.Errorf("expected 'PONG from cluster.easy42.internal', got %q", wPing.Body.String())
	}

	// 7. Test Admin List
	reqList := httptest.NewRequest("GET", "/api/custom-apis", nil)
	reqList.Header.Set("Authorization", "Bearer "+adminToken)
	wList := httptest.NewRecorder()
	srv.router.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("failed to list APIs: %d", wList.Code)
	}
	var apiList []config.CustomAPI
	_ = json.Unmarshal(wList.Body.Bytes(), &apiList)
	if len(apiList) != 2 {
		t.Fatalf("expected 2 APIs, got %d", len(apiList))
	}

	// 8. Test Admin Get by ID
	reqGet := httptest.NewRequest("GET", "/api/custom-apis/fleet-info", nil)
	reqGet.Header.Set("Authorization", "Bearer "+adminToken)
	wGet := httptest.NewRecorder()
	srv.router.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("failed to get API: %d", wGet.Code)
	}

	// 9. Test Admin Update
	created.Description = "Updated description via PUT"
	created.Disabled = true
	bodyUp, _ := json.Marshal(created)
	reqUp := httptest.NewRequest("PUT", "/api/custom-apis/fleet-info", bytes.NewReader(bodyUp))
	reqUp.Header.Set("Authorization", "Bearer "+adminToken)
	reqUp.Header.Set("Content-Type", "application/json")
	wUp := httptest.NewRecorder()
	srv.router.ServeHTTP(wUp, reqUp)
	if wUp.Code != http.StatusOK {
		t.Fatalf("failed to update API: %d", wUp.Code)
	}

	// 9b. Verify calling disabled API returns 403 Forbidden
	reqDisabledCall := httptest.NewRequest("GET", "/api/custom/fleet-info", nil)
	reqDisabledCall.Header.Set("Authorization", "Bearer secret-bearer-xyz")
	wDisabledCall := httptest.NewRecorder()
	srv.router.ServeHTTP(wDisabledCall, reqDisabledCall)
	if wDisabledCall.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for disabled API, got %d: %s", wDisabledCall.Code, wDisabledCall.Body.String())
	}
	if !strings.Contains(wDisabledCall.Body.String(), "is disabled") {
		t.Errorf("expected body to mention 'is disabled', got %s", wDisabledCall.Body.String())
	}

	// 9c. Re-enable API
	created.Disabled = false
	bodyReEnable, _ := json.Marshal(created)
	reqReEnable := httptest.NewRequest("PUT", "/api/custom-apis/fleet-info", bytes.NewReader(bodyReEnable))
	reqReEnable.Header.Set("Authorization", "Bearer "+adminToken)
	reqReEnable.Header.Set("Content-Type", "application/json")
	wReEnable := httptest.NewRecorder()
	srv.router.ServeHTTP(wReEnable, reqReEnable)
	if wReEnable.Code != http.StatusOK {
		t.Fatalf("failed to re-enable API: %d", wReEnable.Code)
	}

	// Verify calling enabled API succeeds again
	reqEnabledCall := httptest.NewRequest("GET", "/api/custom/fleet-info", nil)
	reqEnabledCall.Header.Set("Authorization", "Bearer secret-bearer-xyz")
	wEnabledCall := httptest.NewRecorder()
	srv.router.ServeHTTP(wEnabledCall, reqEnabledCall)
	if wEnabledCall.Code != http.StatusOK {
		t.Fatalf("expected 200 OK after re-enabling API, got %d", wEnabledCall.Code)
	}

	// 10. Test Admin Delete
	reqDel := httptest.NewRequest("DELETE", "/api/custom-apis/fleet-info", nil)
	reqDel.Header.Set("Authorization", "Bearer "+adminToken)
	wDel := httptest.NewRecorder()
	srv.router.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("failed to delete API: %d", wDel.Code)
	}

	// 11. Test Custom API Test endpoint
	testPayload := TestCustomAPIPayload{
		API: config.CustomAPI{
			ID:          "preview-test",
			ContentType: "application/json",
			Template:    `{"received_path": "{{ .request.path }}", "total": {{ len .config.nodes }}}`,
		},
		Request: &engine.CustomAPITestRequestPayload{
			Method: "GET",
			Path:   "/custom/preview-test",
		},
	}
	testBytes, _ := json.Marshal(testPayload)
	reqTest := httptest.NewRequest("POST", "/api/custom-apis/test", bytes.NewReader(testBytes))
	reqTest.Header.Set("Authorization", "Bearer "+adminToken)
	reqTest.Header.Set("Content-Type", "application/json")
	wTest := httptest.NewRecorder()
	srv.router.ServeHTTP(wTest, reqTest)
	if wTest.Code != http.StatusOK {
		t.Fatalf("failed to run test API: %d: %s", wTest.Code, wTest.Body.String())
	}
	var testResp TestCustomAPIResponse
	if err := json.Unmarshal(wTest.Body.Bytes(), &testResp); err != nil {
		t.Fatalf("failed to decode test response: %v", err)
	}
	if !strings.Contains(testResp.Output, `"total": 2`) {
		t.Errorf("expected total: 2 in test output, got %s", testResp.Output)
	}
}
