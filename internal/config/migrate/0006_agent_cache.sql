-- Agent hub cache: everything the sync plan needs while a node agent is
-- offline (right after a server restart, before the agent reconnects).
-- app hashes, probed app files, applied enabled state, probed WireGuard
-- configs, registration metadata (NodeMeta), telemetry and last live report.
CREATE TABLE IF NOT EXISTS agent_cache (
	node_name TEXT NOT NULL,
	cache_key TEXT NOT NULL,
	value TEXT NOT NULL DEFAULT '',
	updated_at INTEGER,
	PRIMARY KEY (node_name, cache_key)
);
