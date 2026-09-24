package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"easy42/internal/config"
)

func TestTemplatesAPI(t *testing.T) {
	srv, tempDir, initPass := setupTestServer(t)
	defer os.RemoveAll(tempDir)

	// Login to get auth cookie
	loginBody, _ := json.Marshal(map[string]string{"password": initPass})
	reqLogin := httptest.NewRequest("POST", "/api/auth/login", bytes.NewReader(loginBody))
	wLogin := httptest.NewRecorder()
	srv.router.ServeHTTP(wLogin, reqLogin)
	if wLogin.Code != http.StatusOK {
		t.Fatalf("Login failed: %d %s", wLogin.Code, wLogin.Body.String())
	}
	cookie := wLogin.Result().Cookies()[0]

	// 1. GET /api/templates should return built-in templates
	reqGet := httptest.NewRequest("GET", "/api/templates", nil)
	reqGet.AddCookie(cookie)
	wGet := httptest.NewRecorder()
	srv.router.ServeHTTP(wGet, reqGet)
	if wGet.Code != http.StatusOK {
		t.Fatalf("GET /api/templates failed: %d %s", wGet.Code, wGet.Body.String())
	}

	var list []config.ConfigTemplate
	if err := json.Unmarshal(wGet.Body.Bytes(), &list); err != nil {
		t.Fatalf("Failed to decode templates list: %v", err)
	}
	if len(list) < 3 {
		t.Fatalf("Expected at least 3 templates, got %d", len(list))
	}

	// 2. POST /api/templates - invalid syntax should return 400
	badPayload, _ := json.Marshal(config.ConfigTemplate{
		ID:      "bad",
		Name:    "Bad",
		Type:    config.TemplateTypeBird,
		Content: "router id {{ .unclosed",
	})
	reqBad := httptest.NewRequest("POST", "/api/templates", bytes.NewReader(badPayload))
	reqBad.AddCookie(cookie)
	wBad := httptest.NewRecorder()
	srv.router.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for bad template, got %d", wBad.Code)
	}

	// 3. POST /api/templates - create valid custom template
	validPayload, _ := json.Marshal(config.ConfigTemplate{
		ID:          "custom-bird-1",
		Name:        "Custom BIRD 1",
		Type:        config.TemplateTypeBird,
		Description: "Testing BIRD",
		Content:     "/* TEST */\nrouter id {{ .ip }};\n",
	})
	reqCreate := httptest.NewRequest("POST", "/api/templates", bytes.NewReader(validPayload))
	reqCreate.AddCookie(cookie)
	wCreate := httptest.NewRecorder()
	srv.router.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusCreated {
		t.Fatalf("Expected 201 for create template, got %d %s", wCreate.Code, wCreate.Body.String())
	}

	var created config.ConfigTemplate
	_ = json.Unmarshal(wCreate.Body.Bytes(), &created)
	if created.ID != "custom-bird-1" || created.IsBuiltin {
		t.Errorf("Unexpected created template: %+v", created)
	}

	// 4. GET /api/templates/{id}
	reqGetOne := httptest.NewRequest("GET", "/api/templates/custom-bird-1", nil)
	reqGetOne.AddCookie(cookie)
	wGetOne := httptest.NewRecorder()
	srv.router.ServeHTTP(wGetOne, reqGetOne)
	if wGetOne.Code != http.StatusOK {
		t.Fatalf("Expected 200 for get template, got %d", wGetOne.Code)
	}

	// 5. PUT /api/templates/{id}
	updatePayload, _ := json.Marshal(config.ConfigTemplate{
		Name:    "Custom BIRD 1 (Updated)",
		Type:    config.TemplateTypeBird,
		Content: "/* UPDATED */\nrouter id {{ .ip }};\n",
	})
	reqUpdate := httptest.NewRequest("PUT", "/api/templates/custom-bird-1", bytes.NewReader(updatePayload))
	reqUpdate.AddCookie(cookie)
	wUpdate := httptest.NewRecorder()
	srv.router.ServeHTTP(wUpdate, reqUpdate)
	if wUpdate.Code != http.StatusOK {
		t.Fatalf("Expected 200 for update template, got %d %s", wUpdate.Code, wUpdate.Body.String())
	}

	// 6. DELETE /api/templates/{id}
	reqDelete := httptest.NewRequest("DELETE", "/api/templates/custom-bird-1", nil)
	reqDelete.AddCookie(cookie)
	wDelete := httptest.NewRecorder()
	srv.router.ServeHTTP(wDelete, reqDelete)
	if wDelete.Code != http.StatusOK {
		t.Fatalf("Expected 200 for delete template, got %d %s", wDelete.Code, wDelete.Body.String())
	}

	// 7. Cannot delete built-in template
	reqDeleteBuiltin := httptest.NewRequest("DELETE", "/api/templates/default_bird", nil)
	reqDeleteBuiltin.AddCookie(cookie)
	wDeleteBuiltin := httptest.NewRecorder()
	srv.router.ServeHTTP(wDeleteBuiltin, reqDeleteBuiltin)
	if wDeleteBuiltin.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 when deleting built-in template, got %d", wDeleteBuiltin.Code)
	}
}
