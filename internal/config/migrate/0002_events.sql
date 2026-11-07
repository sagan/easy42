CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	type TEXT NOT NULL,
	node_name TEXT NOT NULL DEFAULT '',
	task_id TEXT NOT NULL DEFAULT '',
	title TEXT NOT NULL,
	message TEXT NOT NULL,
	metadata TEXT NOT NULL DEFAULT '{}',
	created_at INTEGER NOT NULL,
	is_read INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_events_created ON events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_node ON events(node_name, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_unread ON events(is_read, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_events_cron_limit ON events(type, node_name, task_id, created_at DESC);
