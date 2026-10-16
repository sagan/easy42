package engine

import (
	"os"
	"strings"
	"testing"
	"time"

	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
)

func TestPrometheusExporter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "e42-prom-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := config.NewStore(tmpDir)
	_, err = store.Initialize()
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	cfg := store.Get()
	cfg.Nodes = []config.Node{
		{
			Name: "node-a",
			IP:   "172.20.1.1",
			ASN:  4242421234,
			Mode: "agent",
		},
		{
			Name: "node-b",
			IP:   "172.20.1.2",
			ASN:  4242421235,
			Mode: "agent",
		},
	}
	cfg.Links = []config.Link{
		{
			From: config.LinkEnd{
				Name:      "node-a",
				Interface: "wg42peer1",
				PublicKey: "pubkey_a",
			},
			To: config.LinkEnd{
				Name:      "node-b",
				Interface: "wg42peer2",
				PublicKey: "pubkey_b",
			},
		},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	mgr := NewManager(store)
	defer mgr.Close()

	// Update telemetry for node-a
	mgr.AgentHub().UpdateTelemetry("node-a", &agentpb.TelemetryReport{
		System: &agentpb.SystemMetrics{
			CpuPercent:       25.5,
			MemoryTotalBytes: 8000000000,
			MemoryUsedBytes:  4000000000,
			UptimeSeconds:    3600,
			LoadAvg:          []float32{0.5, 0.4, 0.3},
		},
		Disks: []*agentpb.DiskMetrics{
			{
				Path:       "/",
				TotalBytes: 100000000000,
				UsedBytes:  50000000000,
				FreeBytes:  50000000000,
			},
		},
		Interfaces: []*agentpb.InterfaceMetrics{
			{
				Name:      "eth0",
				IsUp:      true,
				RxBytes:   1000,
				TxBytes:   2000,
				RxPackets: 10,
				TxPackets: 20,
			},
			{
				Name: "wg42peer1",
				IsUp: true,
			},
		},
		WgPeers: []*agentpb.WgPeerMetrics{
			{
				InterfaceName:       "wg42peer1",
				PublicKey:           "pubkey_b",
				Endpoint:            "1.2.3.4:51820",
				LastHandshakeTime:   time.Now().Unix() - 30,
				RxBytes:             5000,
				TxBytes:             6000,
				PersistentKeepalive: 25,
			},
		},
		BirdProtocols: []*agentpb.BirdProtocolStatus{
			{
				Name:  "dn42_node_b",
				Proto: "BGP",
				State: "up",
				Info:  "Established",
			},
		},
	})

	output, err := mgr.RenderPrometheusMetrics()
	if err != nil {
		t.Fatalf("RenderPrometheusMetrics failed: %v", err)
	}
	text := string(output)

	// Verify header formats
	if !strings.Contains(text, "# HELP easy42_exporter_build_info") {
		t.Errorf("missing easy42_exporter_build_info header")
	}
	if !strings.Contains(text, "# TYPE node_load1 gauge") {
		t.Errorf("missing node_load1 TYPE")
	}

	// Verify system metrics
	if !strings.Contains(text, `node_load1{node="node-a"} 0.5`) {
		t.Errorf("expected node_load1 for node-a, got: %s", text)
	}
	if !strings.Contains(text, `node_cpu_percent{node="node-a"} 25.5`) {
		t.Errorf("expected node_cpu_percent for node-a")
	}
	if !strings.Contains(text, `node_memory_MemTotal_bytes{node="node-a"} 8000000000`) {
		t.Errorf("expected memory total")
	}
	if !strings.Contains(text, `node_network_up{device="eth0",node="node-a"} 1`) {
		t.Errorf("expected node_network_up for eth0")
	}

	// Verify WireGuard metrics with MindFlavor conventions & peer_node/friendly_name enrichment
	if !strings.Contains(text, `wireguard_received_bytes_total{endpoint="1.2.3.4:51820",friendly_name="node-b",interface="wg42peer1",node="node-a",peer_node="node-b",public_key="pubkey_b"} 5000`) {
		t.Errorf("expected wireguard_received_bytes_total with peer_node=node-b, got:\n%s", text)
	}
	if !strings.Contains(text, `wireguard_sent_bytes_total{endpoint="1.2.3.4:51820",friendly_name="node-b",interface="wg42peer1",node="node-a",peer_node="node-b",public_key="pubkey_b"} 6000`) {
		t.Errorf("expected wireguard_sent_bytes_total with peer_node=node-b")
	}
	if !strings.Contains(text, `wireguard_latest_handshake_seconds{endpoint="1.2.3.4:51820",friendly_name="node-b",interface="wg42peer1",node="node-a",peer_node="node-b",public_key="pubkey_b"}`) {
		t.Errorf("expected wireguard_latest_handshake_seconds")
	}
	if !strings.Contains(text, `wireguard_peer_connected{friendly_name="node-b",interface="wg42peer1",node="node-a",peer_node="node-b",public_key="pubkey_b"} 1`) {
		t.Errorf("expected wireguard_peer_connected=1")
	}

	// Verify BIRD metrics with peer_node enrichment
	if !strings.Contains(text, `bird_protocol_up{name="dn42_node_b",node="node-a",peer_node="node-b",proto="BGP"} 1`) {
		t.Errorf("expected bird_protocol_up for dn42_node_b")
	}
	if !strings.Contains(text, `bird_bgp_session_established{name="dn42_node_b",node="node-a",peer_node="node-b"} 1`) {
		t.Errorf("expected bird_bgp_session_established=1")
	}

	// Verify easy42 metadata
	if !strings.Contains(text, `easy42_total_nodes 2`) {
		t.Errorf("expected easy42_total_nodes 2")
	}
	if !strings.Contains(text, `easy42_link_up{from_interface="wg42peer1",from_node="node-a",to_interface="wg42peer2",to_node="node-b"} 1`) {
		t.Errorf("expected easy42_link_up 1")
	}

	// Verify caching: second call returns same slice
	output2, err := mgr.RenderPrometheusMetrics()
	if err != nil {
		t.Fatalf("RenderPrometheusMetrics failed on cache hit: %v", err)
	}
	if string(output2) != text {
		t.Errorf("expected identical cached output")
	}

	// Verify reset token
	oldSettings := mgr.GetIntegrationsSettings()
	newSettings, err := mgr.ResetPrometheusToken()
	if err != nil {
		t.Fatalf("ResetPrometheusToken failed: %v", err)
	}
	if newSettings.PrometheusAuthToken == oldSettings.PrometheusAuthToken {
		t.Errorf("expected token to change after reset")
	}
	if !strings.HasPrefix(newSettings.PrometheusAuthToken, "e42_prom_") {
		t.Errorf("expected e42_prom_ prefix on new token: %s", newSettings.PrometheusAuthToken)
	}
}
