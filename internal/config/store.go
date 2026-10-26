package config

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"easy42/internal/crypto"
)

var (
	ErrConfigNotFound = errors.New("config file not found")
)

// Store handles thread-safe loading and persisting of easy42 config
type Store struct {
	mu           sync.RWMutex
	dataDir      string
	filePath     string
	config       *Config
	lastModTime  time.Time
	lastFileSize int64
	lastHash     [32]byte
	listeners    []func()
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

	rawPassword, err := crypto.GenerateRandomPassword(32)
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
			Name:      "Easy42",
			PublicASN: 4242420001,
			Prefixes:  prefixes,
		},
		Nodes:       make([]Node, 0),
		Links:       make([]Link, 0),
		ServerTasks: DefaultServerTasks(),
		Database: DatabaseSettings{
			RetentionDays: DefaultRetentionDays,
		},
		Integrations: IntegrationsSettings{
			PrometheusEnabled:   true,
			PrometheusAuthToken: GeneratePrometheusToken(),
		},
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

	h := sha256.Sum256(data)
	s.lastHash = h
	if stat, err := os.Stat(s.filePath); err == nil {
		s.lastModTime = stat.ModTime()
		s.lastFileSize = stat.Size()
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

	if strings.TrimSpace(cfg.NetworkSettings.Name) == "" {
		cfg.NetworkSettings.Name = "Easy42"
	}
	cfg.NetworkSettings.Prefixes = CleanPrefixes(cfg.NetworkSettings.Prefixes)
	if len(cfg.NetworkSettings.DisallowedDN42Networks) == 0 && len(cfg.NetworkSettings.DisallowedDN42CIDRs) > 0 {
		cfg.NetworkSettings.DisallowedDN42Networks = CleanPrefixes(cfg.NetworkSettings.DisallowedDN42CIDRs)
	} else {
		cfg.NetworkSettings.DisallowedDN42Networks = CleanPrefixes(cfg.NetworkSettings.DisallowedDN42Networks)
	}
	cfg.NetworkSettings.DisallowedDN42CIDRs = cfg.NetworkSettings.DisallowedDN42Networks
	originalTasks := cfg.ServerTasks
	cfg.ServerTasks = NormalizeServerTasks(cfg.ServerTasks)
	needsSave := len(originalTasks) != len(cfg.ServerTasks)
	if !needsSave {
		for i, t := range originalTasks {
			if t.ID != cfg.ServerTasks[i].ID {
				needsSave = true
				break
			}
		}
	}

	if faAddr := cfg.NetworkSettings.GetFlapAlertedAddress(); faAddr != "" {
		cfg.NetworkSettings.FlapAlertedAddress = faAddr
		if cfg.NetworkSettings.FlapAlertedPort == 0 {
			cfg.NetworkSettings.FlapAlertedPort = 1790
		}
		if cfg.NetworkSettings.FlapAlertedASN == 0 {
			cfg.NetworkSettings.FlapAlertedASN = 4224420000
		}
	}
	if faURL := strings.TrimSpace(cfg.NetworkSettings.FlapAlertedURL); faURL != "" {
		cfg.NetworkSettings.FlapAlertedURL = faURL
	}

	if cfg.Integrations.PrometheusAuthToken == "" {
		cfg.Integrations.PrometheusAuthToken = GeneratePrometheusToken()
		cfg.Integrations.PrometheusEnabled = true
		needsSave = true
	}

	if needsSave {
		_ = s.saveLocked(&cfg)
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

// Save persists the config to disk atomically and notifies listeners
func (s *Store) Save(cfg *Config) error {
	s.mu.Lock()
	err := s.saveLocked(cfg)
	s.mu.Unlock()
	if err == nil {
		s.notifyChange()
	}
	return err
}

func (s *Store) saveLocked(cfg *Config) error {
	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return fmt.Errorf("failed to create data dir: %w", err)
	}

	if strings.TrimSpace(cfg.NetworkSettings.Name) == "" {
		cfg.NetworkSettings.Name = "Easy42"
	}
	cfg.NetworkSettings.Prefixes = CleanPrefixes(cfg.NetworkSettings.Prefixes)
	if len(cfg.NetworkSettings.DisallowedDN42Networks) == 0 && len(cfg.NetworkSettings.DisallowedDN42CIDRs) > 0 {
		cfg.NetworkSettings.DisallowedDN42Networks = CleanPrefixes(cfg.NetworkSettings.DisallowedDN42CIDRs)
	} else {
		cfg.NetworkSettings.DisallowedDN42Networks = CleanPrefixes(cfg.NetworkSettings.DisallowedDN42Networks)
	}
	cfg.NetworkSettings.DisallowedDN42CIDRs = cfg.NetworkSettings.DisallowedDN42Networks
	cfg.ServerTasks = NormalizeServerTasks(cfg.ServerTasks)
	if faAddr := cfg.NetworkSettings.GetFlapAlertedAddress(); faAddr != "" {
		cfg.NetworkSettings.FlapAlertedAddress = faAddr
		if cfg.NetworkSettings.FlapAlertedPort == 0 {
			cfg.NetworkSettings.FlapAlertedPort = 1790
		}
		if cfg.NetworkSettings.FlapAlertedASN == 0 {
			cfg.NetworkSettings.FlapAlertedASN = 4224420000
		}
	}
	if faURL := strings.TrimSpace(cfg.NetworkSettings.FlapAlertedURL); faURL != "" {
		cfg.NetworkSettings.FlapAlertedURL = faURL
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
	s.lastHash = sha256.Sum256(data)
	if stat, err := os.Stat(s.filePath); err == nil {
		s.lastModTime = stat.ModTime()
		s.lastFileSize = stat.Size()
	}
	return nil
}

// OnChange registers a listener callback invoked whenever the configuration changes
func (s *Store) OnChange(cb func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, cb)
}

func (s *Store) notifyChange() {
	s.mu.RLock()
	cbs := make([]func(), len(s.listeners))
	copy(cbs, s.listeners)
	s.mu.RUnlock()
	for _, cb := range cbs {
		go cb()
	}
}

// CheckFileModified checks if config.json on disk has been modified externally
func (s *Store) CheckFileModified() (bool, error) {
	stat, err := os.Stat(s.filePath)
	if err != nil {
		return false, err
	}

	s.mu.RLock()
	modTime := s.lastModTime
	fileSize := s.lastFileSize
	s.mu.RUnlock()

	if !modTime.IsZero() && stat.ModTime().Equal(modTime) && stat.Size() == fileSize {
		return false, nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return false, err
	}
	h := sha256.Sum256(data)

	s.mu.RLock()
	sameHash := (h == s.lastHash)
	s.mu.RUnlock()

	if sameHash {
		s.mu.Lock()
		s.lastModTime = stat.ModTime()
		s.lastFileSize = stat.Size()
		s.mu.Unlock()
		return false, nil
	}

	_, err = s.Load()
	if err != nil {
		return false, err
	}
	s.notifyChange()
	return true, nil
}

// StartWatcher starts a background polling watcher that checks for external modifications to config.json
func (s *Store) StartWatcher(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.CheckFileModified()
			}
		}
	}()
}
