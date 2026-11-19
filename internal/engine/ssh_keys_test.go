package engine

import (
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"

	"easy42/internal/config"
	"easy42/internal/version"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func sshTestKey(algorithm, payload, comment string) config.SSHPublicKey {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(len(algorithm)))
	buf = append(buf, algorithm...)
	buf = append(buf, payload...)
	return config.SSHPublicKey{
		Algorithm: algorithm,
		Key:       base64.StdEncoding.EncodeToString(buf),
		Comment:   comment,
		Path:      "/root/.ssh/id_ed25519.pub",
	}
}

func newSSHTestManager(t *testing.T) *Manager {
	t.Helper()
	tmpDir := t.TempDir()
	dir := tmpDir
	store := config.NewStore(dir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	mgr := NewManager(store)
	// Close (and flush) the manager before the temporary directory is removed.
	t.Cleanup(func() { mgr.Close(); time.Sleep(50 * time.Millisecond) })
	if err := mgr.Unlock(pass); err != nil {
		t.Fatalf("failed to unlock: %v", err)
	}
	return mgr
}

func saveSSHTestConfig(t *testing.T, mgr *Manager, cfg *config.Config) {
	t.Helper()
	if err := mgr.store.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
}

func findSSHKeyAction(actions []config.SyncAction, node, target string) *config.SyncAction {
	for i := range actions {
		act := actions[i]
		if act.Type == config.ActionSyncSSHKeys && act.NodeName == node && act.TargetFile == target {
			return &act
		}
	}
	return nil
}

func TestSSHKeyAgentVersionSupport(t *testing.T) {
	if !config.SSHKeySupportedByAgent(version.AgentVersion) {
		t.Fatalf("bundled agent version %s must support SSH keys (min %s)", version.AgentVersion, config.MinAgentVersionForSSHKeys)
	}
}

func TestSSHKeysDefaultRulesAreEmpty(t *testing.T) {
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "a", Mode: "agent", AgentToken: "ta", AgentVersion: "0.1.18"},
			{Name: "b", Mode: "agent", AgentToken: "tb", AgentVersion: "0.1.18"},
		},
	})
	key := sshTestKey("ssh-ed25519", "a", "a")
	mgr.agentHub.SetNodeMeta("a", agentNodeMeta("a", "0.1.18", key))

	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}
	for _, act := range actions {
		if act.Type == config.ActionSyncSSHKeys {
			t.Fatalf("no SSH key actions expected without rules, got %+v", act)
		}
	}
}

// agentNodeMeta builds a persisted hub metadata entry for a node agent.
func agentNodeMeta(name, version string, keys ...config.SSHPublicKey) agent.NodeMeta {
	return agent.NodeMeta{
		AgentVersion: version,
		Hostname:     name,
		SSHKeys:      keys,
		DisableApps:  false,
	}
}

func TestSSHKeyRulePlanning(t *testing.T) {
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "src1", Mode: "agent", AgentToken: "t1", AgentVersion: "0.1.18", Tags: []string{"core"}},
			{Name: "src2", Mode: "agent", AgentToken: "t2", AgentVersion: "0.1.18", Tags: []string{"core"}},
			{Name: "dst1", Mode: "agent", AgentToken: "t3", AgentVersion: "0.1.18", Tags: []string{"edge"}},
			{Name: "dst2", Mode: "agent", AgentToken: "t4", AgentVersion: "0.1.18", Tags: []string{"edge"}},
			{Name: "legacy", Mode: "agent", AgentToken: "t5", AgentVersion: "0.1.17", Tags: []string{"core"}},
			{Name: "ssh-only", Mode: "ssh", Tags: []string{"core"}},
		},
		SSHKeyRules: []config.SSHKeyRule{
			{
				ID:          "r1",
				Type:        "all",
				SrcSelector: map[string]string{"tag": "core"},
				DstSelector: map[string]string{"tag": "edge"},
				Enabled:     true,
			},
			{
				ID:          "r2",
				Type:        "known_hosts",
				SrcSelector: map[string]string{"name": "src2"},
				DstSelector: map[string]string{"name": "dst1"},
				Enabled:     true,
			},
			{
				ID:          "r3",
				Type:        "all",
				SrcSelector: map[string]string{"name": "src1"},
				DstSelector: map[string]string{"name": "dst1"},
				Enabled:     false, // disabled rules must be ignored
			},
		},
	})

	src1 := sshTestKey("ssh-ed25519", "src1", "src1")
	src2 := sshTestKey("ssh-rsa", "src2", "src2")
	legacy := sshTestKey("ssh-ed25519", "legacy", "legacy")
	mgr.agentHub.SetNodeMeta("src1", agentNodeMeta("src1", "0.1.18", src1))
	mgr.agentHub.SetNodeMeta("src2", agentNodeMeta("src2", "0.1.18", src2))
	mgr.agentHub.SetNodeMeta("legacy", agentNodeMeta("legacy", "0.1.17", legacy))
	mgr.agentHub.SetNodeMeta("dst1", agentNodeMeta("dst1", "0.1.18"))
	mgr.agentHub.SetNodeMeta("dst2", agentNodeMeta("dst2", "0.1.18"))

	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}

	// dst1: rule r1 (all) + r2 (known_hosts) -> both files.
	for _, path := range []string{config.SSHKnownHostsPath, config.SSHAuthorizedKeysPath} {
		act := findSSHKeyAction(actions, "dst1", path)
		if act == nil {
			t.Fatalf("expected SSH key action for dst1 %s", path)
		}
		if !act.NeedsApply {
			t.Fatalf("expected NeedsApply=true for dst1 %s", path)
		}
		if !strings.Contains(act.FileContent, src1.Key) || !strings.Contains(act.FileContent, src2.Key) {
			t.Fatalf("expected dst1 %s block to contain both core keys, got:\n%s", path, act.FileContent)
		}
		if strings.Contains(act.FileContent, legacy.Key) {
			t.Fatalf("legacy agent key must not be distributed")
		}
	}
	// authorized_keys entries are marked with the easy42 comment prefix.
	authAct := findSSHKeyAction(actions, "dst1", config.SSHAuthorizedKeysPath)
	if !strings.Contains(authAct.FileContent, "easy42 src1") {
		t.Fatalf("expected easy42 marker comment, got:\n%s", authAct.FileContent)
	}

	// dst2 is only covered by rule r1.
	if act := findSSHKeyAction(actions, "dst2", config.SSHKnownHostsPath); act == nil || !strings.Contains(act.FileContent, src2.Key) {
		t.Fatalf("expected dst2 known_hosts to be managed by rule r1")
	}
	// The disabled rule r3 would target dst1 with src1 only; its effect is
	// indistinguishable from r1 here except that knowledge, so assert r3 never
	// appears alone: dst2 (not selected by r3) must not be affected by it.
	if act := findSSHKeyAction(actions, "dst2", config.SSHAuthorizedKeysPath); act == nil {
		t.Fatal("expected dst2 authorized_keys from rule r1")
	}
	// Sources are never targets.
	if act := findSSHKeyAction(actions, "src1", config.SSHKnownHostsPath); act != nil {
		t.Fatalf("source nodes must not be targeted, got %+v", act)
	}

	// Record the dst1 hashes as applied -> no more pending actions.
	for _, path := range []string{config.SSHKnownHostsPath, config.SSHAuthorizedKeysPath} {
		act := findSSHKeyAction(actions, "dst1", path)
		_ = mgr.stateStore.UpdateSSHKeyState("dst1", "dst1", path, config.HashSSHKeysBlock(act.FileContent), time.Now())
		mgr.agentHub.SetNodeMeta("dst1", agent.NodeMeta{
			AgentVersion:     "0.1.18",
			SSHKeyFileHashes: map[string]string{path: config.HashSSHKeysBlock(act.FileContent)},
		})
	}
	actionsAfter, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync after apply failed: %v", err)
	}
	for _, path := range []string{config.SSHKnownHostsPath, config.SSHAuthorizedKeysPath} {
		act := findSSHKeyAction(actionsAfter, "dst1", path)
		if act == nil {
			t.Fatalf("expected SSH key action to remain listed for dst1 %s", path)
		}
		if act.NeedsApply {
			t.Fatalf("expected NeedsApply=false for dst1 %s after apply", path)
		}
	}

	// A source key change must invalidate the target plan.
	rotatedKey := sshTestKey("ssh-ed25519", "rotated", "src1")
	mgr.agentHub.SetNodeMeta("src1", agentNodeMeta("src1", "0.1.18", rotatedKey))
	actionsRotated, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync after rotation failed: %v", err)
	}
	rotated := findSSHKeyAction(actionsRotated, "dst1", config.SSHKnownHostsPath)
	if rotated == nil || !rotated.NeedsApply {
		t.Fatalf("expected pending SSH key action after source key rotation")
	}
	if !strings.Contains(rotated.FileContent, rotatedKey.Key) || strings.Contains(rotated.FileContent, src1.Key) {
		t.Fatalf("expected rotated key in block, got:\n%s", rotated.FileContent)
	}
}

func TestSSHKeyCollectionFromTelemetry(t *testing.T) {
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "n1", Mode: "agent", AgentToken: "t1", AgentVersion: "0.1.18"},
		},
	})

	key := sshTestKey("ssh-ed25519", "n1", "n1")
	mgr.agentHub.SetNodeMeta("n1", agent.NodeMeta{AgentVersion: "0.1.18"})
	mgr.agentHub.UpdateTelemetry("n1", &agentpb.TelemetryReport{
		SshKeys: []*agentpb.SshPublicKey{
			{Algorithm: key.Algorithm, Key: key.Key, Comment: key.Comment, Path: key.Path},
		},
	})

	// Wait for the asynchronous telemetry listener.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if keys := mgr.GetNodeSSHKeys("n1"); len(keys) == 1 {
			if keys[0].Key != key.Key {
				t.Fatalf("unexpected collected key: %+v", keys[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("telemetry SSH keys were not collected")
		}
		time.Sleep(10 * time.Millisecond)
	}

	overview := mgr.GetSSHKeysOverview()
	if len(overview) != 1 || !overview[0].Supported || len(overview[0].Keys) != 1 {
		t.Fatalf("unexpected overview: %+v", overview)
	}

	summary := mgr.GetSSHKeysSummary()
	if summary.NodesWithKeys != 1 || summary.TotalKeys != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestSSHKeysDisabledWhenAppsDisabled(t *testing.T) {
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "locked", Mode: "agent", AgentToken: "t1", AgentVersion: "0.1.18"},
			{Name: "target", Mode: "agent", AgentToken: "t2", AgentVersion: "0.1.18"},
		},
		SSHKeyRules: []config.SSHKeyRule{
			{ID: "r", Type: "all", SrcSelector: map[string]string{"name": "locked"}, DstSelector: map[string]string{"name": "target"}, Enabled: true},
		},
	})
	key := sshTestKey("ssh-ed25519", "locked", "locked")
	mgr.agentHub.SetNodeMeta("locked", agent.NodeMeta{AgentVersion: "0.1.18", DisableApps: true, SSHKeys: []config.SSHPublicKey{key}})
	mgr.agentHub.SetNodeMeta("target", agent.NodeMeta{AgentVersion: "0.1.18"})

	overview := mgr.GetSSHKeysOverview()
	for _, n := range overview {
		if n.Name == "locked" && n.Supported {
			t.Fatalf("node with disable_apps must not support SSH keys: %+v", n)
		}
	}

	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}
	if act := findSSHKeyAction(actions, "target", config.SSHAuthorizedKeysPath); act != nil {
		t.Fatalf("keys from a disable_apps node must not be distributed, got %+v", act)
	}
}

func TestSSHKeyRulesCRUD(t *testing.T) {
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "a", Mode: "agent", AgentToken: "ta"},
			{Name: "b", Mode: "agent", AgentToken: "tb"},
		},
	})

	if _, err := mgr.SaveSSHKeyRule(config.SSHKeyRule{
		Type:        "bogus",
		SrcSelector: map[string]string{"name": "a"},
		DstSelector: map[string]string{"name": "b"},
	}); err == nil {
		t.Fatal("expected invalid type error")
	}
	if _, err := mgr.SaveSSHKeyRule(config.SSHKeyRule{
		Type:        "all",
		SrcSelector: map[string]string{"name": "a"},
	}); err == nil {
		t.Fatal("expected empty destination selector error")
	}

	saved, err := mgr.SaveSSHKeyRule(config.SSHKeyRule{
		Type:        "ALL",
		SrcSelector: map[string]string{" name ": "a"},
		DstSelector: map[string]string{"name": "b"},
		Enabled:     true,
	})
	if err != nil {
		t.Fatalf("SaveSSHKeyRule failed: %v", err)
	}
	if saved.ID == "" || saved.Type != config.SSHKeyRuleTypeAll {
		t.Fatalf("unexpected saved rule: %+v", saved)
	}
	if _, ok := saved.SrcSelector["name"]; !ok {
		t.Fatalf("expected selector keys to be trimmed, got %+v", saved.SrcSelector)
	}

	toggled, err := mgr.ToggleSSHKeyRule(saved.ID)
	if err != nil || toggled.Enabled {
		t.Fatalf("expected rule to be disabled after toggle, got %+v (%v)", toggled, err)
	}

	// Renaming a node must update the rule selectors.
	if _, err := mgr.RenameNode("b", "b2"); err != nil {
		t.Fatalf("RenameNode failed: %v", err)
	}
	rule, err := mgr.GetSSHKeyRule(saved.ID)
	if err != nil {
		t.Fatalf("GetSSHKeyRule failed: %v", err)
	}
	if rule.DstSelector["name"] != "b2" {
		t.Fatalf("expected rename to update the destination selector, got %+v", rule.DstSelector)
	}

	// Deleting the only destination node removes the rule.
	if err := mgr.DeleteNode("b2"); err != nil {
		t.Fatalf("DeleteNode failed: %v", err)
	}
	if rules := mgr.GetSSHKeyRules(); len(rules) != 0 {
		t.Fatalf("expected rule to be pruned after its destination node was deleted, got %+v", rules)
	}

	// Deleting a missing rule is an error.
	if err := mgr.DeleteSSHKeyRule("nope"); err == nil {
		t.Fatal("expected error deleting an unknown rule")
	}
}

func TestSSHKeySyncActionRequiresConnectedAgent(t *testing.T) {
	mgr := newSSHTestManager(t)
	res := &config.SyncResult{}
	handled := mgr.executeSSHKeySyncAction(config.SyncAction{
		NodeName:   "nope",
		Type:       config.ActionSyncSSHKeys,
		TargetFile: config.SSHKnownHostsPath,
	}, res)
	if !handled || res.Success {
		t.Fatalf("expected handled failure for disconnected node, got handled=%v res=%+v", handled, res)
	}

	if mgr.executeSSHKeySyncAction(config.SyncAction{Type: config.ActionSyncHosts}, res) {
		t.Fatal("non SSH key actions must not be handled")
	}
}

func TestSSHKeyCleanupWhenNodeLeavesAgentMode(t *testing.T) {
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "src", Mode: "agent", AgentToken: "t1", AgentVersion: "0.1.18", Host: "10.0.0.1", IP: "192.168.100.1"},
			{Name: "dst", Mode: "agent", AgentToken: "t2", AgentVersion: "0.1.18", Host: "10.0.0.2", IP: "192.168.100.2"},
		},
		SSHKeyRules: []config.SSHKeyRule{
			{ID: "r", Type: "all", SrcSelector: map[string]string{"name": "src"}, DstSelector: map[string]string{"name": "dst"}, Enabled: true},
		},
	})
	mgr.agentHub.SetNodeMeta("src", agentNodeMeta("src", "0.1.18", sshTestKey("ssh-ed25519", "src", "src")))
	mgr.agentHub.SetNodeMeta("dst", agentNodeMeta("dst", "0.1.18"))

	// Pretend the files were applied earlier.
	for _, path := range []string{config.SSHKnownHostsPath, config.SSHAuthorizedKeysPath} {
		_ = mgr.stateStore.UpdateSSHKeyState("dst", "dst", path, "previous-hash", time.Now())
	}

	// The node is still connected over its agent socket, so it can be cleaned up.
	var mu sync.Mutex
	var targets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := agent.NewConnection("dst", ws, mgr.AgentHub())
		mgr.AgentHub().Register(conn)
		go conn.ReadLoop()
	}))
	defer server.Close()
	cws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("failed to connect mock agent: %v", err)
	}
	defer cws.Close()
	go func() {
		for {
			_, data, err := cws.ReadMessage()
			if err != nil {
				return
			}
			var msg agentpb.AgentMessage
			if proto.Unmarshal(data, &msg) != nil {
				continue
			}
			req, ok := msg.Payload.(*agentpb.AgentMessage_CommandReq)
			if !ok {
				continue
			}
			if sync := req.CommandReq.GetSyncAction(); sync != nil {
				mu.Lock()
				targets = append(targets, sync.TargetFile)
				mu.Unlock()
			}
			resp := &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_CommandResp{CommandResp: &agentpb.CommandResponse{
				RequestId: req.CommandReq.RequestId,
				Success:   true,
				Stdout:    "ok",
			}}}
			if b, err := proto.Marshal(resp); err == nil {
				_ = cws.WriteMessage(websocket.BinaryMessage, b)
			}
		}
	}()
	waitForCondition(t, func() bool { return mgr.AgentHub().IsConnected("dst") })

	// Switching the destination to SSH mode removes it from the plan, but the
	// previously written blocks must be cleaned up. A node is only considered
	// SSH mode once its agent token is gone (IsAgentMode checks it), which is
	// the state produced by "uninstall agent" / manual edits.
	cfg := mgr.store.Get()
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == "dst" {
			cfg.Nodes[i].Mode = "ssh"
			cfg.Nodes[i].AgentToken = ""
		}
	}
	if err := mgr.store.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}
	if n := mgr.FindNode("dst"); n == nil || n.IsAgentMode() {
		t.Fatalf("expected dst to be in SSH mode, got %+v", n)
	}

	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}
	for _, path := range []string{config.SSHKnownHostsPath, config.SSHAuthorizedKeysPath} {
		var cleanup *config.SyncAction
		for i := range actions {
			act := &actions[i]
			if act.NodeName != "dst" || act.TargetFile != path {
				continue
			}
			if act.Type == config.ActionSyncSSHKeys {
				t.Fatalf("node in SSH mode must not receive SSH key sync actions: %+v", act)
			}
			if act.Type == config.ActionCleanupSSHKeys {
				cleanup = act
			}
		}
		if cleanup == nil {
			t.Fatalf("expected cleanup action for dst %s", path)
		}
		if cleanup.DiffStatus != "delete" || !cleanup.NeedsApply {
			t.Fatalf("unexpected cleanup action: %+v", cleanup)
		}
	}

	// Executing the plan removes the blocks on the node and clears the record.
	if _, err := mgr.ExecuteSync(true); err != nil {
		t.Fatalf("ExecuteSync failed: %v", err)
	}
	mu.Lock()
	gotTargets := append([]string(nil), targets...)
	mu.Unlock()
	var sshTargets []string
	for _, target := range gotTargets {
		if config.IsSSHKeyTargetPath(target) {
			sshTargets = append(sshTargets, target)
		}
	}
	if len(sshTargets) != 2 {
		t.Fatalf("expected 2 SSH key file writes on the node, got %v (all writes: %v)", sshTargets, gotTargets)
	}
	st := mgr.stateStore.Get()
	if len(st.Nodes["dst"].SSHKeyFileHashes) != 0 {
		t.Fatalf("expected SSH key state to be cleared, got %+v", st.Nodes["dst"].SSHKeyFileHashes)
	}
}

// waitForCondition polls cond until it returns true (or fails the test after 3s).
func waitForCondition(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition not met within timeout")
}

func TestSSHKeyRuleAllNodesTargeting(t *testing.T) {
	mgr := newSSHTestManager(t)
	saveSSHTestConfig(t, mgr, &config.Config{
		Nodes: []config.Node{
			{Name: "core1", Mode: "agent", AgentToken: "t1", AgentVersion: "0.1.18", Tags: []string{"core"}},
			{Name: "edge1", Mode: "agent", AgentToken: "t2", AgentVersion: "0.1.18"},
			{Name: "edge2", Mode: "agent", AgentToken: "t3", AgentVersion: "0.1.18"},
			{Name: "other", Mode: "agent", AgentToken: "t4", AgentVersion: "0.1.18"},
		},
		SSHKeyRules: []config.SSHKeyRule{
			{
				ID:          "all-to-all",
				Type:        "all",
				SrcSelector: config.AllNodesSelector(),
				DstSelector: config.AllNodesSelector(),
				Enabled:     true,
			},
		},
	})
	coreKey := sshTestKey("ssh-ed25519", "core", "core1")
	mgr.agentHub.SetNodeMeta("core1", agentNodeMeta("core1", "0.1.18", coreKey))
	for _, name := range []string{"edge1", "edge2", "other"} {
		mgr.agentHub.SetNodeMeta(name, agentNodeMeta(name, "0.1.18"))
	}

	actions, err := mgr.PlanSync()
	if err != nil {
		t.Fatalf("PlanSync failed: %v", err)
	}
	// Every node except the source must receive the core key in both files.
	for _, name := range []string{"edge1", "edge2", "other"} {
		for _, path := range []string{config.SSHKnownHostsPath, config.SSHAuthorizedKeysPath} {
			act := findSSHKeyAction(actions, name, path)
			if act == nil || !strings.Contains(act.FileContent, coreKey.Key) {
				t.Fatalf("expected %s %s to include the core key via the all-nodes rule, got %+v", name, path, act)
			}
		}
	}
	if act := findSSHKeyAction(actions, "core1", config.SSHAuthorizedKeysPath); act != nil {
		t.Fatalf("source nodes must not target themselves, got %+v", act)
	}
}
