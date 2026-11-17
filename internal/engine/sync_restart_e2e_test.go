package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"easy42/internal/agent"
	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

func TestE2ESyncThenRestart(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "e2e-*")
	defer os.RemoveAll(tempDir)
	store := config.NewStore(tempDir)
	pass, _ := store.Initialize()
	mgr := NewManager(store)
	_ = mgr.Unlock(pass)
	_ = mgr.AddNode(config.Node{Name: "n1", Host: "192.168.10.30", IP: "10.42.0.30/24", Mode: "agent"})
	mgr.SaveApp(config.App{ID: "a1", Name: "app1", AppType: "docker", Enabled: true, Published: true, Selector: map[string]string{"name": "n1"}, MainContent: "services:\n  x:\n    image: nginx\n"})
	mgr.SaveApp(config.App{ID: "a2", Name: "app2", AppType: "systemd", Enabled: false, Published: true, Selector: map[string]string{"name": "n1"}, MainContent: "[Unit]\n[Service]\nExecStart=/bin/true\n"})
	specs, _ := mgr.BuildAppSpecsForNode("n1")
	hashes := map[string]string{}
	files := map[string]agent.ProbeConfigFile{}
	for _, s := range specs {
		hashes[s.Name] = s.Sha256Hash
		if s.AppType == agentpb.AppType_DOCKER_COMPOSE {
			files["/etc/ezappc/ezapp-"+s.Name+"/compose.yaml"] = agent.ProbeConfigFile{Path: "/etc/ezappc/ezapp-" + s.Name + "/compose.yaml", Content: s.MainContent}
			files["/etc/ezappc/ezapp-"+s.Name+"/.env"] = agent.ProbeConfigFile{Path: "/etc/ezappc/ezapp-" + s.Name + "/.env", Content: s.EnvContent}
		} else {
			files["/etc/systemd/system/ezapp-"+s.Name+".service"] = agent.ProbeConfigFile{Path: "/etc/systemd/system/ezapp-" + s.Name + ".service", Content: s.MainContent}
			files["/etc/systemd/system/ezapp-"+s.Name+".env"] = agent.ProbeConfigFile{Path: "/etc/systemd/system/ezapp-" + s.Name + ".env", Content: s.EnvContent}
		}
	}
	probeJSON, _ := json.Marshal(agentProbeResult{Hostname: "n1", AppHashes: hashes, AppFiles: files})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := agent.NewConnection("n1", ws, mgr.AgentHub())
		mgr.AgentHub().Register(conn)
		go conn.ReadLoop()
	}))
	defer server.Close()
	cws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
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
			if req, ok := msg.Payload.(*agentpb.AgentMessage_CommandReq); ok {
				resp := &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_CommandResp{CommandResp: &agentpb.CommandResponse{RequestId: req.CommandReq.RequestId, Success: true, Stdout: string(probeJSON)}}}
				b, _ := proto.Marshal(resp)
				_ = cws.WriteMessage(websocket.BinaryMessage, b)
			}
		}
	}()
	time.Sleep(200 * time.Millisecond)

	// "Sync everything"
	if _, err := mgr.ExecuteSync(); err != nil {
		t.Fatal(err)
	}
	// Agent reports app statuses
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
	check("after restart", mgr2)
}

// TestE2ESyncThenRestartWithRegisteredNodeMeta reproduces the phantom
// "Deploy app ..." pending changes seen after a server restart: the app spec
// depends on the agent-reported os_info (arch), which used to live only on the
// live connection and was lost until the agent reconnected. The registration
// metadata must be persisted in SQLite and reused while the agent is offline.
func TestE2ESyncThenRestartWithRegisteredNodeMeta(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "e2e-meta-*")
	defer os.RemoveAll(tempDir)
	store := config.NewStore(tempDir)
	pass, _ := store.Initialize()
	mgr := NewManager(store)
	_ = mgr.Unlock(pass)
	_ = mgr.AddNode(config.Node{Name: "n1", Host: "192.168.10.30", IP: "10.42.0.30/24", Mode: "agent"})
	// The env file embeds ${ARCH}, so the desired spec hash depends on the
	// agent-reported os_info. If os_info is lost on restart, the desired hash
	// changes and the app shows up as a pending "Deploy app" change.
	if _, err := mgr.SaveApp(config.App{
		ID:          "a1",
		Name:        "app1",
		AppType:     "docker",
		Enabled:     true,
		Published:   true,
		Selector:    map[string]string{"name": "n1"},
		MainContent: "services:\n  x:\n    image: nginx\n",
		EnvContent:  "ARCH=${ARCH}\n",
	}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conn := agent.NewConnection("n1", ws, mgr.AgentHub())
		mgr.AgentHub().Register(conn)
		go conn.ReadLoop()
	}))
	defer server.Close()
	cws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cws.Close()

	// Register with a non-default arch so the app spec depends on os_info.
	regMsg := &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_RegisterReq{RegisterReq: &agentpb.RegisterRequest{
		NodeName:     "n1",
		AgentVersion: "1.0.0",
		Hostname:     "n1-host",
		OsInfo:       "linux aarch64 little",
		InitSystem:   "systemd",
	}}}
	regData, _ := proto.Marshal(regMsg)
	if err := cws.WriteMessage(websocket.BinaryMessage, regData); err != nil {
		t.Fatal(err)
	}

	// Answer agent probes with the specs as they are right now (registered,
	// arch from os_info = aarch64).
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
			if req, ok := msg.Payload.(*agentpb.AgentMessage_CommandReq); ok {
				specs, _ := mgr.BuildAppSpecsForNode("n1")
				hashes := map[string]string{}
				files := map[string]agent.ProbeConfigFile{}
				for _, s := range specs {
					hashes[s.Name] = s.Sha256Hash
					if s.AppType == agentpb.AppType_DOCKER_COMPOSE {
						files["/etc/ezappc/ezapp-"+s.Name+"/compose.yaml"] = agent.ProbeConfigFile{Path: "/etc/ezappc/ezapp-" + s.Name + "/compose.yaml", Content: s.MainContent}
						files["/etc/ezappc/ezapp-"+s.Name+"/.env"] = agent.ProbeConfigFile{Path: "/etc/ezappc/ezapp-" + s.Name + "/.env", Content: s.EnvContent}
					} else {
						files["/etc/systemd/system/ezapp-"+s.Name+".service"] = agent.ProbeConfigFile{Path: "/etc/systemd/system/ezapp-" + s.Name + ".service", Content: s.MainContent}
						files["/etc/systemd/system/ezapp-"+s.Name+".env"] = agent.ProbeConfigFile{Path: "/etc/systemd/system/ezapp-" + s.Name + ".env", Content: s.EnvContent}
					}
				}
				probeJSON, _ := json.Marshal(agentProbeResult{Hostname: "n1", AppHashes: hashes, AppFiles: files})
				resp := &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_CommandResp{CommandResp: &agentpb.CommandResponse{RequestId: req.CommandReq.RequestId, Success: true, Stdout: string(probeJSON)}}}
				b, _ := proto.Marshal(resp)
				_ = cws.WriteMessage(websocket.BinaryMessage, b)
			}
		}
	}()
	time.Sleep(200 * time.Millisecond)

	if got := mgr.AgentHub().GetOSInfo("n1"); got != "linux aarch64 little" {
		t.Fatalf("expected os_info from registration, got %q", got)
	}

	// "Sync everything"
	if _, err := mgr.ExecuteSync(); err != nil {
		t.Fatal(err)
	}
	mgr.AgentHub().UpdateTelemetry("n1", &agentpb.TelemetryReport{Apps: []*agentpb.AppStatus{
		{Name: "app1", State: "running"},
	}})
	time.Sleep(200 * time.Millisecond)

	check := func(label string, m *Manager) {
		acts, err := m.PlanSync()
		if err != nil {
			t.Fatalf("%s: PlanSync failed: %v", label, err)
		}
		for _, a := range acts {
			if (a.Type == config.ActionSyncApp || a.Type == config.ActionDeleteApp) && a.NeedsApply {
				t.Errorf("%s: pending app action %s (%s) diff_status=%s", label, a.Interface, a.TargetFile, a.DiffStatus)
			}
		}
	}
	check("before restart", mgr)
	mgr.Close()

	// Restart: the agent has not reconnected yet, but its registration
	// metadata (os_info) must be restored from SQLite so the desired app spec
	// still matches the deployed one.
	store2 := config.NewStore(tempDir)
	if _, err := store2.Load(); err != nil {
		t.Fatal(err)
	}
	mgr2 := NewManager(store2)
	defer mgr2.Close()

	if got := mgr2.AgentHub().GetOSInfo("n1"); got != "linux aarch64 little" {
		t.Errorf("after restart: os_info not restored from SQLite, got %q", got)
	}
	check("after restart", mgr2)
}
