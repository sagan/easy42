package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"easy42/internal/config"
	"easy42/internal/engine"
)

func setupTestServerWithCron(t *testing.T) (*Server, *engine.Manager, func()) {
	tmpDir, err := os.MkdirTemp("", "easy42-cron-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfgStore := config.NewStore(tmpDir)
	cfg := &config.Config{
		PasswordHash:  "hash",
		SessionSecret: "secret",
		Nodes: []config.Node{
			{Name: "node1", Host: "10.0.0.1"},
			{Name: "node2", Host: "10.0.0.2"},
		},
		CronTasks: []config.CronTask{},
	}
	if err := cfgStore.Save(cfg); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	mgr := engine.NewManager(cfgStore)
	srv := New(Config{
		Manager:    mgr,
		ListenAddr: "127.0.0.1:0",
	})

	cleanup := func() {
		mgr.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return srv, mgr, cleanup
}

func TestCronTasksAPI(t *testing.T) {
	srv, mgr, cleanup := setupTestServerWithCron(t)
	defer cleanup()

	// 1. Create Cron Task
	createPayload := config.CronTask{
		Name:     "Google Ping Test",
		TaskType: "ping",
		CronExpr: "*/5 * * * *",
		Nodes:    []string{"node1"},
		Params: map[string]string{
			"target": "8.8.8.8",
			"count":  "4",
		},
	}
	body, _ := json.Marshal(createPayload)
	req := httptest.NewRequest("POST", "/api/cron-tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.handleCreateCronTask(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var created config.CronTask
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if created.ID == "" || created.Name != "Google Ping Test" || !created.Enabled {
		t.Fatalf("unexpected created task: %+v", created)
	}

	// 2. Get Cron Tasks
	req = httptest.NewRequest("GET", "/api/cron-tasks", nil)
	w = httptest.NewRecorder()
	srv.handleGetCronTasks(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}
	var list []config.CronTask
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("expected 1 task in list, got %d", len(list))
	}

	// 3. Toggle Cron Task
	req = httptest.NewRequest("POST", "/api/cron-tasks/"+created.ID+"/toggle", nil)
	// chi URL param mock
	rctx := srv.router.Middlewares()
	_ = rctx
	// Test Toggle directly via manager
	toggled, err := mgr.ToggleCronTask(created.ID)
	if err != nil || toggled.Enabled != false {
		t.Fatalf("expected task toggled to false, got err=%v enabled=%v", err, toggled.Enabled)
	}

	// 4. Record Cron Task Result and fetch
	stateStore := mgr.StateStore()
	record := config.CronTaskResultRecord{
		TaskID:          created.ID,
		TaskName:        created.Name,
		TaskType:        created.TaskType,
		NodeName:        "node1",
		ExecutedAt:      time.Now().Unix(),
		Target:          "8.8.8.8",
		PacketsSent:     4,
		PacketsReceived: 4,
		PacketLossPct:   0.0,
		MinRTTMs:        12.1,
		AvgRTTMs:        14.5,
		MaxRTTMs:        16.2,
		RawOutput:       "4 packets transmitted, 4 received, 0% packet loss",
	}
	if err := stateStore.RecordCronTaskResult(record); err != nil {
		t.Fatalf("failed to record cron result: %v", err)
	}

	resResp, err := mgr.GetNodeCronTasksResults("node1", created.ID, "24h", 100)
	if err != nil {
		t.Fatalf("failed to get node cron task results: %v", err)
	}
	if len(resResp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resResp.Results))
	}
	if resResp.Results[0].AvgRTTMs != 14.5 {
		t.Fatalf("expected avg RTT 14.5, got %v", resResp.Results[0].AvgRTTMs)
	}

	// 5. Delete Cron Task
	if err := mgr.DeleteCronTask(created.ID); err != nil {
		t.Fatalf("failed to delete cron task: %v", err)
	}
	listAfter, _ := mgr.GetCronTasks()
	if len(listAfter) != 0 {
		t.Fatalf("expected 0 tasks after delete, got %d", len(listAfter))
	}
}

func TestCronTaskICMPPingParsing(t *testing.T) {
	srv, mgr, cleanup := setupTestServerWithCron(t)
	defer cleanup()

	rawPingOutput := `PING 1.1.1.1 (1.1.1.1) 56(84) bytes of data.
64 bytes from 1.1.1.1: icmp_seq=1 ttl=60 time=1.66 ms
64 bytes from 1.1.1.1: icmp_seq=2 ttl=60 time=2.03 ms
64 bytes from 1.1.1.1: icmp_seq=3 ttl=60 time=1.82 ms
64 bytes from 1.1.1.1: icmp_seq=4 ttl=60 time=3.52 ms

--- 1.1.1.1 ping statistics ---
4 packets transmitted, 4 received, 0% packet loss, time 3005ms
rtt min/avg/max/mdev = 1.663/2.257/3.515/0.737 ms`

	// 1. Test PopulateCronTaskParsedMetrics directly
	record := config.CronTaskResultRecord{
		TaskID:     "task-icmp-1",
		TaskName:   "ping-cf",
		TaskType:   "ping",
		NodeName:   "node1",
		ExecutedAt: time.Now().Unix(),
		Target:     "1.1.1.1",
		RawOutput:  rawPingOutput,
	}

	engine.PopulateCronTaskParsedMetrics(&record)

	if record.PacketsSent != 4 {
		t.Errorf("expected 4 packets sent, got %d", record.PacketsSent)
	}
	if record.PacketsReceived != 4 {
		t.Errorf("expected 4 packets received, got %d", record.PacketsReceived)
	}
	if record.PacketLossPct != 0.0 {
		t.Errorf("expected 0.0 packet loss, got %f", record.PacketLossPct)
	}
	if record.MinRTTMs != 1.663 {
		t.Errorf("expected MinRTT 1.663, got %f", record.MinRTTMs)
	}
	if record.AvgRTTMs != 2.257 {
		t.Errorf("expected AvgRTT 2.257, got %f", record.AvgRTTMs)
	}
	if record.MaxRTTMs != 3.515 {
		t.Errorf("expected MaxRTT 3.515, got %f", record.MaxRTTMs)
	}
	if record.MdevRTTMs != 0.737 {
		t.Errorf("expected MdevRTT 0.737, got %f", record.MdevRTTMs)
	}
	if record.Target != "1.1.1.1" {
		t.Errorf("expected target 1.1.1.1, got %s", record.Target)
	}
	if record.ParsedJSON == "" {
		t.Errorf("expected non-empty ParsedJSON")
	}

	// 2. Test reparsing on GetNodeCronTasksResults for previously unparsed results
	stateStore := mgr.StateStore()
	unparsedRecord := config.CronTaskResultRecord{
		TaskID:     "task-icmp-historical",
		TaskName:   "ping-historical",
		TaskType:   "ping",
		NodeName:   "node1",
		ExecutedAt: time.Now().Unix(),
		Target:     "1.1.1.1",
		RawOutput:  rawPingOutput,
		// PacketsSent: 0, AvgRTTMs: 0
	}
	if err := stateStore.RecordCronTaskResult(unparsedRecord); err != nil {
		t.Fatalf("failed to record unparsed cron result: %v", err)
	}

	resResp, err := mgr.GetNodeCronTasksResults("node1", "task-icmp-historical", "24h", 100)
	if err != nil {
		t.Fatalf("failed to get node cron task results: %v", err)
	}
	if len(resResp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resResp.Results))
	}
	r := resResp.Results[0]
	if r.PacketsSent != 4 || r.PacketsReceived != 4 {
		t.Errorf("expected 4/4 packets after query populate, got %d/%d", r.PacketsReceived, r.PacketsSent)
	}
	if r.AvgRTTMs != 2.257 {
		t.Errorf("expected AvgRTT 2.257 after query populate, got %f", r.AvgRTTMs)
	}
	if r.MinRTTMs != 1.663 || r.MaxRTTMs != 3.515 {
		t.Errorf("expected min/max 1.663/3.515, got %f/%f", r.MinRTTMs, r.MaxRTTMs)
	}
	_ = srv
}
