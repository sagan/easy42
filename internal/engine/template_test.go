package engine

import (
	"strings"
	"testing"

	"easy42/internal/config"
)

func setupTestManagerWithTemplates(t *testing.T) *Manager {
	tempDir := t.TempDir()
	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("Failed to init store: %v", err)
	}

	mgr := NewManager(store)
	if err := mgr.Unlock(pass); err != nil {
		t.Fatalf("Failed to unlock manager: %v", err)
	}

	return mgr
}

func TestTemplateCRUD(t *testing.T) {
	mgr := setupTestManagerWithTemplates(t)

	// 1. Initial list should contain the 3 system default templates
	templates := mgr.GetTemplates()
	if len(templates) < 3 {
		t.Fatalf("expected at least 3 builtin templates, got %d", len(templates))
	}

	var foundWg, foundBird, foundNft bool
	for _, tmpl := range templates {
		if tmpl.ID == config.DefaultWgTemplateID && tmpl.IsBuiltin {
			foundWg = true
		}
		if tmpl.ID == config.DefaultBirdTemplateID && tmpl.IsBuiltin {
			foundBird = true
		}
		if tmpl.ID == config.DefaultNftTemplateID && tmpl.IsBuiltin {
			foundNft = true
		}
	}
	if !foundWg || !foundBird || !foundNft {
		t.Errorf("missing system default templates: wg=%v, bird=%v, nft=%v", foundWg, foundBird, foundNft)
	}

	// 2. Cannot create template with reserved ID
	_, err := mgr.CreateTemplate(config.ConfigTemplate{
		ID:      config.DefaultBirdTemplateID,
		Name:    "Reserved",
		Type:    config.TemplateTypeBird,
		Content: "router id {{ .ip }};",
	})
	if err == nil {
		t.Errorf("expected error creating template with reserved ID")
	}

	// 3. Cannot create template with invalid syntax
	_, err = mgr.CreateTemplate(config.ConfigTemplate{
		ID:      "bad-bird",
		Name:    "Bad Bird",
		Type:    config.TemplateTypeBird,
		Content: "router id {{ .broken",
	})
	if err == nil {
		t.Errorf("expected error creating template with syntax error")
	}

	// 4. Create valid custom BIRD template
	created, err := mgr.CreateTemplate(config.ConfigTemplate{
		ID:          "my-bird-tmpl",
		Name:        "My Custom BIRD",
		Type:        config.TemplateTypeBird,
		Description: "Custom BIRD template for testing",
		Content:     "# CUSTOM_BIRD\nrouter id {{ .ip }};\n",
	})
	if err != nil {
		t.Fatalf("failed to create custom template: %v", err)
	}
	if created.ID != "my-bird-tmpl" || created.IsBuiltin {
		t.Errorf("unexpected created template: %+v", created)
	}

	// 5. Get template by ID
	fetched, err := mgr.GetTemplate("my-bird-tmpl")
	if err != nil {
		t.Fatalf("failed to get template: %v", err)
	}
	if fetched.Name != "My Custom BIRD" {
		t.Errorf("expected name 'My Custom BIRD', got %s", fetched.Name)
	}

	// 6. Update template
	updated, err := mgr.UpdateTemplate("my-bird-tmpl", config.ConfigTemplate{
		Name:    "My Updated BIRD",
		Type:    config.TemplateTypeBird,
		Content: "# UPDATED_BIRD\nrouter id {{ .ip }};\n",
	})
	if err != nil {
		t.Fatalf("failed to update template: %v", err)
	}
	if updated.Name != "My Updated BIRD" || !strings.Contains(updated.Content, "UPDATED_BIRD") {
		t.Errorf("unexpected updated template: %+v", updated)
	}

	// 7. Cannot update built-in template
	_, err = mgr.UpdateTemplate(config.DefaultBirdTemplateID, config.ConfigTemplate{
		Name:    "Hacked",
		Type:    config.TemplateTypeBird,
		Content: "hack",
	})
	if err == nil {
		t.Errorf("expected error updating built-in template")
	}

	// 8. Cannot delete built-in template
	if err := mgr.DeleteTemplate(config.DefaultWgTemplateID); err == nil {
		t.Errorf("expected error deleting built-in template")
	}

	// 9. Add a node referencing the custom template
	err = mgr.AddNode(config.Node{
		Name:         "node1",
		Host:         "1.2.3.4",
		IP:           "192.168.100.1",
		ASN:          4224420001,
		BirdTemplate: "my-bird-tmpl",
	})
	if err != nil {
		t.Fatalf("failed to add node: %v", err)
	}

	// 10. Generate BIRD config for node1 should use the custom template
	birdConf, err := mgr.GenerateBirdConfig("node1")
	if err != nil {
		t.Fatalf("failed to generate bird config: %v", err)
	}
	if !strings.Contains(birdConf, "UPDATED_BIRD") {
		t.Errorf("expected bird config to contain UPDATED_BIRD, got:\n%s", birdConf)
	}

	// 11. Deleting template while in use should fail
	err = mgr.DeleteTemplate("my-bird-tmpl")
	if err == nil {
		t.Errorf("expected error deleting template that is in use by node1")
	}

	// 12. Remove template reference from node and delete
	node1 := config.Node{
		Name:         "node1",
		Host:         "1.2.3.4",
		IP:           "192.168.100.1",
		ASN:          4224420001,
		BirdTemplate: "", // unset
	}
	if err := mgr.UpdateNode("node1", node1); err != nil {
		t.Fatalf("failed to update node: %v", err)
	}

	if err := mgr.DeleteTemplate("my-bird-tmpl"); err != nil {
		t.Fatalf("failed to delete unused custom template: %v", err)
	}

	// 13. Verify template is gone
	_, err = mgr.GetTemplate("my-bird-tmpl")
	if err == nil {
		t.Errorf("expected error getting deleted template")
	}
}
