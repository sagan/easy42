-- Store each node's reported CPU core count in node_info so weighted fleet
-- CPU averages and total cluster cores persist across reconnects and restarts.
ALTER TABLE node_info ADD COLUMN cpu_cores INTEGER NOT NULL DEFAULT 0;
