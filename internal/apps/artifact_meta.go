package apps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AppArtifactMeta contains metadata for a cached app artifact across versions and architectures
type AppArtifactMeta struct {
	AppName     string                          `json:"app_name"`
	LastChecked time.Time                       `json:"last_checked"`
	Versions    map[string]*VersionArtifactMeta `json:"versions"` // version (e.g. "latest", "v1.2.3") -> metadata
}

// VersionArtifactMeta contains version-level metadata and architecture targets
type VersionArtifactMeta struct {
	Version         string                       `json:"version"`
	ResolvedVersion string                       `json:"resolved_version,omitempty"` // Actual resolved upstream tag (e.g. "v1.68.1" when version is "latest")
	ResolvedAt      time.Time                    `json:"resolved_at"`
	Architectures   map[string]*ArchArtifactMeta `json:"architectures"` // arch (e.g. "amd64", "arm64") -> metadata
}

// ArchArtifactMeta contains architecture-level artifact metadata
type ArchArtifactMeta struct {
	Arch           string                        `json:"arch"`
	DownloadURL    string                        `json:"download_url"`
	ArchiveFile    *ArchiveFileInfo              `json:"archive_file,omitempty"`
	ExtractedFiles map[string]*ExtractedFileInfo `json:"extracted_files"`
	CachedAt       time.Time                     `json:"cached_at"`
	LastValidated  time.Time                     `json:"last_validated"`
	FilesOnDisk    bool                          `json:"files_on_disk"` // Whether physical archive/extracted files currently exist on server disk
}

// ArchiveFileInfo holds metadata about a cached upstream archive
type ArchiveFileInfo struct {
	FileName     string    `json:"file_name"`
	LocalPath    string    `json:"local_path"`
	SHA256       string    `json:"sha256"`
	Size         int64     `json:"size"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

// ArtifactValidationResult reports the result of validating a cached artifact against origin
type ArtifactValidationResult struct {
	AppName         string `json:"app_name"`
	Version         string `json:"version"`
	Arch            string `json:"arch"`
	Status          string `json:"status"` // "up_to_date", "updated", "re_downloaded", "error"
	PreviousVersion string `json:"previous_version,omitempty"`
	ResolvedVersion string `json:"resolved_version,omitempty"`
	Message         string `json:"message"`
	Updated         bool   `json:"updated"`
}

// ArtifactClearCacheResult reports the result of purging physical artifact files from disk
type ArtifactClearCacheResult struct {
	AppName       string `json:"app_name"`
	FreedBytes    int64  `json:"freed_bytes"`
	FreedHuman    string `json:"freed_human"`
	MetaPreserved bool   `json:"meta_preserved"`
	Message       string `json:"message"`
}

// AppArtifactStatus represents live status of an app's artifact cache on the server
type AppArtifactStatus struct {
	AppID         string                    `json:"app_id"`
	AppName       string                    `json:"app_name"`
	Version       string                    `json:"version"`
	HasArtifact   bool                      `json:"has_artifact"`
	MetaExists    bool                      `json:"meta_exists"`
	FilesOnDisk   bool                      `json:"files_on_disk"`
	DiskSizeBytes int64                     `json:"disk_size_bytes"`
	DiskSizeHuman string                    `json:"disk_size_human"`
	LastChecked   *time.Time                `json:"last_checked,omitempty"`
	LastValidated *time.Time                `json:"last_validated,omitempty"`
	Versions      map[string]*VersionStatus `json:"versions,omitempty"`
}

// VersionStatus describes a version within AppArtifactStatus
type VersionStatus struct {
	Version         string                `json:"version"`
	ResolvedVersion string                `json:"resolved_version,omitempty"`
	Architectures   map[string]*ArchStatus `json:"architectures"`
}

// ArchStatus describes an architecture target within VersionStatus
type ArchStatus struct {
	Arch           string              `json:"arch"`
	DownloadURL    string              `json:"download_url"`
	ArchiveSize    int64               `json:"archive_size"`
	ArchiveSha256  string              `json:"archive_sha256"`
	FilesOnDisk    bool                `json:"files_on_disk"`
	ExtractedFiles []ExtractedFileInfo `json:"extracted_files"`
	LastValidated  time.Time           `json:"last_validated"`
}

// metaMu synchronizes metadata reads and writes
var metaMu sync.RWMutex

// getMetaDir returns the directory where metadata files are stored: <CacheDir>/meta
func (m *ArtifactManager) getMetaDir() string {
	return filepath.Join(m.CacheDir, "meta")
}

// getAppMetaPath returns the file path for an app's metadata: <CacheDir>/meta/<app>.json
func (m *ArtifactManager) getAppMetaPath(appName string) string {
	cleanName := strings.TrimSpace(appName)
	return filepath.Join(m.getMetaDir(), cleanName+".json")
}

// LoadAppMeta loads persistent metadata for an app from disk
func (m *ArtifactManager) LoadAppMeta(appName string) (*AppArtifactMeta, error) {
	metaMu.RLock()
	defer metaMu.RUnlock()

	metaPath := m.getAppMetaPath(appName)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read artifact metadata for %s: %w", appName, err)
	}

	var meta AppArtifactMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse artifact metadata for %s: %w", appName, err)
	}

	return &meta, nil
}

// SaveAppMeta saves persistent metadata for an app to disk
func (m *ArtifactManager) SaveAppMeta(meta *AppArtifactMeta) error {
	if meta == nil || meta.AppName == "" {
		return fmt.Errorf("cannot save nil or empty artifact metadata")
	}

	metaMu.Lock()
	defer metaMu.Unlock()

	metaDir := m.getMetaDir()
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		return fmt.Errorf("failed to create meta directory: %w", err)
	}

	metaPath := m.getAppMetaPath(meta.AppName)
	tmpPath := metaPath + ".tmp"

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize artifact metadata: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write tmp metadata file: %w", err)
	}

	if err := os.Rename(tmpPath, metaPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to atomically update metadata file: %w", err)
	}

	return nil
}

// DeleteAppMeta removes the metadata file for an app
func (m *ArtifactManager) DeleteAppMeta(appName string) error {
	metaMu.Lock()
	defer metaMu.Unlock()

	metaPath := m.getAppMetaPath(appName)
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// FormatBytes formats a byte count into a human-readable string (e.g. 14.5 MB)
func FormatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// CalculateAppDiskUsage calculates the total physical bytes used by an app's archives and extracted files
func (m *ArtifactManager) CalculateAppDiskUsage(appName string) int64 {
	var total int64

	archivesDir := filepath.Join(m.CacheDir, "archives", appName)
	_ = filepath.Walk(archivesDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})

	extractedDir := filepath.Join(m.CacheDir, "extracted", appName)
	_ = filepath.Walk(extractedDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})

	return total
}
