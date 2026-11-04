package config

import (
	"testing"
)

func TestAppTargetsNode(t *testing.T) {
	node1 := &Node{
		Name: "node-tokyo",
		Tags: []string{"asia", "edge", "prod"},
	}
	node2 := &Node{
		Name: "node-frankfurt",
		Tags: []string{"europe", "core"},
	}

	// 1. Manual selection
	manualApp := App{
		ID:         "app-1",
		Name:       "manual-app",
		TargetType: "manual",
		Nodes:      []string{"node-tokyo"},
	}
	if !AppTargetsNode(manualApp, node1, "") {
		t.Errorf("manualApp should target node1")
	}
	if AppTargetsNode(manualApp, node2, "") {
		t.Errorf("manualApp should NOT target node2")
	}

	// Default/empty TargetType should behave as manual
	defaultApp := App{
		ID:    "app-def",
		Name:  "default-app",
		Nodes: []string{"node-frankfurt"},
	}
	if AppTargetsNode(defaultApp, node1, "") {
		t.Errorf("defaultApp should NOT target node1")
	}
	if !AppTargetsNode(defaultApp, node2, "") {
		t.Errorf("defaultApp should target node2")
	}

	// Selecting none (empty nodes) in manual mode
	noneApp := App{
		ID:         "app-none",
		Name:       "none-app",
		TargetType: "manual",
		Nodes:      []string{},
	}
	if AppTargetsNode(noneApp, node1, "") {
		t.Errorf("noneApp should NOT target node1")
	}
	if AppTargetsNode(noneApp, node2, "") {
		t.Errorf("noneApp should NOT target node2")
	}

	// 2. All nodes
	allApp := App{
		ID:         "app-all",
		Name:       "all-app",
		TargetType: "all",
	}
	if !AppTargetsNode(allApp, node1, "") {
		t.Errorf("allApp should target node1")
	}
	if !AppTargetsNode(allApp, node2, "") {
		t.Errorf("allApp should target node2")
	}

	// 3. Nodes of a selected tag
	tagApp := App{
		ID:         "app-tag",
		Name:       "tag-app",
		TargetType: "tag",
		TargetTag:  "edge",
	}
	if !AppTargetsNode(tagApp, node1, "") {
		t.Errorf("tagApp should target node1 (has tag 'edge')")
	}
	if AppTargetsNode(tagApp, node2, "") {
		t.Errorf("tagApp should NOT target node2 (lacks tag 'edge')")
	}

	// Case-insensitivity in tag matching
	tagUpperApp := App{
		ID:         "app-tag-upper",
		Name:       "tag-upper-app",
		TargetType: "tag",
		TargetTag:  "EUROPE",
	}
	if !AppTargetsNode(tagUpperApp, node2, "") {
		t.Errorf("tagUpperApp should target node2 ('EUROPE' matches 'europe')")
	}
}

func TestValidateAppTargetType(t *testing.T) {
	base := App{
		Name:        "test-app",
		AppType:     "docker",
		MainContent: "version: '3.8'\nservices:\n  app:\n    image: nginx",
	}

	// Valid manual with empty nodes (select none)
	appManualEmpty := base
	appManualEmpty.TargetType = "manual"
	appManualEmpty.Nodes = []string{}
	if err := ValidateApp(appManualEmpty); err != nil {
		t.Errorf("expected manual with empty nodes to be valid, got: %v", err)
	}

	// Valid all
	appAll := base
	appAll.TargetType = "all"
	if err := ValidateApp(appAll); err != nil {
		t.Errorf("expected target_type 'all' to be valid, got: %v", err)
	}

	// Valid tag
	appTag := base
	appTag.TargetType = "tag"
	appTag.TargetTag = "edge"
	if err := ValidateApp(appTag); err != nil {
		t.Errorf("expected target_type 'tag' with tag to be valid, got: %v", err)
	}

	// Invalid tag without target_tag
	appTagEmpty := base
	appTagEmpty.TargetType = "tag"
	appTagEmpty.TargetTag = "   "
	if err := ValidateApp(appTagEmpty); err == nil {
		t.Errorf("expected error when target_tag is empty with target_type 'tag'")
	}

	// Invalid target_type
	appBadType := base
	appBadType.TargetType = "random"
	if err := ValidateApp(appBadType); err == nil {
		t.Errorf("expected error with unknown target_type 'random'")
	}
}
