package roa

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Manager handles caching and downloading ROA files from remote HTTPS URLs
type Manager struct {
	cacheDir string
	client   *http.Client
	mu       sync.RWMutex
}

func isBlockedROAIP(addr netip.Addr) bool {
	addr = addr.Unmap()
	return addr.IsLoopback() ||
		addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified()
}

func newSafeROAHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("invalid ROA target address: %w", err)
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return fmt.Errorf("invalid ROA target IP %q: %w", host, err)
			}
			if isBlockedROAIP(ip) {
				return fmt.Errorf("ROA fetch blocked to non-public IP %s", ip)
			}
			return nil
		},
	}
	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &http.Client{
		Timeout:   25 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if err := ValidateROAURL(req.URL.String()); err != nil {
				return fmt.Errorf("redirect blocked: %w", err)
			}
			return nil
		},
	}
}

// NewManager creates a new ROA cache manager
func NewManager(cacheDir string) *Manager {
	if cacheDir != "" {
		_ = os.MkdirAll(cacheDir, 0755)
	}
	return &Manager{
		cacheDir: cacheDir,
		client:   newSafeROAHTTPClient(),
	}
}

// SetHTTPClient overrides the HTTP client (intended for testing).
func (m *Manager) SetHTTPClient(c *http.Client) {
	if c == nil {
		return
	}
	m.mu.Lock()
	m.client = c
	m.mu.Unlock()
}

// CacheFilePath returns the local cache file path for a given URL
func (m *Manager) CacheFilePath(rawURL string) string {
	if m.cacheDir == "" {
		return ""
	}
	h := sha256.Sum256([]byte(rawURL))
	return filepath.Join(m.cacheDir, "roa_"+hex.EncodeToString(h[:16])+".conf")
}

// ValidateROAURL validates that source is an absolute HTTPS URL without userinfo.
func ValidateROAURL(source string) error {
	s := strings.TrimSpace(source)
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("ROA source must be a valid https:// URL")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("ROA source must use https:// (local files and http:// are not supported)")
	}
	if u.User != nil {
		return fmt.Errorf("ROA URL must not contain credentials")
	}
	return nil
}

// IsURL checks if source string is a valid HTTPS URL
func IsURL(source string) bool {
	s := strings.TrimSpace(source)
	return s != "" && ValidateROAURL(s) == nil
}

// GetROAContent retrieves the content for a given HTTPS ROA URL.
// If forceRefresh is false, cached data younger than 1 hour is reused.
// If downloading fails but a cached version exists, the cached version is returned gracefully.
func (m *Manager) GetROAContent(ctx context.Context, source string, forceRefresh bool) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil
	}

	if err := ValidateROAURL(source); err != nil {
		return "", err
	}

	cacheFile := m.CacheFilePath(source)

	// Check fresh cache under read lock
	if !forceRefresh && cacheFile != "" {
		m.mu.RLock()
		info, err := os.Stat(cacheFile)
		if err == nil && time.Since(info.ModTime()) < 1*time.Hour {
			data, readErr := os.ReadFile(cacheFile)
			m.mu.RUnlock()
			if readErr == nil && len(data) > 0 {
				return string(data), nil
			}
		} else {
			m.mu.RUnlock()
		}
	}

	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()

	readStale := func() (string, bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.readStaleCache(cacheFile)
	}

	// Download from URL without holding m.mu
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		if cached, ok := readStale(); ok {
			return cached, nil
		}
		return "", fmt.Errorf("failed to create request for ROA '%s': %w", source, err)
	}
	req.Header.Set("User-Agent", "easy42-roa-fetcher/1.0")

	resp, err := client.Do(req)
	if err != nil {
		if cached, ok := readStale(); ok {
			return cached, nil
		}
		return "", fmt.Errorf("failed to download ROA from '%s': %w", source, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if cached, ok := readStale(); ok {
			return cached, nil
		}
		return "", fmt.Errorf("download ROA from '%s' failed with HTTP %d", source, resp.StatusCode)
	}

	// Read content up to 10MB limit
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		if cached, ok := readStale(); ok {
			return cached, nil
		}
		return "", fmt.Errorf("failed to read response body from '%s': %w", source, err)
	}

	// Write to cache file atomically under write lock
	if cacheFile != "" {
		m.mu.Lock()
		if err := os.MkdirAll(filepath.Dir(cacheFile), 0755); err == nil {
			tmpFile := cacheFile + ".tmp"
			if err := os.WriteFile(tmpFile, data, 0644); err == nil {
				_ = os.Rename(tmpFile, cacheFile)
			}
		}
		m.mu.Unlock()
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
