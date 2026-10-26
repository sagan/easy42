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
	"easy42/internal/crypto"
	"easy42/internal/engine"
)

func setupApplyTestServer(t *testing.T) (*Server, string, string) {
	tmpDir, err := os.MkdirTemp("", "easy42-server-apply-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	store := config.NewStore(tmpDir)
	_, err = store.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	cfg, _ := store.Load()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name: "foobar",
		Host: "foobar.test",
		IP:   "192.168.142.202",
		ASN:  4224420202,
	})
	_ = store.Save(cfg)

	mgr := engine.NewManager(store)
	srv := New(Config{
		Manager: mgr,
	})

	sessionToken := crypto.CreateSessionToken(cfg.SessionSecret, cfg.PasswordHash, 15*time.Minute)
	return srv, tmpDir, sessionToken
}

func TestServer_ConfigApplySchema(t *testing.T) {
	srv, tmpDir, token := setupApplyTestServer(t)
	defer os.RemoveAll(tmpDir)
	defer srv.mgr.Close()

	req := httptest.NewRequest("GET", "/api/config/apply/schema", nil)
	req.AddCookie(&http.Cookie{Name: "easy42_session", Value: token})
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &schema); err != nil {
		t.Fatalf("failed to parse schema JSON: %v", err)
	}
	if schema["title"] != "Easy42ConfigChange" {
		t.Errorf("expected title 'Easy42ConfigChange', got %v", schema["title"])
	}
}

func TestServer_ConfigApplyValidate(t *testing.T) {
	srv, tmpDir, token := setupApplyTestServer(t)
	defer os.RemoveAll(tmpDir)
	defer srv.mgr.Close()

	// Valid snippet
	body, _ := json.Marshal(map[string]string{
		"content": `{"nodes": [{"name": "foobar", "static_routes": ["1.1.1.1/32"]}]}`,
	})
	req := httptest.NewRequest("POST", "/api/config/apply/validate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "easy42_session", Value: token})
	w := httptest.NewRecorder()

	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var res struct {
		Valid bool `json:"valid"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if !res.Valid {
		t.Errorf("expected snippet to be valid")
	}

	// Invalid snippet with unknown field
	invalidBody, _ := json.Marshal(map[string]string{
		"content": `{"nodes": [{"name": "foobar", "bad_unknown_field": 123}]}`,
	})
	req2 := httptest.NewRequest("POST", "/api/config/apply/validate", bytes.NewReader(invalidBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(&http.Cookie{Name: "easy42_session", Value: token})
	w2 := httptest.NewRecorder()

	srv.router.ServeHTTP(w2, req2)
	var res2 struct {
		Valid bool   `json:"valid"`
		Error string `json:"error"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &res2)
	if res2.Valid {
		t.Errorf("expected invalid snippet to fail validation")
	}
}

func TestServer_ConfigApplyPreviewAndExecute(t *testing.T) {
	srv, tmpDir, token := setupApplyTestServer(t)
	defer os.RemoveAll(tmpDir)
	defer srv.mgr.Close()

	snippet := `{
		"nodes": [
			{
				"name": "foobar",
				"static_routes": ["10.0.0.0/8"]
			}
		]
	}`

	// 1. Preview
	body, _ := json.Marshal(map[string]string{
		"content": snippet,
	})
	reqPreview := httptest.NewRequest("POST", "/api/config/apply/preview", bytes.NewReader(body))
	reqPreview.Header.Set("Content-Type", "application/json")
	reqPreview.AddCookie(&http.Cookie{Name: "easy42_session", Value: token})
	wPreview := httptest.NewRecorder()

	srv.router.ServeHTTP(wPreview, reqPreview)
	if wPreview.Code != http.StatusOK {
		t.Fatalf("preview returned %d: %s", wPreview.Code, wPreview.Body.String())
	}

	// 2. Execute Apply
	reqApply := httptest.NewRequest("POST", "/api/config/apply", bytes.NewReader(body))
	reqApply.Header.Set("Content-Type", "application/json")
	reqApply.AddCookie(&http.Cookie{Name: "easy42_session", Value: token})
	wApply := httptest.NewRecorder()

	srv.router.ServeHTTP(wApply, reqApply)
	if wApply.Code != http.StatusOK {
		t.Fatalf("apply returned %d: %s", wApply.Code, wApply.Body.String())
	}

	node, err := srv.mgr.GetNode("foobar")
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	if len(node.StaticRoutes) != 1 || node.StaticRoutes[0] != "10.0.0.0/8" {
		t.Errorf("static routes not updated: %v", node.StaticRoutes)
	}
}

func TestServer_ConfigChangesCRUD(t *testing.T) {
	srv, tmpDir, token := setupApplyTestServer(t)
	defer os.RemoveAll(tmpDir)
	defer srv.mgr.Close()

	// 1. Create saved change
	createBody, _ := json.Marshal(map[string]string{
		"name":    "Preset 1",
		"content": `{"nodes": [{"name": "foobar", "tags": ["core", "edge"]}]}`,
	})
	reqCreate := httptest.NewRequest("POST", "/api/config/changes", bytes.NewReader(createBody))
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.AddCookie(&http.Cookie{Name: "easy42_session", Value: token})
	wCreate := httptest.NewRecorder()

	srv.router.ServeHTTP(wCreate, reqCreate)
	if wCreate.Code != http.StatusOK {
		t.Fatalf("create failed (%d): %s", wCreate.Code, wCreate.Body.String())
	}

	var saved config.ConfigChange
	_ = json.Unmarshal(wCreate.Body.Bytes(), &saved)
	if saved.ID == "" || saved.Name != "Preset 1" {
		t.Fatalf("unexpected saved snippet: %+v", saved)
	}

	// 1b. Create saved change with same name (should overwrite)
	createBody2, _ := json.Marshal(map[string]string{
		"name":    "preset 1",
		"content": `{"nodes": [{"name": "foobar", "tags": ["edge"]}]}`,
	})
	reqCreate2 := httptest.NewRequest("POST", "/api/config/changes", bytes.NewReader(createBody2))
	reqCreate2.Header.Set("Content-Type", "application/json")
	reqCreate2.AddCookie(&http.Cookie{Name: "easy42_session", Value: token})
	wCreate2 := httptest.NewRecorder()

	srv.router.ServeHTTP(wCreate2, reqCreate2)
	if wCreate2.Code != http.StatusOK {
		t.Fatalf("create overwrite failed (%d): %s", wCreate2.Code, wCreate2.Body.String())
	}
	var saved2 config.ConfigChange
	_ = json.Unmarshal(wCreate2.Body.Bytes(), &saved2)
	if saved2.ID != saved.ID {
		t.Fatalf("expected overwritten snippet ID %s, got %s", saved.ID, saved2.ID)
	}

	// 2. List changes (still exactly 1)
	reqList := httptest.NewRequest("GET", "/api/config/changes", nil)
	reqList.AddCookie(&http.Cookie{Name: "easy42_session", Value: token})
	wList := httptest.NewRecorder()

	srv.router.ServeHTTP(wList, reqList)
	if wList.Code != http.StatusOK {
		t.Fatalf("list failed (%d)", wList.Code)
	}

	var list []config.ConfigChange
	_ = json.Unmarshal(wList.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 change, got %d", len(list))
	}

	// 3. Apply saved change
	reqApply := httptest.NewRequest("POST", "/api/config/changes/"+saved.ID+"/apply", bytes.NewReader([]byte("{}")))
	reqApply.Header.Set("Content-Type", "application/json")
	reqApply.AddCookie(&http.Cookie{Name: "easy42_session", Value: token})
	wApply := httptest.NewRecorder()

	srv.router.ServeHTTP(wApply, reqApply)
	if wApply.Code != http.StatusOK {
		t.Fatalf("apply saved failed (%d): %s", wApply.Code, wApply.Body.String())
	}

	node, _ := srv.mgr.GetNode("foobar")
	if len(node.Tags) != 1 || node.Tags[0] != "edge" {
		t.Errorf("tags not updated to edge: %v", node.Tags)
	}

	// 4. Delete saved change
	reqDel := httptest.NewRequest("DELETE", "/api/config/changes/"+saved.ID, nil)
	reqDel.AddCookie(&http.Cookie{Name: "easy42_session", Value: token})
	wDel := httptest.NewRecorder()

	srv.router.ServeHTTP(wDel, reqDel)
	if wDel.Code != http.StatusOK {
		t.Fatalf("delete failed (%d): %s", wDel.Code, wDel.Body.String())
	}
}
