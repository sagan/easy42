package engine

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"easy42/internal/config"
)

func setupTestManagerForAPI(t *testing.T) (*Manager, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "easy42-customapi-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfgPath := filepath.Join(tmpDir, "config.json")
	store := config.NewStore(cfgPath)
	rawPass, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to init config: %v", err)
	}

	cfg := store.Get()
	cfg.Nodes = []config.Node{
		{Name: "node1", Host: "10.0.0.1", ASN: 4200000001},
		{Name: "node2", Host: "10.0.0.2", ASN: 4200000002},
	}
	_ = store.Save(cfg)

	mgr := NewManager(store)
	if err := mgr.Unlock(rawPass); err != nil {
		t.Fatalf("failed to unlock: %v (pass=%s)", err, rawPass)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}
	return mgr, cleanup
}

func TestCustomAPILifecycle(t *testing.T) {
	mgr, cleanup := setupTestManagerForAPI(t)
	defer cleanup()

	// Initially empty
	apis := mgr.GetCustomAPIs()
	if len(apis) != 0 {
		t.Fatalf("expected 0 APIs, got %d", len(apis))
	}

	// Create
	newAPI := config.CustomAPI{
		ID:          "cluster-nodes",
		Name:        "Cluster Nodes",
		Description: "Returns all cluster nodes",
		ContentType: "application/json",
		Template:    `{"count": {{ len .config.nodes }}, "nodes": {{ .config.nodes | toJson }}}`,
		ExtraHeaders: map[string]string{
			"X-Custom-Cluster": "easy42",
		},
	}

	created, err := mgr.CreateCustomAPI(newAPI)
	if err != nil {
		t.Fatalf("failed to create API: %v", err)
	}
	if created.ID != "cluster-nodes" {
		t.Fatalf("expected ID 'cluster-nodes', got %s", created.ID)
	}

	// Duplicate create should fail
	_, err = mgr.CreateCustomAPI(newAPI)
	if err == nil {
		t.Fatalf("expected error creating duplicate API, got nil")
	}

	// Get
	fetched, err := mgr.GetCustomAPI("cluster-nodes")
	if err != nil {
		t.Fatalf("failed to get API: %v", err)
	}
	if fetched.Name != "Cluster Nodes" {
		t.Fatalf("expected name 'Cluster Nodes', got %s", fetched.Name)
	}

	// Update
	fetched.Description = "Updated description"
	fetched.Template = `{{ .config.nodes | toJson }}`
	updated, err := mgr.UpdateCustomAPI("cluster-nodes", *fetched)
	if err != nil {
		t.Fatalf("failed to update API: %v", err)
	}
	if updated.Description != "Updated description" {
		t.Fatalf("expected updated description, got %s", updated.Description)
	}

	// List
	list := mgr.GetCustomAPIs()
	if len(list) != 1 {
		t.Fatalf("expected 1 API in list, got %d", len(list))
	}

	// Delete
	if err := mgr.DeleteCustomAPI("cluster-nodes"); err != nil {
		t.Fatalf("failed to delete API: %v", err)
	}
	if len(mgr.GetCustomAPIs()) != 0 {
		t.Fatalf("expected 0 APIs after delete")
	}
}

func TestExecuteCustomAPIAndContext(t *testing.T) {
	mgr, cleanup := setupTestManagerForAPI(t)
	defer cleanup()

	// Custom API that tests config, state, and request object in template
	api := &config.CustomAPI{
		ID:          "test-eval",
		ContentType: "application/json",
		Template: `{
  "method": "{{ .request.method }}",
  "query_filter": "{{ .request.query.filter }}",
  "auth_header": "{{ .request.headers.authorization }}",
  "body_msg": "{{ .request.body.message }}",
  "first_node": "{{ (index .config.nodes 0).name }}",
  "total_nodes": {{ len .config.nodes }}
}`,
	}

	reqBody := `{"message": "hello programmable api", "value": 42}`
	req := httptest.NewRequest("POST", "/api/custom/test-eval?filter=active", strings.NewReader(reqBody))
	req.Header.Set("Authorization", "Bearer testtoken123")
	req.Header.Set("Content-Type", "application/json")

	output, err := mgr.ExecuteCustomAPI(api, req)
	if err != nil {
		t.Fatalf("ExecuteCustomAPI failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("failed to parse template output JSON: %v\nOutput: %s", err, output)
	}

	if parsed["method"] != "POST" {
		t.Errorf("expected method POST, got %v", parsed["method"])
	}
	if parsed["query_filter"] != "active" {
		t.Errorf("expected query_filter active, got %v", parsed["query_filter"])
	}
	if parsed["auth_header"] != "Bearer testtoken123" {
		t.Errorf("expected auth_header Bearer testtoken123, got %v", parsed["auth_header"])
	}
	if parsed["body_msg"] != "hello programmable api" {
		t.Errorf("expected body_msg hello programmable api, got %v", parsed["body_msg"])
	}
	if parsed["first_node"] != "node1" {
		t.Errorf("expected first_node node1, got %v", parsed["first_node"])
	}
	if parsed["total_nodes"] != float64(2) {
		t.Errorf("expected total_nodes 2, got %v", parsed["total_nodes"])
	}

	// Verify executing disabled API fails
	api.Disabled = true
	_, err = mgr.ExecuteCustomAPI(api, req)
	if err == nil {
		t.Fatal("expected error executing disabled custom API, got nil")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("expected error to mention 'disabled', got: %v", err)
	}
}

func TestTestCustomAPI(t *testing.T) {
	mgr, cleanup := setupTestManagerForAPI(t)
	defer cleanup()

	api := config.CustomAPI{
		ID:          "mock-api",
		ContentType: "text/plain",
		Template:    `Hello {{ .request.query.name | default "World" }}! Total: {{ len .config.nodes }}`,
	}

	// Test with query
	output, err := mgr.TestCustomAPI(api, &CustomAPITestRequestPayload{
		Method: "GET",
		Query: map[string]string{
			"name": "Easy42Admin",
		},
	})
	if err != nil {
		t.Fatalf("TestCustomAPI error: %v", err)
	}

	expected := "Hello Easy42Admin! Total: 2"
	if output != expected {
		t.Fatalf("expected %q, got %q", expected, output)
	}
}
