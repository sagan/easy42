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

	// The explicit all-nodes selector targets every node.
	allApp := App{
		ID:       "app-all",
		Name:     "all-app",
		Selector: AllNodesSelector(),
	}
	if !AppTargetsNode(allApp, node1, "") {
		t.Errorf("allApp should target node1")
	}
	if !AppTargetsNode(allApp, node2, "") {
		t.Errorf("allApp should target node2")
	}

	// An empty (or missing) selector matches no node.
	emptyApp := App{ID: "app-empty", Name: "empty-app"}
	if AppTargetsNode(emptyApp, node1, "") || AppTargetsNode(emptyApp, node2, "") {
		t.Errorf("empty selector must not target any node")
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

	// Empty selector is valid and matches no node.
	if err := ValidateApp(base); err != nil {
		t.Errorf("expected empty selector to be valid, got: %v", err)
	}

	// The all-nodes selector is valid.
	appAll := base
	appAll.Selector = AllNodesSelector()
	if err := ValidateApp(appAll); err != nil {
		t.Errorf("expected all-nodes selector to be valid, got: %v", err)
	}

	// The all-nodes marker cannot be combined with other conditions.
	appCombined := base
	appCombined.Selector = map[string]string{"all": "1", "tag": "edge"}
	if err := ValidateApp(appCombined); err == nil {
		t.Errorf("expected combined all-nodes selector to be rejected")
	}

	// A comma-separated env value is rejected (only one exact value matches).
	appEnvList := base
	appEnvList.Selector = map[string]string{"env:NODE_IP": "10.0.0.1,10.0.0.2"}
	if err := ValidateApp(appEnvList); err == nil {
		t.Errorf("expected comma-separated env selector to be rejected")
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

	cronWithArtifactAndFiles := cronApp
	cronWithArtifactAndFiles.Files = map[string]string{
		"config.yaml": "key: value\n",
	}
	cronWithArtifactAndFiles.Artifact = &ArtifactSpec{
		URL: "https://example.com/backup-${ARCH}.tar.gz",
		Files: []ArtifactFile{
			{
				Name:       "backup-job",
				TargetPath: "${PATH}/backup-job",
				PullPolicy: "if_changed",
				FileMode:   0755,
			},
		},
	}
	if err := ValidateApp(cronWithArtifactAndFiles); err != nil {
		t.Fatalf("expected valid cron app with artifact and files, got: %v", err)
	}

	dockerWithArtifact := cronWithArtifactAndFiles
	dockerWithArtifact.AppType = "docker"
	if err := ValidateApp(dockerWithArtifact); err == nil {
		t.Errorf("expected error for artifact on docker app")
	}
}

func TestValidateScriptApp(t *testing.T) {
	scriptApp := App{
		Name:        "cleanup-job",
		AppType:     "script",
		MainContent: "#!/bin/sh\necho 'clean'",
		EnvContent:  "RETENTION_DAYS=7",
	}
	if err := ValidateApp(scriptApp); err != nil {
		t.Fatalf("expected valid script app, got error: %v", err)
	}

	scriptEmpty := scriptApp
	scriptEmpty.MainContent = "   "
	if err := ValidateApp(scriptEmpty); err == nil {
		t.Errorf("expected error for empty main_content in script app")
	}

	scriptWithFiles := scriptApp
	scriptWithFiles.Files = map[string]string{
		"config.yaml": "key: value",
	}
	if err := ValidateApp(scriptWithFiles); err == nil {
		t.Errorf("expected error for config files on script app")
	}

	scriptWithArtifact := scriptApp
	scriptWithArtifact.Artifact = &ArtifactSpec{
		URL: "https://example.com/bin.tar.gz",
		Files: []ArtifactFile{
			{Name: "bin", TargetPath: "${PATH}/bin"},
		},
	}
	if err := ValidateApp(scriptWithArtifact); err == nil {
		t.Errorf("expected error for artifact on script app")
	}
}


