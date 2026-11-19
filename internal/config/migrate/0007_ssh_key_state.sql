-- SSH key file management state: per target file (known_hosts / authorized_keys)
-- the hash of the Easy42-managed block last applied to the node.
ALTER TABLE node_states ADD COLUMN ssh_key_file_hashes TEXT NOT NULL DEFAULT '{}';
ALTER TABLE node_states ADD COLUMN ssh_key_applied_at TEXT NOT NULL DEFAULT '{}';
