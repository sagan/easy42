package agent

import (
	"database/sql"
	"fmt"
)

// CacheStore persists the agent hub caches (app hashes, app files, applied
// enabled state, probed WireGuard configs, registration metadata, telemetry
// and last live report timestamps) so they survive a server restart. Without
// it, a restart would make every app look like it was never deployed.
//
// It writes into the shared state database (data.db): the agent_cache table is
// created by migration 0006_agent_cache.sql in internal/config/migrate. The
// store therefore only wraps an existing, already-migrated *sql.DB handle
// owned by config.StateStore.
type CacheStore struct {
	db *sql.DB
}

// NewCacheStore creates a cache store backed by an existing migrated database
// handle (e.g. StateStore.DB()). The caller keeps ownership of the handle.
func NewCacheStore(db *sql.DB) *CacheStore {
	return &CacheStore{db: db}
}

// LoadAll returns every cached entry grouped by node name.
func (s *CacheStore) LoadAll() (map[string]map[string]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("cache store has no database handle")
	}
	rows, err := s.db.Query("SELECT node_name, cache_key, value FROM agent_cache")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string]map[string]string)
	for rows.Next() {
		var nodeName, key, value string
		if err := rows.Scan(&nodeName, &key, &value); err != nil {
			return nil, err
		}
		if res[nodeName] == nil {
			res[nodeName] = make(map[string]string)
		}
		res[nodeName][key] = value
	}
	return res, rows.Err()
}

// Set stores (or replaces) a single cache entry.
func (s *CacheStore) Set(nodeName, key, value string) error {
	if s.db == nil {
		return fmt.Errorf("cache store has no database handle")
	}
	_, err := s.db.Exec(
		`INSERT INTO agent_cache (node_name, cache_key, value, updated_at) VALUES (?, ?, ?, strftime('%s','now'))
		 ON CONFLICT(node_name, cache_key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		nodeName, key, value,
	)
	return err
}

// SetMany stores multiple cache entries for a node in one transaction.
func (s *CacheStore) SetMany(nodeName string, entries map[string]string) error {
	if len(entries) == 0 {
		return nil
	}
	if s.db == nil {
		return fmt.Errorf("cache store has no database handle")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.Prepare(
		`INSERT INTO agent_cache (node_name, cache_key, value, updated_at) VALUES (?, ?, ?, strftime('%s','now'))
		 ON CONFLICT(node_name, cache_key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for key, value := range entries {
		if _, err := stmt.Exec(nodeName, key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Delete removes a single cache entry.
func (s *CacheStore) Delete(nodeName, key string) error {
	if s.db == nil {
		return fmt.Errorf("cache store has no database handle")
	}
	_, err := s.db.Exec("DELETE FROM agent_cache WHERE node_name = ? AND cache_key = ?", nodeName, key)
	return err
}

// DeletePrefix removes every cache entry of a node whose key starts with prefix.
func (s *CacheStore) DeletePrefix(nodeName, prefix string) error {
	if s.db == nil {
		return fmt.Errorf("cache store has no database handle")
	}
	_, err := s.db.Exec(
		"DELETE FROM agent_cache WHERE node_name = ? AND substr(cache_key, 1, ?) = ?",
		nodeName, len(prefix), prefix,
	)
	return err
}

// DeleteNode removes every cache entry of a node.
func (s *CacheStore) DeleteNode(nodeName string) error {
	if s.db == nil {
		return fmt.Errorf("cache store has no database handle")
	}
	_, err := s.db.Exec("DELETE FROM agent_cache WHERE node_name = ?", nodeName)
	return err
}
