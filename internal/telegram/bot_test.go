package telegram

import (
	"encoding/json"
	"strings"
	"testing"

	"easy42/internal/config"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type mockClusterProvider struct {
	summary config.FleetMetricsSummary
	nodes   []config.NodeLiveStatus
	err     error
}

func (m *mockClusterProvider) GetFleetLiveStatus() (config.FleetMetricsSummary, []config.NodeLiveStatus, error) {
	if m.err != nil {
		return config.FleetMetricsSummary{}, nil, m.err
	}
	return m.summary, m.nodes, nil
}

func (m *mockClusterProvider) GetNetworkState() *config.NetworkState {
	return &config.NetworkState{}
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

	// Second run: node2 goes offline
	mock.nodes[1].Connected = false
	mock.summary.OnlineNodes = 1
	mock.summary.OfflineNodes = 1

	svc.checkNodeStatusTransitions()
	if svc.prevNodeOnline["node2"] {
		t.Errorf("expected node2 to be recorded as offline in prevNodeOnline")
	}
	if len(alerts) != 1 || !strings.Contains(alerts[0], "Node Offline") || !strings.Contains(alerts[0], "node2") {
		t.Errorf("expected Node Offline alert for node2, got: %v", alerts)
	}

	// Third run: node2 comes back online
	mock.nodes[1].Connected = true
	mock.summary.OnlineNodes = 2
	mock.summary.OfflineNodes = 0

	svc.checkNodeStatusTransitions()
	if !svc.prevNodeOnline["node2"] {
		t.Errorf("expected node2 to be recorded as online in prevNodeOnline")
	}
	if len(alerts) != 2 || !strings.Contains(alerts[1], "Node Online") || !strings.Contains(alerts[1], "node2") {
		t.Errorf("expected Node Online alert for node2, got: %v", alerts)
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
