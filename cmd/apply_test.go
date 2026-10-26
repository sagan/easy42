package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"easy42/internal/config"
)

func TestApplyCmd_DryRun(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "easy42-cmd-apply-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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

	patchFile := filepath.Join(tmpDir, "patch.json")
	patchContent := `{
		"nodes": [
			{
				"name": "foobar",
				"static_routes": ["192.168.100.0/24"]
			}
		]
	}`
	if err := os.WriteFile(patchFile, []byte(patchContent), 0600); err != nil {
		t.Fatalf("failed to write patch file: %v", err)
	}

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"--data-dir", tmpDir, "apply", "-f", patchFile, "--dry-run"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("RootCmd.Execute() failed: %v", err)
	}

	// Verify that the original config on disk was NOT modified (dry run)
	reloaded, _ := store.Load()
	for _, n := range reloaded.Nodes {
		if n.Name == "foobar" && len(n.StaticRoutes) > 0 {
			t.Errorf("expected static routes to be empty on disk during dry run, got: %v", n.StaticRoutes)
		}
	}
}

func TestApplyCmd_Execute(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "easy42-cmd-apply-exec-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

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

	patchFile := filepath.Join(tmpDir, "patch.yaml")
	patchContent := `
nodes:
  - name: foobar
    static_routes:
      - 192.168.100.0/24
      - 1.2.3.4
`
	if err := os.WriteFile(patchFile, []byte(patchContent), 0600); err != nil {
		t.Fatalf("failed to write patch file: %v", err)
	}

	applyDryRun = false
	applyFilePath = ""
	applySync = false
	applyFormat = "text"

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"--data-dir", tmpDir, "apply", "-f", patchFile})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("RootCmd.Execute() failed: %v", err)
	}

	// Verify that the config on disk WAS updated
	reloaded, _ := store.Load()
	found := false
	for _, n := range reloaded.Nodes {
		if n.Name == "foobar" {
			found = true
			if len(n.StaticRoutes) != 2 {
				t.Fatalf("expected 2 static routes, got %d", len(n.StaticRoutes))
			}
			if n.StaticRoutes[0] != "192.168.100.0/24" || n.StaticRoutes[1] != "1.2.3.4" {
				t.Errorf("unexpected static routes: %v", n.StaticRoutes)
			}
		}
	}
	if !found {
		t.Errorf("node foobar not found in reloaded config")
	}
}

func TestApplyCmd_ByName_DryRunAndExecute(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "easy42-cmd-apply-name-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := config.NewStore(tmpDir)
	_, err = store.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	cfg, _ := store.Load()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name: "testnode",
		Host: "testnode.test",
		IP:   "192.168.142.203",
		ASN:  4224420203,
	})
	cfg.ConfigChanges = append(cfg.ConfigChanges, config.ConfigChange{
		ID:   "snippet-1",
		Name: "Add Static Route",
		Content: `{
			"nodes": [
				{
					"name": "testnode",
					"static_routes": ["10.42.0.0/16"]
				}
			]
		}`,
		Format: "json",
	})
	_ = store.Save(cfg)

	// 1. Dry run with --name
	applyDryRun = false
	applyFilePath = ""
	applyName = ""
	applySync = false
	applyFormat = "text"

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SetArgs([]string{"--data-dir", tmpDir, "apply", "--name", "Add Static Route", "--dry-run"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("RootCmd.Execute() with --name --dry-run failed: %v", err)
	}

	// Verify that the original config on disk was NOT modified during dry-run
	reloaded, _ := store.Load()
	for _, n := range reloaded.Nodes {
		if n.Name == "testnode" && len(n.StaticRoutes) > 0 {
			t.Errorf("expected static routes to be empty on disk during dry run, got: %v", n.StaticRoutes)
		}
	}

	// 2. Execute apply with --name (case-insensitive match test)
	applyDryRun = false
	applyFilePath = ""
	applyName = ""
	applySync = false
	applyFormat = "text"

	buf.Reset()
	RootCmd.SetArgs([]string{"--data-dir", tmpDir, "apply", "--name", "add static route"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("RootCmd.Execute() with --name execute failed: %v", err)
	}

	// Verify that the config on disk WAS updated
	reloaded, _ = store.Load()
	found := false
	for _, n := range reloaded.Nodes {
		if n.Name == "testnode" {
			found = true
			if len(n.StaticRoutes) != 1 || n.StaticRoutes[0] != "10.42.0.0/16" {
				t.Fatalf("unexpected static routes after execute: %v", n.StaticRoutes)
			}
		}
	}
	if !found {
		t.Errorf("node testnode not found in reloaded config")
	}

	// 3. Not found error
	applyDryRun = false
	applyFilePath = ""
	applyName = ""
	applySync = false
	applyFormat = "text"

	buf.Reset()
	RootCmd.SetArgs([]string{"--data-dir", tmpDir, "apply", "--name", "non-existent-snippet"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatalf("expected error for non-existent snippet name, got nil")
	}

	// 4. Conflict between file and --name
	applyDryRun = false
	applyFilePath = ""
	applyName = ""
	applySync = false
	applyFormat = "text"

	buf.Reset()
	RootCmd.SetArgs([]string{"--data-dir", tmpDir, "apply", "-f", "dummy.yaml", "--name", "Add Static Route"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatalf("expected error when specifying both -f and --name, got nil")
	}
}

