package tasks

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DefaultOnlineSource is the hardcoded default GitHub repository raw content URL
const DefaultOnlineSource = "https://raw.githubusercontent.com/sagan/easy42-tasks/refs/heads/master/"

var validTaskIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// OnlineTask represents a task entry in the online repository manifest
type OnlineTask struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Weight      int      `json:"weight"`
	Files       []string `json:"files,omitempty"`
	Author      string   `json:"author,omitempty"`
	Version     string   `json:"version,omitempty"`
}

// OnlineManifest represents the repository manifest.json schema
type OnlineManifest struct {
	Tasks []OnlineTask `json:"tasks"`
}

// GetOnlineSources returns all configured online task source URLs
// It reads <customDir>/sources.txt and prepends the default source
func GetOnlineSources(customDirs ...string) []string {
	customDir := resolveCustomDir(customDirs)
	sources := []string{DefaultOnlineSource}
	seen := map[string]bool{DefaultOnlineSource: true}

	if customDir == "" {
		return sources
	}

	sourcesFile := filepath.Join(customDir, "sources.txt")
	file, err := os.Open(sourcesFile)
	if err != nil {
		return sources
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Ensure valid URL
		if _, err := url.ParseRequestURI(line); err != nil {
			continue
		}
		// Ensure trailing slash
		if !strings.HasSuffix(line, "/") {
			line += "/"
		}
		if !seen[line] {
			seen[line] = true
			sources = append(sources, line)
		}
	}
	if err := scanner.Err(); err != nil {
		// return whatever sources were successfully read
		return sources
	}

	return sources
}

var allowPrivateSources = false

// SetAllowPrivateSourcesForTest enables or disables private network sources during testing
func SetAllowPrivateSourcesForTest(allow bool) {
	allowPrivateSources = allow
}

// isPrivateOrReservedIP checks if an IP is loopback, private, link-local, multicast, or cloud metadata
func isPrivateOrReservedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	// IPv4 checks
	if ip4 := ip.To4(); ip4 != nil {
		// RFC 1918 (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
		// RFC 6598 (100.64.0.0/10)
		// RFC 3927 (169.254.0.0/16 link-local / cloud metadata)
		// RFC 5737 (192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24)
		// RFC 2544 (198.18.0.0/15)
		if ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			(ip4[0] == 192 && ip4[1] == 168) ||
			(ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127) ||
			(ip4[0] == 169 && ip4[1] == 254) ||
			(ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2) ||
			(ip4[0] == 198 && (ip4[1] == 51 || ip4[1] == 18 || ip4[1] == 19)) ||
			(ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113) ||
			ip4[0] >= 224 {
			return true
		}
		return false
	}

	// IPv6 checks
	// Unique Local Address fc00::/7
	if len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc {
		return true
	}
	// Cloud IPv6 metadata (e.g. AWS fd00:ec2::254)
	if len(ip) == net.IPv6len && ip[0] == 0xfd && ip[1] == 0x00 {
		return true
	}

	return false
}

// newSafeHTTPClient creates an HTTP client that prevents SSRF attacks
func newSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("invalid address %s: %w", addr, err)
			}

			// If allowed in tests, dial directly
			if allowPrivateSources {
				return dialer.DialContext(ctx, network, addr)
			}

			// Resolve host to IPs
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve host %s: %w", host, err)
			}

			var safeIP net.IP
			for _, ipAddr := range ips {
				if !isPrivateOrReservedIP(ipAddr.IP) {
					safeIP = ipAddr.IP
					break
				}
			}

			if safeIP == nil {
				return nil, fmt.Errorf("access to host %s (%v) is blocked for security (private or reserved IP address)", host, ips)
			}

			targetAddr := net.JoinHostPort(safeIP.String(), port)
			return dialer.DialContext(ctx, network, targetAddr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("unsupported redirect scheme: %s", req.URL.Scheme)
			}
			return nil
		},
	}
}

// validateSourceURL validates that the source URL is valid, uses http/https, and ends with a slash
func validateSourceURL(rawURL string) (string, error) {
	s := strings.TrimSpace(rawURL)
	if s == "" {
		return DefaultOnlineSource, nil
	}
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return "", fmt.Errorf("invalid source URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("source URL must use http or https scheme")
	}
	if u.Host == "" {
		return "", fmt.Errorf("source URL missing host")
	}
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s, nil
}

// FetchOnlineManifest fetches and parses manifest.json from the given source URL
func FetchOnlineManifest(ctx context.Context, sourceURL string) (*OnlineManifest, error) {
	normURL, err := validateSourceURL(sourceURL)
	if err != nil {
		return nil, err
	}
	manifestURL := normURL + "manifest.json"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", manifestURL, err)
	}

	client := newSafeHTTPClient(15 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest from %s: %w", manifestURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP %d when fetching manifest from %s", resp.StatusCode, manifestURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024)) // 5MB limit
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest body: %w", err)
	}

	var manifest OnlineManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest.json: %w", err)
	}

	return &manifest, nil
}

// InstallOnlineTask downloads all files of a task into a temp directory and renames it to <data-dir>/tasks/<task-id>
func InstallOnlineTask(ctx context.Context, customDir string, sourceURL string, taskID string, files []string) error {
	taskID = strings.TrimSpace(taskID)
	if !validTaskIDRegex.MatchString(taskID) {
		return fmt.Errorf("invalid task id '%s': only alphanumeric, underscore, and dash are allowed", taskID)
	}
	if customDir == "" {
		customDir = GetCustomTasksDir()
	}
	if customDir == "" {
		return fmt.Errorf("custom tasks directory is not configured")
	}

	if err := os.MkdirAll(customDir, 0755); err != nil {
		return fmt.Errorf("failed to create tasks directory %s: %w", customDir, err)
	}

	normURL, err := validateSourceURL(sourceURL)
	if err != nil {
		return err
	}

	// Default files if none provided
	if len(files) == 0 {
		files = []string{"run.sh", "status.sh", "task.json"}
	}

	// Always ensure run.sh is included in download attempt
	hasRunSh := false
	for _, f := range files {
		if f == "run.sh" {
			hasRunSh = true
			break
		}
	}
	if !hasRunSh {
		files = append([]string{"run.sh"}, files...)
	}

	// 1. Download to temporary folder: <customDir>/.download-<taskID>-<rand>
	randSuffix := randomHex(4)
	tempDirName := fmt.Sprintf(".download-%s-%s", taskID, randSuffix)
	tempDirPath := filepath.Join(customDir, tempDirName)
	if err := os.MkdirAll(tempDirPath, 0755); err != nil {
		return fmt.Errorf("failed to create temp download dir: %w", err)
	}

	// Clean up temp dir if installation fails
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(tempDirPath)
		}
	}()

	client := newSafeHTTPClient(30 * time.Second)
	downloadedCount := 0

	for _, fname := range files {
		fname = filepath.Clean(fname)
		if strings.Contains(fname, "..") || filepath.IsAbs(fname) || strings.Contains(fname, "/") || strings.Contains(fname, "\\") {
			continue // Prevent path traversal
		}

		fileURL := fmt.Sprintf("%s%s/%s", normURL, taskID, fname)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			if fname == "run.sh" {
				return fmt.Errorf("failed to download required file run.sh from %s: %w", fileURL, err)
			}
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if fname == "run.sh" {
				return fmt.Errorf("required file run.sh not found at %s (HTTP %d)", fileURL, resp.StatusCode)
			}
			// Optional file (status.sh, task.json) not found, skip
			continue
		}

		destPath := filepath.Join(tempDirPath, fname)
		out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to create local file %s: %w", destPath, err)
		}

		_, copyErr := io.Copy(out, io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit per file
		resp.Body.Close()
		_ = out.Close()
		if copyErr != nil {
			return fmt.Errorf("failed to save content to %s: %w", destPath, copyErr)
		}

		// Make shell scripts executable
		if strings.HasSuffix(fname, ".sh") {
			_ = os.Chmod(destPath, 0755)
		}

		downloadedCount++
	}

	// Verify run.sh exists in downloaded files
	runShPath := filepath.Join(tempDirPath, "run.sh")
	if info, err := os.Stat(runShPath); err != nil || info.IsDir() {
		return fmt.Errorf("run.sh was not found or is not a file in downloaded task")
	}

	// 2. Final destination path
	destTaskPath := filepath.Join(customDir, taskID)

	// Remove destination if it already exists
	_ = os.RemoveAll(destTaskPath)

	// Rename temp directory to target directory
	if err := os.Rename(tempDirPath, destTaskPath); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tempDirPath, destTaskPath, err)
	}

	success = true
	return nil
}

// DeleteCustomTask safely removes a custom task directory from <data-dir>/tasks/<task-id>
func DeleteCustomTask(customDir string, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if !validTaskIDRegex.MatchString(taskID) {
		return fmt.Errorf("invalid task id '%s'", taskID)
	}
	if customDir == "" {
		customDir = GetCustomTasksDir()
	}
	if customDir == "" {
		return fmt.Errorf("custom tasks directory is not configured")
	}

	targetDir := filepath.Join(customDir, taskID)
	info, err := os.Stat(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("custom task '%s' does not exist in %s", taskID, customDir)
		}
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("target '%s' is not a directory", targetDir)
	}

	if err := os.RemoveAll(targetDir); err != nil {
		return fmt.Errorf("failed to delete task directory: %w", err)
	}

	return nil
}
