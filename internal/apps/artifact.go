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

// NormalizeArch standardizes an architecture string and applies any app-specific arch mappings.
//
// MIPS is intentionally left untouched: `mips` and `mipsel` are distinct
// upstream targets, and a host reporting `mips` may be either endianness. Use
// ResolveMipsEndianness with the endianness reported by the agent to settle it.
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

// NormalizeEndianness maps the various spellings an agent or operator may use
// for CPU byte order onto "little" or "big". It returns "" when unknown.
func NormalizeEndianness(endian string) string {
	switch strings.ToLower(strings.TrimSpace(endian)) {
	case "little", "le", "little-endian", "little_endian", "littleendian", "lsb":
		return "little"
	case "big", "be", "big-endian", "big_endian", "bigendian", "msb":
		return "big"
	default:
		return ""
	}
}

// ResolveMipsEndianness disambiguates a generic "mips"/"mips64" architecture
// using the endianness reported by the node agent. Architectures that already
// encode their byte order (or are not MIPS at all) are returned unchanged.
func ResolveMipsEndianness(arch, endian string) string {
	clean := strings.ToLower(strings.TrimSpace(arch))
	little := NormalizeEndianness(endian) == "little"
	big := NormalizeEndianness(endian) == "big"

	switch clean {
	case "mips":
		if little {
			return "mipsel"
		}
		return clean
	case "mips64":
		if little {
			return "mips64le"
		}
		return clean
	case "mipsel", "mipsle":
		if big {
			return "mips"
		}
		return "mipsel"
	case "mips64le", "mips64el":
		if big {
			return "mips64"
		}
		return "mips64le"
	default:
		return clean
	}
}

// NodeArchFromOSInfo extracts the machine architecture from an agent-reported
// os_info string. The expected form is "<os> <arch> [<endian>]" (e.g.
// "linux mips little"), but legacy agents may report a bare architecture.
func NodeArchFromOSInfo(osInfo string) string {
	parts := strings.Fields(osInfo)
	switch {
	case len(parts) >= 2:
		return parts[1]
	case len(parts) == 1 && !strings.Contains(strings.ToLower(parts[0]), "linux"):
		return parts[0]
	default:
		return ""
	}
}

// NodeEndianFromOSInfo extracts the optional endianness field from an
// agent-reported os_info string. It returns "" for legacy agents that do not
// report one.
func NodeEndianFromOSInfo(osInfo string) string {
	parts := strings.Fields(osInfo)
	if len(parts) >= 3 {
		return NormalizeEndianness(parts[2])
	}
	return ""
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
// The endian parameter ("little"/"big", may be empty) disambiguates MIPS targets, where a host
// reporting "mips" may be either byte order.
// Returns a map of extracted file name -> ExtractedFileInfo.
func (m *ArtifactManager) EnsureArtifact(ctx context.Context, app config.App, nodeArch, osName, endian string, globalVars map[string]string) (map[string]*ExtractedFileInfo, error) {
	if app.Artifact == nil {
		return nil, nil
	}

	version := app.Version
	if version == "" {
		version = "latest"
	}

	arch := NormalizeArch(nodeArch, app.Artifact.ArchMapping)
	arch = ResolveMipsEndianness(arch, endian)
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

	files, err := m.doEnsureArtifact(ctx, app, version, arch, osName, endian, globalVars)
	m.mu.Lock()
	m.inFlightErr[cacheKey] = err
	m.mu.Unlock()

	return files, err
}

func (m *ArtifactManager) loadExtractedFromCache(appName, version, arch string, files []config.ArtifactFile) (map[string]*ExtractedFileInfo, error) {
	// 1. Try reading from persistent metadata first
	if meta, _ := m.LoadAppMeta(appName); meta != nil {
		if vMeta, ok := meta.Versions[version]; ok && vMeta != nil {
			if aMeta, ok := vMeta.Architectures[arch]; ok && aMeta != nil && aMeta.ExtractedFiles != nil {
				allFound := true
				res := make(map[string]*ExtractedFileInfo)
				for _, f := range files {
					if info, exists := aMeta.ExtractedFiles[f.Name]; exists && info != nil {
						res[f.Name] = &ExtractedFileInfo{
							Name:      info.Name,
							LocalPath: info.LocalPath,
							SHA256:    info.SHA256,
							Size:      info.Size,
						}
					} else {
						allFound = false
						break
					}
				}
				if allFound {
					return res, nil
				}
			}
		}
	}

	// 2. Fallback: inspect disk files and compute sha256
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

	// Record to metadata so subsequent loads avoid re-hashing
	_ = m.recordExtractedMeta(appName, version, arch, "", nil, res, true)

	return res, nil
}

func (m *ArtifactManager) doEnsureArtifact(ctx context.Context, app config.App, version, arch, osName, endian string, globalVars map[string]string) (map[string]*ExtractedFileInfo, error) {
	destDir := filepath.Join(m.CacheDir, "extracted", app.Name, version, arch)

	// Check persistent metadata
	meta, _ := m.LoadAppMeta(app.Name)
	if meta != nil && meta.Versions != nil {
		if vMeta, ok := meta.Versions[version]; ok && vMeta != nil {
			if aMeta, ok := vMeta.Architectures[arch]; ok && aMeta != nil && aMeta.ExtractedFiles != nil {
				allMetaPresent := true
				for _, reqFile := range app.Artifact.Files {
					if _, exists := aMeta.ExtractedFiles[reqFile.Name]; !exists {
						allMetaPresent = false
						break
					}
				}
				if allMetaPresent {
					// Check physical files on disk
					allExistOnDisk := true
					for _, reqFile := range app.Artifact.Files {
						targetPath := filepath.Join(destDir, reqFile.Name)
						if _, err := os.Stat(targetPath); err != nil {
							allExistOnDisk = false
							break
						}
					}

					// Update files_on_disk status if it changed
					if aMeta.FilesOnDisk != allExistOnDisk {
						aMeta.FilesOnDisk = allExistOnDisk
						_ = m.SaveAppMeta(meta)
					}

					// Whether on disk or purged to free space, return metadata info
					// so node sync can evaluate sha256. If a node needs to download,
					// EnsurePhysicalFile will be triggered on demand.
					res := make(map[string]*ExtractedFileInfo)
					for k, v := range aMeta.ExtractedFiles {
						res[k] = &ExtractedFileInfo{
							Name:      v.Name,
							LocalPath: v.LocalPath,
							SHA256:    v.SHA256,
							Size:      v.Size,
						}
					}
					return res, nil
				}
			}
		}
	}

	// Legacy cache check: check if already extracted on disk but without metadata
	allCachedOnDisk := true
	for _, reqFile := range app.Artifact.Files {
		targetPath := filepath.Join(destDir, reqFile.Name)
		if _, err := os.Stat(targetPath); err != nil {
			allCachedOnDisk = false
			break
		}
	}
	if allCachedOnDisk {
		return m.loadExtractedFromCache(app.Name, version, arch, app.Artifact.Files)
	}

	// Not cached yet: download, verify, extract, and record metadata
	return m.downloadAndExtract(ctx, app, version, arch, osName, endian, globalVars)
}

func (m *ArtifactManager) downloadAndExtract(ctx context.Context, app config.App, version, arch, osName, endian string, globalVars map[string]string) (map[string]*ExtractedFileInfo, error) {
	destDir := filepath.Join(m.CacheDir, "extracted", app.Name, version, arch)

	// 1. Resolve upstream artifact URL and version
	resolved, err := ResolveArtifact(ctx, app.Artifact, app.Name, version, arch, osName, endian, globalVars, m.HTTPClient)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve artifact: %w", err)
	}

	// 2. Download upstream archive
	archiveExt := getArchiveExtension(resolved.DownloadURL)
	archivesDir := filepath.Join(m.CacheDir, "archives", app.Name, version)
	if err := os.MkdirAll(archivesDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create archives dir: %w", err)
	}
	archivePath := filepath.Join(archivesDir, fmt.Sprintf("%s%s", arch, archiveExt))

	var arcInfo *ArchiveFileInfo
	if _, err := os.Stat(archivePath); err != nil {
		dlInfo, err := m.downloadFile(ctx, resolved.DownloadURL, archivePath)
		if err != nil {
			return nil, fmt.Errorf("failed to download artifact from %s: %w", resolved.DownloadURL, err)
		}
		arcInfo = dlInfo
	} else {
		hash, size, err := ComputeFileSHA256(archivePath)
		if err == nil {
			arcInfo = &ArchiveFileInfo{
				FileName:     filepath.Base(archivePath),
				LocalPath:    archivePath,
				SHA256:       hash,
				Size:         size,
				DownloadedAt: time.Now(),
			}
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
	extracted, err := DecompressAndExtract(archivePath, app.Artifact.Files, destDir)
	if err != nil {
		return nil, err
	}

	// 5. Record metadata to persistent file
	_ = m.recordFullMeta(app.Name, version, arch, resolved.ResolvedVersion, resolved.DownloadURL, arcInfo, extracted, true)

	return extracted, nil
}

func (m *ArtifactManager) recordExtractedMeta(appName, version, arch string, downloadURL string, arcInfo *ArchiveFileInfo, extracted map[string]*ExtractedFileInfo, onDisk bool) error {
	return m.recordFullMeta(appName, version, arch, "", downloadURL, arcInfo, extracted, onDisk)
}

func (m *ArtifactManager) recordFullMeta(appName, version, arch, resolvedVersion, downloadURL string, arcInfo *ArchiveFileInfo, extracted map[string]*ExtractedFileInfo, onDisk bool) error {
	meta, _ := m.LoadAppMeta(appName)
	if meta == nil {
		meta = &AppArtifactMeta{
			AppName:  appName,
			Versions: make(map[string]*VersionArtifactMeta),
		}
	}
	meta.LastChecked = time.Now()

	if meta.Versions == nil {
		meta.Versions = make(map[string]*VersionArtifactMeta)
	}

	vMeta, exists := meta.Versions[version]
	if !exists || vMeta == nil {
		vMeta = &VersionArtifactMeta{
			Version:       version,
			Architectures: make(map[string]*ArchArtifactMeta),
		}
		meta.Versions[version] = vMeta
	}

	if resolvedVersion != "" {
		vMeta.ResolvedVersion = resolvedVersion
		vMeta.ResolvedAt = time.Now()
	}

	if vMeta.Architectures == nil {
		vMeta.Architectures = make(map[string]*ArchArtifactMeta)
	}

	aMeta := vMeta.Architectures[arch]
	if aMeta == nil {
		aMeta = &ArchArtifactMeta{
			Arch:          arch,
			DownloadURL:   downloadURL,
			CachedAt:      time.Now(),
			LastValidated: time.Now(),
		}
		vMeta.Architectures[arch] = aMeta
	}

	if downloadURL != "" {
		aMeta.DownloadURL = downloadURL
	}
	if arcInfo != nil {
		aMeta.ArchiveFile = arcInfo
	}
	aMeta.ExtractedFiles = extracted
	aMeta.FilesOnDisk = onDisk
	aMeta.LastValidated = time.Now()

	return m.SaveAppMeta(meta)
}

// EnsurePhysicalFile ensures that the physical extracted files and archive for an app exist on disk.
// If they were purged by the user to free space, it re-downloads and re-extracts on demand.
func (m *ArtifactManager) EnsurePhysicalFile(ctx context.Context, app config.App, version, arch, osName, endian string, globalVars map[string]string) error {
	if app.Artifact == nil {
		return nil
	}
	if version == "" {
		version = "latest"
	}
	arch = NormalizeArch(arch, app.Artifact.ArchMapping)
	arch = ResolveMipsEndianness(arch, endian)
	if osName == "" {
		osName = "linux"
	}

	destDir := filepath.Join(m.CacheDir, "extracted", app.Name, version, arch)
	allExist := true
	for _, reqFile := range app.Artifact.Files {
		targetPath := filepath.Join(destDir, reqFile.Name)
		if _, err := os.Stat(targetPath); err != nil {
			allExist = false
			break
		}
	}
	if allExist {
		return nil
	}

	// Re-download and extract
	_, err := m.downloadAndExtract(ctx, app, version, arch, osName, endian, globalVars)
	return err
}

// ValidateArtifact checks if the origin source for the app artifact has updated.
// If "latest" resolves to a different version/tag, or origin file has a different sha256/content,
// it flushes the server cache and re-downloads.
func (m *ArtifactManager) ValidateArtifact(ctx context.Context, app config.App, nodeArch, osName, endian string, globalVars map[string]string) (*ArtifactValidationResult, error) {
	if app.Artifact == nil {
		return nil, fmt.Errorf("app '%s' has no artifact spec configured", app.Name)
	}

	version := app.Version
	if version == "" {
		version = "latest"
	}
	arch := NormalizeArch(nodeArch, app.Artifact.ArchMapping)
	arch = ResolveMipsEndianness(arch, endian)
	if osName == "" {
		osName = "linux"
	}

	// 1. Resolve upstream artifact
	resolved, err := ResolveArtifact(ctx, app.Artifact, app.Name, version, arch, osName, endian, globalVars, m.HTTPClient)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve origin artifact: %w", err)
	}

	meta, _ := m.LoadAppMeta(app.Name)
	var archMeta *ArchArtifactMeta
	var vMeta *VersionArtifactMeta
	if meta != nil && meta.Versions != nil {
		vMeta = meta.Versions[version]
		if vMeta != nil && vMeta.Architectures != nil {
			archMeta = vMeta.Architectures[arch]
		}
	}

	// If not currently cached or metadata missing, download and initialize cache
	if archMeta == nil || archMeta.ArchiveFile == nil || len(archMeta.ExtractedFiles) == 0 {
		m.flushArchFiles(app.Name, version, arch)
		_, err := m.downloadAndExtract(ctx, app, version, arch, osName, endian, globalVars)
		if err != nil {
			return nil, fmt.Errorf("failed to cache artifact: %w", err)
		}
		return &ArtifactValidationResult{
			AppName:         app.Name,
			Version:         version,
			Arch:            arch,
			Status:          "re_downloaded",
			Updated:         true,
			ResolvedVersion: resolved.ResolvedVersion,
			Message:         fmt.Sprintf("Artifact successfully downloaded and cached (version: %s)", resolved.ResolvedVersion),
		}, nil
	}

	// Check A: Has upstream version tag changed? (e.g. "latest" now resolves to a newer tag)
	if resolved.ResolvedVersion != "" && vMeta.ResolvedVersion != "" && resolved.ResolvedVersion != vMeta.ResolvedVersion {
		oldVer := vMeta.ResolvedVersion
		m.flushArchFiles(app.Name, version, arch)
		_, err := m.downloadAndExtract(ctx, app, version, arch, osName, endian, globalVars)
		if err != nil {
			return nil, fmt.Errorf("failed to re-download updated artifact: %w", err)
		}
		return &ArtifactValidationResult{
			AppName:         app.Name,
			Version:         version,
			Arch:            arch,
			Status:          "updated",
			Updated:         true,
			PreviousVersion: oldVer,
			ResolvedVersion: resolved.ResolvedVersion,
			Message:         fmt.Sprintf("Upstream version updated: %s (previous: %s). Cache flushed and re-downloaded.", resolved.ResolvedVersion, oldVer),
		}, nil
	}

	// Check B: Has the download URL changed?
	if resolved.DownloadURL != archMeta.DownloadURL {
		m.flushArchFiles(app.Name, version, arch)
		_, err := m.downloadAndExtract(ctx, app, version, arch, osName, endian, globalVars)
		if err != nil {
			return nil, fmt.Errorf("failed to download from updated URL: %w", err)
		}
		return &ArtifactValidationResult{
			AppName:         app.Name,
			Version:         version,
			Arch:            arch,
			Status:          "updated",
			Updated:         true,
			ResolvedVersion: resolved.ResolvedVersion,
			Message:         "Origin artifact download URL changed. Cache flushed and re-downloaded.",
		}, nil
	}

	// Check C: Download to temp file and verify if origin binary content changed
	archiveExt := getArchiveExtension(resolved.DownloadURL)
	archivesDir := filepath.Join(m.CacheDir, "archives", app.Name, version)
	_ = os.MkdirAll(archivesDir, 0755)
	archivePath := filepath.Join(archivesDir, fmt.Sprintf("%s%s", arch, archiveExt))
	tempValidatePath := archivePath + ".validate.tmp"

	dlInfo, err := m.downloadFile(ctx, resolved.DownloadURL, tempValidatePath)
	if err != nil {
		_ = os.Remove(tempValidatePath)
		return nil, fmt.Errorf("failed to check origin file at %s: %w", resolved.DownloadURL, err)
	}

	// If sha256 of newly downloaded archive matches cached archive:
	if dlInfo.SHA256 == archMeta.ArchiveFile.SHA256 {
		_ = os.Remove(tempValidatePath)
		// Ensure physical file is present on disk
		destDir := filepath.Join(m.CacheDir, "extracted", app.Name, version, arch)
		filesOnDisk := true
		for _, reqFile := range app.Artifact.Files {
			if _, err := os.Stat(filepath.Join(destDir, reqFile.Name)); err != nil {
				filesOnDisk = false
				break
			}
		}
		archMeta.FilesOnDisk = filesOnDisk
		archMeta.LastValidated = time.Now()
		meta.LastChecked = time.Now()
		_ = m.SaveAppMeta(meta)

		return &ArtifactValidationResult{
			AppName:         app.Name,
			Version:         version,
			Arch:            arch,
			Status:          "up_to_date",
			Updated:         false,
			ResolvedVersion: resolved.ResolvedVersion,
			Message:         fmt.Sprintf("Artifact is up to date (SHA-256 verified: %s...)", dlInfo.SHA256[:12]),
		}, nil
	}

	// Checksum differs: Origin source has an updated file!
	m.flushArchFiles(app.Name, version, arch)
	_ = os.MkdirAll(archivesDir, 0755)
	if err := os.Rename(tempValidatePath, archivePath); err != nil {
		_ = os.Remove(tempValidatePath)
		return nil, fmt.Errorf("failed to replace archive: %w", err)
	}

	destDir := filepath.Join(m.CacheDir, "extracted", app.Name, version, arch)
	extracted, err := DecompressAndExtract(archivePath, app.Artifact.Files, destDir)
	if err != nil {
		return nil, fmt.Errorf("failed to extract updated artifact: %w", err)
	}

	_ = m.recordFullMeta(app.Name, version, arch, resolved.ResolvedVersion, resolved.DownloadURL, dlInfo, extracted, true)

	return &ArtifactValidationResult{
		AppName:         app.Name,
		Version:         version,
		Arch:            arch,
		Status:          "updated",
		Updated:         true,
		PreviousVersion: vMeta.ResolvedVersion,
		ResolvedVersion: resolved.ResolvedVersion,
		Message:         "Origin file content updated. Cache flushed and re-downloaded.",
	}, nil
}

// ClearAppCache deletes cached physical archive and extracted files for an app to free disk space,
// while keeping the metadata file intact so node sync can continue seamlessly.
func (m *ArtifactManager) ClearAppCache(appName string) (*ArtifactClearCacheResult, error) {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		return nil, fmt.Errorf("app name cannot be empty")
	}

	freedBytes := m.CalculateAppDiskUsage(appName)

	// Remove physical archive and extracted directories
	archivesDir := filepath.Join(m.CacheDir, "archives", appName)
	extractedDir := filepath.Join(m.CacheDir, "extracted", appName)

	_ = os.RemoveAll(archivesDir)
	_ = os.RemoveAll(extractedDir)

	// Update metadata: mark files_on_disk = false, but preserve the metadata file
	if meta, _ := m.LoadAppMeta(appName); meta != nil {
		for _, vMeta := range meta.Versions {
			if vMeta != nil {
				for _, aMeta := range vMeta.Architectures {
					if aMeta != nil {
						aMeta.FilesOnDisk = false
					}
				}
			}
		}
		_ = m.SaveAppMeta(meta)
	}

	return &ArtifactClearCacheResult{
		AppName:       appName,
		FreedBytes:    freedBytes,
		FreedHuman:    FormatBytes(freedBytes),
		MetaPreserved: true,
		Message:       fmt.Sprintf("Deleted server cached artifact files (%s freed). Metadata preserved for node sync.", FormatBytes(freedBytes)),
	}, nil
}

// ClearAllArtifactCaches deletes physical artifact files across all apps while preserving metadata.
func (m *ArtifactManager) ClearAllArtifactCaches() (*ArtifactClearCacheResult, error) {
	var totalFreed int64

	archivesBase := filepath.Join(m.CacheDir, "archives")
	extractedBase := filepath.Join(m.CacheDir, "extracted")

	_ = filepath.Walk(archivesBase, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			totalFreed += info.Size()
		}
		return nil
	})
	_ = filepath.Walk(extractedBase, func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			totalFreed += info.Size()
		}
		return nil
	})

	_ = os.RemoveAll(archivesBase)
	_ = os.RemoveAll(extractedBase)

	// Mark files_on_disk = false in all metadata files
	metaDir := m.getMetaDir()
	entries, _ := os.ReadDir(metaDir)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			appName := strings.TrimSuffix(entry.Name(), ".json")
			if meta, _ := m.LoadAppMeta(appName); meta != nil {
				for _, vMeta := range meta.Versions {
					if vMeta != nil {
						for _, aMeta := range vMeta.Architectures {
							if aMeta != nil {
								aMeta.FilesOnDisk = false
							}
						}
					}
				}
				_ = m.SaveAppMeta(meta)
			}
		}
	}

	return &ArtifactClearCacheResult{
		AppName:       "all",
		FreedBytes:    totalFreed,
		FreedHuman:    FormatBytes(totalFreed),
		MetaPreserved: true,
		Message:       fmt.Sprintf("Cleared all cached artifact files (%s freed). All metadata preserved.", FormatBytes(totalFreed)),
	}, nil
}

// GetAppCacheStatus returns the current cache and metadata status for an app
func (m *ArtifactManager) GetAppCacheStatus(app config.App) (*AppArtifactStatus, error) {
	status := &AppArtifactStatus{
		AppID:         app.ID,
		AppName:       app.Name,
		Version:       app.Version,
		HasArtifact:   app.Artifact != nil,
		DiskSizeBytes: m.CalculateAppDiskUsage(app.Name),
		Versions:      make(map[string]*VersionStatus),
	}
	status.DiskSizeHuman = FormatBytes(status.DiskSizeBytes)

	meta, _ := m.LoadAppMeta(app.Name)
	if meta != nil {
		status.MetaExists = true
		status.LastChecked = &meta.LastChecked
		anyFilesOnDisk := false

		for vKey, vMeta := range meta.Versions {
			vStatus := &VersionStatus{
				Version:         vMeta.Version,
				ResolvedVersion: vMeta.ResolvedVersion,
				Architectures:   make(map[string]*ArchStatus),
			}
			for aKey, aMeta := range vMeta.Architectures {
				var arcSize int64
				var arcSha string
				if aMeta.ArchiveFile != nil {
					arcSize = aMeta.ArchiveFile.Size
					arcSha = aMeta.ArchiveFile.SHA256
				}
				var extList []ExtractedFileInfo
				for _, ext := range aMeta.ExtractedFiles {
					if ext != nil {
						extList = append(extList, *ext)
					}
				}
				if aMeta.FilesOnDisk {
					anyFilesOnDisk = true
				}
				if status.LastValidated == nil || aMeta.LastValidated.After(*status.LastValidated) {
					t := aMeta.LastValidated
					status.LastValidated = &t
				}
				vStatus.Architectures[aKey] = &ArchStatus{
					Arch:           aMeta.Arch,
					DownloadURL:    aMeta.DownloadURL,
					ArchiveSize:    arcSize,
					ArchiveSha256:  arcSha,
					FilesOnDisk:    aMeta.FilesOnDisk,
					ExtractedFiles: extList,
					LastValidated:  aMeta.LastValidated,
				}
			}
			status.Versions[vKey] = vStatus
		}
		status.FilesOnDisk = anyFilesOnDisk
	} else {
		status.FilesOnDisk = status.DiskSizeBytes > 0
	}

	return status, nil
}

func (m *ArtifactManager) flushArchFiles(appName, version, arch string) {
	_ = os.RemoveAll(filepath.Join(m.CacheDir, "extracted", appName, version, arch))
	// Remove matching archive file
	archivesDir := filepath.Join(m.CacheDir, "archives", appName, version)
	entries, _ := os.ReadDir(archivesDir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".tmp") && (strings.HasPrefix(e.Name(), arch+".") || e.Name() == arch) {
			_ = os.Remove(filepath.Join(archivesDir, e.Name()))
		}
	}
}

func (m *ArtifactManager) downloadFile(ctx context.Context, url, destPath string) (*ArchiveFileInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Easy42-Server/1.0")

	resp, err := m.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download returned HTTP status %d %s", resp.StatusCode, resp.Status)
	}

	tmpPath := destPath + ".tmp"
	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}

	hasher := sha256.New()
	mw := io.MultiWriter(out, hasher)
	size, err := io.Copy(mw, resp.Body)
	out.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	return &ArchiveFileInfo{
		FileName:     filepath.Base(destPath),
		LocalPath:    destPath,
		SHA256:       hash,
		Size:         size,
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
		DownloadedAt: time.Now(),
	}, nil
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
