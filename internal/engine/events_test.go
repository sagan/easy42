package engine

import (
	"os"
	"testing"
	"time"

	"easy42/internal/config"
)

func TestManagerEventsAndCronFailureRateLimit(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "easy42-mgr-events-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := config.NewStore(tmpDir)
	mgr := NewManager(store)
	defer mgr.Close()

	var createdEvents []*config.Event
	mgr.SetOnEventCreated(func(evt *config.Event) {
		createdEvents = append(createdEvents, evt)
	})

	// 1. Record an event manually via mgr
	e1, err := mgr.RecordEvent(&config.Event{
		Type:      config.EventTypeNodeOffline,
		NodeName:  "node-test-1",
		Title:     "Node node-test-1 is offline",
		Message:   "Node is unreachable",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}
	if e1.ID <= 0 {
		t.Fatalf("expected positive ID, got %d", e1.ID)
	}
	if len(createdEvents) != 1 {
		t.Fatalf("expected 1 created event callback, got %d", len(createdEvents))
	}

	// 2. Query events via mgr
	list, total, err := mgr.GetEvents(config.EventFilter{})
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("expected 1 event, got total=%d len=%d", total, len(list))
	}

	unread, err := mgr.GetUnreadEventsCount()
	if err != nil {
		t.Fatalf("GetUnreadEventsCount failed: %v", err)
	}
	if unread != 1 {
		t.Fatalf("expected 1 unread, got %d", unread)
	}

	// 3. Mark read
	if err := mgr.MarkEventAsRead(e1.ID); err != nil {
		t.Fatalf("MarkEventAsRead failed: %v", err)
	}
	unread, _ = mgr.GetUnreadEventsCount()
	if unread != 0 {
		t.Fatalf("expected 0 unread after mark read, got %d", unread)
	}

	// 4. Test isCronTaskFailed helper
	failedRec := &config.CronTaskResultRecord{
		TaskID:          "cron-ping-1",
		TaskName:        "Scheduled Ping 1",
		TaskType:        "ping",
		NodeName:        "node-test-1",
		Target:          "1.1.1.1",
		PacketsSent:     5,
		PacketsReceived: 0,
		PacketLossPct:   100.0,
	}
	if !isCronTaskFailed(failedRec) {
		t.Fatalf("expected failedRec to be failed")
	}

	okRec := &config.CronTaskResultRecord{
		TaskID:          "cron-ping-1",
		PacketsSent:     5,
		PacketsReceived: 5,
		PacketLossPct:   0.0,
	}
	if isCronTaskFailed(okRec) {
		t.Fatalf("expected okRec not to be failed")
	}

	// 5. Test 24-hour rate limit for cron task failure event
	cronEvt1, err := mgr.RecordEvent(&config.Event{
		Type:      config.EventTypeCronTaskFailure,
		NodeName:  failedRec.NodeName,
		TaskID:    failedRec.TaskID,
		Title:     "Cron task failed",
		Message:   "100% loss",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("RecordEvent failed: %v", err)
	}
	if cronEvt1.ID <= 0 {
		t.Fatalf("expected valid event ID, got %d", cronEvt1.ID)
	}

	// Check if recent failure exists in last 24h
	hasRecent, err := mgr.stateStore.HasRecentCronTaskFailure(failedRec.TaskID, failedRec.NodeName, 24*time.Hour)
	if err != nil {
		t.Fatalf("HasRecentCronTaskFailure failed: %v", err)
	}
	if !hasRecent {
		t.Fatalf("expected recent cron failure to be true (rate limiting subsequent failures)")
	}

	// Check for a different node or different task
	hasRecentDiffNode, _ := mgr.stateStore.HasRecentCronTaskFailure(failedRec.TaskID, "different-node", 24*time.Hour)
	if hasRecentDiffNode {
		t.Fatalf("expected recent failure for different node to be false")
	}
	hasRecentDiffTask, _ := mgr.stateStore.HasRecentCronTaskFailure("different-task", failedRec.NodeName, 24*time.Hour)
	if hasRecentDiffTask {
		t.Fatalf("expected recent failure for different task to be false")
	}
}
