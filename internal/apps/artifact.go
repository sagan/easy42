package apps

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"easy42/internal/config"
)

// ExtractedFileInfo holds metadata about a cached artifact binary/file
type ExtractedFileInfo struct {
	Name      string `json:"name"`
	LocalPath string `json:"local_path"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
}

// ArtifactManager handles downloading, decompressing, caching, and token generation for native app artifacts
type ArtifactManager struct {
	CacheDir    string
	HTTPClient  *http.Client
	mu          sync.RWMutex
	inFlight    map[string]*sync.WaitGroup
	inFlightErr map[string]error
}

// NewArtifactManager creates a new ArtifactManager with a designated cache directory
func NewArtifactManager(cacheDir string) *ArtifactManager {
	if cacheDir == "" {
		cacheDir = "/var/lib/easy42/cache/artifacts"
	}
	// Verify or fallback to temp dir if cacheDir cannot be created
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		cacheDir = filepath.Join(os.TempDir(), "easy42", "cache", "artifacts")
		_ = os.MkdirAll(cacheDir, 0755)
	}

	return &ArtifactManager{
		CacheDir: cacheDir,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Minute, // Allow ample time for large binary downloads
		},
		inFlight:    make(map[string]*sync.WaitGroup),
		inFlightErr: make(map[string]error),
	}
}

// NormalizeArch standardizes an architecture string and applies any app-specific arch mappings
func NormalizeArch(nodeArch string, mapping map[string]string) string {
	arch := strings.ToLower(strings.TrimSpace(nodeArch))
	switch arch {
	case "x86_64", "x64":
		arch = "amd64"
	case "aarch64", "arm64v8":
		arch = "arm64"
	case "armv7l", "armhf", "arm":
		arch = "armv7"
	case "mipsle":
		arch = "mipsel"
	}

	if mapping != nil {
		if mapped, ok := mapping[arch]; ok && mapped != "" {
			return mapped
		}
	}
	return arch
}

// ToGNUArch maps standard architecture names to GNU arch / uname -m format (e.g. x86_64, aarch64, armv7l)
func ToGNUArch(arch string) string {
	clean := strings.ToLower(strings.TrimSpace(arch))
	switch clean {
	case "amd64", "x64", "x86_64":
		return "x86_64"
	case "arm64", "arm64v8", "aarch64":
		return "aarch64"
	case "386", "i386", "i686", "x86":
		return "i386"
	case "armv7", "armv7l", "armhf":
		return "armv7l"
	case "armv6", "armv6l":
		return "armv6l"
	case "arm", "armel":
		return "arm"
	case "riscv64":
		return "riscv64"
	case "s390x":
		return "s390x"
	case "ppc64le":
		return "ppc64le"
	case "ppc64":
		return "ppc64"
	case "mips":
		return "mips"
	case "mipsel", "mipsle":
		return "mipsel"
	case "mips64":
		return "mips64"
	case "mips64le":
		return "mips64le"
	case "loong64", "loongarch64":
		return "loongarch64"
	default:
		return clean
	}
}

// ToRustArch maps standard architecture names to Rust target triple architecture format (e.g. x86_64, aarch64, armv7)
func ToRustArch(arch string) string {
	clean := strings.ToLower(strings.TrimSpace(arch))
	switch clean {
	case "amd64", "x64", "x86_64":
		return "x86_64"
	case "arm64", "arm64v8", "aarch64":
		return "aarch64"
	case "386", "i386", "i686", "x86":
		return "i686"
	case "armv7", "armv7l", "armhf":
		return "armv7"
	case "armv6", "armv6l":
		return "armv6"
	case "arm", "armel":
		return "arm"
	case "riscv64":
		return "riscv64gc"
	case "s390x":
		return "s390x"
	case "ppc64le":
		return "powerpc64le"
	case "ppc64":
		return "powerpc64"
	case "mips":
		return "mips"
	case "mipsel", "mipsle":
		return "mipsel"
	case "mips64":
		return "mips64"
	case "mips64le":
		return "mips64le"
	case "loong64", "loongarch64":
		return "loongarch64"
	default:
		return clean
	}
}

// ToDebArch maps standard architecture names to Debian / dpkg format (e.g. amd64, arm64, armhf)
func ToDebArch(arch string) string {
	clean := strings.ToLower(strings.TrimSpace(arch))
	switch clean {
	case "amd64", "x64", "x86_64":
		return "amd64"
	case "arm64", "arm64v8", "aarch64":
		return "arm64"
	case "386", "i386", "i686", "x86":
		return "i386"
	case "armv7", "armv7l", "armhf":
		return "armhf"
	case "armv6", "armv6l", "armel":
		return "armel"
	case "arm":
		return "armhf"
	case "riscv64":
		return "riscv64"
	case "s390x":
		return "s390x"
	case "ppc64le":
		return "ppc64el"
	case "mips":
		return "mips"
	case "mipsel", "mipsle":
		return "mipsel"
	case "mips64el", "mips64le":
		return "mips64el"
	default:
		return clean
	}
}

// MatchExtractPath checks if an archive entry path matches the configured extract_path
func MatchExtractPath(entryName, extractPath string) bool {
	cleanEntry := strings.TrimPrefix(filepath.ToSlash(entryName), "/")
	cleanPattern := strings.TrimPrefix(filepath.ToSlash(extractPath), "/")

	// Exact match
	if cleanEntry == cleanPattern {
		return true
	}

	// Glob match (e.g. "rclone-*-linux-*/rclone" or "*/rclone")
	if matched, _ := path.Match(cleanPattern, cleanEntry); matched {
		return true
	}

	// If pattern is just a bare filename (no slashes), match basename
	if !strings.Contains(cleanPattern, "/") {
		if path.Base(cleanEntry) == cleanPattern {
			return true
		}
	}

	// Suffix / endswith match (e.g. entry ends with "/rclone" when extractPath is "rclone")
	if strings.HasSuffix(cleanEntry, "/"+cleanPattern) {
		return true
	}

	return false
}

// ComputeFileSHA256 returns the lowercase hex SHA-256 hash and size of a file on disk
func ComputeFileSHA256(filePath string) (string, int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, f)
	if err != nil {
		return "", 0, err
	}

	return hex.EncodeToString(hasher.Sum(nil)), size, nil
}

// VerifyChecksum verifies that a file matches an expected SHA256 string (with or without "sha256:" prefix)
func VerifyChecksum(filePath, expectedChecksum string) error {
	expected := strings.TrimSpace(strings.ToLower(expectedChecksum))
	expected = strings.TrimPrefix(expected, "sha256:")
	if expected == "" {
		return nil
	}

	actual, _, err := ComputeFileSHA256(filePath)
	if err != nil {
		return fmt.Errorf("failed to compute file checksum: %w", err)
	}

	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}
	return nil
}

// DecompressAndExtract unpacks target files from archivePath into destDir
func DecompressAndExtract(archivePath string, files []config.ArtifactFile, destDir string) (map[string]*ExtractedFileInfo, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create destination dir: %w", err)
	}

	lowerPath := strings.ToLower(archivePath)
	result := make(map[string]*ExtractedFileInfo)

	switch {
	case strings.HasSuffix(lowerPath, ".zip"):
		if err := extractZip(archivePath, files, destDir, result); err != nil {
			return nil, err
		}
	case strings.HasSuffix(lowerPath, ".tar.gz") || strings.HasSuffix(lowerPath, ".tgz"):
		if err := extractTarGz(archivePath, files, destDir, result); err != nil {
			return nil, err
		}
	case strings.HasSuffix(lowerPath, ".tar.bz2") || strings.HasSuffix(lowerPath, ".tbz2"):
		if err := extractTarBz2(archivePath, files, destDir, result); err != nil {
			return nil, err
		}
	case strings.HasSuffix(lowerPath, ".gz"):
		if err := extractSingleGz(archivePath, files, destDir, result); err != nil {
			return nil, err
		}
	default:
		// Raw uncompressed binary
		if err := extractRawBinary(archivePath, files, destDir, result); err != nil {
			return nil, err
		}
	}

	// Verify all requested files were extracted
	for _, reqFile := range files {
		info, ok := result[reqFile.Name]
		if !ok || info == nil {
			return nil, fmt.Errorf("file '%s' (pattern: '%s') not found in archive %s", reqFile.Name, reqFile.ExtractPath, filepath.Base(archivePath))
		}
	}

	return result, nil
}

func extractZip(archivePath string, files []config.ArtifactFile, destDir string, result map[string]*ExtractedFileInfo) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip archive: %w", err)
	}
	defer r.Close()

	for _, reqFile := range files {
		for _, f := range r.File {
			if f.FileInfo().IsDir() {
				continue
			}
			pat := reqFile.ExtractPath
			if pat == "" {
				pat = reqFile.Name
			}
			if MatchExtractPath(f.Name, pat) {
				rc, err := f.Open()
				if err != nil {
					return fmt.Errorf("failed to read zip entry %s: %w", f.Name, err)
				}

				destPath := filepath.Join(destDir, reqFile.Name)
				info, err := copyAndHash(rc, destPath)
				rc.Close()
				if err != nil {
					return err
				}
				info.Name = reqFile.Name
				result[reqFile.Name] = info
				break
			}
		}
	}
	return nil
}

func extractTarGz(archivePath string, files []config.ArtifactFile, destDir string, result map[string]*ExtractedFileInfo) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to open gzip stream: %w", err)
	}
	defer gzr.Close()

	return extractTarStream(tar.NewReader(gzr), files, destDir, result)
}

func extractTarBz2(archivePath string, files []config.ArtifactFile, destDir string, result map[string]*ExtractedFileInfo) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	bzr := bzip2.NewReader(f)
	return extractTarStream(tar.NewReader(bzr), files, destDir, result)
}

func extractTarStream(tr *tar.Reader, files []config.ArtifactFile, destDir string, result map[string]*ExtractedFileInfo) error {
	remaining := make(map[string]config.ArtifactFile)
	for _, f := range files {
		remaining[f.Name] = f
	}

	for len(remaining) > 0 {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}

		for name, reqFile := range remaining {
			pat := reqFile.ExtractPath
			if pat == "" {
				pat = reqFile.Name
			}
			if MatchExtractPath(hdr.Name, pat) {
				destPath := filepath.Join(destDir, reqFile.Name)
				info, err := copyAndHash(tr, destPath)
				if err != nil {
					return err
				}
				info.Name = reqFile.Name
				result[name] = info
				delete(remaining, name)
				break
			}
		}
	}
	return nil
}

func extractSingleGz(archivePath string, files []config.ArtifactFile, destDir string, result map[string]*ExtractedFileInfo) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to open gzip stream: %w", err)
	}
	defer gzr.Close()

	for _, reqFile := range files {
		destPath := filepath.Join(destDir, reqFile.Name)
		info, err := copyAndHash(gzr, destPath)
		if err != nil {
			return err
		}
		info.Name = reqFile.Name
		result[reqFile.Name] = info
		break
	}
	return nil
}

func extractRawBinary(archivePath string, files []config.ArtifactFile, destDir string, result map[string]*ExtractedFileInfo) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, reqFile := range files {
		destPath := filepath.Join(destDir, reqFile.Name)
		info, err := copyAndHash(f, destPath)
		if err != nil {
			return err
		}
		info.Name = reqFile.Name
		result[reqFile.Name] = info
		break
	}
	return nil
}

func copyAndHash(src io.Reader, destPath string) (*ExtractedFileInfo, error) {
	// Write to temp file first to prevent partial files
	tmpPath := destPath + ".tmp"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}

	hasher := sha256.New()
	multiWriter := io.MultiWriter(out, hasher)
	size, err := io.Copy(multiWriter, src)
	out.Close()

	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to rename temp file: %w", err)
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	return &ExtractedFileInfo{
		LocalPath: destPath,
		SHA256:    hash,
		Size:      size,
	}, nil
}

// EnsureArtifact downloads, verifies, unpacks, and caches an app artifact for a specific architecture.
// Returns a map of extracted file name -> ExtractedFileInfo.
func (m *ArtifactManager) EnsureArtifact(ctx context.Context, app config.App, nodeArch, osName string, globalVars map[string]string) (map[string]*ExtractedFileInfo, error) {
	if app.Artifact == nil {
		return nil, nil
	}

	version := app.Version
	if version == "" {
		version = "latest"
	}

	arch := NormalizeArch(nodeArch, app.Artifact.ArchMapping)
	if osName == "" {
		osName = "linux"
	}

	cacheKey := fmt.Sprintf("%s:%s:%s", app.Name, version, arch)

	// In-flight single flight de-duplication
	m.mu.Lock()
	if wg, exists := m.inFlight[cacheKey]; exists {
		m.mu.Unlock()
		wg.Wait()
		m.mu.RLock()
		err := m.inFlightErr[cacheKey]
		m.mu.RUnlock()
		if err != nil {
			return nil, err
		}
		return m.loadExtractedFromCache(app.Name, version, arch, app.Artifact.Files)
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	m.inFlight[cacheKey] = wg
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.inFlight, cacheKey)
		m.mu.Unlock()
		wg.Done()
	}()

	files, err := m.doEnsureArtifact(ctx, app, version, arch, osName, globalVars)
	m.mu.Lock()
	m.inFlightErr[cacheKey] = err
	m.mu.Unlock()

	return files, err
}

func (m *ArtifactManager) loadExtractedFromCache(appName, version, arch string, files []config.ArtifactFile) (map[string]*ExtractedFileInfo, error) {
	destDir := filepath.Join(m.CacheDir, "extracted", appName, version, arch)
	res := make(map[string]*ExtractedFileInfo)
	for _, f := range files {
		targetFile := filepath.Join(destDir, f.Name)
		hash, size, err := ComputeFileSHA256(targetFile)
		if err != nil {
			return nil, fmt.Errorf("cached file '%s' missing or unreadable: %w", f.Name, err)
		}
		res[f.Name] = &ExtractedFileInfo{
			Name:      f.Name,
			LocalPath: targetFile,
			SHA256:    hash,
			Size:      size,
		}
	}
	return res, nil
}

func (m *ArtifactManager) doEnsureArtifact(ctx context.Context, app config.App, version, arch, osName string, globalVars map[string]string) (map[string]*ExtractedFileInfo, error) {
	destDir := filepath.Join(m.CacheDir, "extracted", app.Name, version, arch)

	// Check if already extracted and valid in cache
	allCached := true
	for _, reqFile := range app.Artifact.Files {
		targetPath := filepath.Join(destDir, reqFile.Name)
		if _, err := os.Stat(targetPath); err != nil {
			allCached = false
			break
		}
	}
	if allCached {
		return m.loadExtractedFromCache(app.Name, version, arch, app.Artifact.Files)
	}

	// 1. Resolve upstream download URL
	downloadURL, err := ResolveArtifactURL(ctx, app.Artifact, app.Name, version, arch, osName, globalVars, m.HTTPClient)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve artifact URL: %w", err)
	}

	// 2. Download upstream archive
	archiveExt := getArchiveExtension(downloadURL)
	archivesDir := filepath.Join(m.CacheDir, "archives", app.Name, version)
	if err := os.MkdirAll(archivesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create archives dir: %w", err)
	}
	archivePath := filepath.Join(archivesDir, fmt.Sprintf("%s%s", arch, archiveExt))

	// Download archive if not present
	if _, err := os.Stat(archivePath); err != nil {
		if err := m.downloadFile(ctx, downloadURL, archivePath); err != nil {
			return nil, fmt.Errorf("failed to download artifact from %s: %w", downloadURL, err)
		}
	}

	// 3. Verify upstream archive checksum if provided
	if app.Artifact.Checksums != nil {
		chk := app.Artifact.Checksums[arch]
		if chk == "" {
			chk = app.Artifact.Checksums[NormalizeArch(arch, nil)]
		}
		if chk == "" {
			chk = app.Artifact.Checksums[ToGNUArch(arch)]
		}
		if chk == "" {
			chk = app.Artifact.Checksums[ToRustArch(arch)]
		}
		if chk == "" {
			chk = app.Artifact.Checksums[ToDebArch(arch)]
		}
		if chk != "" {
			if err := VerifyChecksum(archivePath, chk); err != nil {
				_ = os.Remove(archivePath)
				return nil, fmt.Errorf("upstream archive checksum verification failed: %w", err)
			}
		}
	}

	// 4. Decompress and extract files
	return DecompressAndExtract(archivePath, app.Artifact.Files, destDir)
}

func (m *ArtifactManager) downloadFile(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Easy42-Server/1.0")

	resp, err := m.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download returned HTTP status %d %s", resp.StatusCode, resp.Status)
	}

	tmpPath := destPath + ".tmp"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, destPath)
}

func getArchiveExtension(urlStr string) string {
	cleanURL := strings.Split(urlStr, "?")[0]
	lower := strings.ToLower(cleanURL)
	if strings.HasSuffix(lower, ".tar.gz") {
		return ".tar.gz"
	}
	if strings.HasSuffix(lower, ".tar.bz2") {
		return ".tar.bz2"
	}
	if strings.HasSuffix(lower, ".tgz") {
		return ".tgz"
	}
	if strings.HasSuffix(lower, ".zip") {
		return ".zip"
	}
	if strings.HasSuffix(lower, ".gz") {
		return ".gz"
	}
	return ".bin"
}
