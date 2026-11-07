package config

import (
	"testing"
)

func TestAppTargetsNode(t *testing.T) {
	node1 := &Node{
		Name: "node-tokyo",
		Tags: []string{"asia", "edge", "prod"},
		IP:   "192.168.100.1/24",
	}
	node2 := &Node{
		Name: "node-frankfurt",
		Tags: []string{"europe", "core"},
		IP:   "192.168.100.2/24",
	}

	// 1. Manual selection
	manualApp := App{
		ID:       "app-1",
		Name:     "manual-app",
		Selector: map[string]string{"name": "node-tokyo"},
	}
	if !AppTargetsNode(manualApp, node1, "") {
		t.Errorf("manualApp should target node1")
	}
	if AppTargetsNode(manualApp, node2, "") {
		t.Errorf("manualApp should NOT target node2")
	}

	// An empty selector matches every node.
	allApp := App{
		ID:   "app-all",
		Name: "all-app",
	}
	if !AppTargetsNode(allApp, node1, "") {
		t.Errorf("allApp should target node1")
	}
	if !AppTargetsNode(allApp, node2, "") {
		t.Errorf("allApp should target node2")
	}

	// Selecting none with an empty name list.
	noneApp := App{
		ID:       "app-none",
		Name:     "none-app",
		Selector: map[string]string{"name": ""},
	}
	if AppTargetsNode(noneApp, node1, "") {
		t.Errorf("noneApp should NOT target node1")
	}
	if AppTargetsNode(noneApp, node2, "") {
		t.Errorf("noneApp should NOT target node2")
	}

	// 2. Nodes of a selected tag
	tagApp := App{
		ID:       "app-tag",
		Name:     "tag-app",
		Selector: map[string]string{"tag": "edge"},
	}
	if !AppTargetsNode(tagApp, node1, "") {
		t.Errorf("tagApp should target node1 (has tag 'edge')")
	}
	if AppTargetsNode(tagApp, node2, "") {
		t.Errorf("tagApp should NOT target node2 (lacks tag 'edge')")
	}

	// Node environment variables can be used as selector conditions.
	envApp := App{
		ID:       "app-env",
		Name:     "env-app",
		Selector: map[string]string{"env:NODE_IP": "192.168.100.2"},
	}
	if AppTargetsNode(envApp, node1, "") {
		t.Errorf("envApp should NOT target node1")
	}
	if !AppTargetsNode(envApp, node2, "") {
		t.Errorf("envApp should target node2")
	}

	// Name-only selectors work even when only the node name is known.
	if !AppTargetsNode(manualApp, nil, "node-tokyo") {
		t.Errorf("manualApp should target node-tokyo by name without a full node")
	}
	if AppTargetsNode(tagApp, nil, "node-tokyo") {
		t.Errorf("tagApp cannot match without a full node")
	}
}

func TestValidateAppSelector(t *testing.T) {
	base := App{
		Name:        "test-app",
		AppType:     "docker",
		MainContent: "version: '3.8'\nservices:\n  app:\n    image: nginx",
	}

	// Empty selector is valid and targets all nodes.
	if err := ValidateApp(base); err != nil {
		t.Errorf("expected empty selector to be valid, got: %v", err)
	}

	// Valid manual selection with an empty list.
	appManualEmpty := base
	appManualEmpty.Selector = map[string]string{"name": ""}
	if err := ValidateApp(appManualEmpty); err != nil {
		t.Errorf("expected empty name selector to be valid, got: %v", err)
	}

	// Valid tag selector.
	appTag := base
	appTag.Selector = map[string]string{"tag": "edge"}
	if err := ValidateApp(appTag); err != nil {
		t.Errorf("expected tag selector to be valid, got: %v", err)
	}

	// Valid environment selector.
	appEnv := base
	appEnv.Selector = map[string]string{"env:NODE_IP": "10.0.0.1"}
	if err := ValidateApp(appEnv); err != nil {
		t.Errorf("expected env selector to be valid, got: %v", err)
	}

	// Invalid selector key.
	appBadKey := base
	appBadKey.Selector = map[string]string{"random": "value"}
	if err := ValidateApp(appBadKey); err == nil {
		t.Errorf("expected error with unknown selector key")
	}

	// Invalid env selector without a variable name.
	appBadEnv := base
	appBadEnv.Selector = map[string]string{"env:": "value"}
	if err := ValidateApp(appBadEnv); err == nil {
		t.Errorf("expected error with empty env selector variable name")
	}

	// Invalid empty selector key.
	appEmptyKey := base
	appEmptyKey.Selector = map[string]string{" ": "value"}
	if err := ValidateApp(appEmptyKey); err == nil {
		t.Errorf("expected error with empty selector key")
	}
}

func TestValidateCronApp(t *testing.T) {
	cronApp := App{
		Name:        "backup-job",
		AppType:     "cron",
		MainContent: "0 0 * * * /root/backup.sh",
	}
	if err := ValidateApp(cronApp); err != nil {
		t.Fatalf("expected valid cron app, got error: %v", err)
	}

	cronEmpty := cronApp
	cronEmpty.MainContent = "   "
	if err := ValidateApp(cronEmpty); err == nil {
		t.Errorf("expected error for empty main_content in cron app")
	}
}
