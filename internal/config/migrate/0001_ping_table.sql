CREATE TABLE IF NOT EXISTS ping_table_tasks (
	id TEXT PRIMARY KEY,
	created_at INTEGER NOT NULL,
	finished_at INTEGER NOT NULL DEFAULT 0,
	latency_type TEXT NOT NULL,
	status TEXT NOT NULL,
	total_pairs INTEGER NOT NULL DEFAULT 0,
	completed_pairs INTEGER NOT NULL DEFAULT 0,
	nodes_json TEXT NOT NULL DEFAULT '[]',
	results_json TEXT NOT NULL DEFAULT '{}',
	error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_ping_table_tasks_created ON ping_table_tasks(created_at DESC);
