package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestGetOnlineSources(t *testing.T) {
	tmpDir := t.TempDir()

	// Initially only default source
	sources := GetOnlineSources(tmpDir)
	if len(sources) != 1 || sources[0] != DefaultOnlineSource {
		t.Fatalf("expected only default source, got %v", sources)
	}

	// Add sources.txt with comments, whitespace, and a custom URL
	sourcesContent := `
# Custom sources
https://custom-repo.example.com/easy42-tasks/raw/main
  https://another-repo.example.com/tasks/  
https://raw.githubusercontent.com/sagan/easy42-tasks/refs/heads/master/
invalid-url
`
	if err := os.WriteFile(filepath.Join(tmpDir, "sources.txt"), []byte(sourcesContent), 0644); err != nil {
		t.Fatalf("failed to write sources.txt: %v", err)
	}

	sources = GetOnlineSources(tmpDir)
	if len(sources) != 3 {
		t.Fatalf("expected 3 unique valid sources, got %d: %v", len(sources), sources)
	}
	if sources[0] != DefaultOnlineSource {
		t.Errorf("expected default source first, got %s", sources[0])
	}
	if sources[1] != "https://custom-repo.example.com/easy42-tasks/raw/main/" {
		t.Errorf("expected trailing slash normalized, got %s", sources[1])
	}
	if sources[2] != "https://another-repo.example.com/tasks/" {
		t.Errorf("expected custom source 2, got %s", sources[2])
	}
}

func TestFetchOnlineManifestAndInstall(t *testing.T) {
	manifest := OnlineManifest{
		Tasks: []OnlineTask{
			{
				ID:          "install_bind",
				Title:       "Install BIND DNS server",
				Description: "Install BIND DNS server.",
				Category:    "Packages",
				Weight:      20,
				Files:       []string{"run.sh", "status.sh", "task.json"},
			},
		},
	}
	manifestBytes, _ := json.Marshal(manifest)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(manifestBytes)
		case "/install_bind/run.sh":
			_, _ = w.Write([]byte("#!/bin/sh\necho 'installing bind'\nexit 0\n"))
		case "/install_bind/status.sh":
			_, _ = w.Write([]byte("#!/bin/sh\nexit 0\n"))
		case "/install_bind/task.json":
			_, _ = w.Write([]byte(`{"title":"Install BIND DNS server","category":"Packages"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := context.Background()

	// 1. Fetch Manifest
	fetched, err := FetchOnlineManifest(ctx, server.URL)
	if err != nil {
		t.Fatalf("FetchOnlineManifest failed: %v", err)
	}
	if len(fetched.Tasks) != 1 || fetched.Tasks[0].ID != "install_bind" {
		t.Fatalf("unexpected fetched tasks: %+v", fetched.Tasks)
	}

	// 2. Install Task
	tmpTasksDir := t.TempDir()
	err = InstallOnlineTask(ctx, tmpTasksDir, server.URL, "install_bind", []string{"run.sh", "status.sh", "task.json"})
	if err != nil {
		t.Fatalf("InstallOnlineTask failed: %v", err)
	}

	installedRunSh := filepath.Join(tmpTasksDir, "install_bind", "run.sh")
	runInfo, err := os.Stat(installedRunSh)
	if err != nil {
		t.Fatalf("installed run.sh missing: %v", err)
	}
	if runInfo.Mode()&0111 == 0 {
		t.Errorf("expected run.sh to have executable permissions, got %v", runInfo.Mode())
	}

	installedStatusSh := filepath.Join(tmpTasksDir, "install_bind", "status.sh")
	if _, err := os.Stat(installedStatusSh); err != nil {
		t.Fatalf("installed status.sh missing: %v", err)
	}

	// 3. Delete Task
	if err := DeleteCustomTask(tmpTasksDir, "install_bind"); err != nil {
		t.Fatalf("DeleteCustomTask failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpTasksDir, "install_bind")); !os.IsNotExist(err) {
		t.Errorf("expected task directory to be deleted")
	}

	// 4. Invalid Task ID check
	if err := DeleteCustomTask(tmpTasksDir, "../bad"); err == nil {
		t.Errorf("expected error for path traversal task id")
	}
}
