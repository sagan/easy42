package engine

import (
	"os"
	"testing"
	"time"

	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
)

func TestRestartNoAgentReconnect(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "persist-*")
	defer os.RemoveAll(tempDir)

	store := config.NewStore(tempDir)
	pass, _ := store.Initialize()
	mgr := NewManager(store)
	_ = mgr.Unlock(pass)
	_ = mgr.AddNode(config.Node{Name: "n1", Host: "192.168.10.30", IP: "10.42.0.30/24", Mode: "agent"})
	mgr.SaveApp(config.App{ID: "a1", Name: "app1", AppType: "docker", Enabled: true, Published: true, Selector: map[string]string{"name": "n1"}, MainContent: "services:\n  x:\n    image: nginx\n"})
	mgr.SaveApp(config.App{ID: "a2", Name: "app2", AppType: "systemd", Enabled: false, Published: true, Selector: map[string]string{"name": "n1"}, MainContent: "[Unit]\n[Service]\nExecStart=/bin/true\n"})

	// Simulate a completed sync: only the hub caches are written (as the real
	// agent sync path does), plus telemetry.
	specs, _ := mgr.BuildAppSpecsForNode("n1")
	for _, s := range specs {
		mgr.AgentHub().SetAppHash("n1", s.Name, s.Sha256Hash)
		mgr.AgentHub().SetAppEnabled("n1", s.Name, s.Enabled)
	}
	mgr.AgentHub().UpdateTelemetry("n1", &agentpb.TelemetryReport{Apps: []*agentpb.AppStatus{
		{Name: "app1", State: "running"}, {Name: "app2", State: "stopped"},
	}})
	time.Sleep(200 * time.Millisecond)

	check := func(label string, m *Manager) {
		acts, _ := m.PlanSync()
		for _, a := range acts {
			if a.Type == config.ActionSyncApp && a.NeedsApply {
				t.Errorf("%s: pending app action %s (%s) %s", label, a.Interface, a.TargetFile, a.DiffStatus)
			}
		}
	}
	check("before restart", mgr)
	mgr.Close()

	store2 := config.NewStore(tempDir)
	_, _ = store2.Load()
	mgr2 := NewManager(store2)
	defer mgr2.Close()
	check("after restart (agent offline, no state update)", mgr2)
}
