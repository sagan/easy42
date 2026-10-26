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

func TestApply_NodeRouteReflector(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)
	defer os.RemoveAll(tmpDir)
	defer mgr.Close()

	patch := `{
		"nodes": [
			{
				"name": "foobar",
				"route_reflector": true
			}
		]
	}`

	preview, err := mgr.PreviewApply(patch)
	if err != nil {
		t.Fatalf("unexpected error previewing route_reflector patch: %v", err)
	}
	if !preview.Valid {
		t.Fatalf("expected preview to be valid")
	}
	if len(preview.Nodes) != 1 || len(preview.Nodes[0].Changes) != 1 {
		t.Fatalf("expected 1 change on foobar, got %+v", preview.Nodes)
	}
	if preview.Nodes[0].Changes[0].Field != "route_reflector" || preview.Nodes[0].Changes[0].NewValue != true {
		t.Fatalf("unexpected diff: %+v", preview.Nodes[0].Changes)
	}

	res, err := mgr.ApplyConfigChange(patch)
	if err != nil {
		t.Fatalf("unexpected error executing route_reflector patch: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply failed: %+v", res)
	}

	node, err := mgr.GetNode("foobar")
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	if !node.RouteReflector {
		t.Errorf("expected node route_reflector to be true, got %v", node.RouteReflector)
	}
}

func TestApply_StaticRoutesDelta(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)
	defer os.RemoveAll(tmpDir)
	defer mgr.Close()

	// Initial node has static_routes: ["0.0.0.0/0", "::/0"] (from setupTestManager)

	// 1. Add new routes using static_routes_add
	patchAdd := `{
		"nodes": [
			{
				"name": "foobar",
				"static_routes_add": [
					"1.2.3.4",
					"192.168.100.0/24"
				]
			}
		]
	}`

	preview, err := mgr.PreviewApply(patchAdd)
	if err != nil {
		t.Fatalf("unexpected error previewing static_routes_add: %v", err)
	}
	if !preview.Valid {
		t.Fatalf("expected preview to be valid")
	}
	if preview.Summary.ModifiedNodes != 1 {
		t.Fatalf("expected 1 modified node, got %d", preview.Summary.ModifiedNodes)
	}
	if len(preview.Nodes[0].Changes) != 1 || preview.Nodes[0].Changes[0].Field != "static_routes" {
		t.Fatalf("expected 1 change on static_routes, got %+v", preview.Nodes[0].Changes)
	}

	res, err := mgr.ApplyConfigChange(patchAdd)
	if err != nil {
		t.Fatalf("unexpected error applying static_routes_add: %v", err)
	}
	if !res.Success {
		t.Fatalf("apply failed")
	}

	node, err := mgr.GetNode("foobar")
	if err != nil {
		t.Fatalf("failed to get node: %v", err)
	}
	expected := []string{"0.0.0.0/0", "::/0", "1.2.3.4", "192.168.100.0/24"}
	if len(node.StaticRoutes) != len(expected) {
		t.Fatalf("expected %d routes, got %d: %v", len(expected), len(node.StaticRoutes), node.StaticRoutes)
	}
	for i, r := range expected {
		if node.StaticRoutes[i] != r {
			t.Errorf("expected route [%d]=%s, got %s", i, r, node.StaticRoutes[i])
		}
	}

	// 2. Idempotency: applying the exact same static_routes_add when node already includes them must do nothing
	previewIdempotent, err := mgr.PreviewApply(patchAdd)
	if err != nil {
		t.Fatalf("unexpected error previewing idempotent patch: %v", err)
	}
	if !previewIdempotent.Valid {
		t.Fatalf("expected preview to be valid")
	}
	if previewIdempotent.Summary.ModifiedNodes != 0 || previewIdempotent.Summary.TotalChanges != 0 {
		t.Fatalf("expected 0 modified nodes and 0 changes when routes already present, got %d nodes / %d changes",
			previewIdempotent.Summary.ModifiedNodes, previewIdempotent.Summary.TotalChanges)
	}
	if len(previewIdempotent.Nodes) != 0 {
		t.Fatalf("expected empty nodes diff for no-op change, got %+v", previewIdempotent.Nodes)
	}

	resIdempotent, err := mgr.ApplyConfigChange(patchAdd)
	if err != nil {
		t.Fatalf("unexpected error applying idempotent patch: %v", err)
	}
	if len(resIdempotent.AffectedNodes) != 0 {
		t.Errorf("expected 0 affected nodes for idempotent apply, got %v", resIdempotent.AffectedNodes)
	}

	// 3. Delete routes using static_routes_del
	patchDel := `{
		"nodes": [
			{
				"name": "foobar",
				"static_routes_del": [
					"1.2.3.4"
				]
			}
		]
	}`

	resDel, err := mgr.ApplyConfigChange(patchDel)
	if err != nil {
		t.Fatalf("unexpected error applying static_routes_del: %v", err)
	}
	if len(resDel.AffectedNodes) != 1 {
		t.Errorf("expected 1 affected node, got %d", len(resDel.AffectedNodes))
	}

	nodeAfterDel, _ := mgr.GetNode("foobar")
	expectedAfterDel := []string{"0.0.0.0/0", "::/0", "192.168.100.0/24"}
	if len(nodeAfterDel.StaticRoutes) != len(expectedAfterDel) {
		t.Fatalf("expected %d routes after del, got %d: %v", len(expectedAfterDel), len(nodeAfterDel.StaticRoutes), nodeAfterDel.StaticRoutes)
	}
	for i, r := range expectedAfterDel {
		if nodeAfterDel.StaticRoutes[i] != r {
			t.Errorf("expected route [%d]=%s, got %s", i, r, nodeAfterDel.StaticRoutes[i])
		}
	}

	// 4. Deleting a route that doesn't exist is a no-op
	patchDelNonExistent := `{
		"nodes": [
			{
				"name": "foobar",
				"static_routes_del": [
					"10.0.0.0/8"
				]
			}
		]
	}`
	previewNonExistent, err := mgr.PreviewApply(patchDelNonExistent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if previewNonExistent.Summary.TotalChanges != 0 || len(previewNonExistent.Nodes) != 0 {
		t.Errorf("expected 0 changes for deleting non-existent route, got %d", previewNonExistent.Summary.TotalChanges)
	}
}

func TestApply_TagsDelta(t *testing.T) {
	mgr, tmpDir := setupTestManager(t)
	defer os.RemoveAll(tmpDir)
	defer mgr.Close()

	// Give foobar an initial tag
	patchInit := `{
		"nodes": [
			{
				"name": "foobar",
				"tags": ["core", "gateway"]
			}
		]
	}`
	_, _ = mgr.ApplyConfigChange(patchInit)

	// Add tag "edge" and delete tag "gateway"
	patchDelta := `{
		"nodes": [
			{
				"name": "foobar",
				"tags_add": ["edge"],
				"tags_del": ["gateway"]
			}
		]
	}`
	res, err := mgr.ApplyConfigChange(patchDelta)
	if err != nil {
		t.Fatalf("failed to apply tags delta: %v", err)
	}
	if len(res.AffectedNodes) != 1 {
		t.Fatalf("expected 1 affected node, got %d", len(res.AffectedNodes))
	}

	node, _ := mgr.GetNode("foobar")
	if len(node.Tags) != 2 || node.Tags[0] != "core" || node.Tags[1] != "edge" {
		t.Errorf("unexpected tags after delta: %v", node.Tags)
	}
}


