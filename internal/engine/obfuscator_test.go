package engine

import (
	"os"
	"strings"
	"testing"

	"easy42/internal/compiler"
	"easy42/internal/config"
)

func newObfuscatorTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "easy42-engine-obfuscator-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	store := config.NewStore(tempDir)
	pass, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}
	mgr := NewManager(store)
	if err := mgr.Unlock(pass); err != nil {
		t.Fatalf("failed to unlock manager: %v", err)
	}
	return mgr, tempDir
}

func addObfuscatorTestNode(t *testing.T, mgr *Manager, name, ip, host, mode string) config.Node {
	t.Helper()
	node := config.Node{
		Name:      name,
		Host:      host,
		IP:        ip,
		Mode:      mode,
		Interface: "lo",
		ASN:       4224420001,
		Entrypoints: []config.Entrypoint{
			{IP: host, Tags: []string{"default"}, MTU: 1500},
		},
	}
	if err := mgr.AddNode(node); err != nil {
		t.Fatalf("failed to add node %s: %v", name, err)
	}
	return node
}

func TestAddLinkGeneratesComplementObfuscatorParameters(t *testing.T) {
	mgr, dir := newObfuscatorTestManager(t)
	defer os.RemoveAll(dir)
	defer mgr.Close()

	nodeA := addObfuscatorTestNode(t, mgr, "alpha-node", "192.168.100.1", "1.1.1.1", "agent")
	nodeB := addObfuscatorTestNode(t, mgr, "beta-node", "192.168.100.2", "2.2.2.2", "agent")

	fromEnd := &config.LinkEnd{Name: "alpha-node", Obfuscator: config.ObfuscatorPhantunClient}
	toEnd := &config.LinkEnd{Name: "beta-node"}
	link, err := mgr.AddLinkWithAllOptions("alpha-node", "beta-node", fromEnd, toEnd, nil, false, false)
	if err != nil {
		t.Fatalf("AddLinkWithAllOptions failed: %v", err)
	}

	// Lexical ordering keeps alpha-node at From and beta-node at To.
	if link.From.ObfuscatorRole() != config.ObfuscatorPhantunClient {
		t.Fatalf("expected client role on From, got %s", link.From.ObfuscatorRole())
	}
	if link.To.ObfuscatorRole() != config.ObfuscatorPhantunServer {
		t.Fatalf("expected server role on To, got %s", link.To.ObfuscatorRole())
	}
	if err := config.ValidateLinkObfuscator(link, &nodeA, &nodeB); err != nil {
		t.Fatalf("generated obfuscator configuration is invalid: %v", err)
	}

	cp := link.From.ObfuscatorParameters
	sp := link.To.ObfuscatorParameters
	if cp == nil || sp == nil {
		t.Fatal("expected generated obfuscator parameters on both ends")
	}
	if cp.LocalPort <= 0 || cp.ServerPort <= 0 {
		t.Fatalf("expected generated client ports, got local=%d server=%d", cp.LocalPort, cp.ServerPort)
	}
	if sp.ServerPort != cp.ServerPort {
		t.Fatalf("expected mirrored server port, client=%d server=%d", cp.ServerPort, sp.ServerPort)
	}
	if sp.WireGuardPort != link.To.ListenPort {
		t.Fatalf("expected server wireguard port %d, got %d", link.To.ListenPort, sp.WireGuardPort)
	}
	if len(cp.TunName) > config.ObfuscatorTunNameMaxLen || len(sp.TunName) > config.ObfuscatorTunNameMaxLen {
		t.Fatalf("tun names exceed Linux limit: %s/%s", cp.TunName, sp.TunName)
	}
	if !strings.HasPrefix(cp.TunName, "tunpt") || !strings.HasPrefix(sp.TunName, "tunpt") {
		t.Fatalf("expected tunpt-prefixed tun names, got %s/%s", cp.TunName, sp.TunName)
	}
	if cp.TunLocal != sp.TunPeer || cp.TunPeer != sp.TunLocal {
		t.Fatalf("expected complementary tun IPs, client=%s/%s server=%s/%s", cp.TunLocal, cp.TunPeer, sp.TunLocal, sp.TunPeer)
	}

	// The WireGuard peer endpoint is loopback on the client and unset on the server.
	if got := compiler.ResolveLinkEndpoint(&nodeA, &nodeB, &link.From, &link.To); got != "127.0.0.1:"+intToString(cp.LocalPort) {
		t.Fatalf("expected loopback client endpoint, got %s", got)
	}
	if got := compiler.ResolveLinkEndpoint(&nodeB, &nodeA, &link.To, &link.From); got != "" {
		t.Fatalf("expected empty server endpoint, got %s", got)
	}

	specs, err := BuildObfuscatorSpecsForNode(mgr.store.Get(), &nodeB)
	if err != nil {
		t.Fatalf("BuildObfuscatorSpecsForNode failed: %v", err)
	}
	if len(specs) != 1 || specs[0].Remote != "127.0.0.1:"+intToString(sp.WireGuardPort) {
		t.Fatalf("unexpected server spec: %+v", specs)
	}
}

func intToString(v int) string {
	if v == 0 {
		return "0"
	}
	digits := make([]byte, 0, 5)
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

func TestUpdateLinkExplicitNoneDisablesObfuscator(t *testing.T) {
	mgr, dir := newObfuscatorTestManager(t)
	defer os.RemoveAll(dir)
	defer mgr.Close()

	addObfuscatorTestNode(t, mgr, "gamma-node", "192.168.100.1", "1.1.1.1", "agent")
	addObfuscatorTestNode(t, mgr, "delta-node", "192.168.100.2", "2.2.2.2", "agent")

	fromEnd := &config.LinkEnd{Name: "gamma-node", Obfuscator: config.ObfuscatorPhantunClient}
	toEnd := &config.LinkEnd{Name: "delta-node"}
	if _, err := mgr.AddLinkWithAllOptions("gamma-node", "delta-node", fromEnd, toEnd, nil, false, false); err != nil {
		t.Fatalf("failed to add obfuscated link: %v", err)
	}

	noneClient := &config.LinkEnd{Name: "gamma-node", Obfuscator: config.ObfuscatorNone}
	updated, err := mgr.UpdateLinkWithAllOptions("gamma-node", "delta-node", noneClient, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to disable obfuscator: %v", err)
	}
	if updated.From.ObfuscatorRole() != config.ObfuscatorNone || updated.To.ObfuscatorRole() != config.ObfuscatorNone {
		t.Fatalf("expected both ends disabled, got %s/%s", updated.From.ObfuscatorRole(), updated.To.ObfuscatorRole())
	}
	if updated.From.ObfuscatorParameters != nil || updated.To.ObfuscatorParameters != nil {
		t.Fatal("expected parameters to be cleared when obfuscator is disabled")
	}
}

func TestBuildLinkRejectsObfuscatorForNonAgentNode(t *testing.T) {
	mgr, dir := newObfuscatorTestManager(t)
	defer os.RemoveAll(dir)
	defer mgr.Close()

	addObfuscatorTestNode(t, mgr, "epsilon-node", "192.168.100.1", "1.1.1.1", "agent")
	addObfuscatorTestNode(t, mgr, "zeta-node", "192.168.100.2", "2.2.2.2", "ssh")

	fromEnd := &config.LinkEnd{Name: "epsilon-node", Obfuscator: config.ObfuscatorPhantunClient}
	toEnd := &config.LinkEnd{Name: "zeta-node"}
	_, err := mgr.AddLinkWithAllOptions("epsilon-node", "zeta-node", fromEnd, toEnd, nil, false, false)
	if err == nil {
		t.Fatal("expected non-agent node to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "agent mode") {
		t.Fatalf("expected agent-mode error, got: %v", err)
	}
}

func TestPlanObfuscatorActionsRejectsSshNode(t *testing.T) {
	mgr, dir := newObfuscatorTestManager(t)
	defer os.RemoveAll(dir)
	defer mgr.Close()

	fromNode := config.Node{Name: "eta-node", Mode: "agent"}
	toNode := config.Node{Name: "theta-node", Mode: "ssh"}
	cfg := &config.Config{
		Nodes: []config.Node{fromNode, toNode},
		Links: []config.Link{
			{
				From: config.LinkEnd{
					Name:                 "eta-node",
					Interface:            "wg42theta-node",
					Obfuscator:           config.ObfuscatorPhantunClient,
					ObfuscatorParameters: &config.ObfuscatorParameters{LocalPort: 30000, ServerPort: 30001, TunName: "tunpttheta", TunLocal: "192.168.200.1", TunPeer: "192.168.200.2"},
				},
				To: config.LinkEnd{
					Name:                 "theta-node",
					Interface:            "wg42eta-node",
					Obfuscator:           config.ObfuscatorPhantunServer,
					ObfuscatorParameters: &config.ObfuscatorParameters{ServerPort: 30001, WireGuardPort: 25532, TunName: "tunpteta", TunLocal: "192.168.200.2", TunPeer: "192.168.200.1"},
				},
			},
		},
	}

	_, err := mgr.planObfuscatorActions(cfg, cfg.Nodes, map[string]bool{"eta-node": true, "theta-node": true}, true, &config.NetworkState{Nodes: map[string]config.StateNode{}})
	if err == nil {
		t.Fatal("expected planning to fail for SSH-mode endpoint")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "agent mode") {
		t.Fatalf("expected agent-mode error, got: %v", err)
	}
}

func TestApplyConfigChangeEnablesAndDisablesObfuscator(t *testing.T) {
	mgr, dir := newObfuscatorTestManager(t)
	defer os.RemoveAll(dir)
	defer mgr.Close()

	addObfuscatorTestNode(t, mgr, "iota-node", "192.168.100.1", "1.1.1.1", "agent")
	addObfuscatorTestNode(t, mgr, "kappa-node", "192.168.100.2", "2.2.2.2", "agent")
	link, err := mgr.AddLink("iota-node", "kappa-node", 51000, 51001, nil)
	if err != nil {
		t.Fatalf("failed to create plain link: %v", err)
	}
	if link.From.ObfuscatorEnabled() || link.To.ObfuscatorEnabled() {
		t.Fatal("plain link must not start with an obfuscator")
	}

	enablePatch := `{
		"links": [
			{
				"from": {"name": "iota-node", "obfuscator": "phantun_client"},
				"to": {"name": "kappa-node"}
			}
		]
	}`
	if _, err := mgr.ApplyConfigChange(enablePatch); err != nil {
		t.Fatalf("failed to apply obfuscator enable patch: %v", err)
	}

	cfg := mgr.store.Get()
	if len(cfg.Links) != 1 {
		t.Fatalf("expected one link, got %d", len(cfg.Links))
	}
	enabled := cfg.Links[0]
	if enabled.From.ObfuscatorRole() != config.ObfuscatorPhantunClient || enabled.To.ObfuscatorRole() != config.ObfuscatorPhantunServer {
		t.Fatalf("expected complementary generated roles, got %s/%s", enabled.From.ObfuscatorRole(), enabled.To.ObfuscatorRole())
	}
	if err := config.ValidateLinkObfuscator(&enabled, &cfg.Nodes[0], &cfg.Nodes[1]); err != nil {
		t.Fatalf("applied obfuscator configuration is invalid: %v", err)
	}
	if enabled.To.ObfuscatorParameters == nil || enabled.To.ObfuscatorParameters.WireGuardPort != 51001 {
		t.Fatalf("expected WG port 51001, got %+v", enabled.To.ObfuscatorParameters)
	}

	disablePatch := `{
		"links": [
			{
				"from": {"name": "iota-node", "obfuscator": "none"},
				"to": {"name": "kappa-node"}
			}
		]
	}`
	if _, err := mgr.ApplyConfigChange(disablePatch); err != nil {
		t.Fatalf("failed to apply obfuscator disable patch: %v", err)
	}

	cfg = mgr.store.Get()
	if len(cfg.Links) != 1 {
		t.Fatalf("expected one link, got %d", len(cfg.Links))
	}
	disabled := cfg.Links[0]
	if disabled.From.ObfuscatorRole() != config.ObfuscatorNone || disabled.To.ObfuscatorRole() != config.ObfuscatorNone {
		t.Fatalf("expected disabled roles, got %s/%s", disabled.From.ObfuscatorRole(), disabled.To.ObfuscatorRole())
	}
	if disabled.From.ObfuscatorParameters != nil || disabled.To.ObfuscatorParameters != nil {
		t.Fatal("expected parameters cleared on disable")
	}
}

func TestSimpleUpdateLinkRefreshesObfuscatorWireGuardPort(t *testing.T) {
	mgr, dir := newObfuscatorTestManager(t)
	defer os.RemoveAll(dir)
	defer mgr.Close()

	addObfuscatorTestNode(t, mgr, "lambda-node", "192.168.100.1", "1.1.1.1", "agent")
	addObfuscatorTestNode(t, mgr, "mu-node", "192.168.100.2", "2.2.2.2", "agent")

	fromEnd := &config.LinkEnd{Name: "lambda-node", Obfuscator: config.ObfuscatorPhantunClient}
	toEnd := &config.LinkEnd{Name: "mu-node"}
	if _, err := mgr.AddLinkWithAllOptions("lambda-node", "mu-node", fromEnd, toEnd, nil, false, false); err != nil {
		t.Fatalf("failed to add obfuscated link: %v", err)
	}

	updated, err := mgr.UpdateLink("lambda-node", "mu-node", 51000, 51010, nil)
	if err != nil {
		t.Fatalf("simple update failed: %v", err)
	}
	if updated.To.ObfuscatorParameters == nil || updated.To.ObfuscatorParameters.WireGuardPort != 51010 {
		t.Fatalf("expected refreshed server WireGuard port 51010, got %+v", updated.To.ObfuscatorParameters)
	}
}
