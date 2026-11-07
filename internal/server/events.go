package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"easy42/internal/config"
	"github.com/go-chi/chi/v5"
)

// broadcastEventCreated notifies all connected browser WebSocket sessions about a new fleet event
func (s *Server) broadcastEventCreated(evt *config.Event) {
	if s.wsHub != nil && evt != nil {
		unread := 0
		if s.mgr != nil {
			unread, _ = s.mgr.GetUnreadEventsCount()
		}
		_ = s.wsHub.BroadcastJSON(map[string]any{
			"type":         "event_created",
			"event":        evt,
			"unread_count": unread,
			"timestamp":    time.Now().UnixMilli(),
		})
	}
}

// handleGetEvents returns recorded events with pagination and filters
func (s *Server) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		http.Error(w, "manager unavailable", http.StatusServiceUnavailable)
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	unreadOnly := q.Get("unread_only") == "true" || q.Get("unread_only") == "1"
	eventType := config.EventType(q.Get("type"))
	nodeName := q.Get("node")

	filter := config.EventFilter{
		Limit:      limit,
		Offset:     offset,
		UnreadOnly: unreadOnly,
		Type:       eventType,
		NodeName:   nodeName,
	}

	events, total, err := s.mgr.GetEvents(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	unreadCount, err := s.mgr.GetUnreadEventsCount()
	if err != nil {
		unreadCount = 0
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(config.EventsListResponse{
		Events:      events,
		Total:       total,
		UnreadCount: unreadCount,
	})
}

// handleGetUnreadEventsCount returns the unread event counter
func (s *Server) handleGetUnreadEventsCount(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		http.Error(w, "manager unavailable", http.StatusServiceUnavailable)
		return
	}

	count, err := s.mgr.GetUnreadEventsCount()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{
		"unread_count": count,
	})
}

// handleMarkEventRead marks a single event as read
func (s *Server) handleMarkEventRead(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		http.Error(w, "manager unavailable", http.StatusServiceUnavailable)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}

	if err := s.mgr.MarkEventAsRead(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleMarkAllEventsRead marks all unread events as read
func (s *Server) handleMarkAllEventsRead(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		http.Error(w, "manager unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := s.mgr.MarkAllEventsAsRead(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleDeleteEvent removes an event by ID
func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		http.Error(w, "manager unavailable", http.StatusServiceUnavailable)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid event id", http.StatusBadRequest)
		return
	}

	if err := s.mgr.DeleteEvent(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleClearEvents deletes all recorded events
func (s *Server) handleClearEvents(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		http.Error(w, "manager unavailable", http.StatusServiceUnavailable)
		return
	}

	if err := s.mgr.ClearEvents(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
