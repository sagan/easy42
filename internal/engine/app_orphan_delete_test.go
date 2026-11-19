package engine

import (
	"testing"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/apps"
	"easy42/internal/config"
)

// scriptOrphanDeleteActions returns the app actions planned for appName on the
// test node. appInput, when non-nil, registers the app as desired.
func scriptOrphanDeleteActions(t *testing.T, mgr *Manager) []config.SyncAction {
	t.Helper()
	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}
	return actions
}

func findAppAction(actions []config.SyncAction, appName string) *config.SyncAction {
	for i := range actions {
		if actions[i].Interface == appName {
			return &actions[i]
		}
	}
	return nil
}

// A disabled/removed script app must be garbage collected via /etc/ezscript/<name>
// and the plan must converge once the script is gone. Regression: the fallback
// target used to be the (nonexistent) systemd unit, which never disappeared
// because a stale live status kept the app in the deployed set.
func TestScriptOrphanDeleteTargetsScriptFileAndConverges(t *testing.T) {
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "alihk", Host: "alihk.e.sagan.me", Mode: "agent", AgentToken: "t1", AgentVersion: "0.1.18"},
		},
	})

	const appName = "script-test"
	const scriptPath = "/etc/ezscript/script-test"
	const content = "#!/bin/sh\necho hi\n"

	// The app was applied before and is not desired anymore (disabled).
	_ = mgr.stateStore.UpdateAppState("alihk", "alihk.e.sagan.me", appName, "somehash", time.Now())

	// The agent probe still sees the script and reports its status.
	mgr.agentHub.SetAppFiles("alihk", map[string]agent.ProbeConfigFile{
		scriptPath: {Path: scriptPath, Content: content, Hash: apps.HashSingleContent(content)},
	})
	mgr.agentHub.SetAppHashes("alihk", map[string]string{appName: "somehash"})
	mgr.agentHub.UpdateTelemetry("alihk", &agentpb.TelemetryReport{
		Apps: []*agentpb.AppStatus{{Name: appName, AppType: agentpb.AppType_SCRIPT, State: "stopped"}},
	})

	act := findAppAction(scriptOrphanDeleteActions(t, mgr), appName)
	if act == nil {
		t.Fatal("expected a delete action for the unassigned script app")
	}
	if act.Type != config.ActionDeleteApp || !act.NeedsApply {
		t.Fatalf("expected a pending delete action, got %+v", act)
	}
	if act.TargetFile != scriptPath {
		t.Fatalf("expected target %s, got %s (%s)", scriptPath, act.TargetFile, act.Description)
	}

	// After the agent removed the script the probe is empty, but telemetry may
	// still report the app for one more cycle. The plan must keep targeting the
	// script (never the systemd unit) and must converge once the stale records
	// are cleaned by the delete execution.
	mgr.agentHub.SetAppFiles("alihk", map[string]agent.ProbeConfigFile{})
	mgr.agentHub.SetAppHashes("alihk", map[string]string{})

	act = findAppAction(scriptOrphanDeleteActions(t, mgr), appName)
	if act == nil {
		t.Fatal("expected the plan to still clean up the stale status")
	}
	if act.TargetFile != scriptPath {
		t.Fatalf("stale script status must not produce a systemd target, got %s", act.TargetFile)
	}

	// Deleting the app clears its recorded state and cached status.
	_ = mgr.stateStore.RemoveAppState("alihk", appName)
	mgr.removeAppStatus("alihk", appName)

	if act := findAppAction(scriptOrphanDeleteActions(t, mgr), appName); act != nil {
		t.Fatalf("expected no action for a fully removed app, got %+v", act)
	}
}

// A script app whose file is already gone (probe empty) and whose type is only
// known from a live status must still be deleted via /etc/ezscript/<name>.
func TestOrphanScriptAppWithoutProbedFileUsesScriptTarget(t *testing.T) {
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "n1", Host: "10.0.0.1", Mode: "agent", AgentToken: "t1", AgentVersion: "0.1.18"},
		},
	})

	mgr.agentHub.UpdateTelemetry("n1", &agentpb.TelemetryReport{
		Apps: []*agentpb.AppStatus{{Name: "script-test", AppType: agentpb.AppType_SCRIPT, State: "stopped"}},
	})

	act := findAppAction(scriptOrphanDeleteActions(t, mgr), "script-test")
	if act == nil {
		t.Fatal("expected a delete action for the orphaned script app")
	}
	if act.TargetFile != "/etc/ezscript/script-test" {
		t.Fatalf("expected the script file target, got %s (%s)", act.TargetFile, act.Description)
	}
}

// A non-script orphan keeps the systemd unit target when its type is unknown.
func TestOrphanAppWithoutTypeKeepsServiceTarget(t *testing.T) {
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "n1", Host: "10.0.0.1", Mode: "agent", AgentToken: "t1", AgentVersion: "0.1.18"},
		},
	})

	_ = mgr.stateStore.UpdateAppState("n1", "10.0.0.1", "web-app", "somehash", time.Now())

	act := findAppAction(scriptOrphanDeleteActions(t, mgr), "web-app")
	if act == nil {
		t.Fatal("expected a delete action for the orphaned app")
	}
	if act.TargetFile != "/etc/systemd/system/ezapp-web-app.service" {
		t.Fatalf("expected the service unit target, got %s (%s)", act.TargetFile, act.Description)
	}
}
