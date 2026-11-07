package config

import (
	"os"
	"testing"
	"time"
)

func TestStateStoreEvents(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "easy42-events-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStateStore(tmpDir)
	defer store.Close()

	// 1. Initial count should be 0
	unread, err := store.GetUnreadEventsCount()
	if err != nil {
		t.Fatalf("GetUnreadEventsCount failed: %v", err)
	}
	if unread != 0 {
		t.Fatalf("expected 0 unread events, got %d", unread)
	}

	// 2. Record node offline event
	e1, err := store.RecordEvent(Event{
		Type:      EventTypeNodeOffline,
		NodeName:  "node-tokyo",
		Title:     "Node node-tokyo is offline",
		Message:   "Node node-tokyo is disconnected / unreachable",
		Metadata:  map[string]any{"hostname": "tokyo-gw1.net"},
		CreatedAt: time.Now().Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to record node_offline event: %v", err)
	}
	if e1.ID <= 0 {
		t.Fatalf("expected valid event ID, got %d", e1.ID)
	}

	// 3. Record Looking Glass cron task failure event
	e2, err := store.RecordEvent(Event{
		Type:     EventTypeCronTaskFailure,
		NodeName: "node-tokyo",
		TaskID:   "cron-ping-google",
		Title:    "Cron task 'ping google' failed on node-tokyo",
		Message:  "Looking Glass cron task 'ping google' failed on node node-tokyo: all packets timed out (100% loss)",
		Metadata: map[string]any{
			"task_id":         "cron-ping-google",
			"task_name":       "ping google",
			"target":          "8.8.8.8",
			"packet_loss_pct": 100.0,
		},
		CreatedAt: time.Now().Add(-5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to record cron_task_failure event: %v", err)
	}
	if e2.ID <= 0 {
		t.Fatalf("expected valid event ID, got %d", e2.ID)
	}

	// 4. Record node online event
	e3, err := store.RecordEvent(Event{
		Type:      EventTypeNodeOnline,
		NodeName:  "node-tokyo",
		Title:     "Node node-tokyo is online",
		Message:   "Node node-tokyo is connected & active",
		Metadata:  map[string]any{"hostname": "tokyo-gw1.net", "agent_version": "v1.2.0"},
		CreatedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to record node_online event: %v", err)
	}

	// 5. Test HasRecentCronTaskFailure
	hasRecent, err := store.HasRecentCronTaskFailure("cron-ping-google", "node-tokyo", 24*time.Hour)
	if err != nil {
		t.Fatalf("HasRecentCronTaskFailure failed: %v", err)
	}
	if !hasRecent {
		t.Fatalf("expected HasRecentCronTaskFailure to be true")
	}

	hasOther, err := store.HasRecentCronTaskFailure("cron-ping-cloudflare", "node-tokyo", 24*time.Hour)
	if err != nil {
		t.Fatalf("HasRecentCronTaskFailure check failed: %v", err)
	}
	if hasOther {
		t.Fatalf("expected HasRecentCronTaskFailure for other task to be false")
	}

	// 6. Test Unread Count (should be 3)
	unread, err = store.GetUnreadEventsCount()
	if err != nil {
		t.Fatalf("GetUnreadEventsCount failed: %v", err)
	}
	if unread != 3 {
		t.Fatalf("expected 3 unread events, got %d", unread)
	}

	// 7. Test GetEvents
	list, total, err := store.GetEvents(EventFilter{})
	if err != nil {
		t.Fatalf("GetEvents failed: %v", err)
	}
	if total != 3 || len(list) != 3 {
		t.Fatalf("expected 3 events, got total=%d len=%d", total, len(list))
	}
	// Verify descending order
	if list[0].ID != e3.ID || list[1].ID != e2.ID || list[2].ID != e1.ID {
		t.Fatalf("events not in descending order: IDs=%d,%d,%d", list[0].ID, list[1].ID, list[2].ID)
	}

	// 8. Filter by type
	cronList, totalCron, err := store.GetEvents(EventFilter{Type: EventTypeCronTaskFailure})
	if err != nil {
		t.Fatalf("GetEvents filter failed: %v", err)
	}
	if totalCron != 1 || len(cronList) != 1 || cronList[0].TaskID != "cron-ping-google" {
		t.Fatalf("filter by cron_task_failure failed: total=%d", totalCron)
	}

	// 9. Mark single event as read
	if err := store.MarkEventAsRead(e2.ID); err != nil {
		t.Fatalf("MarkEventAsRead failed: %v", err)
	}
	unread, _ = store.GetUnreadEventsCount()
	if unread != 2 {
		t.Fatalf("expected 2 unread events after marking e2 read, got %d", unread)
	}

	// 10. Mark all events as read
	if err := store.MarkAllEventsAsRead(); err != nil {
		t.Fatalf("MarkAllEventsAsRead failed: %v", err)
	}
	unread, _ = store.GetUnreadEventsCount()
	if unread != 0 {
		t.Fatalf("expected 0 unread events after marking all read, got %d", unread)
	}

	// 11. Delete single event
	if err := store.DeleteEvent(e1.ID); err != nil {
		t.Fatalf("DeleteEvent failed: %v", err)
	}
	listAfterDel, totalAfterDel, _ := store.GetEvents(EventFilter{})
	if totalAfterDel != 2 || len(listAfterDel) != 2 {
		t.Fatalf("expected 2 events after deletion, got %d", totalAfterDel)
	}

	// 12. Clear events
	if err := store.ClearEvents(); err != nil {
		t.Fatalf("ClearEvents failed: %v", err)
	}
	listEmpty, totalEmpty, _ := store.GetEvents(EventFilter{})
	if totalEmpty != 0 || len(listEmpty) != 0 {
		t.Fatalf("expected 0 events after clear, got %d", totalEmpty)
	}
}
