package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"easy42/internal/config"
	"easy42/internal/engine"
)

func setupTestServerWithSSHKeys(t *testing.T) (*Server, *engine.Manager, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "easy42-ssh-keys-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfgStore := config.NewStore(tmpDir)
	cfg := &config.Config{
		PasswordHash:  "hash",
		SessionSecret: "secret",
		Nodes: []config.Node{
			{Name: "node1", Host: "10.0.0.1", Mode: "agent", AgentToken: "t1", AgentVersion: "0.1.17"},
			{Name: "node2", Host: "10.0.0.2", Mode: "agent", AgentToken: "t2", AgentVersion: "0.1.18"},
			{Name: "node3", Host: "10.0.0.3", Mode: "ssh"},
		},
	}
	if err := cfgStore.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	mgr := engine.NewManager(cfgStore)
	srv := New(Config{
		Manager:    mgr,
		ListenAddr: "127.0.0.1:0",
	})

	cleanup := func() {
		mgr.Close()
		_ = os.RemoveAll(tmpDir)
	}
	return srv, mgr, cleanup
}

func TestSSHKeysAPI(t *testing.T) {
	srv, _, cleanup := setupTestServerWithSSHKeys(t)
	defer cleanup()

	// 1. Overview is empty but the nodes are reported with a support reason.
	req := httptest.NewRequest("GET", "/api/ssh-keys", nil)
	w := httptest.NewRecorder()
	srv.handleGetSSHKeys(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// The UI iterates over node keys unconditionally, so the payload must never
	// encode a nil slice as JSON null (regression: crashed the SSH tab).
	if strings.Contains(w.Body.String(), `"keys":null`) {
		t.Fatalf("node keys must be JSON arrays, never null: %s", w.Body.String())
	}
	var overview sshKeysResponse
	if err := json.Unmarshal(w.Body.Bytes(), &overview); err != nil {
		t.Fatalf("failed to parse overview: %v", err)
	}
	if len(overview.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(overview.Nodes))
	}
	for _, n := range overview.Nodes {
		switch n.Name {
		case "node2": // agent reports a supporting version
			if !n.Supported {
				t.Fatalf("expected node2 to be supported, got %+v", n)
			}
			if len(n.Keys) != 0 {
				t.Fatalf("expected no collected keys yet, got %+v", n.Keys)
			}
		default: // node1 runs an old agent, node3 is not in agent mode
			if n.Supported {
				t.Fatalf("expected %s to be unsupported, got %+v", n.Name, n)
			}
			if n.Reason == "" {
				t.Fatalf("expected a reason for unsupported node %s", n.Name)
			}
		}
	}
	if overview.Summary.RuleCount != 0 {
		t.Fatalf("expected no rules by default, got %+v", overview.Summary)
	}

	// 2. Create a rule.
	ruleBody := map[string]any{
		"type":         "all",
		"src_selector": map[string]string{"name": "node1"},
		"dst_selector": map[string]string{"name": "node2"},
		"enabled":      true,
	}
	body, _ := json.Marshal(ruleBody)
	req = httptest.NewRequest("POST", "/api/ssh-keys/rules", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.handleCreateSSHKeyRule(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created config.SSHKeyRule
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == "" || created.Type != "all" {
		t.Fatalf("unexpected created rule: %+v", created)
	}

	// 2b. "All nodes" selectors (sent by the UI) are accepted for both sides.
	allBody, _ := json.Marshal(map[string]any{
		"type":         "authorized_keys",
		"src_selector": map[string]string{"all": "1"},
		"dst_selector": map[string]string{"all": "1"},
		"enabled":      true,
	})
	req = httptest.NewRequest("POST", "/api/ssh-keys/rules", bytes.NewReader(allBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.handleCreateSSHKeyRule(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 for an all-nodes rule, got %d: %s", w.Code, w.Body.String())
	}
	var allRule config.SSHKeyRule
	_ = json.Unmarshal(w.Body.Bytes(), &allRule)
	if !config.IsAllNodesSelector(allRule.DstSelector) {
		t.Fatalf("expected the all-nodes marker to be preserved, got %+v", allRule.DstSelector)
	}
	// Clean up so later assertions see a single rule.
	req = httptest.NewRequest("DELETE", "/api/ssh-keys/rules/"+allRule.ID, nil)
	req = withURLParam(req, "id", allRule.ID)
	w = httptest.NewRecorder()
	srv.handleDeleteSSHKeyRule(w, req)

	// 3. Invalid rules are rejected.
	for name, payload := range map[string]map[string]any{
		"bad type":         {"type": "wat", "src_selector": map[string]string{"name": "node1"}, "dst_selector": map[string]string{"name": "node2"}},
		"empty dst":        {"type": "all", "src_selector": map[string]string{"name": "node1"}, "dst_selector": map[string]string{}},
		"bad src selector": {"type": "all", "src_selector": map[string]string{"nope": "x"}, "dst_selector": map[string]string{"name": "node2"}},
	} {
		b, _ := json.Marshal(payload)
		req = httptest.NewRequest("POST", "/api/ssh-keys/rules", bytes.NewReader(b))
		w = httptest.NewRecorder()
		srv.handleCreateSSHKeyRule(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d: %s", name, w.Code, w.Body.String())
		}
	}

	// 4. List / get / toggle / update / delete.
	req = httptest.NewRequest("GET", "/api/ssh-keys/rules", nil)
	w = httptest.NewRecorder()
	srv.handleGetSSHKeyRules(w, req)
	var rules []config.SSHKeyRule
	_ = json.Unmarshal(w.Body.Bytes(), &rules)
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	req = httptest.NewRequest("GET", "/api/ssh-keys/rules/"+created.ID, nil)
	req = withURLParam(req, "id", created.ID)
	w = httptest.NewRecorder()
	srv.handleGetSSHKeyRule(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for get, got %d", w.Code)
	}

	req = httptest.NewRequest("GET", "/api/ssh-keys/rules/"+created.ID+"/keys", nil)
	req = withURLParam(req, "id", created.ID)
	w = httptest.NewRecorder()
	srv.handleGetSSHKeyRuleKeys(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for get rule keys, got %d: %s", w.Code, w.Body.String())
	}
	var matched engine.SSHKeyRuleMatchedKeys
	if err := json.Unmarshal(w.Body.Bytes(), &matched); err != nil {
		t.Fatalf("failed to parse matched keys: %v", err)
	}
	if matched.RuleID != created.ID || matched.Type != "all" {
		t.Fatalf("unexpected matched keys payload: %+v", matched)
	}

	req = httptest.NewRequest("POST", "/api/ssh-keys/rules/"+created.ID+"/toggle", nil)
	req = withURLParam(req, "id", created.ID)
	w = httptest.NewRecorder()
	srv.handleToggleSSHKeyRule(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for toggle, got %d: %s", w.Code, w.Body.String())
	}
	var toggled config.SSHKeyRule
	_ = json.Unmarshal(w.Body.Bytes(), &toggled)
	if toggled.Enabled {
		t.Fatal("expected rule to be disabled after toggle")
	}

	updateBody := map[string]any{
		"type":         "known_hosts",
		"src_selector": map[string]string{"tag": "core"},
		"dst_selector": map[string]string{"name": "node2,node1"},
		"enabled":      true,
	}
	body, _ = json.Marshal(updateBody)
	req = httptest.NewRequest("PUT", "/api/ssh-keys/rules/"+created.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withURLParam(req, "id", created.ID)
	w = httptest.NewRecorder()
	srv.handleUpdateSSHKeyRule(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for update, got %d: %s", w.Code, w.Body.String())
	}
	var updated config.SSHKeyRule
	_ = json.Unmarshal(w.Body.Bytes(), &updated)
	if updated.Type != "known_hosts" || updated.DstSelector["name"] != "node2,node1" {
		t.Fatalf("unexpected updated rule: %+v", updated)
	}

	// Refresh requires a connected agent.
	req = httptest.NewRequest("POST", "/api/ssh-keys/refresh", strings.NewReader(`{"node":"node2"}`))
	w = httptest.NewRecorder()
	srv.handleRefreshSSHKeys(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 refreshing a disconnected agent, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("DELETE", "/api/ssh-keys/rules/"+created.ID, nil)
	req = withURLParam(req, "id", created.ID)
	w = httptest.NewRecorder()
	srv.handleDeleteSSHKeyRule(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for delete, got %d: %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest("GET", "/api/ssh-keys/rules/"+created.ID, nil)
	req = withURLParam(req, "id", created.ID)
	w = httptest.NewRecorder()
	srv.handleGetSSHKeyRule(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", w.Code)
	}
}
