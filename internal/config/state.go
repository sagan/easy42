package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
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
	Name       string                    `json:"name"`
	Host       string                    `json:"host"`
	LastSeen   time.Time                 `json:"last_seen,omitempty"`
	Interfaces map[string]StateInterface `json:"interfaces"` // key: interface name e.g. "wg42node2"
}

// NetworkState represents the recorded state in state.json
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

// StateStore handles thread-safe loading and persisting of state.json
type StateStore struct {
	mu       sync.RWMutex
	dataDir  string
	filePath string
	state    *NetworkState
}

// NewStateStore creates a new StateStore pointing to the given data directory
func NewStateStore(dataDir string) *StateStore {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	return &StateStore{
		dataDir:  dataDir,
		filePath: filepath.Join(dataDir, "state.json"),
	}
}

// FilePath returns the path to state.json
func (s *StateStore) FilePath() string {
	return s.filePath
}

// Exists checks if state.json exists
func (s *StateStore) Exists() bool {
	_, err := os.Stat(s.filePath)
	return err == nil
}

// Load reads state.json from disk or initializes an empty state if not found
func (s *StateStore) Load() (*NetworkState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			s.state = &NetworkState{
				Version:   1,
				UpdatedAt: time.Now(),
				Nodes:     make(map[string]StateNode),
			}
			return s.state, nil
		}
		return nil, fmt.Errorf("failed to read state.json: %w", err)
	}

	var st NetworkState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("failed to parse state.json: %w", err)
	}
	if st.Nodes == nil {
		st.Nodes = make(map[string]StateNode)
	}
	for k, n := range st.Nodes {
		if n.Interfaces == nil {
			n.Interfaces = make(map[string]StateInterface)
			st.Nodes[k] = n
		}
	}
	s.state = &st
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

// Save persists the state to disk atomically
func (s *StateStore) Save(st *NetworkState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	st.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	tmpFile := filepath.Join(s.dataDir, ".state.json.tmp")
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write tmp state: %w", err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return fmt.Errorf("failed to atomic rename state: %w", err)
	}

	s.state = st
	return nil
}

// UpdateInterface records or updates an interface state for a node
func (s *StateStore) UpdateInterface(nodeName, host string, iface StateInterface) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	return s.saveLocked()
}

// RemoveInterface removes an interface state from a node
func (s *StateStore) RemoveInterface(nodeName, ifaceName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	return s.saveLocked()
}

// RemoveNode removes an entire node from state
func (s *StateStore) RemoveNode(nodeName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == nil || s.state.Nodes == nil {
		return nil
	}

	delete(s.state.Nodes, nodeName)
	return s.saveLocked()
}

// Reset clears the state
func (s *StateStore) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state = &NetworkState{
		Version:   1,
		UpdatedAt: time.Now(),
		Nodes:     make(map[string]StateNode),
	}
	return s.saveLocked()
}

// saveLocked persists state while mutex is held
func (s *StateStore) saveLocked() error {
	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	s.state.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}
	tmpFile := filepath.Join(s.dataDir, ".state.json.tmp")
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write tmp state: %w", err)
	}
	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return fmt.Errorf("failed to atomic rename state: %w", err)
	}
	return nil
}
