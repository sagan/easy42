package engine

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"easy42/internal/config"
)

// artifactTestConfig registers one agent node and a systemd app whose artifact
// points at a test server that always fails.
func artifactTestManager(t *testing.T, upstreamURL string) *Manager {
	t.Helper()
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "n1", Host: "10.0.0.1", Mode: "agent", AgentToken: "t", AgentVersion: "0.1.19", IP: "10.1.0.1"},
		},
		Apps: []config.App{
			{
				ID:          "a1",
				Name:        "bin-app",
				AppType:     "systemd",
				MainContent: "[Service]\nExecStart=/bin/true\n",
				Selector:    config.AllNodesSelector(),
				Published:   true,
				Enabled:     true,
				Artifact: &config.ArtifactSpec{
					URL:   upstreamURL + "/asset-${ARCH}.tar.gz",
					Files: []config.ArtifactFile{{Name: "tool", TargetPath: "${PATH}/tool"}},
				},
			},
		},
	})
	mgr.agentHub.SetNodeMeta("n1", agentNodeMeta("n1", "0.1.19"))
	return mgr
}

// Opening the sync dialog must never contact upstream servers: an uncached
// artifact is a cache miss, not a download trigger.
func TestSyncPreviewDoesNotFetchArtifacts(t *testing.T) {
	var hits int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(200 * time.Millisecond)
		http.Error(w, "unavailable", http.StatusBadGateway)
	}))
	defer upstream.Close()

	mgr := artifactTestManager(t, upstream.URL)

	start := time.Now()
	actions, err := mgr.PlanSyncPreview("")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("PlanSyncPreview failed: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Fatalf("preview must not contact upstream, got %d request(s)", got)
	}
	if elapsed > 150*time.Millisecond {
		t.Fatalf("preview must not block on artifacts, took %v", elapsed)
	}
	if actions == nil {
		t.Fatal("preview must still return a plan")
	}

	// A real plan (used by execute) still resolves artifacts.
	_, _ = mgr.PlanSync()
	if got := atomic.LoadInt32(&hits); got == 0 {
		t.Fatal("executing a sync must still resolve artifacts upstream")
	}
}
