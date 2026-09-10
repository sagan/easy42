package roa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Manager handles caching and downloading ROA files from remote URLs or reading local files
type Manager struct {
	cacheDir string
	client   *http.Client
	mu       sync.RWMutex
}

// NewManager creates a new ROA cache manager
func NewManager(cacheDir string) *Manager {
	if cacheDir != "" {
		_ = os.MkdirAll(cacheDir, 0755)
	}
	return &Manager{
		cacheDir: cacheDir,
		client: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

// CacheFilePath returns the local cache file path for a given URL
func (m *Manager) CacheFilePath(url string) string {
	if m.cacheDir == "" {
		return ""
	}
	h := sha256.Sum256([]byte(url))
	return filepath.Join(m.cacheDir, "roa_"+hex.EncodeToString(h[:16])+".conf")
}

// IsURL checks if source string is an HTTP/HTTPS URL
func IsURL(source string) bool {
	s := strings.TrimSpace(source)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// GetROAContent retrieves the content for a given source (URL or local path).
// If source is a URL, it retrieves from cache or downloads from the URL.
// If forceRefresh is false, cached data younger than maxCacheAge is reused.
// If downloading fails but a cached version exists, the cached version is returned gracefully.
func (m *Manager) GetROAContent(ctx context.Context, source string, forceRefresh bool) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil
	}

	if !IsURL(source) {
		// Local file path
		data, err := os.ReadFile(source)
		if err != nil {
			return "", fmt.Errorf("failed to read local ROA file '%s': %w", source, err)
		}
		return string(data), nil
	}

	cacheFile := m.CacheFilePath(source)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if cached file exists and is fresh enough (< 1 hour)
	if !forceRefresh && cacheFile != "" {
		info, err := os.Stat(cacheFile)
		if err == nil && time.Since(info.ModTime()) < 1*time.Hour {
			data, err := os.ReadFile(cacheFile)
			if err == nil && len(data) > 0 {
				return string(data), nil
			}
		}
	}

	// Download from URL
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		// If request creation fails, attempt stale cache fallback
		if cached, ok := m.readStaleCache(cacheFile); ok {
			return cached, nil
		}
		return "", fmt.Errorf("failed to create request for ROA '%s': %w", source, err)
	}
	req.Header.Set("User-Agent", "easy42-roa-fetcher/1.0")

	resp, err := m.client.Do(req)
	if err != nil {
		if cached, ok := m.readStaleCache(cacheFile); ok {
			return cached, nil
		}
		return "", fmt.Errorf("failed to download ROA from '%s': %w", source, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if cached, ok := m.readStaleCache(cacheFile); ok {
			return cached, nil
		}
		return "", fmt.Errorf("download ROA from '%s' failed with HTTP %d", source, resp.StatusCode)
	}

	// Read content up to 10MB limit
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		if cached, ok := m.readStaleCache(cacheFile); ok {
			return cached, nil
		}
		return "", fmt.Errorf("failed to read response body from '%s': %w", source, err)
	}

	// Write to cache file atomically
	if cacheFile != "" {
		if err := os.MkdirAll(filepath.Dir(cacheFile), 0755); err == nil {
			tmpFile := cacheFile + ".tmp"
			if err := os.WriteFile(tmpFile, data, 0644); err == nil {
				_ = os.Rename(tmpFile, cacheFile)
			}
		}
	}

	return string(data), nil
}

// readStaleCache attempts to read the cache file even if expired
func (m *Manager) readStaleCache(cacheFile string) (string, bool) {
	if cacheFile == "" {
		return "", false
	}
	data, err := os.ReadFile(cacheFile)
	if err == nil && len(data) > 0 {
		return string(data), true
	}
	return "", false
}

// ClearCache removes all cached ROA files
func (m *Manager) ClearCache() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cacheDir == "" {
		return nil
	}
	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "roa_") {
			_ = os.Remove(filepath.Join(m.cacheDir, entry.Name()))
		}
	}
	return nil
}

// ClearSourceCache removes cache for a specific source URL
func (m *Manager) ClearSourceCache(source string) error {
	if !IsURL(source) {
		return nil
	}
	cacheFile := m.CacheFilePath(source)
	if cacheFile == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return os.Remove(cacheFile)
}
