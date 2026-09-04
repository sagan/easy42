package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"easy42/internal/crypto"
)

var (
	ErrConfigNotFound = errors.New("config file not found")
)

// Store handles thread-safe loading and persisting of easy42 config
type Store struct {
	mu       sync.RWMutex
	dataDir  string
	filePath string
	config   *Config
}

// DefaultDataDir returns the default ~/.config/easy42 directory
func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".easy42"
	}
	return filepath.Join(home, ".config", "easy42")
}

// NewStore creates a new Store pointing to the given data directory
func NewStore(dataDir string) *Store {
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}
	return &Store{
		dataDir:  dataDir,
		filePath: filepath.Join(dataDir, "config.json"),
	}
}

// DataDir returns the path to the store's data directory
func (s *Store) DataDir() string {
	return s.dataDir
}

// Exists checks if config.json exists
func (s *Store) Exists() bool {
	_, err := os.Stat(s.filePath)
	return err == nil
}

// Initialize creates a new config.json with a random password, DEK, and session secret
func (s *Store) Initialize() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create data dir: %w", err)
	}

	rawPassword, err := crypto.GenerateRandomPassword()
	if err != nil {
		return "", fmt.Errorf("failed to generate random password: %w", err)
	}

	passHash, err := crypto.HashPassword(rawPassword)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	dek, err := crypto.GenerateRandomBytes(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate DEK: %w", err)
	}

	encryptedDEK, err := crypto.EncryptDEK(rawPassword, dek)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt DEK: %w", err)
	}

	sessionSecret, err := crypto.GenerateSessionSecret()
	if err != nil {
		return "", fmt.Errorf("failed to generate session secret: %w", err)
	}

	prefixes := []string{
		"172.20.0.0/14{21,29}", // dn42
		"172.20.0.0/24{28,32}", // dn42 Anycast
		"172.21.0.0/24{28,32}", // dn42 Anycast
		"172.22.0.0/24{28,32}", // dn42 Anycast
		"172.23.0.0/24{28,32}", // dn42 Anycast
		"172.31.0.0/16+",       // ChaosVPN
		"10.100.0.0/14+",       // ChaosVPN
		"10.0.0.0/8{15,24}",    // Freifunk.net
		"10.127.0.0/16+",       // NeoNetwork
		"fd00::/8{44,64}",      // DN42 ipv6
	}
	cfg := &Config{
		PasswordHash:  passHash,
		EncryptedDEK:  encryptedDEK,
		SessionSecret: sessionSecret,
		NetworkSettings: NetworkSettings{
			PublicASN:      4242420001,
			ConfedMembers:  "4224420000..4224429999",
			ExportPrefixes: prefixes,
			ImportPrefixes: prefixes,
		},
		Nodes: make([]Node, 0),
		Links: make([]Link, 0),
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write config.json: %w", err)
	}

	s.config = cfg
	return rawPassword, nil
}

// Load reads the config from disk
func (s *Store) Load() (*Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrConfigNotFound
		}
		return nil, fmt.Errorf("failed to read config.json: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config.json: %w", err)
	}

	if cfg.Nodes == nil {
		cfg.Nodes = make([]Node, 0)
	}
	if cfg.Links == nil {
		cfg.Links = make([]Link, 0)
	}

	s.config = &cfg
	return &cfg, nil
}

// Get returns the in-memory config snapshot
func (s *Store) Get() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// Save persists the config to disk atomically
func (s *Store) Save(cfg *Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	tmpFile := filepath.Join(s.dataDir, ".config.json.tmp")
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write tmp config: %w", err)
	}

	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return fmt.Errorf("failed to atomic rename config: %w", err)
	}

	s.config = cfg
	return nil
}
