package migrate

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed *.sql
var MigrationFS embed.FS

// Migration represents a single SQL migration file
type Migration struct {
	Version int
	Name    string
	SQL     string
}

var migrationFileRegex = regexp.MustCompile(`^(\d+)_.+\.sql$`)

// LoadMigrations reads and parses all embedded SQL migration files sorted by version
func LoadMigrations() ([]Migration, error) {
	entries, err := fs.ReadDir(MigrationFS, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded migrations: %w", err)
	}

	var migrations []Migration
	seenVersions := make(map[int]string)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		matches := migrationFileRegex.FindStringSubmatch(entry.Name())
		if len(matches) < 2 {
			continue
		}

		version, err := strconv.Atoi(matches[1])
		if err != nil {
			return nil, fmt.Errorf("invalid migration version in filename %s: %w", entry.Name(), err)
		}

		if existing, exists := seenVersions[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %d in %s and %s", version, existing, entry.Name())
		}
		seenVersions[version] = entry.Name()

		content, err := fs.ReadFile(MigrationFS, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", entry.Name(), err)
		}

		migrations = append(migrations, Migration{
			Version: version,
			Name:    entry.Name(),
			SQL:     string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// Apply executes all pending migrations in ascending order inside transactions
func Apply(db *sql.DB) error {
	// Create schema_migrations table if it doesn't exist
	createMigrationsTable := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		applied_at INTEGER NOT NULL
	);`
	if _, err := db.Exec(createMigrationsTable); err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	migrations, err := LoadMigrations()
	if err != nil {
		return err
	}

	// Check if this is an existing database created before schema_migrations was introduced
	var migrationCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		return fmt.Errorf("failed to query schema_migrations count: %w", err)
	}

	if migrationCount == 0 {
		var hasNodeStates int
		_ = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='node_states'").Scan(&hasNodeStates)
		if hasNodeStates > 0 {
			// Legacy database detected. Ensure legacy columns are present before baseline stamping.
			if err := ensureLegacyColumns(db); err != nil {
				return err
			}
		}
	}

	// Query already applied migration versions
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("failed to scan applied migration version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error reading applied migrations: %w", err)
	}

	// Execute unapplied migrations
	for _, m := range migrations {
		if applied[m.Version] {
			continue
		}

		if err := applyMigration(db, m); err != nil {
			return err
		}
	}

	return nil
}

// applyMigration executes a single migration and records its execution
func applyMigration(db *sql.DB, m Migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction for migration %s: %w", m.Name, err)
	}

	if _, err := tx.Exec(m.SQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to execute migration %s: %w", m.Name, err)
	}

	recordSQL := "INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)"
	if _, err := tx.Exec(recordSQL, m.Version, m.Name, time.Now().Unix()); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to record migration %s in schema_migrations: %w", m.Name, err)
	}

	setVersionSQL := fmt.Sprintf("PRAGMA user_version = %d", m.Version)
	if _, err := tx.Exec(setVersionSQL); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to update user_version for migration %s: %w", m.Name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit migration %s: %w", m.Name, err)
	}

	return nil
}

// CurrentVersion returns the highest applied migration version or -1 if none applied
func CurrentVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("SELECT COALESCE(MAX(version), -1) FROM schema_migrations").Scan(&version)
	if err != nil {
		// If table doesn't exist, return -1
		return -1, nil
	}
	return version, nil
}

// ensureLegacyColumns ensures columns added in earlier ad-hoc migrations exist on legacy databases
func ensureLegacyColumns(db *sql.DB) error {
	cols, err := getTableColumns(db, "node_states")
	if err != nil {
		return err
	}

	if !cols["hosts_config_hash"] {
		if _, err := db.Exec("ALTER TABLE node_states ADD COLUMN hosts_config_hash TEXT NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("failed to add hosts_config_hash column: %w", err)
		}
	}

	if !cols["hosts_applied_at"] {
		if _, err := db.Exec("ALTER TABLE node_states ADD COLUMN hosts_applied_at INTEGER"); err != nil {
			return fmt.Errorf("failed to add hosts_applied_at column: %w", err)
		}
	}

	return nil
}

// getTableColumns returns a set of column names for the given table
func getTableColumns(db *sql.DB, tableName string) (map[string]bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, fmt.Errorf("failed to query table_info for %s: %w", tableName, err)
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return nil, fmt.Errorf("failed to scan column info: %w", err)
		}
		cols[name] = true
	}
	return cols, rows.Err()
}
