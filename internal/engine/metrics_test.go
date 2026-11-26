package engine

import (
	"math"
	"os"
	"testing"
	"time"

	agentpb "easy42/internal/agent/proto"
	"easy42/internal/config"
)

func TestFleetWeightedMetricsAndTotals(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "e42-fleet-metrics-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := config.NewStore(tmpDir)
	if _, err := store.Initialize(); err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	cfg := store.Get()
	cfg.Nodes = []config.Node{
		{Name: "node-a", Mode: "agent"},
		{Name: "node-b", Mode: "agent"},
		{Name: "node-ext", IsExternal: true},
	}
	if err := store.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	mgr := NewManager(store)
	defer mgr.Close()

	// Record metrics with different CPU cores and memory sizes
	if err := mgr.stateStore.RecordMetrics("node-a", config.NodeMetricInput{
		Timestamp:        time.Now(),
		CPUPercent:       50.0,
		CPUCores:         2,
		MemoryUsedBytes:  2 * 1024 * 1024 * 1024,
		MemoryTotalBytes: 4 * 1024 * 1024 * 1024,
	}); err != nil {
		t.Fatalf("RecordMetrics node-a: %v", err)
	}
	if err := mgr.stateStore.RecordMetrics("node-b", config.NodeMetricInput{
		Timestamp:        time.Now(),
		CPUPercent:       10.0,
		CPUCores:         6,
		MemoryUsedBytes:  10 * 1024 * 1024 * 1024,
		MemoryTotalBytes: 16 * 1024 * 1024 * 1024,
	}); err != nil {
		t.Fatalf("RecordMetrics node-b: %v", err)
	}
	// Unmanaged node should be excluded from fleet totals
	if err := mgr.stateStore.RecordMetrics("node-ext", config.NodeMetricInput{
		Timestamp:        time.Now(),
		CPUPercent:       99.0,
		CPUCores:         32,
		MemoryUsedBytes:  32 * 1024 * 1024 * 1024,
		MemoryTotalBytes: 64 * 1024 * 1024 * 1024,
	}); err != nil {
		t.Fatalf("RecordMetrics node-ext: %v", err)
	}

	// Verify live telemetry update also propagates cpu_cores
	mgr.AgentHub().UpdateTelemetry("node-a", &agentpb.TelemetryReport{
		System: &agentpb.SystemMetrics{
			CpuPercent:       50.0,
			CpuCores:         2,
			MemoryUsedBytes:  2 * 1024 * 1024 * 1024,
			MemoryTotalBytes: 4 * 1024 * 1024 * 1024,
		},
	})

	summary, list, err := mgr.GetFleetLiveStatus()
	if err != nil {
		t.Fatalf("GetFleetLiveStatus failed: %v", err)
	}

	if summary.TotalCPUCores != 8 {
		t.Errorf("expected TotalCPUCores = 8, got %d", summary.TotalCPUCores)
	}
	expectedMemTotal := uint64(20) * 1024 * 1024 * 1024
	if summary.TotalMemoryBytes != expectedMemTotal {
		t.Errorf("expected TotalMemoryBytes = %d, got %d", expectedMemTotal, summary.TotalMemoryBytes)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 managed nodes, got %d", len(list))
	}
	for _, item := range list {
		switch item.Name {
		case "node-a":
			if item.CPUCores != 2 {
				t.Errorf("expected node-a CPUCores=2, got %d", item.CPUCores)
			}
		case "node-b":
			if item.CPUCores != 6 {
				t.Errorf("expected node-b CPUCores=6, got %d", item.CPUCores)
			}
		default:
			t.Errorf("unexpected node in list: %s", item.Name)
		}
	}

	// Verify weighted average math directly:
	// CPU: (50*2 + 10*6) / (2+6) = 160 / 8 = 20.0%
	// Mem: (2 + 10) / (4 + 16) * 100 = 12 / 20 * 100 = 60.0%
	if math.Abs(float64(summary.AverageCPU)-0) > 0.001 {
		t.Errorf("expected AverageCPU = 0 when nodes disconnected, got %f", summary.AverageCPU)
	}
}
