package migrate

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	return db
}

func TestLoadMigrations(t *testing.T) {
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations failed: %v", err)
	}

	if len(migrations) == 0 {
		t.Fatalf("Expected at least 1 migration, got 0")
	}

	if migrations[0].Version != 0 {
		t.Errorf("Expected migration 0 to be baseline, got version %d", migrations[0].Version)
	}
	if migrations[0].Name != "0000_baseline.sql" {
		t.Errorf("Expected migration name 0000_baseline.sql, got %s", migrations[0].Name)
	}
}

func TestApplyFreshDB(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if err := Apply(db); err != nil {
		t.Fatalf("Apply failed on fresh DB: %v", err)
	}

	v, err := CurrentVersion(db)
	if err != nil {
		t.Fatalf("CurrentVersion failed: %v", err)
	}
	if v != 1 {
		t.Errorf("Expected current version 1, got %d", v)
	}

	// Verify PRAGMA user_version
	var userVersion int
	if err := db.QueryRow("PRAGMA user_version").Scan(&userVersion); err != nil {
		t.Fatalf("Failed to query user_version: %v", err)
	}
	if userVersion != 1 {
		t.Errorf("Expected PRAGMA user_version 1, got %d", userVersion)
	}

	// Verify key baseline tables exist
	tables := []string{
		"meta",
		"node_states",
		"node_info",
		"node_metrics",
		"node_interface_metrics",
		"node_interface_traffic_stats",
		"cron_task_results",
		"server_task_states",
		"node_metrics_5m",
		"node_interface_traffic_stats_5m",
		"cron_task_results_5m",
		"ping_table_tasks",
	}

	for _, tbl := range tables {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&exists)
		if err != nil || exists == 0 {
			t.Errorf("Expected table %s to exist", tbl)
		}
	}

	// Verify idempotency
	if err := Apply(db); err != nil {
		t.Fatalf("Second Apply failed: %v", err)
	}
}

func TestApplyLegacyDB(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Simulate a legacy database before hosts_config_hash was added
	legacyDDL := `
	CREATE TABLE node_states (
		node_name TEXT PRIMARY KEY,
		host TEXT NOT NULL DEFAULT '',
		last_seen INTEGER
	);
	`
	if _, err := db.Exec(legacyDDL); err != nil {
		t.Fatalf("Failed to create legacy table: %v", err)
	}

	// Apply migrations
	if err := Apply(db); err != nil {
		t.Fatalf("Apply failed on legacy DB: %v", err)
	}

	v, err := CurrentVersion(db)
	if err != nil {
		t.Fatalf("CurrentVersion failed: %v", err)
	}
	if v != 1 {
		t.Errorf("Expected current version 1, got %d", v)
	}

	// Verify that hosts_config_hash and hosts_applied_at columns were added
	cols, err := getTableColumns(db, "node_states")
	if err != nil {
		t.Fatalf("getTableColumns failed: %v", err)
	}
	if !cols["hosts_config_hash"] {
		t.Errorf("Expected column hosts_config_hash to be added to legacy table")
	}
	if !cols["hosts_applied_at"] {
		t.Errorf("Expected column hosts_applied_at to be added to legacy table")
	}
}

func TestApplyExistingDBWithMissingColumns(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	// Simulate an existing database where migrations 0 and 1 are already recorded
	initSQL := `
	CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at INTEGER NOT NULL
	);
	INSERT INTO schema_migrations (version, name, applied_at) VALUES (0, '0000_baseline.sql', 1700000000);
	INSERT INTO schema_migrations (version, name, applied_at) VALUES (1, '0001_ping_table.sql', 1700000001);

	CREATE TABLE node_states (
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
	`
	if _, err := db.Exec(initSQL); err != nil {
		t.Fatalf("Failed to initialize existing DB: %v", err)
	}

	// Verify columns are initially missing
	cols, err := getTableColumns(db, "node_states")
	if err != nil {
		t.Fatalf("getTableColumns failed: %v", err)
	}
	if cols["app_config_hashes"] || cols["app_applied_at"] {
		t.Fatalf("Expected app columns to be absent initially")
	}

	// Apply migrations
	if err := Apply(db); err != nil {
		t.Fatalf("Apply failed on existing DB: %v", err)
	}

	// Verify columns were added by ensureLegacyColumns even with migrationCount > 0
	cols, err = getTableColumns(db, "node_states")
	if err != nil {
		t.Fatalf("getTableColumns failed: %v", err)
	}
	if !cols["app_config_hashes"] {
		t.Errorf("Expected column app_config_hashes to be added to existing table")
	}
	if !cols["app_applied_at"] {
		t.Errorf("Expected column app_applied_at to be added to existing table")
	}

	// Verify querying the full column list succeeds
	var count int
	err = db.QueryRow("SELECT COUNT(app_config_hashes) FROM node_states").Scan(&count)
	if err != nil {
		t.Fatalf("SELECT with app_config_hashes failed: %v", err)
	}
}
