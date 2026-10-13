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
