ALTER TABLE node_states ADD COLUMN obfuscator_config_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE node_states ADD COLUMN obfuscator_applied_at INTEGER;
