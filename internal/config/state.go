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
	NetRxRate        float64 `json:"net_rx_rate"` // bytes/second
	NetTxRate        float64 `json:"net_tx_rate"` // bytes/second
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
	AgentInstalled bool             `json:"agent_installed"`
	LastSeen       time.Time        `json:"last_seen"`
	Metrics        *NodeMetricPoint `json:"metrics,omitempty"`
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
	prevNetMu sync.Mutex
	prevNet   map[string]prevNetSample
}

// NewStateStore creates a new StateStore pointing to the given data directory
func NewStateStore(dataDir string) *StateStore {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	return &StateStore{
		dataDir:  dataDir,
		filePath: filepath.Join(dataDir, "data.db"),
		prevNet:  make(map[string]prevNetSample),
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
	`

	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to initialize sqlite schema: %w", err)
	}

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
		       roa_applied_at, interfaces
		FROM node_states
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query node states: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			nodeName, host, birdHash, nftHash string
			lastSeenUnix                      sql.NullInt64
			birdAppliedUnix                   sql.NullInt64
			nftAppliedUnix                    sql.NullInt64
			roaHashesJSON, roaAppliedJSON     string
			ifacesJSON                        string
		)

		if err := rows.Scan(
			&nodeName, &host, &lastSeenUnix, &birdHash, &birdAppliedUnix,
			&nftHash, &nftAppliedUnix, &roaHashesJSON, &roaAppliedJSON, &ifacesJSON,
		); err != nil {
			continue
		}

		node := StateNode{
			Name:               nodeName,
			Host:               host,
			BirdConfigHash:     birdHash,
			NftablesConfigHash: nftHash,
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

	query := `
	INSERT INTO node_states (
		node_name, host, last_seen, bird_config_hash, bird_applied_at,
		nftables_config_hash, nftables_applied_at, roa_config_hashes,
		roa_applied_at, interfaces
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(node_name) DO UPDATE SET
		host=excluded.host,
		last_seen=excluded.last_seen,
		bird_config_hash=excluded.bird_config_hash,
		bird_applied_at=excluded.bird_applied_at,
		nftables_config_hash=excluded.nftables_config_hash,
		nftables_applied_at=excluded.nftables_applied_at,
		roa_config_hashes=excluded.roa_config_hashes,
		roa_applied_at=excluded.roa_applied_at,
		interfaces=excluded.interfaces;
	`

	_, err := s.db.Exec(query,
		node.Name, node.Host, lastSeenUnix, node.BirdConfigHash, birdUnix,
		node.NftablesConfigHash, nftUnix, string(roaHashesJSON), string(roaAppliedJSON), string(ifacesJSON),
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

	_, _ = s.db.Exec("DELETE FROM node_states; DELETE FROM node_metrics; DELETE FROM node_info; DELETE FROM meta;")

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

// GetMetricsHistory returns historical metric snapshots, downsampled if exceeding maxPoints
func (s *StateStore) GetMetricsHistory(nodeName string, start time.Time, maxPoints int) ([]NodeMetricPoint, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return []NodeMetricPoint{}, nil
	}

	if maxPoints <= 0 {
		maxPoints = 250
	}

	rows, err := s.db.Query(`
		SELECT timestamp, cpu_percent, memory_used_bytes, memory_total_bytes,
		       uptime_seconds, load_1m, load_5m, load_15m, net_rx_bytes, net_tx_bytes,
		       net_rx_rate, net_tx_rate
		FROM node_metrics
		WHERE node_name = ? AND timestamp >= ?
		ORDER BY timestamp ASC
	`, nodeName, start.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []NodeMetricPoint
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

	if len(points) <= maxPoints {
		return points, nil
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
		})
	}

	return downsampled, nil
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
	return res.RowsAffected()
}
