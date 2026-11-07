package config

import (
	"time"
)

// EventType defines the category of fleet event
type EventType string

const (
	EventTypeNodeOffline     EventType = "node_offline"
	EventTypeNodeOnline      EventType = "node_online"
	EventTypeCronTaskFailure EventType = "cron_task_failure"
)

// Event represents a system notification or audit log event
type Event struct {
	ID        int64          `json:"id"`
	Type      EventType      `json:"type"`
	NodeName  string         `json:"node_name"`
	TaskID    string         `json:"task_id,omitempty"`
	Title     string         `json:"title"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"created_at"`
	IsRead    bool           `json:"is_read"`
}

// EventFilter specifies filtering and pagination criteria for querying events
type EventFilter struct {
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
	UnreadOnly bool      `json:"unread_only"`
	Type       EventType `json:"type,omitempty"`
	NodeName   string    `json:"node_name,omitempty"`
}

// EventsListResponse wraps paginated events and counters
type EventsListResponse struct {
	Events      []Event `json:"events"`
	Total       int     `json:"total"`
	UnreadCount int     `json:"unread_count"`
}
