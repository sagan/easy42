package engine

import (
	"os"
	"path/filepath"
	"testing"

	"easy42/internal/config"
)

func setupTestManager(t *testing.T) (*Manager, string) {
	tmpDir, err := os.MkdirTemp("", "easy42-apply-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	store := config.NewStore(tmpDir)
	_, err = store.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	mgr := NewManager(store)
	cfg := store.Get()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name:         "foobar",
		Host:         "foobar.test",
		IP:           "192.168.142.202",
		IP6:          "fd42:a159:f9f0:8000::ca",
		Interface:    "dummy0",
		ASN:          4224420202,
		Table:        254,
		StaticRoutes: []string{"0.0.0.0/0", "::/0"},
	})
	if err := store.Save(cfg); err != nil {
		t.Fatalf("failed to save initial config: %v", err)
	}

	return mgr, tmpDir
}

func TestApply_PreviewSingleNode(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)
	defer os.RemoveAll(tmpDir)
	defer mgr.Close()

	patch := `{
		"nodes": [
			{
				"name": "foobar",
				"static_routes": [
					"1.2.3.4",
					"192.168.100.0/24"
				]
			}
		]
	}`

	preview, err := mgr.PreviewApply(patch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !preview.Valid {
		t.Fatalf("expected preview to be valid")
	}
	if preview.Summary.ModifiedNodes != 1 {
		t.Errorf("expected 1 modified node, got %d", preview.Summary.ModifiedNodes)
	}
	if len(preview.Nodes) != 1 || preview.Nodes[0].Name != "foobar" {
		t.Fatalf("unexpected nodes diff: %+v", preview.Nodes)
	}

	// Verify original config is untouched
	node, err := mgr.GetNode("foobar")
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	if len(node.StaticRoutes) != 2 || node.StaticRoutes[0] != "0.0.0.0/0" {
		t.Errorf("original node was modified during preview: %+v", node.StaticRoutes)
	}
}

func TestApply_ExecuteSingleNode(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)
	defer os.RemoveAll(tmpDir)
	defer mgr.Close()

	patch := `{
		"nodes": [
			{
				"name": "foobar",
				"static_routes": [
					"1.2.3.4",
					"192.168.100.0/24"
				]
			}
		]
	}`

	res, err := mgr.ApplyConfigChange(patch)
	if err != nil {
		t.Fatalf("failed to apply config change: %v", err)
	}

	if !res.Success {
		t.Fatalf("expected success = true")
	}
	if len(res.AffectedNodes) != 1 || res.AffectedNodes[0] != "foobar" {
		t.Errorf("expected affected nodes ['foobar'], got %v", res.AffectedNodes)
	}

	// Verify node static routes updated
	node, err := mgr.GetNode("foobar")
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	if len(node.StaticRoutes) != 2 || node.StaticRoutes[0] != "1.2.3.4" || node.StaticRoutes[1] != "192.168.100.0/24" {
		t.Errorf("unexpected updated static routes: %+v", node.StaticRoutes)
	}

	// Verify backup file exists
	bakPath := filepath.Join(tmpDir, "config.json.bak")
	if _, err := os.Stat(bakPath); err != nil {
		t.Errorf("backup file was not created: %v", err)
	}
}

func TestApply_CreateNewNodeViaApply(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)
	defer os.RemoveAll(tmpDir)
	defer mgr.Close()

	patch := `{
		"nodes": [
			{
				"name": "fra01",
				"host": "fra.test",
				"ip": "192.168.142.10",
				"ip6": "fd42:a159:f9f0:8000::10",
				"asn": 4224420010
			}
		]
	}`

	res, err := mgr.ApplyConfigChange(patch)
	if err != nil {
		t.Fatalf("failed to apply new node: %v", err)
	}

	if res.Summary.AddedNodes != 1 {
		t.Errorf("expected 1 added node, got %d", res.Summary.AddedNodes)
	}

	node, err := mgr.GetNode("fra01")
	if err != nil {
		t.Fatalf("new node was not found: %v", err)
	}
	if node.ASN != 4224420010 {
		t.Errorf("expected ASN 4224420010, got %d", node.ASN)
	}
}

func TestApply_SavedConfigChanges(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)
	defer os.RemoveAll(tmpDir)
	defer mgr.Close()

	validSnippet := `{
		"nodes": [
			{
				"name": "foobar",
				"static_routes": ["1.1.1.1/32"]
			}
		]
	}`

	// 1. Save valid change
	saved, err := mgr.SaveConfigChange(config.ConfigChange{
		Name:    "Update foobar DNS route",
		Content: validSnippet,
	})
	if err != nil {
		t.Fatalf("failed to save config change: %v", err)
	}
	if saved.ID == "" {
		t.Fatalf("expected generated ID")
	}

	// 2. Try to save invalid change (should fail strict verification)
	invalidSnippet := `{
		"nodes": [
			{
				"name": "foobar",
				"static_routes": ["not-a-valid-cidr-or-ip"]
			}
		]
	}`
	_, err = mgr.SaveConfigChange(config.ConfigChange{
		Name:    "Bad route",
		Content: invalidSnippet,
	})
	if err == nil {
		t.Fatalf("expected error saving invalid config change, got nil")
	}

	// 3. List saved changes
	list := mgr.ListConfigChanges()
	if len(list) != 1 {
		t.Fatalf("expected 1 saved change, got %d", len(list))
	}

	// 4. Apply saved change
	res, err := mgr.ApplySavedConfigChange(saved.ID)
	if err != nil {
		t.Fatalf("failed to apply saved change: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected apply success")
	}

	node, _ := mgr.GetNode("foobar")
	if len(node.StaticRoutes) != 1 || node.StaticRoutes[0] != "1.1.1.1/32" {
		t.Errorf("expected route 1.1.1.1/32, got %v", node.StaticRoutes)
	}

	// 5. Delete saved change
	if err := mgr.DeleteConfigChange(saved.ID); err != nil {
		t.Fatalf("failed to delete saved change: %v", err)
	}
	if len(mgr.ListConfigChanges()) != 0 {
		t.Errorf("expected 0 saved changes after delete")
	}
}

func TestApply_NodeCaption(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)
	defer os.RemoveAll(tmpDir)
	defer mgr.Close()

	patch := `{
		"nodes": [
			{
				"name": "foobar",
				"caption": "Frankfurt Edge Node"
			}
		]
	}`

	preview, err := mgr.PreviewApply(patch)
	if err != nil {
		t.Fatalf("unexpected error previewing caption patch: %v", err)
	}
	if !preview.Valid {
		t.Fatalf("expected preview to be valid")
	}
	if len(preview.Nodes) != 1 || len(preview.Nodes[0].Changes) != 1 {
		t.Fatalf("expected 1 change on foobar, got %+v", preview.Nodes)
	}
	if preview.Nodes[0].Changes[0].Field != "caption" || preview.Nodes[0].Changes[0].NewValue != "Frankfurt Edge Node" {
		t.Fatalf("unexpected diff: %+v", preview.Nodes[0].Changes)
	}

	res, err := mgr.ApplyConfigChange(patch)
	if err != nil {
		t.Fatalf("unexpected error executing caption patch: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply failed: %+v", res)
	}

	node, err := mgr.GetNode("foobar")
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	if node.Caption != "Frankfurt Edge Node" {
		t.Errorf("expected node caption 'Frankfurt Edge Node', got %q", node.Caption)
	}
}
