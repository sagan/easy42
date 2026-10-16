package config

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// WorkingState constants for WireGuard interface links
const (
	WorkingStateWorking    = "working"     // Latest handshake <= 3 minutes
	WorkingStateNotWorking = "not_working" // No handshake or > 3 min with PersistentKeepalive
	WorkingStateUnknown    = "unknown"     // No handshake or > 3 min without PersistentKeepalive
)

// StateInterface represents an applied/observed WireGuard interface on a device
type StateInterface struct {
	Name            string     `json:"name"`
	TargetFile      string     `json:"target_file"`
	ConfigHash      string     `json:"config_hash"`
	PeerNode        string     `json:"peer_node,omitempty"`
	PeerPubKey      string     `json:"peer_pub_key,omitempty"`
	ListenPort      int        `json:"listen_port,omitempty"`
	Address         string     `json:"address,omitempty"`
	Status          string     `json:"status,omitempty"` // "active", "down"
	LatestHandshake *time.Time `json:"latest_handshake,omitempty"`
	WorkingState    string     `json:"working_state,omitempty"` // "working", "not_working", "unknown"
	TransferRxBytes int64      `json:"transfer_rx_bytes,omitempty"`
	TransferTxBytes int64      `json:"transfer_tx_bytes,omitempty"`
	AppliedAt       time.Time  `json:"applied_at,omitempty"`
}

// StateNode represents the actual applied/observed state of a node
type StateNode struct {
	Name               string                    `json:"name"`
	Host               string                    `json:"host"`
	LastSeen           time.Time                 `json:"last_seen,omitempty"`
	BirdConfigHash     string                    `json:"bird_config_hash,omitempty"`
	BirdAppliedAt      *time.Time                `json:"bird_applied_at,omitempty"`
	NftablesConfigHash string                    `json:"nftables_config_hash,omitempty"`
	NftablesAppliedAt  *time.Time                `json:"nftables_applied_at,omitempty"`
	RoaConfigHashes    map[string]string         `json:"roa_config_hashes,omitempty"` // key: targetFile, val: sha256 hash
	RoaAppliedAt       map[string]time.Time      `json:"roa_applied_at,omitempty"`
	HostsConfigHash    string                    `json:"hosts_config_hash,omitempty"`
	HostsAppliedAt     *time.Time                `json:"hosts_applied_at,omitempty"`
	Interfaces         map[string]StateInterface `json:"interfaces"` // key: interface name e.g. "wg42node2"
}

// NetworkState represents the recorded state in the database
type NetworkState struct {
	Version   int                  `json:"version"`
	UpdatedAt time.Time            `json:"updated_at"`
	Nodes     map[string]StateNode `json:"nodes"` // key: node name
}

// HashConfig computes a SHA256 hash of normalized config content
func HashConfig(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// InterfaceMetricInput represents a per-interface snapshot reported by an agent
type InterfaceMetricInput struct {
	Name      string `json:"name"`
	RxBytes   uint64 `json:"rx_bytes"`
	TxBytes   uint64 `json:"tx_bytes"`
	IsPrimary bool   `json:"is_primary,omitempty"`
}

// NodeMetricInput represents an incoming telemetry point to be recorded
type NodeMetricInput struct {
	Timestamp        time.Time
	CPUPercent       float32
	MemoryUsedBytes  uint64
	MemoryTotalBytes uint64
	UptimeSeconds    uint64
	Load1m           float32
	Load5m           float32
	Load15m          float32
	NetRxBytes       uint64
	NetTxBytes       uint64
	Interfaces       []InterfaceMetricInput
}

// NodeMetricPoint represents a recorded telemetry snapshot at a point in time
type NodeMetricPoint struct {
	Timestamp        int64   `json:"timestamp"` // Unix timestamp in seconds
	CPUPercent       float32 `json:"cpu_percent"`
	MemoryUsedBytes  uint64  `json:"memory_used_bytes"`
	MemoryTotalBytes uint64  `json:"memory_total_bytes"`
	UptimeSeconds    uint64  `json:"uptime_seconds"`
	Load1m           float32 `json:"load_1m"`
	Load5m           float32 `json:"load_5m"`
	Load15m          float32 `json:"load_15m"`
	NetRxBytes       uint64  `json:"net_rx_bytes"`
	NetTxBytes       uint64  `json:"net_tx_bytes"`
	NetRxRate        float64 `json:"net_rx_rate"`        // bytes/second
	NetTxRate        float64 `json:"net_tx_rate"`        // bytes/second
	TrafficRxBytes   uint64  `json:"traffic_rx_bytes"`   // Accumulated rx traffic starting from 0 in queried timespan
	TrafficTxBytes   uint64  `json:"traffic_tx_bytes"`   // Accumulated tx traffic starting from 0 in queried timespan
}

// InterfaceTrafficPoint represents a point in the derived traffic time series
type InterfaceTrafficPoint struct {
	Timestamp      int64   `json:"timestamp"`
	RawRxBytes     uint64  `json:"raw_rx_bytes"`
	RawTxBytes     uint64  `json:"raw_tx_bytes"`
	AccumRxBytes   uint64  `json:"accum_rx_bytes"`
	AccumTxBytes   uint64  `json:"accum_tx_bytes"`
	TrafficRxBytes uint64  `json:"traffic_rx_bytes"` // Normalized to 0 at timespan start
	TrafficTxBytes uint64  `json:"traffic_tx_bytes"` // Normalized to 0 at timespan start
	RxRate         float64 `json:"rx_rate"`
	TxRate         float64 `json:"tx_rate"`
	ResetDetected  bool    `json:"reset_detected"`
}

// NodeInterfaceTrafficSummary represents derived traffic statistics over a timespan
type NodeInterfaceTrafficSummary struct {
	NodeName      string                  `json:"node_name"`
	InterfaceName string                  `json:"interface_name"`
	StartTime     int64                   `json:"start_time"`
	EndTime       int64                   `json:"end_time"`
	TotalRxBytes  uint64                  `json:"total_rx_bytes"`
	TotalTxBytes  uint64                  `json:"total_tx_bytes"`
	Points        []InterfaceTrafficPoint `json:"points"`
}

// NodeInfo represents transient metadata about a node agent
type NodeInfo struct {
	NodeName      string    `json:"node_name"`
	AgentVersion  string    `json:"agent_version"`
	OSInfo        string    `json:"os_info"`
	Hostname      string    `json:"hostname"`
	UptimeSeconds uint64    `json:"uptime_seconds"`
	LastSeen      time.Time `json:"last_seen"`
}

// FleetMetricsSummary represents high-level metrics across all nodes
type FleetMetricsSummary struct {
	TotalNodes     int     `json:"total_nodes"`
	OnlineNodes    int     `json:"online_nodes"`
	OfflineNodes   int     `json:"offline_nodes"`
	NoAgentNodes   int     `json:"no_agent_nodes"`
	TotalRxRate    float64 `json:"total_rx_rate"`    // bytes/sec aggregate
	TotalTxRate    float64 `json:"total_tx_rate"`    // bytes/sec aggregate
	AverageCPU     float32 `json:"average_cpu"`      // fleet average CPU %
	AverageMemPerc float32 `json:"average_mem_perc"` // fleet average Mem %
}

// NodeLiveStatus represents live node info + latest metric snapshot
type NodeLiveStatus struct {
	Name           string           `json:"name"`
	Hostname       string           `json:"hostname"`
	OSInfo         string           `json:"os_info"`
	AgentVersion   string           `json:"agent_version"`
	UptimeSeconds  uint64           `json:"uptime_seconds"`
	Connected      bool             `json:"connected"`
	AgentInstalled   bool             `json:"agent_installed"`
	LastSeen         time.Time        `json:"last_seen"`
	PrimaryInterface string           `json:"primary_interface,omitempty"`
	Metrics          *NodeMetricPoint `json:"metrics,omitempty"`
}

type prevNetSample struct {
	timestamp time.Time
	rxBytes   uint64
	txBytes   uint64
}

// StateStore handles thread-safe loading and persisting of state into SQLite (data.db)
type StateStore struct {
	mu       sync.RWMutex
	dataDir  string
	filePath string // data.db
	db       *sql.DB
	state    *NetworkState

	// in-memory tracking of previous network transfer for rate calculation
	prevNetMu    sync.Mutex
	prevNet      map[string]prevNetSample
	prevIfaceNet map[string]prevNetSample
}

// NewStateStore creates a new StateStore pointing to the given data directory
func NewStateStore(dataDir string) *StateStore {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	return &StateStore{
		dataDir:      dataDir,
		filePath:     filepath.Join(dataDir, "data.db"),
		prevNet:      make(map[string]prevNetSample),
		prevIfaceNet: make(map[string]prevNetSample),
	}
}

// FilePath returns the path to data.db
func (s *StateStore) FilePath() string {
	return s.filePath
}

// Exists checks if data.db exists
func (s *StateStore) Exists() bool {
	_, err := os.Stat(s.filePath)
	return err == nil
}

// initDBLocked opens and migrates the SQLite database while mu is held
func (s *StateStore) initDBLocked() error {
	if s.db != nil {
		return nil
	}

	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	dsn := fmt.Sprintf("%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", s.filePath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Limit to single connection to serialize writes and eliminate SQLite lock errors
	db.SetMaxOpenConns(1)

	schema := `
	PRAGMA journal_mode = WAL;
	PRAGMA synchronous = NORMAL;
	PRAGMA busy_timeout = 5000;

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
	`

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to initialize sqlite schema: %w", err)
	}

	// Schema migrations for new columns
	_, _ = db.Exec("ALTER TABLE node_states ADD COLUMN hosts_config_hash TEXT NOT NULL DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE node_states ADD COLUMN hosts_applied_at INTEGER")

	s.db = db

	// Check if legacy state.json exists and migrate it
	s.migrateLegacyStateJSONLocked()

	return nil
}

// migrateLegacyStateJSONLocked migrates state.json into node_states if present
func (s *StateStore) migrateLegacyStateJSONLocked() {
	legacyFile := filepath.Join(s.dataDir, "state.json")
	if _, err := os.Stat(legacyFile); err != nil {
		return
	}

	data, err := os.ReadFile(legacyFile)
	if err != nil {
		return
	}

	var legacySt NetworkState
	if err := json.Unmarshal(data, &legacySt); err != nil || len(legacySt.Nodes) == 0 {
		return
	}

	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM node_states").Scan(&count)
	if count == 0 {
		for _, n := range legacySt.Nodes {
			_ = s.saveNodeLocked(n)
		}
	}

	// Rename legacy state.json so it won't be processed again
	_ = os.Rename(legacyFile, filepath.Join(s.dataDir, "state.json.migrated"))
}

// Load reads state from data.db or initializes an empty state if not found
func (s *StateStore) Load() (*NetworkState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return nil, err
	}

	st := &NetworkState{
		Version:   1,
		UpdatedAt: time.Now(),
		Nodes:     make(map[string]StateNode),
	}

	var updatedAtStr string
	err := s.db.QueryRow("SELECT value FROM meta WHERE key = 'updated_at'").Scan(&updatedAtStr)
	if err == nil {
		if t, err := time.Parse(time.RFC3339Nano, updatedAtStr); err == nil {
			st.UpdatedAt = t
		}
	}

	rows, err := s.db.Query(`
		SELECT node_name, host, last_seen, bird_config_hash, bird_applied_at,
		       nftables_config_hash, nftables_applied_at, roa_config_hashes,
		       roa_applied_at, hosts_config_hash, hosts_applied_at, interfaces
		FROM node_states
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query node states: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			nodeName, host, birdHash, nftHash, hostsHash string
			lastSeenUnix                                 sql.NullInt64
			birdAppliedUnix                              sql.NullInt64
			nftAppliedUnix                               sql.NullInt64
			hostsAppliedUnix                             sql.NullInt64
			roaHashesJSON, roaAppliedJSON                string
			ifacesJSON                                   string
		)

		if err := rows.Scan(
			&nodeName, &host, &lastSeenUnix, &birdHash, &birdAppliedUnix,
			&nftHash, &nftAppliedUnix, &roaHashesJSON, &roaAppliedJSON,
			&hostsHash, &hostsAppliedUnix, &ifacesJSON,
		); err != nil {
			continue
		}

		node := StateNode{
			Name:               nodeName,
			Host:               host,
			BirdConfigHash:     birdHash,
			NftablesConfigHash: nftHash,
			HostsConfigHash:    hostsHash,
			RoaConfigHashes:    make(map[string]string),
			RoaAppliedAt:       make(map[string]time.Time),
			Interfaces:         make(map[string]StateInterface),
		}

		if lastSeenUnix.Valid && lastSeenUnix.Int64 > 0 {
			node.LastSeen = time.Unix(lastSeenUnix.Int64, 0)
		}
		if birdAppliedUnix.Valid && birdAppliedUnix.Int64 > 0 {
			t := time.Unix(birdAppliedUnix.Int64, 0)
			node.BirdAppliedAt = &t
		}
		if nftAppliedUnix.Valid && nftAppliedUnix.Int64 > 0 {
			t := time.Unix(nftAppliedUnix.Int64, 0)
			node.NftablesAppliedAt = &t
		}
		if hostsAppliedUnix.Valid && hostsAppliedUnix.Int64 > 0 {
			t := time.Unix(hostsAppliedUnix.Int64, 0)
			node.HostsAppliedAt = &t
		}

		if roaHashesJSON != "" {
			_ = json.Unmarshal([]byte(roaHashesJSON), &node.RoaConfigHashes)
		}
		if roaAppliedJSON != "" {
			_ = json.Unmarshal([]byte(roaAppliedJSON), &node.RoaAppliedAt)
		}
		if ifacesJSON != "" {
			_ = json.Unmarshal([]byte(ifacesJSON), &node.Interfaces)
		}

		st.Nodes[nodeName] = node
	}

	s.state = st
	return s.state, nil
}

// Get returns the in-memory state snapshot
func (s *StateStore) Get() *NetworkState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state == nil {
		return &NetworkState{
			Version:   1,
			UpdatedAt: time.Now(),
			Nodes:     make(map[string]StateNode),
		}
	}
	return s.state
}

// saveNodeLocked saves a single StateNode into SQLite
func (s *StateStore) saveNodeLocked(node StateNode) error {
	roaHashesJSON, _ := json.Marshal(node.RoaConfigHashes)
	roaAppliedJSON, _ := json.Marshal(node.RoaAppliedAt)
	ifacesJSON, _ := json.Marshal(node.Interfaces)

	var lastSeenUnix *int64
	if !node.LastSeen.IsZero() {
		u := node.LastSeen.Unix()
		lastSeenUnix = &u
	}

	var birdUnix *int64
	if node.BirdAppliedAt != nil {
		u := node.BirdAppliedAt.Unix()
		birdUnix = &u
	}

	var nftUnix *int64
	if node.NftablesAppliedAt != nil {
		u := node.NftablesAppliedAt.Unix()
		nftUnix = &u
	}

	var hostsUnix *int64
	if node.HostsAppliedAt != nil {
		u := node.HostsAppliedAt.Unix()
		hostsUnix = &u
	}

	query := `
	INSERT INTO node_states (
		node_name, host, last_seen, bird_config_hash, bird_applied_at,
		nftables_config_hash, nftables_applied_at, roa_config_hashes,
		roa_applied_at, hosts_config_hash, hosts_applied_at, interfaces
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(node_name) DO UPDATE SET
		host=excluded.host,
		last_seen=excluded.last_seen,
		bird_config_hash=excluded.bird_config_hash,
		bird_applied_at=excluded.bird_applied_at,
		nftables_config_hash=excluded.nftables_config_hash,
		nftables_applied_at=excluded.nftables_applied_at,
		roa_config_hashes=excluded.roa_config_hashes,
		roa_applied_at=excluded.roa_applied_at,
		hosts_config_hash=excluded.hosts_config_hash,
		hosts_applied_at=excluded.hosts_applied_at,
		interfaces=excluded.interfaces;
	`

	_, err := s.db.Exec(query,
		node.Name, node.Host, lastSeenUnix, node.BirdConfigHash, birdUnix,
		node.NftablesConfigHash, nftUnix, string(roaHashesJSON), string(roaAppliedJSON),
		node.HostsConfigHash, hostsUnix, string(ifacesJSON),
	)
	return err
}

// Save persists the full NetworkState to SQLite
func (s *StateStore) Save(st *NetworkState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	st.UpdatedAt = time.Now()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec("DELETE FROM node_states"); err != nil {
		return err
	}

	for _, node := range st.Nodes {
		roaHashesJSON, _ := json.Marshal(node.RoaConfigHashes)
		roaAppliedJSON, _ := json.Marshal(node.RoaAppliedAt)
		ifacesJSON, _ := json.Marshal(node.Interfaces)

		var lastSeenUnix *int64
		if !node.LastSeen.IsZero() {
			u := node.LastSeen.Unix()
			lastSeenUnix = &u
		}

		var birdUnix *int64
		if node.BirdAppliedAt != nil {
			u := node.BirdAppliedAt.Unix()
			birdUnix = &u
		}

		var nftUnix *int64
		if node.NftablesAppliedAt != nil {
			u := node.NftablesAppliedAt.Unix()
			nftUnix = &u
		}

		query := `
		INSERT INTO node_states (
			node_name, host, last_seen, bird_config_hash, bird_applied_at,
			nftables_config_hash, nftables_applied_at, roa_config_hashes,
			roa_applied_at, interfaces
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
		`
		if _, err := tx.Exec(query,
			node.Name, node.Host, lastSeenUnix, node.BirdConfigHash, birdUnix,
			node.NftablesConfigHash, nftUnix, string(roaHashesJSON), string(roaAppliedJSON), string(ifacesJSON),
		); err != nil {
			return err
		}
	}

	_, _ = tx.Exec(
		"INSERT INTO meta (key, value) VALUES ('updated_at', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value",
		st.UpdatedAt.Format(time.RFC3339Nano),
	)

	if err := tx.Commit(); err != nil {
		return err
	}

	s.state = st
	return nil
}

// UpdateInterface records or updates an interface state for a node
func (s *StateStore) UpdateInterface(nodeName, host string, iface StateInterface) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if s.state == nil {
		s.state = &NetworkState{
			Version:   1,
			UpdatedAt: time.Now(),
			Nodes:     make(map[string]StateNode),
		}
	}

	node, exists := s.state.Nodes[nodeName]
	if !exists {
		node = StateNode{
			Name:       nodeName,
			Host:       host,
			LastSeen:   time.Now(),
			Interfaces: make(map[string]StateInterface),
		}
	}
	if node.Interfaces == nil {
		node.Interfaces = make(map[string]StateInterface)
	}
	node.Host = host
	node.LastSeen = time.Now()
	node.Interfaces[iface.Name] = iface
	s.state.Nodes[nodeName] = node
	s.state.UpdatedAt = time.Now()

	return s.saveNodeLocked(node)
}

// UpdateInterfacesTelemetry updates live telemetry metrics (status, handshakes, transfer bytes, working state)
// for the given node's interfaces in stateStore without overwriting configuration hashes.
func (s *StateStore) UpdateInterfacesTelemetry(nodeName string, updates map[string]StateInterface) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if s.state == nil {
		s.state = &NetworkState{
			Version:   1,
			UpdatedAt: time.Now(),
			Nodes:     make(map[string]StateNode),
		}
	}

	node, exists := s.state.Nodes[nodeName]
	if !exists {
		node = StateNode{
			Name:       nodeName,
			LastSeen:   time.Now(),
			Interfaces: make(map[string]StateInterface),
		}
	}
	if node.Interfaces == nil {
		node.Interfaces = make(map[string]StateInterface)
	}

	hasChanges := false
	now := time.Now()

	for name, upd := range updates {
		existing, ok := node.Interfaces[name]
		if !ok {
			node.Interfaces[name] = upd
			hasChanges = true
			continue
		}

		changed := existing.Status != upd.Status ||
			existing.WorkingState != upd.WorkingState ||
			existing.TransferRxBytes != upd.TransferRxBytes ||
			existing.TransferTxBytes != upd.TransferTxBytes

		if upd.LatestHandshake != nil {
			if existing.LatestHandshake == nil || !existing.LatestHandshake.Equal(*upd.LatestHandshake) {
				changed = true
			}
		} else if existing.LatestHandshake != nil {
			changed = true
		}

		if changed {
			hasChanges = true
			existing.Status = upd.Status
			existing.WorkingState = upd.WorkingState
			existing.LatestHandshake = upd.LatestHandshake
			existing.TransferRxBytes = upd.TransferRxBytes
			existing.TransferTxBytes = upd.TransferTxBytes
			if upd.PeerNode != "" {
				existing.PeerNode = upd.PeerNode
			}
			if upd.PeerPubKey != "" {
				existing.PeerPubKey = upd.PeerPubKey
			}
			node.Interfaces[name] = existing
		}
	}

	if !hasChanges {
		return nil
	}

	node.LastSeen = now
	s.state.Nodes[nodeName] = node
	s.state.UpdatedAt = now

	return s.saveNodeLocked(node)
}

// UpdateBirdState records or updates the applied BIRD config hash for a node
func (s *StateStore) UpdateBirdState(nodeName, host, configHash string, appliedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if s.state == nil {
		s.state = &NetworkState{
			Version:   1,
			UpdatedAt: time.Now(),
			Nodes:     make(map[string]StateNode),
		}
	}

	node, exists := s.state.Nodes[nodeName]
	if !exists {
		node = StateNode{
			Name:       nodeName,
			Host:       host,
			LastSeen:   time.Now(),
			Interfaces: make(map[string]StateInterface),
		}
	}
	if node.Interfaces == nil {
		node.Interfaces = make(map[string]StateInterface)
	}
	node.Host = host
	node.LastSeen = time.Now()
	node.BirdConfigHash = configHash
	node.BirdAppliedAt = &appliedAt
	s.state.Nodes[nodeName] = node
	s.state.UpdatedAt = time.Now()

	return s.saveNodeLocked(node)
}

// UpdateNftablesState records or updates the applied nftables config hash for a node
func (s *StateStore) UpdateNftablesState(nodeName, host, configHash string, appliedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if s.state == nil {
		s.state = &NetworkState{
			Version:   1,
			UpdatedAt: time.Now(),
			Nodes:     make(map[string]StateNode),
		}
	}

	node, exists := s.state.Nodes[nodeName]
	if !exists {
		node = StateNode{
			Name:       nodeName,
			Host:       host,
			LastSeen:   time.Now(),
			Interfaces: make(map[string]StateInterface),
		}
	}
	if node.Interfaces == nil {
		node.Interfaces = make(map[string]StateInterface)
	}
	node.Host = host
	node.LastSeen = time.Now()
	node.NftablesConfigHash = configHash
	node.NftablesAppliedAt = &appliedAt
	s.state.Nodes[nodeName] = node
	s.state.UpdatedAt = time.Now()

	return s.saveNodeLocked(node)
}

// UpdateHostsState records or updates the applied /etc/hosts easy42 config hash for a node
func (s *StateStore) UpdateHostsState(nodeName, host, configHash string, appliedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if s.state == nil {
		s.state = &NetworkState{
			Version:   1,
			UpdatedAt: time.Now(),
			Nodes:     make(map[string]StateNode),
		}
	}

	node, exists := s.state.Nodes[nodeName]
	if !exists {
		node = StateNode{
			Name:       nodeName,
			Host:       host,
			LastSeen:   time.Now(),
			Interfaces: make(map[string]StateInterface),
		}
	}
	if node.Interfaces == nil {
		node.Interfaces = make(map[string]StateInterface)
	}
	node.Host = host
	node.LastSeen = time.Now()
	node.HostsConfigHash = configHash
	node.HostsAppliedAt = &appliedAt
	s.state.Nodes[nodeName] = node
	s.state.UpdatedAt = time.Now()

	return s.saveNodeLocked(node)
}

// UpdateRoaState records or updates the applied ROA config hash for a specific target file on a node
func (s *StateStore) UpdateRoaState(nodeName, host, targetFile, configHash string, appliedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if s.state == nil {
		s.state = &NetworkState{
			Version:   1,
			UpdatedAt: time.Now(),
			Nodes:     make(map[string]StateNode),
		}
	}

	node, exists := s.state.Nodes[nodeName]
	if !exists {
		node = StateNode{
			Name:            nodeName,
			Host:            host,
			LastSeen:        time.Now(),
			Interfaces:      make(map[string]StateInterface),
			RoaConfigHashes: make(map[string]string),
			RoaAppliedAt:    make(map[string]time.Time),
		}
	}
	if node.Interfaces == nil {
		node.Interfaces = make(map[string]StateInterface)
	}
	if node.RoaConfigHashes == nil {
		node.RoaConfigHashes = make(map[string]string)
	}
	if node.RoaAppliedAt == nil {
		node.RoaAppliedAt = make(map[string]time.Time)
	}
	node.Host = host
	node.LastSeen = time.Now()
	node.RoaConfigHashes[targetFile] = configHash
	node.RoaAppliedAt[targetFile] = appliedAt
	s.state.Nodes[nodeName] = node
	s.state.UpdatedAt = time.Now()

	return s.saveNodeLocked(node)
}

// RemoveInterface removes an interface state from a node
func (s *StateStore) RemoveInterface(nodeName, ifaceName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if s.state == nil || s.state.Nodes == nil {
		return nil
	}

	node, exists := s.state.Nodes[nodeName]
	if !exists || node.Interfaces == nil {
		return nil
	}

	delete(node.Interfaces, ifaceName)
	node.LastSeen = time.Now()
	s.state.Nodes[nodeName] = node
	s.state.UpdatedAt = time.Now()

	return s.saveNodeLocked(node)
}

// RemoveNode removes an entire node from state
func (s *StateStore) RemoveNode(nodeName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if s.state != nil && s.state.Nodes != nil {
		delete(s.state.Nodes, nodeName)
		s.state.UpdatedAt = time.Now()
	}

	_, err := s.db.Exec("DELETE FROM node_states WHERE node_name = ?", nodeName)
	return err
}

// RenameNode renames a node and updates interface references in state
func (s *StateStore) RenameNode(oldName, newName string, oldIfaces []string, newIface string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if s.state == nil || s.state.Nodes == nil {
		return nil
	}

	// Rename node entry if it exists
	if node, exists := s.state.Nodes[oldName]; exists {
		node.Name = newName
		s.state.Nodes[newName] = node
		delete(s.state.Nodes, oldName)
		_, _ = s.db.Exec("DELETE FROM node_states WHERE node_name = ?", oldName)
	}

	// Update interface peer references and interface names across all nodes
	for nName, nVal := range s.state.Nodes {
		if nVal.Interfaces == nil {
			continue
		}
		newIfacesMap := make(map[string]StateInterface)
		for ifKey, ifVal := range nVal.Interfaces {
			if ifVal.PeerNode == oldName {
				ifVal.PeerNode = newName
			}
			matchedOld := false
			for _, oldIf := range oldIfaces {
				if oldIf != "" && (ifKey == oldIf || ifVal.Name == oldIf) {
					matchedOld = true
					break
				}
			}
			if matchedOld {
				ifVal.Name = newIface
				if strings.HasPrefix(ifVal.TargetFile, "/etc/wireguard/") {
					ifVal.TargetFile = "/etc/wireguard/" + newIface + ".conf"
				}
				newIfacesMap[newIface] = ifVal
			} else {
				newIfacesMap[ifKey] = ifVal
			}
		}
		nVal.Interfaces = newIfacesMap
		s.state.Nodes[nName] = nVal
		_ = s.saveNodeLocked(nVal)
	}

	s.state.UpdatedAt = time.Now()
	return nil
}

// Reset clears the state
func (s *StateStore) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	_, _ = s.db.Exec("DELETE FROM node_states; DELETE FROM node_metrics; DELETE FROM node_info; DELETE FROM meta; DELETE FROM node_interface_metrics; DELETE FROM node_interface_traffic_stats;")

	s.state = &NetworkState{
		Version:   1,
		UpdatedAt: time.Now(),
		Nodes:     make(map[string]StateNode),
	}
	return nil
}

// Close closes the underlying SQLite database
func (s *StateStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		err := s.db.Close()
		s.db = nil
		return err
	}
	return nil
}

// ----------------- Monitoring & Metrics Time-Series -----------------

// RecordMetrics records a telemetry snapshot and calculates network transfer rates
func (s *StateStore) RecordMetrics(nodeName string, input NodeMetricInput) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if input.Timestamp.IsZero() {
		input.Timestamp = time.Now()
	}

	// Calculate rx/tx rates
	s.prevNetMu.Lock()
	prev, hasPrev := s.prevNet[nodeName]
	var rxRate, txRate float64
	if hasPrev {
		dt := input.Timestamp.Sub(prev.timestamp).Seconds()
		if dt >= 0.5 {
			if input.NetRxBytes >= prev.rxBytes {
				rxRate = float64(input.NetRxBytes-prev.rxBytes) / dt
			}
			if input.NetTxBytes >= prev.txBytes {
				txRate = float64(input.NetTxBytes-prev.txBytes) / dt
			}
		}
	}
	s.prevNet[nodeName] = prevNetSample{
		timestamp: input.Timestamp,
		rxBytes:   input.NetRxBytes,
		txBytes:   input.NetTxBytes,
	}
	s.prevNetMu.Unlock()

	ts := input.Timestamp.Unix()
	_, err := s.db.Exec(`
		INSERT INTO node_metrics (
			node_name, timestamp, cpu_percent, memory_used_bytes, memory_total_bytes,
			uptime_seconds, load_1m, load_5m, load_15m, net_rx_bytes, net_tx_bytes,
			net_rx_rate, net_tx_rate
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, nodeName, ts, input.CPUPercent, input.MemoryUsedBytes, input.MemoryTotalBytes,
		input.UptimeSeconds, input.Load1m, input.Load5m, input.Load15m,
		input.NetRxBytes, input.NetTxBytes, rxRate, txRate)

	// Record per-interface snapshots if provided
	if len(input.Interfaces) > 0 {
		stmt, ifErr := s.db.Prepare(`
			INSERT INTO node_interface_metrics (
				node_name, interface_name, timestamp, rx_bytes, tx_bytes, rx_rate, tx_rate
			) VALUES (?, ?, ?, ?, ?, ?, ?)
		`)
		if ifErr == nil {
			defer stmt.Close()
			s.prevNetMu.Lock()
			for _, iface := range input.Interfaces {
				if iface.Name == "" {
					continue
				}
				key := nodeName + "/" + iface.Name
				prevIf, hasPrevIf := s.prevIfaceNet[key]
				var ifRxRate, ifTxRate float64
				if hasPrevIf {
					dt := input.Timestamp.Sub(prevIf.timestamp).Seconds()
					if dt >= 0.5 {
						if iface.RxBytes >= prevIf.rxBytes {
							ifRxRate = float64(iface.RxBytes-prevIf.rxBytes) / dt
						} else {
							ifRxRate = float64(iface.RxBytes) / dt
						}
						if iface.TxBytes >= prevIf.txBytes {
							ifTxRate = float64(iface.TxBytes-prevIf.txBytes) / dt
						} else {
							ifTxRate = float64(iface.TxBytes) / dt
						}
					}
				}
				s.prevIfaceNet[key] = prevNetSample{
					timestamp: input.Timestamp,
					rxBytes:   iface.RxBytes,
					txBytes:   iface.TxBytes,
				}
				_, _ = stmt.Exec(nodeName, iface.Name, ts, iface.RxBytes, iface.TxBytes, ifRxRate, ifTxRate)
			}
			s.prevNetMu.Unlock()
		}
		// Update derived state table periodically when underlying data changes
		_ = s.updateInterfaceTrafficStatsLocked(nodeName, "")
	}

	// Update node_info last_seen & uptime
	_, _ = s.db.Exec(`
		INSERT INTO node_info (node_name, uptime_seconds, last_seen)
		VALUES (?, ?, ?)
		ON CONFLICT(node_name) DO UPDATE SET
			uptime_seconds=excluded.uptime_seconds,
			last_seen=excluded.last_seen;
	`, nodeName, input.UptimeSeconds, ts)

	return err
}

// RecordNodeInfo records static or registration info about a node
func (s *StateStore) RecordNodeInfo(nodeName, agentVersion, osInfo, hostname string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	ts := time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT INTO node_info (node_name, agent_version, os_info, hostname, last_seen)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(node_name) DO UPDATE SET
			agent_version=excluded.agent_version,
			os_info=excluded.os_info,
			hostname=excluded.hostname,
			last_seen=excluded.last_seen;
	`, nodeName, agentVersion, osInfo, hostname, ts)
	return err
}

// DeleteNodeInfo removes recorded agent info for a node
func (s *StateStore) DeleteNodeInfo(nodeName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return nil
	}

	_, err := s.db.Exec(`DELETE FROM node_info WHERE node_name = ?`, nodeName)
	return err
}

// GetNodeInfo returns recorded agent info for a node
func (s *StateStore) GetNodeInfo(nodeName string) (*NodeInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return nil, nil
	}

	var (
		info     NodeInfo
		lastSeen int64
	)
	info.NodeName = nodeName
	err := s.db.QueryRow(`
		SELECT agent_version, os_info, hostname, uptime_seconds, last_seen
		FROM node_info WHERE node_name = ?
	`, nodeName).Scan(&info.AgentVersion, &info.OSInfo, &info.Hostname, &info.UptimeSeconds, &lastSeen)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastSeen > 0 {
		info.LastSeen = time.Unix(lastSeen, 0)
	}
	return &info, nil
}

// GetAllNodeInfo returns info for all nodes
func (s *StateStore) GetAllNodeInfo() (map[string]NodeInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make(map[string]NodeInfo)
	if s.db == nil {
		return res, nil
	}

	rows, err := s.db.Query(`
		SELECT node_name, agent_version, os_info, hostname, uptime_seconds, last_seen
		FROM node_info
	`)
	if err != nil {
		return res, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			name, ver, osI, host string
			uptime, lastSeen     int64
		)
		if err := rows.Scan(&name, &ver, &osI, &host, &uptime, &lastSeen); err == nil {
			info := NodeInfo{
				NodeName:      name,
				AgentVersion:  ver,
				OSInfo:        osI,
				Hostname:      host,
				UptimeSeconds: uint64(uptime),
			}
			if lastSeen > 0 {
				info.LastSeen = time.Unix(lastSeen, 0)
			}
			res[name] = info
		}
	}
	return res, nil
}

// GetLatestMetrics returns the most recent metric point for each node
func (s *StateStore) GetLatestMetrics() (map[string]NodeMetricPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make(map[string]NodeMetricPoint)
	if s.db == nil {
		return res, nil
	}

	query := `
	SELECT m.node_name, m.timestamp, m.cpu_percent, m.memory_used_bytes, m.memory_total_bytes,
	       m.uptime_seconds, m.load_1m, m.load_5m, m.load_15m, m.net_rx_bytes, m.net_tx_bytes,
	       m.net_rx_rate, m.net_tx_rate
	FROM node_metrics m
	INNER JOIN (
		SELECT node_name, MAX(timestamp) as max_ts
		FROM node_metrics
		GROUP BY node_name
	) latest ON m.node_name = latest.node_name AND m.timestamp = latest.max_ts
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return res, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			name string
			pt   NodeMetricPoint
		)
		if err := rows.Scan(
			&name, &pt.Timestamp, &pt.CPUPercent, &pt.MemoryUsedBytes, &pt.MemoryTotalBytes,
			&pt.UptimeSeconds, &pt.Load1m, &pt.Load5m, &pt.Load15m,
			&pt.NetRxBytes, &pt.NetTxBytes, &pt.NetRxRate, &pt.NetTxRate,
		); err == nil {
			res[name] = pt
		}
	}

	return res, nil
}

// updateInterfaceTrafficStatsLocked updates the derived traffic stats table for a node and interface
func (s *StateStore) updateInterfaceTrafficStatsLocked(nodeName, interfaceName string) error {
	if s.db == nil {
		return nil
	}

	if interfaceName == "" {
		var query string
		var args []any
		if nodeName != "" {
			query = "SELECT DISTINCT interface_name FROM node_interface_metrics WHERE node_name = ?"
			args = append(args, nodeName)
		} else {
			query = "SELECT DISTINCT node_name, interface_name FROM node_interface_metrics"
		}
		rows, err := s.db.Query(query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		type nodeIfacePair struct {
			node  string
			iface string
		}
		var pairs []nodeIfacePair
		for rows.Next() {
			if nodeName != "" {
				var iface string
				if err := rows.Scan(&iface); err == nil && iface != "" {
					pairs = append(pairs, nodeIfacePair{node: nodeName, iface: iface})
				}
			} else {
				var n, iface string
				if err := rows.Scan(&n, &iface); err == nil && iface != "" {
					pairs = append(pairs, nodeIfacePair{node: n, iface: iface})
				}
			}
		}
		rows.Close()

		for _, p := range pairs {
			_ = s.updateSingleInterfaceTrafficStatsLocked(p.node, p.iface)
		}
		return nil
	}

	return s.updateSingleInterfaceTrafficStatsLocked(nodeName, interfaceName)
}

// updateSingleInterfaceTrafficStatsLocked processes new snapshots for a single node interface
// into the derived node_interface_traffic_stats table.
func (s *StateStore) updateSingleInterfaceTrafficStatsLocked(nodeName, interfaceName string) error {
	if s.db == nil || nodeName == "" || interfaceName == "" {
		return nil
	}

	// 1. Get the latest derived state for this interface
	var (
		lastTs     int64
		lastRawRx  uint64
		lastRawTx  uint64
		accumRx    uint64
		accumTx    uint64
		hasDerived bool
	)
	row := s.db.QueryRow(`
		SELECT timestamp, raw_rx_bytes, raw_tx_bytes, accum_rx_bytes, accum_tx_bytes
		FROM node_interface_traffic_stats
		WHERE node_name = ? AND interface_name = ?
		ORDER BY timestamp DESC LIMIT 1
	`, nodeName, interfaceName)
	if err := row.Scan(&lastTs, &lastRawRx, &lastRawTx, &accumRx, &accumTx); err == nil {
		hasDerived = true
	}

	// 2. Query newer raw snapshots from node_interface_metrics
	var query string
	var args []any
	if hasDerived {
		query = `
			SELECT timestamp, rx_bytes, tx_bytes, rx_rate, tx_rate
			FROM node_interface_metrics
			WHERE node_name = ? AND interface_name = ? AND timestamp > ?
			ORDER BY timestamp ASC
		`
		args = []any{nodeName, interfaceName, lastTs}
	} else {
		query = `
			SELECT timestamp, rx_bytes, tx_bytes, rx_rate, tx_rate
			FROM node_interface_metrics
			WHERE node_name = ? AND interface_name = ?
			ORDER BY timestamp ASC
		`
		args = []any{nodeName, interfaceName}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rawSnapshot struct {
		ts     int64
		rx     uint64
		tx     uint64
		rxRate float64
		txRate float64
	}
	var newSnaps []rawSnapshot
	for rows.Next() {
		var sn rawSnapshot
		if err := rows.Scan(&sn.ts, &sn.rx, &sn.tx, &sn.rxRate, &sn.txRate); err == nil {
			newSnaps = append(newSnaps, sn)
		}
	}
	rows.Close()

	if len(newSnaps) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT INTO node_interface_traffic_stats (
			node_name, interface_name, timestamp, raw_rx_bytes, raw_tx_bytes,
			accum_rx_bytes, accum_tx_bytes, rx_rate, tx_rate, reset_detected
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_name, interface_name, timestamp) DO UPDATE SET
			raw_rx_bytes=excluded.raw_rx_bytes,
			raw_tx_bytes=excluded.raw_tx_bytes,
			accum_rx_bytes=excluded.accum_rx_bytes,
			accum_tx_bytes=excluded.accum_tx_bytes,
			rx_rate=excluded.rx_rate,
			tx_rate=excluded.tx_rate,
			reset_detected=excluded.reset_detected
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, snap := range newSnaps {
		resetDetected := 0
		if !hasDerived {
			// First snapshot ever recorded for this interface: initial baseline
			accumRx = snap.rx
			accumTx = snap.tx
			lastRawRx = snap.rx
			lastRawTx = snap.tx
			hasDerived = true
		} else {
			// RX: check for interface restart / counter reset
			if snap.rx < lastRawRx {
				// Reset event detected: counter restarted or reset to 0.
				// Add the new value to previous snapshot accumulated value.
				accumRx += snap.rx
				resetDetected = 1
			} else {
				accumRx += (snap.rx - lastRawRx)
			}
			lastRawRx = snap.rx

			// TX: check for interface restart / counter reset
			if snap.tx < lastRawTx {
				// Reset event detected: counter restarted or reset to 0.
				// Add the new value to previous snapshot accumulated value.
				accumTx += snap.tx
				resetDetected = 1
			} else {
				accumTx += (snap.tx - lastRawTx)
			}
			lastRawTx = snap.tx
		}

		if _, err := stmt.Exec(
			nodeName, interfaceName, snap.ts, snap.rx, snap.tx,
			accumRx, accumTx, snap.rxRate, snap.txRate, resetDetected,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

// UpdateInterfaceTrafficStats generates/updates the derived interface traffic stats table
func (s *StateStore) UpdateInterfaceTrafficStats(nodeName, interfaceName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}
	return s.updateInterfaceTrafficStatsLocked(nodeName, interfaceName)
}

// GetNodeInterfaces returns a list of distinct interfaces recorded for the node
func (s *StateStore) GetNodeInterfaces(nodeName string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return []string{}, nil
	}

	rows, err := s.db.Query(`
		SELECT DISTINCT interface_name
		FROM node_interface_metrics
		WHERE node_name = ?
		ORDER BY interface_name ASC
	`, nodeName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []string
	for rows.Next() {
		var iface string
		if err := rows.Scan(&iface); err == nil && iface != "" {
			list = append(list, iface)
		}
	}
	return list, nil
}

// GetMetricsHistoryWithInterface returns historical metric snapshots with interface-specific traffic and rates
func (s *StateStore) GetMetricsHistoryWithInterface(nodeName string, interfaceName string, start time.Time, maxPoints int) ([]NodeMetricPoint, []string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return []NodeMetricPoint{}, nil, "", err
	}

	if maxPoints <= 0 {
		maxPoints = 250
	}

	// Fetch available interfaces for this node from raw metrics
	var availableIfaces []string
	if ifRows, err := s.db.Query("SELECT DISTINCT interface_name FROM node_interface_metrics WHERE node_name = ? ORDER BY interface_name ASC", nodeName); err == nil {
		defer ifRows.Close()
		for ifRows.Next() {
			var ifn string
			if ifRows.Scan(&ifn) == nil && ifn != "" {
				availableIfaces = append(availableIfaces, ifn)
			}
		}
		ifRows.Close()
	}

	// If no interface was specified, pick the first available interface if any exist
	selectedIface := interfaceName
	if selectedIface == "" && len(availableIfaces) > 0 {
		selectedIface = availableIfaces[0]
	}

	// On-demand update derived traffic stats for the selected interface
	if selectedIface != "" {
		_ = s.updateInterfaceTrafficStatsLocked(nodeName, selectedIface)
	}

	var (
		points []NodeMetricPoint
		err    error
	)

	if selectedIface != "" {
		rows, qErr := s.db.Query(`
			SELECT m.timestamp, m.cpu_percent, m.memory_used_bytes, m.memory_total_bytes,
			       m.uptime_seconds, m.load_1m, m.load_5m, m.load_15m,
			       COALESCE(t.raw_rx_bytes, m.net_rx_bytes),
			       COALESCE(t.raw_tx_bytes, m.net_tx_bytes),
			       COALESCE(t.rx_rate, m.net_rx_rate),
			       COALESCE(t.tx_rate, m.net_tx_rate),
			       COALESCE(t.accum_rx_bytes, 0),
			       COALESCE(t.accum_tx_bytes, 0)
			FROM node_metrics m
			LEFT JOIN node_interface_traffic_stats t
			  ON m.node_name = t.node_name AND m.timestamp = t.timestamp AND t.interface_name = ?
			WHERE m.node_name = ? AND m.timestamp >= ?
			ORDER BY m.timestamp ASC
		`, selectedIface, nodeName, start.Unix())
		if qErr != nil {
			return nil, availableIfaces, selectedIface, qErr
		}
		defer rows.Close()

		type pointWithAccum struct {
			pt      NodeMetricPoint
			accumRx uint64
			accumTx uint64
		}
		var rawPts []pointWithAccum
		for rows.Next() {
			var p pointWithAccum
			if err := rows.Scan(
				&p.pt.Timestamp, &p.pt.CPUPercent, &p.pt.MemoryUsedBytes, &p.pt.MemoryTotalBytes,
				&p.pt.UptimeSeconds, &p.pt.Load1m, &p.pt.Load5m, &p.pt.Load15m,
				&p.pt.NetRxBytes, &p.pt.NetTxBytes, &p.pt.NetRxRate, &p.pt.NetTxRate,
				&p.accumRx, &p.accumTx,
			); err == nil {
				rawPts = append(rawPts, p)
			}
		}
		rows.Close()

		if len(rawPts) > 0 {
			// Find baseline offset at the start point of the queried timespan
			baseRx := rawPts[0].accumRx
			baseTx := rawPts[0].accumTx
			for i := range rawPts {
				var tRx, tTx uint64
				if rawPts[i].accumRx >= baseRx {
					tRx = rawPts[i].accumRx - baseRx
				}
				if rawPts[i].accumTx >= baseTx {
					tTx = rawPts[i].accumTx - baseTx
				}
				rawPts[i].pt.TrafficRxBytes = tRx
				rawPts[i].pt.TrafficTxBytes = tTx
				points = append(points, rawPts[i].pt)
			}
		}
	} else {
		rows, qErr := s.db.Query(`
			SELECT timestamp, cpu_percent, memory_used_bytes, memory_total_bytes,
			       uptime_seconds, load_1m, load_5m, load_15m, net_rx_bytes, net_tx_bytes,
			       net_rx_rate, net_tx_rate
			FROM node_metrics
			WHERE node_name = ? AND timestamp >= ?
			ORDER BY timestamp ASC
		`, nodeName, start.Unix())
		if qErr != nil {
			return nil, availableIfaces, selectedIface, qErr
		}
		defer rows.Close()

		for rows.Next() {
			var pt NodeMetricPoint
			if err := rows.Scan(
				&pt.Timestamp, &pt.CPUPercent, &pt.MemoryUsedBytes, &pt.MemoryTotalBytes,
				&pt.UptimeSeconds, &pt.Load1m, &pt.Load5m, &pt.Load15m,
				&pt.NetRxBytes, &pt.NetTxBytes, &pt.NetRxRate, &pt.NetTxRate,
			); err == nil {
				points = append(points, pt)
			}
		}
		rows.Close()
	}

	if len(points) <= maxPoints {
		return points, availableIfaces, selectedIface, nil
	}

	// Downsample to fit maxPoints buckets
	downsampled := make([]NodeMetricPoint, 0, maxPoints)
	bucketSize := float64(len(points)) / float64(maxPoints)

	for i := 0; i < maxPoints; i++ {
		startIdx := int(float64(i) * bucketSize)
		endIdx := int(float64(i+1) * bucketSize)
		if endIdx > len(points) {
			endIdx = len(points)
		}
		if startIdx >= endIdx {
			continue
		}

		var (
			sumCPU, sumLoad1, sumRxRate, sumTxRate float64
			sumMemUsed, sumMemTotal                uint64
		)
		count := float64(endIdx - startIdx)
		for j := startIdx; j < endIdx; j++ {
			sumCPU += float64(points[j].CPUPercent)
			sumMemUsed += points[j].MemoryUsedBytes
			sumMemTotal += points[j].MemoryTotalBytes
			sumLoad1 += float64(points[j].Load1m)
			sumRxRate += points[j].NetRxRate
			sumTxRate += points[j].NetTxRate
		}

		lastPt := points[endIdx-1]
		downsampled = append(downsampled, NodeMetricPoint{
			Timestamp:        lastPt.Timestamp,
			CPUPercent:       float32(sumCPU / count),
			MemoryUsedBytes:  uint64(float64(sumMemUsed) / count),
			MemoryTotalBytes: uint64(float64(sumMemTotal) / count),
			UptimeSeconds:    lastPt.UptimeSeconds,
			Load1m:           float32(sumLoad1 / count),
			Load5m:           lastPt.Load5m,
			Load15m:          lastPt.Load15m,
			NetRxBytes:       lastPt.NetRxBytes,
			NetTxBytes:       lastPt.NetTxBytes,
			NetRxRate:        sumRxRate / count,
			NetTxRate:        sumTxRate / count,
			TrafficRxBytes:   lastPt.TrafficRxBytes,
			TrafficTxBytes:   lastPt.TrafficTxBytes,
		})
	}

	return downsampled, availableIfaces, selectedIface, err
}

// GetMetricsHistory returns historical metric snapshots, downsampled if exceeding maxPoints
func (s *StateStore) GetMetricsHistory(nodeName string, start time.Time, maxPoints int) ([]NodeMetricPoint, error) {
	pts, _, _, err := s.GetMetricsHistoryWithInterface(nodeName, "", start, maxPoints)
	return pts, err
}

// GetNodeInterfaceTraffic returns the derived network traffic usage for a node interface in any specified timespan
func (s *StateStore) GetNodeInterfaceTraffic(nodeName, interfaceName string, start, end time.Time, maxPoints int) (*NodeInterfaceTrafficSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return nil, err
	}

	if maxPoints <= 0 {
		maxPoints = 250
	}
	if end.IsZero() {
		end = time.Now()
	}

	// On-demand update
	_ = s.updateInterfaceTrafficStatsLocked(nodeName, interfaceName)

	summary := &NodeInterfaceTrafficSummary{
		NodeName:      nodeName,
		InterfaceName: interfaceName,
		StartTime:     start.Unix(),
		EndTime:       end.Unix(),
		Points:        make([]InterfaceTrafficPoint, 0),
	}

	rows, err := s.db.Query(`
		SELECT timestamp, raw_rx_bytes, raw_tx_bytes, accum_rx_bytes, accum_tx_bytes,
		       rx_rate, tx_rate, reset_detected
		FROM node_interface_traffic_stats
		WHERE node_name = ? AND interface_name = ? AND timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp ASC
	`, nodeName, interfaceName, start.Unix(), end.Unix())
	if err != nil {
		return summary, err
	}
	defer rows.Close()

	var rawPoints []InterfaceTrafficPoint
	for rows.Next() {
		var pt InterfaceTrafficPoint
		var resetInt int
		if err := rows.Scan(
			&pt.Timestamp, &pt.RawRxBytes, &pt.RawTxBytes, &pt.AccumRxBytes, &pt.AccumTxBytes,
			&pt.RxRate, &pt.TxRate, &resetInt,
		); err == nil {
			pt.ResetDetected = (resetInt != 0)
			rawPoints = append(rawPoints, pt)
		}
	}
	rows.Close()

	if len(rawPoints) == 0 {
		return summary, nil
	}

	// Calculate traffic relative to timespan start point (starting from 0)
	baseRx := rawPoints[0].AccumRxBytes
	baseTx := rawPoints[0].AccumTxBytes
	for i := range rawPoints {
		if rawPoints[i].AccumRxBytes >= baseRx {
			rawPoints[i].TrafficRxBytes = rawPoints[i].AccumRxBytes - baseRx
		}
		if rawPoints[i].AccumTxBytes >= baseTx {
			rawPoints[i].TrafficTxBytes = rawPoints[i].AccumTxBytes - baseTx
		}
	}

	last := rawPoints[len(rawPoints)-1]
	summary.TotalRxBytes = last.TrafficRxBytes
	summary.TotalTxBytes = last.TrafficTxBytes

	if len(rawPoints) <= maxPoints {
		summary.Points = rawPoints
		return summary, nil
	}

	// Downsample to fit maxPoints buckets
	downsampled := make([]InterfaceTrafficPoint, 0, maxPoints)
	bucketSize := float64(len(rawPoints)) / float64(maxPoints)

	for i := 0; i < maxPoints; i++ {
		startIdx := int(float64(i) * bucketSize)
		endIdx := int(float64(i+1) * bucketSize)
		if endIdx > len(rawPoints) {
			endIdx = len(rawPoints)
		}
		if startIdx >= endIdx {
			continue
		}

		var sumRxRate, sumTxRate float64
		anyReset := false
		count := float64(endIdx - startIdx)
		for j := startIdx; j < endIdx; j++ {
			sumRxRate += rawPoints[j].RxRate
			sumTxRate += rawPoints[j].TxRate
			if rawPoints[j].ResetDetected {
				anyReset = true
			}
		}

		lastPt := rawPoints[endIdx-1]
		downsampled = append(downsampled, InterfaceTrafficPoint{
			Timestamp:      lastPt.Timestamp,
			RawRxBytes:     lastPt.RawRxBytes,
			RawTxBytes:     lastPt.RawTxBytes,
			AccumRxBytes:   lastPt.AccumRxBytes,
			AccumTxBytes:   lastPt.AccumTxBytes,
			TrafficRxBytes: lastPt.TrafficRxBytes,
			TrafficTxBytes: lastPt.TrafficTxBytes,
			RxRate:         sumRxRate / count,
			TxRate:         sumTxRate / count,
			ResetDetected:  anyReset,
		})
	}
	summary.Points = downsampled
	return summary, nil
}

// CleanMetricsOlderThan deletes historical metrics older than the given retention period
func (s *StateStore) CleanMetricsOlderThan(retention time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-retention).Unix()
	res, err := s.db.Exec("DELETE FROM node_metrics WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	_, _ = s.db.Exec("DELETE FROM node_interface_metrics WHERE timestamp < ?", cutoff)
	_, _ = s.db.Exec("DELETE FROM node_interface_traffic_stats WHERE timestamp < ?", cutoff)
	return res.RowsAffected()
}

// RecordCronTaskResult persists a parsed cron task execution report into SQLite
func (s *StateStore) RecordCronTaskResult(record CronTaskResultRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if record.ExecutedAt == 0 {
		record.ExecutedAt = time.Now().Unix()
	}

	_, err := s.db.Exec(`
		INSERT INTO cron_task_results (
			task_id, task_name, task_type, node_name, executed_at, duration_ms, exit_code,
			error, target, packets_sent, packets_received, packet_loss_pct,
			min_rtt_ms, avg_rtt_ms, max_rtt_ms, mdev_rtt_ms, raw_output, parsed_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.TaskID, record.TaskName, record.TaskType, record.NodeName, record.ExecutedAt,
		record.DurationMs, record.ExitCode, record.Error, record.Target,
		record.PacketsSent, record.PacketsReceived, record.PacketLossPct,
		record.MinRTTMs, record.AvgRTTMs, record.MaxRTTMs, record.MdevRTTMs,
		record.RawOutput, record.ParsedJSON)

	return err
}

// GetNodeCronTaskResults retrieves chronological cron task results for a node
func (s *StateStore) GetNodeCronTaskResults(nodeName string, taskID string, startTime time.Time, limit int) ([]CronTaskResultRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return []CronTaskResultRecord{}, nil
	}

	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var (
		rows *sql.Rows
		err  error
	)

	startTs := startTime.Unix()
	if taskID != "" {
		rows, err = s.db.Query(`
			SELECT id, task_id, task_name, task_type, node_name, executed_at, duration_ms,
			       exit_code, error, target, packets_sent, packets_received, packet_loss_pct,
			       min_rtt_ms, avg_rtt_ms, max_rtt_ms, mdev_rtt_ms, raw_output, parsed_json
			FROM cron_task_results
			WHERE node_name = ? AND task_id = ? AND executed_at >= ?
			ORDER BY executed_at DESC
			LIMIT ?
		`, nodeName, taskID, startTs, limit)
	} else {
		rows, err = s.db.Query(`
			SELECT id, task_id, task_name, task_type, node_name, executed_at, duration_ms,
			       exit_code, error, target, packets_sent, packets_received, packet_loss_pct,
			       min_rtt_ms, avg_rtt_ms, max_rtt_ms, mdev_rtt_ms, raw_output, parsed_json
			FROM cron_task_results
			WHERE node_name = ? AND executed_at >= ?
			ORDER BY executed_at DESC
			LIMIT ?
		`, nodeName, startTs, limit)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CronTaskResultRecord
	for rows.Next() {
		var r CronTaskResultRecord
		if err := rows.Scan(
			&r.ID, &r.TaskID, &r.TaskName, &r.TaskType, &r.NodeName, &r.ExecutedAt, &r.DurationMs,
			&r.ExitCode, &r.Error, &r.Target, &r.PacketsSent, &r.PacketsReceived, &r.PacketLossPct,
			&r.MinRTTMs, &r.AvgRTTMs, &r.MaxRTTMs, &r.MdevRTTMs, &r.RawOutput, &r.ParsedJSON,
		); err == nil {
			records = append(records, r)
		}
	}

	return records, nil
}

// CleanCronResultsOlderThan deletes historical cron task results older than retention period
func (s *StateStore) CleanCronResultsOlderThan(retention time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-retention).Unix()
	res, err := s.db.Exec("DELETE FROM cron_task_results WHERE executed_at < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ServerTaskState represents the recorded state of a server background task
type ServerTaskState struct {
	TaskID         string    `json:"task_id"`
	LastRun        time.Time `json:"last_run"`
	LastStatus     string    `json:"last_status"` // "never_run", "idle", "running", "success", "error"
	LastDurationMs int64     `json:"last_duration_ms"`
	LastMessage    string    `json:"last_message"`
}

// RecordServerTaskState records or updates execution state of a server background task
func (s *StateStore) RecordServerTaskState(state ServerTaskState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	var lastRunUnix int64
	if !state.LastRun.IsZero() {
		lastRunUnix = state.LastRun.Unix()
	}

	_, err := s.db.Exec(`
		INSERT INTO server_task_states (task_id, last_run, last_status, last_duration_ms, last_message)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(task_id) DO UPDATE SET
			last_run = excluded.last_run,
			last_status = excluded.last_status,
			last_duration_ms = excluded.last_duration_ms,
			last_message = excluded.last_message
	`, state.TaskID, lastRunUnix, state.LastStatus, state.LastDurationMs, state.LastMessage)

	return err
}

// GetServerTaskStates retrieves recorded execution states of all server background tasks
func (s *StateStore) GetServerTaskStates() (map[string]ServerTaskState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return map[string]ServerTaskState{}, nil
	}

	rows, err := s.db.Query(`
		SELECT task_id, last_run, last_status, last_duration_ms, last_message
		FROM server_task_states
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := make(map[string]ServerTaskState)
	for rows.Next() {
		var (
			taskID         string
			lastRunUnix    int64
			lastStatus     string
			lastDurationMs int64
			lastMessage    string
		)
		if err := rows.Scan(&taskID, &lastRunUnix, &lastStatus, &lastDurationMs, &lastMessage); err == nil {
			var lastRun time.Time
			if lastRunUnix > 0 {
				lastRun = time.Unix(lastRunUnix, 0)
			}
			states[taskID] = ServerTaskState{
				TaskID:         taskID,
				LastRun:        lastRun,
				LastStatus:     lastStatus,
				LastDurationMs: lastDurationMs,
				LastMessage:    lastMessage,
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return states, nil
}

// DatabaseStats represents size and storage metrics of the SQLite database
type DatabaseStats struct {
	FilePath              string `json:"file_path"`
	TotalSizeBytes        int64  `json:"total_size_bytes"`
	TotalSizeHuman        string `json:"total_size_human"`
	PageSize              int64  `json:"page_size,omitempty"`
	PageCount             int64  `json:"page_count,omitempty"`
	FreelistCount         int64  `json:"freelist_count,omitempty"`
	WALSizeBytes          int64  `json:"wal_size_bytes,omitempty"`
	MetricsCount          int64  `json:"metrics_count"`
	InterfaceMetricsCount int64  `json:"interface_metrics_count"`
	CronResultsCount      int64  `json:"cron_results_count"`
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// GetDatabaseStats returns size, page statistics, and row counts of the SQLite database
func (s *StateStore) GetDatabaseStats() (DatabaseStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return DatabaseStats{}, err
	}

	stats := DatabaseStats{
		FilePath: s.filePath,
	}

	// Calculate total file size from main db file, WAL, and SHM
	if fi, err := os.Stat(s.filePath); err == nil {
		stats.TotalSizeBytes += fi.Size()
	}
	if fi, err := os.Stat(s.filePath + "-wal"); err == nil {
		stats.WALSizeBytes = fi.Size()
		stats.TotalSizeBytes += fi.Size()
	}
	if fi, err := os.Stat(s.filePath + "-shm"); err == nil {
		stats.TotalSizeBytes += fi.Size()
	}
	stats.TotalSizeHuman = formatBytes(stats.TotalSizeBytes)

	// SQLite pragmas
	_ = s.db.QueryRow("PRAGMA page_size").Scan(&stats.PageSize)
	_ = s.db.QueryRow("PRAGMA page_count").Scan(&stats.PageCount)
	_ = s.db.QueryRow("PRAGMA freelist_count").Scan(&stats.FreelistCount)

	// Table row counts
	_ = s.db.QueryRow("SELECT COUNT(*) FROM node_metrics").Scan(&stats.MetricsCount)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM node_interface_metrics").Scan(&stats.InterfaceMetricsCount)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM cron_task_results").Scan(&stats.CronResultsCount)

	return stats, nil
}

// Vacuum executes SQLite VACUUM command and truncates WAL to reclaim free space
func (s *StateStore) Vacuum() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.initDBLocked(); err != nil {
		return err
	}

	if _, err := s.db.Exec("VACUUM"); err != nil {
		return err
	}
	_, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return nil
}


