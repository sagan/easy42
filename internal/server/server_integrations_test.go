package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"easy42/internal/config"
	"easy42/internal/engine"
)

func setupTestServerWithIntegrations(t *testing.T) (*Server, *engine.Manager, func()) {
	tmpDir, err := os.MkdirTemp("", "e42-srv-int-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	store := config.NewStore(tmpDir)
	_, err = store.Initialize()
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to initialize store: %v", err)
	}

	mgr := engine.NewManager(store)
	srv := New(Config{
		Manager: mgr,
	})

	cleanup := func() {
		mgr.Close()
		os.RemoveAll(tmpDir)
	}

	return srv, mgr, cleanup
}

func TestPrometheusMetricsEndpointAuth(t *testing.T) {
	srv, mgr, cleanup := setupTestServerWithIntegrations(t)
	defer cleanup()

	settings := mgr.GetIntegrationsSettings()
	validToken := settings.PrometheusAuthToken
	if validToken == "" {
		t.Fatal("expected non-empty auth token")
	}

	// 1. Request without auth -> 401
	req1 := httptest.NewRequest("GET", "/metrics", nil)
	rr1 := httptest.NewRecorder()
	srv.router.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr1.Code)
	}
	if !strings.Contains(rr1.Header().Get("WWW-Authenticate"), "Bearer") {
		t.Errorf("expected WWW-Authenticate header")
	}

	// 2. Request with invalid token -> 401
	req2 := httptest.NewRequest("GET", "/metrics", nil)
	req2.Header.Set("Authorization", "Bearer invalid_token_123")
	rr2 := httptest.NewRecorder()
	srv.router.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", rr2.Code)
	}

	// 3. Request with valid Bearer token -> 200
	req3 := httptest.NewRequest("GET", "/metrics", nil)
	req3.Header.Set("Authorization", "Bearer "+validToken)
	rr3 := httptest.NewRecorder()
	srv.router.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr3.Code, rr3.Body.String())
	}
	ct := rr3.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") || !strings.Contains(ct, "version=0.0.4") {
		t.Errorf("expected Prometheus text exposition content-type, got: %s", ct)
	}
	if !strings.Contains(rr3.Body.String(), "easy42_exporter_build_info") {
		t.Errorf("expected metrics body to contain easy42_exporter_build_info")
	}

	// 4. Request with query param ?token= -> 200
	req4 := httptest.NewRequest("GET", "/metrics?token="+validToken, nil)
	rr4 := httptest.NewRecorder()
	srv.router.ServeHTTP(rr4, req4)
	if rr4.Code != http.StatusOK {
		t.Fatalf("expected 200 with query token, got %d", rr4.Code)
	}

	// 5. Reset token via API
	req5 := httptest.NewRequest("POST", "/settings/integrations/reset-token", nil)
	rr5 := httptest.NewRecorder()
	srv.handleResetPrometheusToken(rr5, req5)
	if rr5.Code != http.StatusOK {
		t.Fatalf("failed to reset token: %d", rr5.Code)
	}
	var resetResp integrationsResponse
	if err := json.Unmarshal(rr5.Body.Bytes(), &resetResp); err != nil {
		t.Fatalf("failed to decode reset response: %v", err)
	}
	if resetResp.PrometheusAuthToken == validToken {
		t.Errorf("token was not changed after reset")
	}

	// Old token should now be rejected
	req6 := httptest.NewRequest("GET", "/metrics", nil)
	req6.Header.Set("Authorization", "Bearer "+validToken)
	rr6 := httptest.NewRecorder()
	srv.router.ServeHTTP(rr6, req6)
	if rr6.Code != http.StatusUnauthorized {
		t.Fatalf("expected old token to be rejected with 401, got %d", rr6.Code)
	}

	// New token should be accepted
	req7 := httptest.NewRequest("GET", "/metrics", nil)
	req7.Header.Set("Authorization", "Bearer "+resetResp.PrometheusAuthToken)
	rr7 := httptest.NewRecorder()
	srv.router.ServeHTTP(rr7, req7)
	if rr7.Code != http.StatusOK {
		t.Fatalf("expected new token to be accepted with 200, got %d", rr7.Code)
	}

	// 6. Disable Prometheus
	disabled := false
	updatePayload, _ := json.Marshal(map[string]interface{}{
		"prometheus_enabled": &disabled,
	})
	req8 := httptest.NewRequest("PUT", "/settings/integrations", strings.NewReader(string(updatePayload)))
	rr8 := httptest.NewRecorder()
	srv.handleUpdateIntegrationsSettings(rr8, req8)
	if rr8.Code != http.StatusOK {
		t.Fatalf("failed to update settings: %d", rr8.Code)
	}

	// Scrape when disabled should return 403 Forbidden
	req9 := httptest.NewRequest("GET", "/metrics", nil)
	req9.Header.Set("Authorization", "Bearer "+resetResp.PrometheusAuthToken)
	rr9 := httptest.NewRecorder()
	srv.router.ServeHTTP(rr9, req9)
	if rr9.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when disabled, got %d", rr9.Code)
	}
}

func TestTelegramSettingsAndEndpoints(t *testing.T) {
	srv, _, cleanup := setupTestServerWithIntegrations(t)
	defer cleanup()

	// 1. Initial GET /settings/integrations should have telegram_enabled: false
	reqGet := httptest.NewRequest("GET", "/settings/integrations", nil)
	rrGet := httptest.NewRecorder()
	srv.handleGetIntegrationsSettings(rrGet, reqGet)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rrGet.Code)
	}
	var getResp integrationsResponse
	if err := json.Unmarshal(rrGet.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("failed to parse get response: %v", err)
	}
	if getResp.TelegramEnabled {
		t.Errorf("expected telegram_enabled to be false by default")
	}

	// 2. PUT /settings/integrations to configure Telegram
	tgEnabled := true
	tgToken := "123456789:TEST_MOCK_TOKEN"
	tgAdminID := int64(987654321)
	serverURL := "https://easy42.example.com"

	updatePayload, _ := json.Marshal(map[string]interface{}{
		"telegram_enabled":   &tgEnabled,
		"telegram_bot_token": &tgToken,
		"telegram_admin_id":  tgAdminID,
		"server_url":         &serverURL,
	})
	reqPut := httptest.NewRequest("PUT", "/settings/integrations", strings.NewReader(string(updatePayload)))
	rrPut := httptest.NewRecorder()
	srv.handleUpdateIntegrationsSettings(rrPut, reqPut)
	if rrPut.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rrPut.Code, rrPut.Body.String())
	}

	var putResp integrationsResponse
	if err := json.Unmarshal(rrPut.Body.Bytes(), &putResp); err != nil {
		t.Fatalf("failed to parse put response: %v", err)
	}
	if !putResp.TelegramEnabled {
		t.Errorf("expected TelegramEnabled to be true")
	}
	if putResp.TelegramBotToken != tgToken {
		t.Errorf("expected bot token %s, got %s", tgToken, putResp.TelegramBotToken)
	}
	if putResp.TelegramAdminID != tgAdminID {
		t.Errorf("expected admin ID %d, got %d", tgAdminID, putResp.TelegramAdminID)
	}
	if putResp.ServerURL != serverURL {
		t.Errorf("expected server URL %s, got %s", serverURL, putResp.ServerURL)
	}
	if putResp.TelegramSecretToken == "" || !strings.HasPrefix(putResp.TelegramSecretToken, "e42_tg_") {
		t.Fatalf("expected generated telegram secret token starting with e42_tg_, got %q", putResp.TelegramSecretToken)
	}

	// 3. Test Telegram Webhook public endpoint POST /api/telegram/webhook
	webhookPayload := `{
		"update_id": 1001,
		"message": {
			"message_id": 1,
			"from": { "id": 111222, "is_bot": false, "first_name": "TestUser" },
			"chat": { "id": 111222, "type": "private" },
			"date": 1600000000,
			"text": "/status"
		}
	}`

	// 3a. Request without secret token header -> 401 Unauthorized
	reqWhNoHeader := httptest.NewRequest("POST", "/api/telegram/webhook", strings.NewReader(webhookPayload))
	rrWhNoHeader := httptest.NewRecorder()
	srv.router.ServeHTTP(rrWhNoHeader, reqWhNoHeader)
	if rrWhNoHeader.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without secret token header, got %d", rrWhNoHeader.Code)
	}

	// 3b. Request with wrong secret token header -> 401 Unauthorized
	reqWhWrongHeader := httptest.NewRequest("POST", "/api/telegram/webhook", strings.NewReader(webhookPayload))
	reqWhWrongHeader.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong-secret-token")
	rrWhWrongHeader := httptest.NewRecorder()
	srv.router.ServeHTTP(rrWhWrongHeader, reqWhWrongHeader)
	if rrWhWrongHeader.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized with wrong secret token header, got %d", rrWhWrongHeader.Code)
	}

	// 3c. Request with correct secret token header -> 200 OK
	reqWhValid := httptest.NewRequest("POST", "/api/telegram/webhook", strings.NewReader(webhookPayload))
	reqWhValid.Header.Set("X-Telegram-Bot-Api-Secret-Token", putResp.TelegramSecretToken)
	rrWhValid := httptest.NewRecorder()
	srv.router.ServeHTTP(rrWhValid, reqWhValid)
	if rrWhValid.Code != http.StatusOK {
		t.Fatalf("expected webhook endpoint to return 200 OK with valid secret token, got %d: %s", rrWhValid.Code, rrWhValid.Body.String())
	}
	var whResp map[string]interface{}
	if err := json.Unmarshal(rrWhValid.Body.Bytes(), &whResp); err != nil {
		t.Fatalf("failed to parse webhook response: %v", err)
	}
	if whResp["ok"] != true {
		t.Errorf("expected ok: true in webhook response, got %v", whResp["ok"])
	}

	// 4. Test Telegram Test Connection with missing/invalid credentials -> should return 400
	testReqBad := httptest.NewRequest("POST", "/settings/integrations/telegram/test", strings.NewReader(`{"telegram_bot_token":"invalid","telegram_admin_id":0}`))
	rrTestBad := httptest.NewRecorder()
	srv.handleTestTelegramIntegration(rrTestBad, testReqBad)
	if rrTestBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid token/admin id, got %d", rrTestBad.Code)
	}
}

