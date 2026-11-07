package engine

import (
	"easy42/internal/config"
)

// SetOnEventCreated registers a listener invoked when a new event is recorded
func (m *Manager) SetOnEventCreated(fn func(evt *config.Event)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onEventCreated = fn
}

// RecordEvent records a fleet event into SQLite, notifies Telegram, and dispatches to subscribers
func (m *Manager) RecordEvent(evt *config.Event) (*config.Event, error) {
	if evt == nil || m.stateStore == nil {
		return evt, nil
	}

	saved, err := m.stateStore.RecordEvent(*evt)
	if err != nil {
		return nil, err
	}

	if m.telegramBot != nil {
		_ = m.telegramBot.NotifyEvent(saved)
	}

	m.mu.RLock()
	cb := m.onEventCreated
	m.mu.RUnlock()

	if cb != nil {
		cb(saved)
	}

	return saved, nil
}

// GetEvents queries events with filtering and pagination
func (m *Manager) GetEvents(filter config.EventFilter) ([]config.Event, int, error) {
	if m.stateStore == nil {
		return []config.Event{}, 0, nil
	}
	return m.stateStore.GetEvents(filter)
}

// GetUnreadEventsCount returns the count of unread events
func (m *Manager) GetUnreadEventsCount() (int, error) {
	if m.stateStore == nil {
		return 0, nil
	}
	return m.stateStore.GetUnreadEventsCount()
}

// MarkEventAsRead marks an event as read
func (m *Manager) MarkEventAsRead(id int64) error {
	if m.stateStore == nil {
		return nil
	}
	return m.stateStore.MarkEventAsRead(id)
}

// MarkAllEventsAsRead marks all events as read
func (m *Manager) MarkAllEventsAsRead() error {
	if m.stateStore == nil {
		return nil
	}
	return m.stateStore.MarkAllEventsAsRead()
}

// DeleteEvent deletes an event by ID
func (m *Manager) DeleteEvent(id int64) error {
	if m.stateStore == nil {
		return nil
	}
	return m.stateStore.DeleteEvent(id)
}

// ClearEvents deletes all recorded events
func (m *Manager) ClearEvents() error {
	if m.stateStore == nil {
		return nil
	}
	return m.stateStore.ClearEvents()
}
