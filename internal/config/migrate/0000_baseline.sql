CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS node_states (
	node_name TEXT PRIMARY KEY,
	host TEXT NOT NULL DEFAULT '',
	last_seen INTEGER,
	bird_config_hash TEXT NOT NULL DEFAULT '',
	bird_applied_at INTEGER,
	nftables_config_hash TEXT NOT NULL DEFAULT '',
	nftables_applied_at INTEGER,
	roa_config_hashes TEXT NOT NULL DEFAULT '{}',
	roa_applied_at TEXT NOT NULL DEFAULT '{}',
	hosts_config_hash TEXT NOT NULL DEFAULT '',
	hosts_applied_at INTEGER,
	interfaces TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS node_info (
	node_name TEXT PRIMARY KEY,
	agent_version TEXT NOT NULL DEFAULT '',
	os_info TEXT NOT NULL DEFAULT '',
	hostname TEXT NOT NULL DEFAULT '',
	uptime_seconds INTEGER NOT NULL DEFAULT 0,
	last_seen INTEGER
);

CREATE TABLE IF NOT EXISTS node_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	node_name TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	cpu_percent REAL NOT NULL DEFAULT 0,
	memory_used_bytes INTEGER NOT NULL DEFAULT 0,
	memory_total_bytes INTEGER NOT NULL DEFAULT 0,
	uptime_seconds INTEGER NOT NULL DEFAULT 0,
	load_1m REAL NOT NULL DEFAULT 0,
	load_5m REAL NOT NULL DEFAULT 0,
	load_15m REAL NOT NULL DEFAULT 0,
	net_rx_bytes INTEGER NOT NULL DEFAULT 0,
	net_tx_bytes INTEGER NOT NULL DEFAULT 0,
	net_rx_rate REAL NOT NULL DEFAULT 0,
	net_tx_rate REAL NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_node_metrics_node_time ON node_metrics(node_name, timestamp);

CREATE TABLE IF NOT EXISTS node_interface_metrics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	node_name TEXT NOT NULL,
	interface_name TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	rx_bytes INTEGER NOT NULL DEFAULT 0,
	tx_bytes INTEGER NOT NULL DEFAULT 0,
	rx_rate REAL NOT NULL DEFAULT 0,
	tx_rate REAL NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_node_iface_metrics_lookup ON node_interface_metrics(node_name, interface_name, timestamp);
CREATE INDEX IF NOT EXISTS idx_node_iface_metrics_time ON node_interface_metrics(timestamp);

CREATE TABLE IF NOT EXISTS node_interface_traffic_stats (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	node_name TEXT NOT NULL,
	interface_name TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	raw_rx_bytes INTEGER NOT NULL DEFAULT 0,
	raw_tx_bytes INTEGER NOT NULL DEFAULT 0,
	accum_rx_bytes INTEGER NOT NULL DEFAULT 0,
	accum_tx_bytes INTEGER NOT NULL DEFAULT 0,
	rx_rate REAL NOT NULL DEFAULT 0,
	tx_rate REAL NOT NULL DEFAULT 0,
	reset_detected INTEGER NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_traffic_stats_unique ON node_interface_traffic_stats(node_name, interface_name, timestamp);
CREATE INDEX IF NOT EXISTS idx_traffic_stats_lookup ON node_interface_traffic_stats(node_name, interface_name, timestamp);
CREATE INDEX IF NOT EXISTS idx_traffic_stats_time ON node_interface_traffic_stats(timestamp);

CREATE TABLE IF NOT EXISTS cron_task_results (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id TEXT NOT NULL,
	task_name TEXT NOT NULL DEFAULT '',
	task_type TEXT NOT NULL DEFAULT '',
	node_name TEXT NOT NULL,
	executed_at INTEGER NOT NULL,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	exit_code INTEGER NOT NULL DEFAULT 0,
	error TEXT NOT NULL DEFAULT '',
	target TEXT NOT NULL DEFAULT '',
	packets_sent INTEGER NOT NULL DEFAULT 0,
	packets_received INTEGER NOT NULL DEFAULT 0,
	packet_loss_pct REAL NOT NULL DEFAULT 0,
	min_rtt_ms REAL NOT NULL DEFAULT 0,
	avg_rtt_ms REAL NOT NULL DEFAULT 0,
	max_rtt_ms REAL NOT NULL DEFAULT 0,
	mdev_rtt_ms REAL NOT NULL DEFAULT 0,
	raw_output TEXT NOT NULL DEFAULT '',
	parsed_json TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_cron_task_results_node_time ON cron_task_results(node_name, executed_at);
CREATE INDEX IF NOT EXISTS idx_cron_task_results_task_time ON cron_task_results(task_id, executed_at);

CREATE TABLE IF NOT EXISTS server_task_states (
	task_id TEXT PRIMARY KEY,
	last_run INTEGER NOT NULL DEFAULT 0,
	last_status TEXT NOT NULL DEFAULT '',
	last_duration_ms INTEGER NOT NULL DEFAULT 0,
	last_message TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS node_metrics_5m (
	node_name TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	cpu_percent REAL NOT NULL DEFAULT 0,
	memory_used_bytes INTEGER NOT NULL DEFAULT 0,
	memory_total_bytes INTEGER NOT NULL DEFAULT 0,
	uptime_seconds INTEGER NOT NULL DEFAULT 0,
	load_1m REAL NOT NULL DEFAULT 0,
	load_5m REAL NOT NULL DEFAULT 0,
	load_15m REAL NOT NULL DEFAULT 0,
	net_rx_bytes INTEGER NOT NULL DEFAULT 0,
	net_tx_bytes INTEGER NOT NULL DEFAULT 0,
	net_rx_rate REAL NOT NULL DEFAULT 0,
	net_tx_rate REAL NOT NULL DEFAULT 0,
	PRIMARY KEY (node_name, timestamp)
);

CREATE INDEX IF NOT EXISTS idx_node_metrics_5m_node_time ON node_metrics_5m(node_name, timestamp);

CREATE TABLE IF NOT EXISTS node_interface_traffic_stats_5m (
	node_name TEXT NOT NULL,
	interface_name TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	raw_rx_bytes INTEGER NOT NULL DEFAULT 0,
	raw_tx_bytes INTEGER NOT NULL DEFAULT 0,
	accum_rx_bytes INTEGER NOT NULL DEFAULT 0,
	accum_tx_bytes INTEGER NOT NULL DEFAULT 0,
	rx_rate REAL NOT NULL DEFAULT 0,
	tx_rate REAL NOT NULL DEFAULT 0,
	reset_detected INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (node_name, interface_name, timestamp)
);

CREATE INDEX IF NOT EXISTS idx_traffic_stats_5m_lookup ON node_interface_traffic_stats_5m(node_name, interface_name, timestamp);
CREATE INDEX IF NOT EXISTS idx_traffic_stats_5m_time ON node_interface_traffic_stats_5m(timestamp);

CREATE TABLE IF NOT EXISTS cron_task_results_5m (
	task_id TEXT NOT NULL,
	node_name TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	task_name TEXT NOT NULL DEFAULT '',
	task_type TEXT NOT NULL DEFAULT '',
	target TEXT NOT NULL DEFAULT '',
	runs_count INTEGER NOT NULL DEFAULT 0,
	success_count INTEGER NOT NULL DEFAULT 0,
	failure_count INTEGER NOT NULL DEFAULT 0,
	avg_rtt_ms REAL NOT NULL DEFAULT 0,
	min_rtt_ms REAL NOT NULL DEFAULT 0,
	max_rtt_ms REAL NOT NULL DEFAULT 0,
	packet_loss_pct REAL NOT NULL DEFAULT 0,
	packets_sent INTEGER NOT NULL DEFAULT 0,
	packets_received INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (task_id, node_name, timestamp)
);

CREATE INDEX IF NOT EXISTS idx_cron_task_5m_node_time ON cron_task_results_5m(node_name, timestamp);
CREATE INDEX IF NOT EXISTS idx_cron_task_5m_task_time ON cron_task_results_5m(task_id, timestamp);
