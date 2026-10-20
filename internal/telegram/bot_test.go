package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"easy42/internal/config"
	"easy42/internal/lookingglass"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type mockClusterProvider struct {
	summary     config.FleetMetricsSummary
	nodes       []config.NodeLiveStatus
	configNodes []config.Node
	links       []config.Link
	netState    *config.NetworkState
	runResponse lookingglass.RunResponse
	err         error
}

func (m *mockClusterProvider) GetFleetLiveStatus() (config.FleetMetricsSummary, []config.NodeLiveStatus, error) {
	if m.err != nil {
		return config.FleetMetricsSummary{}, nil, m.err
	}
	return m.summary, m.nodes, nil
}

func (m *mockClusterProvider) GetNodes() []config.Node {
	return m.configNodes
}

func (m *mockClusterProvider) GetLinks() []config.Link {
	return m.links
}

func (m *mockClusterProvider) GetNetworkState() *config.NetworkState {
	if m.netState != nil {
		return m.netState
	}
	return &config.NetworkState{}
}

func (m *mockClusterProvider) RunLookingGlass(ctx context.Context, req lookingglass.RunRequest) (lookingglass.RunResponse, error) {
	if m.err != nil {
		return lookingglass.RunResponse{}, m.err
	}
	return m.runResponse, nil
}

func TestFormatHelpers(t *testing.T) {
	if got := formatBytesRate(0); got != "0 B/s" {
		t.Errorf("expected '0 B/s', got %q", got)
	}
	if got := formatBytesRate(500); got != "500 B/s" {
		t.Errorf("expected '500 B/s', got %q", got)
	}
	if got := formatBytesRate(1024 * 1024 * 5.5); !strings.Contains(got, "MB/s") {
		t.Errorf("expected MB/s, got %q", got)
	}

	if got := formatUptime(0); got != "just started" {
		t.Errorf("expected 'just started', got %q", got)
	}
	if got := formatUptime(90000); got != "1d 1h" {
		t.Errorf("expected '1d 1h', got %q", got)
	}
	if got := formatUptime(3660); got != "1h 1m" {
		t.Errorf("expected '1h 1m', got %q", got)
	}
}

func TestBotServiceLifecycle(t *testing.T) {
	mock := &mockClusterProvider{
		summary: config.FleetMetricsSummary{
			TotalNodes:  3,
			OnlineNodes: 2,
		},
	}
	svc := NewBotService(mock)

	// Start with disabled
	err := svc.Start(config.IntegrationsSettings{
		TelegramEnabled: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info := svc.GetBotInfo()
	if info.WebhookStatus != "disabled" {
		t.Errorf("expected webhookStatus 'disabled', got %q", info.WebhookStatus)
	}

	svc.Stop()
}

func TestNodeStatusTransitionDetection(t *testing.T) {
	mock := &mockClusterProvider{
		summary: config.FleetMetricsSummary{
			TotalNodes:  2,
			OnlineNodes: 2,
		},
		nodes: []config.NodeLiveStatus{
			{Name: "node1", Hostname: "host1", Connected: true},
			{Name: "node2", Hostname: "host2", Connected: true},
		},
	}

	svc := NewBotService(mock)
	svc.startupGracePeriod = 0 // bypass startup grace period for this transition test
	svc.debounceCount = 2
	svc.settings = config.IntegrationsSettings{
		TelegramEnabled: true,
		TelegramAdminID: 12345,
		ServerURL:       "https://e42.test",
	}

	var alerts []string
	svc.onAlert = func(text string) {
		alerts = append(alerts, text)
	}

	// First run: initializes baseline without alerts
	svc.checkNodeStatusTransitions()
	if !svc.monitorInitDone {
		t.Fatal("expected monitorInitDone to be true after first run")
	}
	if !svc.prevNodeOnline["node1"] || !svc.prevNodeOnline["node2"] {
		t.Fatal("expected both nodes recorded online initially")
	}
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts on initial baseline, got %d", len(alerts))
	}

	// Second run: node2 goes offline for 1 cycle (pending state, debounced)
	mock.nodes[1].Connected = false
	mock.summary.OnlineNodes = 1
	mock.summary.OfflineNodes = 1

	svc.checkNodeStatusTransitions()
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts after 1 cycle due to debounce, got: %v", alerts)
	}

	// Third run: node2 stays offline for 2nd consecutive cycle -> state stabilized!
	svc.checkNodeStatusTransitions()
	if svc.prevNodeOnline["node2"] {
		t.Errorf("expected node2 to be recorded as offline in prevNodeOnline")
	}
	if len(alerts) != 1 || !strings.Contains(alerts[0], "Node Offline") || !strings.Contains(alerts[0], "node2") {
		t.Errorf("expected Node Offline alert for node2 after debounce, got: %v", alerts)
	}

	// Fourth run: node2 comes back online for 1 cycle (pending state, debounced)
	mock.nodes[1].Connected = true
	mock.summary.OnlineNodes = 2
	mock.summary.OfflineNodes = 0

	svc.checkNodeStatusTransitions()
	if len(alerts) != 1 {
		t.Errorf("expected still 1 alert after 1st cycle back online due to debounce, got: %v", alerts)
	}

	// Fifth run: node2 stays online for 2nd consecutive cycle -> state stabilized!
	svc.checkNodeStatusTransitions()
	if !svc.prevNodeOnline["node2"] {
		t.Errorf("expected node2 to be recorded as online in prevNodeOnline")
	}
	if len(alerts) != 2 || !strings.Contains(alerts[1], "Node Online") || !strings.Contains(alerts[1], "node2") {
		t.Errorf("expected Node Online alert for node2 after debounce, got: %v", alerts)
	}
}

func TestNodeFlappingDebounced(t *testing.T) {
	mock := &mockClusterProvider{
		summary: config.FleetMetricsSummary{TotalNodes: 1, OnlineNodes: 1},
		nodes: []config.NodeLiveStatus{
			{Name: "flapping-node", Hostname: "host-flap", Connected: true},
		},
	}

	svc := NewBotService(mock)
	svc.startupGracePeriod = 0
	svc.debounceCount = 2
	svc.settings = config.IntegrationsSettings{
		TelegramEnabled: true,
		TelegramAdminID: 12345,
	}

	var alerts []string
	svc.onAlert = func(text string) {
		alerts = append(alerts, text)
	}

	// 1. Initial baseline
	svc.checkNodeStatusTransitions()
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts on baseline, got %d", len(alerts))
	}

	// 2. Node drops connection for only 1 check interval (temporary flap)
	mock.nodes[0].Connected = false
	svc.checkNodeStatusTransitions()
	if len(alerts) != 0 {
		t.Errorf("expected no offline alert on momentary drop, got: %v", alerts)
	}

	// 3. Node reconnects immediately in next check interval (10s later)
	mock.nodes[0].Connected = true
	svc.checkNodeStatusTransitions()
	if len(alerts) != 0 {
		t.Errorf("expected flap to be debounced with 0 alerts, got: %v", alerts)
	}

	// 4. Stays online
	svc.checkNodeStatusTransitions()
	if len(alerts) != 0 {
		t.Errorf("expected still 0 alerts, got: %v", alerts)
	}
}

func TestServerStartupOnlineSuppression(t *testing.T) {
	// Simulate server startup: node is initially offline before reconnecting
	mock := &mockClusterProvider{
		summary: config.FleetMetricsSummary{TotalNodes: 2, OnlineNodes: 0},
		nodes: []config.NodeLiveStatus{
			{Name: "node-a", Hostname: "host-a", Connected: false},
			{Name: "node-b", Hostname: "host-b", Connected: false},
		},
	}

	svc := NewBotService(mock)
	svc.startupGracePeriod = 90 * time.Second
	svc.debounceCount = 2
	svc.monitorStartedAt = time.Now() // just started
	svc.settings = config.IntegrationsSettings{
		TelegramEnabled: true,
		TelegramAdminID: 12345,
	}

	var alerts []string
	svc.onAlert = func(text string) {
		alerts = append(alerts, text)
	}

	// 1. Initial baseline: nodes are offline on initial boot check
	svc.checkNodeStatusTransitions()
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts on baseline, got %d", len(alerts))
	}

	// 2. All nodes connect 15s later as agents finish handshake
	mock.nodes[0].Connected = true
	mock.nodes[1].Connected = true

	// Cycle 1: pending online
	svc.checkNodeStatusTransitions()
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts on first online cycle, got: %v", alerts)
	}

	// Cycle 2: stabilized online during startup grace period
	svc.checkNodeStatusTransitions()
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts during startup grace period, but got: %v", alerts)
	}

	// Verify internal state knows nodes are online
	if !svc.prevNodeOnline["node-a"] || !svc.prevNodeOnline["node-b"] {
		t.Errorf("expected nodes recorded as online in state")
	}

	// 3. Fast-forward past startup grace period
	svc.monitorStartedAt = time.Now().Add(-5 * time.Minute)

	// Now an actual outage occurs
	mock.nodes[0].Connected = false
	// Cycle 1 of drop: debouncing
	svc.checkNodeStatusTransitions()
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts on 1st drop cycle, got: %v", alerts)
	}
	// Cycle 2 of drop: stabilized offline -> alert should fire!
	svc.checkNodeStatusTransitions()
	if len(alerts) != 1 || !strings.Contains(alerts[0], "Node Offline") {
		t.Errorf("expected Node Offline alert after grace period, got: %v", alerts)
	}

	// Node-a reconnects after genuine outage
	mock.nodes[0].Connected = true
	// Cycle 1 of reconnect
	svc.checkNodeStatusTransitions()
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert on 1st reconnect cycle, got: %v", alerts)
	}
	// Cycle 2 of reconnect: stabilized online outside grace period -> alert should fire!
	svc.checkNodeStatusTransitions()
	if len(alerts) != 2 || !strings.Contains(alerts[1], "Node Online") {
		t.Errorf("expected Node Online alert after genuine recovery, got: %v", alerts)
	}
}

func TestHandleWebhookAccessDenied(t *testing.T) {
	mock := &mockClusterProvider{}
	svc := NewBotService(mock)
	svc.settings = config.IntegrationsSettings{
		TelegramEnabled: true,
		TelegramAdminID: 999999, // Allowed admin
	}

	updateJSON := `{
		"update_id": 1,
		"message": {
			"message_id": 10,
			"from": { "id": 111111, "is_bot": false, "first_name": "Stranger" },
			"chat": { "id": 111111, "type": "private" },
			"date": 1600000000,
			"text": "/status"
		}
	}`

	var update tgbotapi.Update
	if err := json.Unmarshal([]byte(updateJSON), &update); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// HandleWebhookUpdate returns error only if bot is nil or fatal
	// Here bot is nil so it returns error
	err := svc.HandleWebhookUpdate(&update)
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("expected 'not initialized' error, got %v", err)
	}
}

func TestFormatBytes(t *testing.T) {
	if got := formatBytes(0); got != "0 B" {
		t.Errorf("expected '0 B', got %q", got)
	}
	if got := formatBytes(512); got != "512 B" {
		t.Errorf("expected '512 B', got %q", got)
	}
	if got := formatBytes(1024); got != "1.0 KiB" {
		t.Errorf("expected '1.0 KiB', got %q", got)
	}
	if got := formatBytes(1024 * 1024 * 10); got != "10.0 MiB" {
		t.Errorf("expected '10.0 MiB', got %q", got)
	}
}

func TestNodeResolution(t *testing.T) {
	nodes := []config.Node{
		{Name: "foobar"},
		{Name: "foobaz"},
		{Name: "foo"},
		{Name: "london-gw"},
		{Name: "tokyo-gw"},
	}

	// 1. No argument -> returns all nodes
	r := resolveNode(nodes, "")
	if !r.NoArg || len(r.MultipleNodes) != len(nodes) {
		t.Errorf("expected NoArg with all nodes, got %+v", r)
	}

	// 2. Exact match takes precedence
	r = resolveNode(nodes, "foo")
	if r.MatchedNode == nil || r.MatchedNode.Name != "foo" {
		t.Errorf("expected exact match 'foo', got %+v", r)
	}

	// 3. Exact match case-insensitive
	r = resolveNode(nodes, "FoObAr")
	if r.MatchedNode == nil || r.MatchedNode.Name != "foobar" {
		t.Errorf("expected exact match 'foobar', got %+v", r)
	}

	// 4. Single prefix match
	r = resolveNode(nodes, "lon")
	if r.MatchedNode == nil || r.MatchedNode.Name != "london-gw" {
		t.Errorf("expected single prefix match 'london-gw', got %+v", r)
	}

	// 5. Multiple prefix matches (no exact match exists)
	r = resolveNode(nodes, "foob")
	if len(r.MultipleNodes) != 2 {
		t.Errorf("expected 2 multiple matches for 'foob', got %+v", r)
	}

	// 6. Not found
	r = resolveNode(nodes, "berlin")
	if !r.NotFound {
		t.Errorf("expected NotFound for 'berlin', got %+v", r)
	}
}

func TestBuildNodeSelectionKeyboard(t *testing.T) {
	nodes := []config.Node{
		{Name: "node-a"},
		{Name: "node-b"},
		{Name: "node-c"},
	}

	kb := buildNodeSelectionKeyboard(nodes)
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("expected 2 rows for 3 nodes, got %d", len(kb.InlineKeyboard))
	}

	// Row 1: node-a, node-b
	if len(kb.InlineKeyboard[0]) != 2 {
		t.Fatalf("expected 2 buttons in row 1, got %d", len(kb.InlineKeyboard[0]))
	}
	btn0 := kb.InlineKeyboard[0][0]
	if btn0.Text != "node-a" || *btn0.CallbackData != "/node node-a" {
		t.Errorf("expected button payload '/node node-a', got text=%q, data=%v", btn0.Text, btn0.CallbackData)
	}

	btn1 := kb.InlineKeyboard[0][1]
	if btn1.Text != "node-b" || *btn1.CallbackData != "/node node-b" {
		t.Errorf("expected button payload '/node node-b', got text=%q, data=%v", btn1.Text, btn1.CallbackData)
	}

	// Row 2: node-c
	if len(kb.InlineKeyboard[1]) != 1 {
		t.Fatalf("expected 1 button in row 2, got %d", len(kb.InlineKeyboard[1]))
	}
	btn2 := kb.InlineKeyboard[1][0]
	if btn2.Text != "node-c" || *btn2.CallbackData != "/node node-c" {
		t.Errorf("expected button payload '/node node-c', got text=%q, data=%v", btn2.Text, btn2.CallbackData)
	}
}

func TestFormatNodeDetail(t *testing.T) {
	node := &config.Node{
		Name:         "node1",
		Host:         "198.51.100.1",
		IP:           "172.20.0.1",
		IP6:          "fd42:a159:f9f0::1",
		ASN:          4224420001,
		Description:  "Main Gateway",
		Tags:         []string{"core", "eu"},
		Mode:         "agent",
		AgentVersion: "v1.2.0",
	}

	live := &config.NodeLiveStatus{
		Name:             "node1",
		Hostname:         "gw-lon-01",
		OSInfo:           "Linux 6.8 Ubuntu 24.04",
		AgentVersion:     "v1.2.0",
		AgentInstalled:   true,
		Connected:        true,
		UptimeSeconds:    86400 * 2,
		PrimaryInterface: "eth0",
		Metrics: &config.NodeMetricPoint{
			CPUPercent:       24.5,
			MemoryUsedBytes:  1024 * 1024 * 1024,
			MemoryTotalBytes: 1024 * 1024 * 2048,
			Load1m:           0.45,
			Load5m:           0.30,
			Load15m:          0.20,
			NetRxRate:        1024 * 512,
			NetTxRate:        1024 * 256,
		},
	}

	// Mock links
	links := []config.Link{
		{
			Type: "wireguard",
			From: config.LinkEnd{Name: "node1", Interface: "wg42node2"},
			To:   config.LinkEnd{Name: "node2", Interface: "wg42node1"},
		},
		{
			Type: "wireguard",
			From: config.LinkEnd{Name: "node3", Interface: "wg42node1"},
			To:   config.LinkEnd{Name: "node1", Interface: "wg42node3"},
		},
		{
			Type: "manual",
			From: config.LinkEnd{Name: "node1"},
			To:   config.LinkEnd{Name: "node4"},
		},
		{
			Type: "manual",
			From: config.LinkEnd{Name: "node1"},
			To:   config.LinkEnd{Name: "node5"},
		},
		{
			Type: "wireguard",
			From: config.LinkEnd{Name: "node2", Interface: "wg42node3"},
			To:   config.LinkEnd{Name: "node3", Interface: "wg42node2"},
		},
	}

	netState := &config.NetworkState{
		Nodes: map[string]config.StateNode{
			"node1": {
				Name: "node1",
				Interfaces: map[string]config.StateInterface{
					"wg42node2": {
						Name:         "wg42node2",
						WorkingState: config.WorkingStateWorking,
					},
					"wg42node3": {
						Name:         "wg42node3",
						WorkingState: config.WorkingStateNotWorking,
					},
				},
			},
		},
	}

	liveMap := map[string]config.NodeLiveStatus{
		"node1": {Name: "node1", Connected: true},
		"node2": {Name: "node2", Connected: true},
		"node3": {Name: "node3", Connected: false},
		"node4": {Name: "node4", Connected: true},
		"node5": {Name: "node5", Connected: false},
	}

	text := formatNodeDetail(node, live, links, netState, liveMap, "https://easy42.example.com")

	if !strings.Contains(text, "node1") {
		t.Errorf("expected node name 'node1' in formatted text")
	}
	if !strings.Contains(text, "gw-lon-01") {
		t.Errorf("expected hostname 'gw-lon-01' in formatted text")
	}
	if !strings.Contains(text, "Online / Connected") {
		t.Errorf("expected 'Online / Connected' status in formatted text")
	}
	if !strings.Contains(text, "4224420001") {
		t.Errorf("expected ASN in formatted text")
	}
	if !strings.Contains(text, "172.20.0.1") {
		t.Errorf("expected IPv4 in formatted text")
	}
	if !strings.Contains(text, "fd42:a159:f9f0::1") {
		t.Errorf("expected IPv6 in formatted text")
	}
	if !strings.Contains(text, "24.5%") {
		t.Errorf("expected CPU 24.5%% in formatted text")
	}
	if !strings.Contains(text, "https://easy42.example.com") {
		t.Errorf("expected server URL in formatted text")
	}
	// Verify node details page link
	if !strings.Contains(text, "/nodes/node1") {
		t.Errorf("expected '/nodes/node1' link in formatted text")
	}
	if !strings.Contains(text, "https://easy42.example.com/nodes/node1") {
		t.Errorf("expected full node details URL 'https://easy42.example.com/nodes/node1' in formatted text")
	}

	// Verify links section and up/down states
	if !strings.Contains(text, "Links (4):") {
		t.Errorf("expected 'Links (4):' in formatted text, got: %s", text)
	}
	if !strings.Contains(text, "up ↔ <b>node2</b> (<code>wg42node2</code>)") {
		t.Errorf("expected up wg link for node2, got: %s", text)
	}
	if !strings.Contains(text, "down ↔ <b>node3</b> (<code>wg42node3</code>)") {
		t.Errorf("expected down wg link for node3, got: %s", text)
	}
	if !strings.Contains(text, "up ↔ <b>node4</b>") {
		t.Errorf("expected up manual link for node4, got: %s", text)
	}
	if !strings.Contains(text, "down ↔ <b>node5</b>") {
		t.Errorf("expected down manual link for node5, got: %s", text)
	}
	// Verify unrelated link node2 <-> node3 is not included
	if strings.Contains(text, "node2 (wg42node3)") || strings.Contains(text, "node3 (wg42node2)") {
		t.Errorf("unrelated link between node2 and node3 should not be in node1 links")
	}

	// Test offline node with empty serverURL
	liveOffline := &config.NodeLiveStatus{
		Name:           "node1",
		Connected:      false,
		AgentInstalled: true,
		LastSeen:       time.Now().Add(-10 * time.Minute),
	}
	offText := formatNodeDetail(node, liveOffline, nil, nil, nil, "")
	if !strings.Contains(offText, "Offline") {
		t.Errorf("expected Offline status for disconnected node")
	}
	if !strings.Contains(offText, "/nodes/node1") {
		t.Errorf("expected '/nodes/node1' link in formatted text even with empty serverURL")
	}
	if !strings.Contains(offText, "Links (0):") || !strings.Contains(offText, "No links configured") {
		t.Errorf("expected 0 links message for nil links, got: %s", offText)
	}
}

func TestLookingGlassHelpMessages(t *testing.T) {
	pingHelp := pingHelpMessage()
	if !strings.Contains(pingHelp, "/ping [node] &lt;target&gt; [count]") {
		t.Errorf("expected usage line in pingHelpMessage, got %s", pingHelp)
	}
	if !strings.Contains(pingHelp, "Examples:") {
		t.Errorf("expected examples in pingHelpMessage")
	}

	tcpPingHelp := tcpPingHelpMessage()
	if !strings.Contains(tcpPingHelp, "/tcp_ping [node] &lt;host:port&gt; [count]") {
		t.Errorf("expected usage line in tcpPingHelpMessage, got %s", tcpPingHelp)
	}
	if !strings.Contains(tcpPingHelp, "&lt;host&gt;:&lt;port&gt;") {
		t.Errorf("expected host:port specification in tcpPingHelpMessage")
	}
}

func TestFormatPingResult(t *testing.T) {
	nr := lookingglass.NodeResult{
		NodeName: "node1",
		Command:  "ping -c 4 -W 2 1.1.1.1",
		Parsed: lookingglass.PingResult{
			Host:            "1.1.1.1",
			PacketsSent:     4,
			PacketsReceived: 4,
			PacketLossPct:   0,
			MinRTT:          12.34,
			AvgRTT:          14.56,
			MaxRTT:          18.90,
			MdevRTT:         1.20,
		},
	}

	res := formatPingResult("node1", "1.1.1.1", nr)
	if !strings.Contains(res, "node1") || !strings.Contains(res, "1.1.1.1") {
		t.Errorf("expected node and target in ping result, got: %s", res)
	}
	if !strings.Contains(res, "14.56ms") {
		t.Errorf("expected avg RTT in ping result, got: %s", res)
	}

	// Error formatting
	errRes := formatPingResult("node1", "1.1.1.1", lookingglass.NodeResult{
		NodeName: "node1",
		Error:    "Host unreachable",
	})
	if !strings.Contains(errRes, "Host unreachable") {
		t.Errorf("expected error message in result, got: %s", errRes)
	}
}

func TestFormatTCPPingResult(t *testing.T) {
	nr := lookingglass.NodeResult{
		NodeName: "node1",
		Command:  "tcp_ping 1.1.1.1:80 4",
		Parsed: &lookingglass.TCPPingResult{
			Target:          "1.1.1.1:80",
			ResolvedAddr:    "1.1.1.1:80",
			PacketsSent:     4,
			PacketsReceived: 4,
			PacketLossPct:   0,
			MinRTT:          10.12,
			AvgRTT:          11.34,
			MaxRTT:          13.45,
		},
	}

	res := formatTCPPingResult("node1", "1.1.1.1:80", nr)
	if !strings.Contains(res, "1.1.1.1:80") || !strings.Contains(res, "node1") {
		t.Errorf("expected target and node in tcp ping result, got: %s", res)
	}
	if !strings.Contains(res, "11.34ms") {
		t.Errorf("expected avg RTT in tcp ping result, got: %s", res)
	}

	// Error formatting
	errRes := formatTCPPingResult("node1", "1.1.1.1:80", lookingglass.NodeResult{
		NodeName: "node1",
		Error:    "Connection refused",
	})
	if !strings.Contains(errRes, "Connection refused") {
		t.Errorf("expected error message in result, got: %s", errRes)
	}
}

func TestWebhookCommandRouting(t *testing.T) {
	var mu sync.Mutex
	var sentTexts []string
	var sentMarkups []tgbotapi.InlineKeyboardMarkup

	fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":999,"is_bot":true,"first_name":"testbot","username":"testbot"}}`))
			return
		}

		if strings.HasSuffix(r.URL.Path, "sendMessage") {
			_ = r.ParseForm()
			text := r.FormValue("text")
			replyMarkup := r.FormValue("reply_markup")

			mu.Lock()
			sentTexts = append(sentTexts, text)
			if replyMarkup != "" {
				var km tgbotapi.InlineKeyboardMarkup
				_ = json.Unmarshal([]byte(replyMarkup), &km)
				sentMarkups = append(sentMarkups, km)
			}
			mu.Unlock()

			_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42,"date":1600000000,"chat":{"id":12345,"type":"private"}}}`))
			return
		}

		// answerCallbackQuery, sendChatAction, setWebhook, etc.
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	}))
	defer fakeServer.Close()

	bot, err := tgbotapi.NewBotAPIWithClient("test-token", fakeServer.URL+"/bot%s/%s", fakeServer.Client())
	if err != nil {
		t.Fatalf("failed to create bot: %v", err)
	}

	mock := &mockClusterProvider{
		summary: config.FleetMetricsSummary{
			TotalNodes:  3,
			OnlineNodes: 2,
		},
		configNodes: []config.Node{
			{Name: "foobar", IP: "172.20.0.1", Mode: "agent"},
			{Name: "foobaz", IP: "172.20.0.2", Mode: "ssh"},
			{Name: "london-gw", IP: "172.20.0.3", Mode: "agent"},
			{Name: "external-peer", IP: "172.20.0.99", IsExternal: true},
		},
		nodes: []config.NodeLiveStatus{
			{Name: "foobar", Connected: true, AgentInstalled: true},
			{Name: "foobaz", Connected: false, AgentInstalled: false},
			{Name: "london-gw", Connected: true, AgentInstalled: true},
		},
		links: []config.Link{
			{
				Type: "wireguard",
				From: config.LinkEnd{Name: "foobar", Interface: "wg42lon"},
				To:   config.LinkEnd{Name: "london-gw", Interface: "wg42foo"},
			},
			{
				Type: "wireguard",
				From: config.LinkEnd{Name: "foobar", Interface: "wg42baz"},
				To:   config.LinkEnd{Name: "foobaz", Interface: "wg42foo"},
			},
		},
		netState: &config.NetworkState{
			Nodes: map[string]config.StateNode{
				"foobar": {
					Name: "foobar",
					Interfaces: map[string]config.StateInterface{
						"wg42lon": {
							Name:         "wg42lon",
							WorkingState: config.WorkingStateWorking,
						},
						"wg42baz": {
							Name:         "wg42baz",
							WorkingState: config.WorkingStateNotWorking,
						},
					},
				},
			},
		},
		runResponse: lookingglass.RunResponse{
			TaskID: "ping",
			Results: map[string]lookingglass.NodeResult{
				"foobar": {
					NodeName: "foobar",
					Parsed: lookingglass.PingResult{
						Host:            "1.1.1.1",
						PacketsSent:     4,
						PacketsReceived: 4,
						PacketLossPct:   0,
						AvgRTT:          15.2,
					},
				},
			},
		},
	}

	adminID := int64(12345)
	svc := NewBotService(mock)
	svc.bot = bot
	svc.settings = config.IntegrationsSettings{
		TelegramEnabled: true,
		TelegramAdminID: adminID,
		ServerURL:       "https://easy42.test",
	}

	sendMsgUpdate := func(text string) *tgbotapi.Update {
		return &tgbotapi.Update{
			UpdateID: 1,
			Message: &tgbotapi.Message{
				MessageID: 10,
				From:      &tgbotapi.User{ID: adminID},
				Chat:      &tgbotapi.Chat{ID: adminID, Type: "private"},
				Text:      text,
			},
		}
	}

	sendCallbackUpdate := func(data string) *tgbotapi.Update {
		return &tgbotapi.Update{
			UpdateID: 2,
			CallbackQuery: &tgbotapi.CallbackQuery{
				ID:      "cb-123",
				From:    &tgbotapi.User{ID: adminID},
				Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: adminID}},
				Data:    data,
			},
		}
	}

	// 1. /node with no args -> outputs message with inline buttons listing all nodes
	mu.Lock()
	sentTexts = nil
	sentMarkups = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/node"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "easy42 Cluster Nodes") {
		t.Errorf("expected node list message, got: %v", sentTexts)
	}
	if len(sentMarkups) != 1 || len(sentMarkups[0].InlineKeyboard) == 0 {
		t.Fatalf("expected inline keyboard with nodes, got none")
	}
	// Verify buttons have payload "/node <name>"
	btnPayload := *sentMarkups[0].InlineKeyboard[0][0].CallbackData
	if !strings.HasPrefix(btnPayload, "/node ") {
		t.Errorf("expected button payload '/node <name>', got %q", btnPayload)
	}
	// Verify external-peer is ignored and not in keyboard
	for _, row := range sentMarkups[0].InlineKeyboard {
		for _, btn := range row {
			if btn.Text == "external-peer" || (btn.CallbackData != nil && *btn.CallbackData == "/node external-peer") {
				t.Errorf("expected external node 'external-peer' to be ignored in /node list")
			}
		}
	}

	// 2. /node foobar -> exact match displays node info
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/node foobar"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Node: foobar") {
		t.Errorf("expected foobar node detail, got: %v", sentTexts)
	}
	if !strings.Contains(sentTexts[0], "Links (2):") {
		t.Errorf("expected Links (2) in foobar detail, got: %s", sentTexts[0])
	}
	if !strings.Contains(sentTexts[0], "up ↔ <b>london-gw</b>") {
		t.Errorf("expected up link to london-gw, got: %s", sentTexts[0])
	}
	if !strings.Contains(sentTexts[0], "down ↔ <b>foobaz</b>") {
		t.Errorf("expected down link to foobaz, got: %s", sentTexts[0])
	}
	if !strings.Contains(sentTexts[0], "Node Details: https://easy42.test/nodes/foobar") {
		t.Errorf("expected node details link in foobar output, got: %s", sentTexts[0])
	}

	// 3. /node lon -> prefix match (single) displays london-gw
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/node lon"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Node: london-gw") {
		t.Errorf("expected london-gw node detail, got: %v", sentTexts)
	}

	// 4. /node foob -> prefix match (multiple) outputs message with inline buttons
	mu.Lock()
	sentTexts = nil
	sentMarkups = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/node foob"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Multiple Nodes Matched") {
		t.Errorf("expected multiple nodes matched message, got: %v", sentTexts)
	}
	if len(sentMarkups) != 1 {
		t.Fatalf("expected inline markup for multiple matches")
	}

	// 5. /node nonexistent -> not found
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/node nonexistent"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Node Not Found") {
		t.Errorf("expected node not found message, got: %v", sentTexts)
	}

	// 5b. /node external-peer -> external node is ignored and not found
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/node external-peer"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Node Not Found") {
		t.Errorf("expected external node to be ignored with Node Not Found message, got: %v", sentTexts)
	}

	// 6. Callback query with payload "/node foobar" -> displays node info
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendCallbackUpdate("/node foobar"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Node: foobar") {
		t.Errorf("expected foobar detail from callback query, got: %v", sentTexts)
	}

	// 7. /ping without args -> help message
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/ping"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Ping Command Usage") {
		t.Errorf("expected ping help message, got: %v", sentTexts)
	}

	// 8. /ping foobar (missing target) -> help message with missing target alert
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/ping foobar"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Missing Target") {
		t.Errorf("expected missing target warning, got: %v", sentTexts)
	}

	// 9. /ping 1.1.1.1 -> runs ping on online node foobar
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/ping 1.1.1.1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Ping Result: 1.1.1.1") {
		t.Errorf("expected ping result, got: %v", sentTexts)
	}

	// 10. /ping foobar 1.1.1.1 5 -> runs ping with explicit node and count
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/ping foobar 1.1.1.1 5"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Ping Result: 1.1.1.1") {
		t.Errorf("expected ping result, got: %v", sentTexts)
	}

	// 11. /tcp_ping without args -> help message
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/tcp_ping"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "TCP Ping Command Usage") {
		t.Errorf("expected tcp_ping help message, got: %v", sentTexts)
	}

	// 12. /tcp_ping 1.1.1.1 (missing port) -> error with help message
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/tcp_ping 1.1.1.1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "Invalid Target") {
		t.Errorf("expected invalid target for tcp_ping without port, got: %v", sentTexts)
	}

	// 13. /tcp_ping foobar 1.1.1.1:80 -> runs tcp_ping
	mock.runResponse = lookingglass.RunResponse{
		TaskID: "tcp_ping",
		Results: map[string]lookingglass.NodeResult{
			"foobar": {
				NodeName: "foobar",
				Parsed: &lookingglass.TCPPingResult{
					Target:          "1.1.1.1:80",
					PacketsSent:     4,
					PacketsReceived: 4,
					AvgRTT:          12.5,
				},
			},
		},
	}
	mu.Lock()
	sentTexts = nil
	mu.Unlock()
	err = svc.HandleWebhookUpdate(sendMsgUpdate("/tcp_ping foobar 1.1.1.1:80"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sentTexts) != 1 || !strings.Contains(sentTexts[0], "TCP Ping Result: 1.1.1.1:80") {
		t.Errorf("expected tcp ping result, got: %v", sentTexts)
	}
}
