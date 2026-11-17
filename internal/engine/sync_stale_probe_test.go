package engine

import (
	"os"
	"testing"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
)

// Reproduces the reported symptom: recorded hash matches the desired state, but
// a probed file (from a stale probe after restart) has different content.
func TestStaleProbeDoesNotMarkUpdate(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "stale-*")
	defer os.RemoveAll(tempDir)
	store := config.NewStore(tempDir)
	pass, _ := store.Initialize()
	mgr := NewManager(store)
	_ = mgr.Unlock(pass)
	_ = mgr.AddNode(config.Node{Name: "home2", Host: "192.168.2.1", IP: "10.42.0.30/24", Mode: "agent"})
	mgr.SaveApp(config.App{ID: "a1", Name: "vproxy", AppType: "systemd", Enabled: true, Published: true, Selector: map[string]string{"name": "home2"}, MainContent: "[Unit]\n[Service]\nExecStart=/bin/true\n"})
	specs, _ := mgr.BuildAppSpecsForNode("home2")
	s := specs[0]

	// Successful sync recorded the correct hash.
	_ = mgr.stateStore.UpdateAppState("home2", "192.168.2.1", s.Name, s.Sha256Hash, time.Now())
	_ = mgr.stateStore.UpdateAppEnabledState("home2", "192.168.2.1", s.Name, true)
	mgr.AgentHub().SetAppHash("home2", s.Name, s.Sha256Hash)
	mgr.AgentHub().SetAppEnabled("home2", s.Name, true)

	// A probe reports an OLD file content (e.g. stale probe right after restart).
	mgr.AgentHub().SetAppFiles("home2", map[string]agent.ProbeConfigFile{
		"/etc/systemd/system/ezapp-vproxy.service": {Path: "/etc/systemd/system/ezapp-vproxy.service", Content: "[Unit]\nDescription=old\n"},
		"/etc/systemd/system/ezapp-vproxy.env":     {Path: "/etc/systemd/system/ezapp-vproxy.env", Content: "OLD=1\n"},
	})

	acts, _ := mgr.PlanSync()
	for _, a := range acts {
		if a.Type == config.ActionSyncApp {
			t.Logf("%s needs=%v status=%s", a.TargetFile, a.NeedsApply, a.DiffStatus)
			if a.NeedsApply {
				t.Errorf("stale probe produced a pending action for %s", a.TargetFile)
			}
		}
	}
	_ = agentpb.AppType_SYSTEMD
}
