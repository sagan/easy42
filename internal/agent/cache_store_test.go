package agent

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"easy42/internal/config/migrate"
	agentpb "easy42/internal/agent/proto"

	_ "modernc.org/sqlite"
)

// openTestCacheDB opens (or creates) the SQLite file at path and applies the
// shared state migrations, which include the agent_cache table (0006).
func openTestCacheDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate.Apply(db); err != nil {
		_ = db.Close()
		t.Fatalf("migrate.Apply failed: %v", err)
	}
	return db
}

func TestHubCachePersistsAcrossRestart(t *testing.T) {
	dir, err := os.MkdirTemp("", "agent-cache-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "data.db")

	// First "server run": populate every cache.
	db := openTestCacheDB(t, path)
	hub := NewHub()
	if err := hub.SetCacheStore(NewCacheStore(db)); err != nil {
		t.Fatalf("SetCacheStore failed: %v", err)
	}
	hub.SetAppHashes("n1", map[string]string{"app1": "hash1", "app2": "hash2"})
	hub.SetAppFiles("n1", map[string]ProbeConfigFile{
		"/etc/ezappc/ezapp-app1/compose.yaml": {Path: "/etc/ezappc/ezapp-app1/compose.yaml", Content: "services: {}", Hash: "h"},
	})
	hub.SetAppEnabled("n1", "app1", true)
	hub.SetAppEnabled("n1", "app2", false)
	hub.SetProbeConfigs("n1", map[string]ProbeConfigFile{
		"wg42peer": {Path: "/etc/wireguard/wg42peer.conf", Content: "[Interface]", Hash: "wghash"},
	})
	hub.RecordLiveReport("n1")
	hub.SetAppHash("n1", "app3", "hash3")
	hub.RemoveAppHash("n1", "app3")

	// Second "server run": a brand new hub must restore everything.
	hub2 := NewHub()
	if err := hub2.SetCacheStore(NewCacheStore(db)); err != nil {
		t.Fatalf("SetCacheStore (restart) failed: %v", err)
	}

	hashes := hub2.GetAppHashes("n1")
	if hashes["app1"] != "hash1" || hashes["app2"] != "hash2" {
		t.Errorf("app hashes not restored: %+v", hashes)
	}
	if _, exists := hashes["app3"]; exists {
		t.Errorf("removed app hash was restored: %+v", hashes)
	}
	if files := hub2.GetAppFiles("n1"); files["/etc/ezappc/ezapp-app1/compose.yaml"].Content != "services: {}" {
		t.Errorf("app files not restored: %+v", files)
	}
	if enabled, known := hub2.GetAppEnabled("n1", "app1"); !known || !enabled {
		t.Errorf("app1 enabled state not restored")
	}
	if enabled, known := hub2.GetAppEnabled("n1", "app2"); !known || enabled {
		t.Errorf("app2 disabled state not restored")
	}
	if cfgs := hub2.GetProbeConfigs("n1"); cfgs["wg42peer"].Hash != "wghash" {
		t.Errorf("probe configs not restored: %+v", cfgs)
	}
	if hub2.LastLiveReport("n1").IsZero() {
		t.Errorf("last live report not restored")
	}

	// Removal must also be persisted.
	hub2.RemoveAppHash("n1", "app1")
	hub3 := NewHub()
	if err := hub3.SetCacheStore(NewCacheStore(db)); err != nil {
		t.Fatalf("SetCacheStore (restart 2) failed: %v", err)
	}
	if _, exists := hub3.GetAppHashes("n1")["app1"]; exists {
		t.Errorf("removed app hash reappeared after restart")
	}

	// RemoveNode clears the persisted cache for that node.
	hub3.RemoveNode("n1")
	hub4 := NewHub()
	if err := hub4.SetCacheStore(NewCacheStore(db)); err != nil {
		t.Fatalf("SetCacheStore (restart 3) failed: %v", err)
	}
	if len(hub4.GetAppHashes("n1")) != 0 {
		t.Errorf("node cache not cleared after RemoveNode")
	}
	_ = time.Now()
	_ = db.Close()
}

// TestAgentCacheTableComesFromSharedMigrations verifies the agent_cache table is
// created by the shared migration set in internal/config/migrate (not by a
// separate agent-local schema), is recorded in schema_migrations, and that
// re-applying migrations is idempotent while data is preserved.
func TestAgentCacheTableComesFromSharedMigrations(t *testing.T) {
	dir, err := os.MkdirTemp("", "agent-cache-migrate-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "data.db")

	db := openTestCacheDB(t, path)

	var tbl int
	if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='agent_cache'").Scan(&tbl); err != nil || tbl == 0 {
		t.Fatalf("agent_cache table missing after migrate.Apply (err=%v)", err)
	}
	v, err := migrate.CurrentVersion(db)
	if err != nil {
		t.Fatal(err)
	}
	if v != 6 {
		t.Errorf("current migration version = %d, want 6 (0006_agent_cache.sql)", v)
	}
	var migName string
	if err := db.QueryRow("SELECT name FROM schema_migrations WHERE version = 6").Scan(&migName); err != nil {
		t.Fatalf("schema_migrations row for version 6 missing: %v", err)
	}
	if migName != "0006_agent_cache.sql" {
		t.Errorf("version 6 migration = %q, want 0006_agent_cache.sql", migName)
	}
	var userVersion int
	if err := db.QueryRow("PRAGMA user_version").Scan(&userVersion); err != nil || userVersion != 6 {
		t.Errorf("user_version = %d (err=%v), want 6", userVersion, err)
	}

	// Write through the cache store, then re-apply migrations (idempotency).
	s := NewCacheStore(db)
	if err := s.Set("n1", "app_hash:app1", "h1"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := migrate.Apply(db); err != nil {
		t.Fatalf("re-apply migrations failed: %v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 6").Scan(&count); err != nil || count != 1 {
		t.Errorf("migration 6 applied %d times, want 1 (err=%v)", count, err)
	}

	// Reopen the file: data must still be there and migrations must not re-run.
	_ = db.Close()
	db2 := openTestCacheDB(t, path)
	defer db2.Close()
	all, err := NewCacheStore(db2).LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if all["n1"]["app_hash:app1"] != "h1" {
		t.Errorf("data not preserved across reopen: %+v", all["n1"])
	}
	if err := migrate.Apply(db2); err != nil {
		t.Fatalf("second re-apply failed: %v", err)
	}
	if err := db2.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 6").Scan(&count); err != nil || count != 1 {
		t.Errorf("migration 6 re-applied on existing DB (count=%d err=%v)", count, err)
	}
}

func TestHubNodeMetaPersistsAcrossRestart(t *testing.T) {
	dir, err := os.MkdirTemp("", "agent-meta-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "data.db")

	// First "server run": registration metadata arrives and is persisted.
	db := openTestCacheDB(t, path)
	hub := NewHub()
	if err := hub.SetCacheStore(NewCacheStore(db)); err != nil {
		t.Fatalf("SetCacheStore failed: %v", err)
	}
	hub.SetNodeMeta("n1", NodeMeta{
		AgentVersion: "1.2.3",
		OSInfo:       "linux aarch64 little",
		InitSystem:   "openrc",
		Hostname:     "router1",
		HostsHash:    "h1",
	})
	hub.UpdateHostsHash("n1", "h2") // heartbeat
	hub.UpdateTelemetry("n1", &agentpb.TelemetryReport{HostsHash: "h3", DisableApps: true})

	// Second "server run": the agent has not connected yet, but every getter
	// must still answer from the persisted metadata.
	hub2 := NewHub()
	if err := hub2.SetCacheStore(NewCacheStore(db)); err != nil {
		t.Fatalf("SetCacheStore (restart) failed: %v", err)
	}
	if hub2.IsConnected("n1") {
		t.Fatalf("node must not be connected in the second run")
	}
	if got := hub2.GetOSInfo("n1"); got != "linux aarch64 little" {
		t.Errorf("GetOSInfo after restart = %q", got)
	}
	if got := hub2.GetInitSystem("n1"); got != "openrc" {
		t.Errorf("GetInitSystem after restart = %q", got)
	}
	if got := hub2.GetHostname("n1"); got != "router1" {
		t.Errorf("GetHostname after restart = %q", got)
	}
	if got := hub2.GetAgentVersion("n1"); got != "1.2.3" {
		t.Errorf("GetAgentVersion after restart = %q", got)
	}
	if got := hub2.GetHostsHash("n1"); got != "h3" {
		t.Errorf("GetHostsHash after restart = %q, want latest telemetry value h3", got)
	}
	if !hub2.IsAppsDisabled("n1") {
		t.Errorf("IsAppsDisabled after restart = false, want true")
	}
	meta := hub2.GetNodeMeta("n1")
	if meta == nil || meta.OSInfo != "linux aarch64 little" || meta.InitSystem != "openrc" {
		t.Errorf("GetNodeMeta after restart = %+v", meta)
	}

	// RemoveNode must clear the persisted metadata as well.
	hub2.RemoveNode("n1")
	hub3 := NewHub()
	if err := hub3.SetCacheStore(NewCacheStore(db)); err != nil {
		t.Fatalf("SetCacheStore (restart 2) failed: %v", err)
	}
	if got := hub3.GetOSInfo("n1"); got != "" {
		t.Errorf("GetOSInfo after RemoveNode = %q, want empty", got)
	}
	if hub3.GetNodeMeta("n1") != nil {
		t.Errorf("node meta not cleared after RemoveNode")
	}
	if hub3.GetHostsHash("n1") != "" {
		t.Errorf("GetHostsHash after RemoveNode = %q, want empty", hub3.GetHostsHash("n1"))
	}
	_ = db.Close()
}
