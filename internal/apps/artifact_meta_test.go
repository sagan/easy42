package apps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"easy42/internal/config"
)

func createTestTarGz(t *testing.T, binaryName, content string) ([]byte, string) {
	t.Helper()
	data := []byte(content)
	h := sha256.Sum256(data)
	fileHash := hex.EncodeToString(h[:])

	buf := new(bytes.Buffer)
	gzw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gzw)

	hdr := &tar.Header{
		Name:     binaryName,
		Mode:     0755,
		Size:     int64(len(data)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("failed to write tar body: %v", err)
	}
	_ = tw.Close()
	_ = gzw.Close()

	return buf.Bytes(), fileHash
}

func TestArtifactMetadataPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "artifact-meta-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	mgr := NewArtifactManager(tempDir)

	meta := &AppArtifactMeta{
		AppName:     "testapp",
		LastChecked: time.Now(),
		Versions: map[string]*VersionArtifactMeta{
			"v1.0.0": {
				Version:         "v1.0.0",
				ResolvedVersion: "v1.0.0",
				ResolvedAt:      time.Now(),
				Architectures: map[string]*ArchArtifactMeta{
					"amd64": {
						Arch:        "amd64",
						DownloadURL: "https://example.com/download",
						ArchiveFile: &ArchiveFileInfo{
							FileName:  "test.tar.gz",
							LocalPath: "/path/to/test.tar.gz",
							SHA256:    "abcd1234abcd",
							Size:      1024,
						},
						ExtractedFiles: map[string]*ExtractedFileInfo{
							"testbin": {
								Name:      "testbin",
								LocalPath: "/path/to/testbin",
								SHA256:    "998877665544",
								Size:      512,
							},
						},
						FilesOnDisk: true,
					},
				},
			},
		},
	}

	if err := mgr.SaveAppMeta(meta); err != nil {
		t.Fatalf("SaveAppMeta failed: %v", err)
	}

	loaded, err := mgr.LoadAppMeta("testapp")
	if err != nil {
		t.Fatalf("LoadAppMeta failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil metadata")
	}
	if loaded.AppName != "testapp" {
		t.Errorf("expected AppName testapp, got %s", loaded.AppName)
	}
	vMeta := loaded.Versions["v1.0.0"]
	if vMeta == nil {
		t.Fatal("expected v1.0.0 metadata")
	}
	aMeta := vMeta.Architectures["amd64"]
	if aMeta == nil {
		t.Fatal("expected amd64 metadata")
	}
	if aMeta.ArchiveFile.SHA256 != "abcd1234abcd" {
		t.Errorf("expected archive sha abcd1234abcd, got %s", aMeta.ArchiveFile.SHA256)
	}
	if aMeta.ExtractedFiles["testbin"].SHA256 != "998877665544" {
		t.Errorf("expected extracted sha 998877665544, got %s", aMeta.ExtractedFiles["testbin"].SHA256)
	}
}

func TestArtifactManagerClearCachePreservesMeta(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "artifact-clear-cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	mgr := NewArtifactManager(tempDir)

	archiveBytes, expectedHash := createTestTarGz(t, "mybin", "#!/bin/sh\necho test binary")
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archiveBytes)
	}))
	defer mockServer.Close()

	app := config.App{
		ID:      "app-1",
		Name:    "mytestapp",
		Version: "latest",
		AppType: "systemd",
		Artifact: &config.ArtifactSpec{
			URL: mockServer.URL + "/test-${ARCH}.tar.gz",
			Files: []config.ArtifactFile{
				{
					Name:        "mybin",
					ExtractPath: "mybin",
				},
			},
		},
	}

	// 1. Initial EnsureArtifact: downloads, extracts, writes metadata
	files, err := mgr.EnsureArtifact(context.Background(), app, "amd64", "linux", "", nil)
	if err != nil {
		t.Fatalf("EnsureArtifact failed: %v", err)
	}
	info, ok := files["mybin"]
	if !ok || info == nil {
		t.Fatal("expected mybin extracted")
	}
	if info.SHA256 != expectedHash {
		t.Errorf("expected sha256 %s, got %s", expectedHash, info.SHA256)
	}

	// Verify metadata exists on disk
	meta, err := mgr.LoadAppMeta("mytestapp")
	if err != nil || meta == nil {
		t.Fatalf("metadata missing: %v", err)
	}
	vMeta := meta.Versions["latest"]
	if vMeta == nil || vMeta.Architectures["amd64"] == nil {
		t.Fatal("architecture metadata missing")
	}
	if !vMeta.Architectures["amd64"].FilesOnDisk {
		t.Error("expected FilesOnDisk = true")
	}

	// 2. Clear cache: should delete physical archives and extracted files, but PRESERVE metadata
	clearRes, err := mgr.ClearAppCache("mytestapp")
	if err != nil {
		t.Fatalf("ClearAppCache failed: %v", err)
	}
	if !clearRes.MetaPreserved {
		t.Error("expected MetaPreserved = true")
	}
	if clearRes.FreedBytes <= 0 {
		t.Errorf("expected freed bytes > 0, got %d", clearRes.FreedBytes)
	}

	// Verify physical files are gone
	binaryPath := filepath.Join(tempDir, "extracted", "mytestapp", "latest", "amd64", "mybin")
	if _, err := os.Stat(binaryPath); !os.IsNotExist(err) {
		t.Errorf("expected binary %s to be deleted, but still exists", binaryPath)
	}

	// Verify metadata file still exists!
	metaAfter, err := mgr.LoadAppMeta("mytestapp")
	if err != nil || metaAfter == nil {
		t.Fatal("metadata file was deleted, but it should be preserved!")
	}
	if metaAfter.Versions["latest"].Architectures["amd64"].FilesOnDisk {
		t.Error("expected FilesOnDisk = false after clear cache")
	}

	// 3. EnsureArtifact after cache cleared: should STILL return extracted file info from metadata
	// so node sync continues to work!
	filesAfter, err := mgr.EnsureArtifact(context.Background(), app, "amd64", "linux", "", nil)
	if err != nil {
		t.Fatalf("EnsureArtifact after clear failed: %v", err)
	}
	infoAfter, ok := filesAfter["mybin"]
	if !ok || infoAfter == nil {
		t.Fatal("expected mybin extracted info from metadata")
	}
	if infoAfter.SHA256 != expectedHash {
		t.Errorf("expected sha256 %s from preserved meta, got %s", expectedHash, infoAfter.SHA256)
	}

	// 4. EnsurePhysicalFile: should re-download and extract on demand when node requests it
	if err := mgr.EnsurePhysicalFile(context.Background(), app, "latest", "amd64", "linux", "", nil); err != nil {
		t.Fatalf("EnsurePhysicalFile failed: %v", err)
	}
	if _, err := os.Stat(binaryPath); err != nil {
		t.Errorf("expected binary %s to be restored, err: %v", binaryPath, err)
	}
}

func TestArtifactManagerValidateArtifact(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "artifact-validate-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	mgr := NewArtifactManager(tempDir)

	var currentVersion int32 = 1
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		if atomic.LoadInt32(&currentVersion) == 1 {
			archive, _ := createTestTarGz(t, "tool", "version 1 binary content")
			_, _ = w.Write(archive)
		} else {
			archive, _ := createTestTarGz(t, "tool", "version 2 updated binary content")
			_, _ = w.Write(archive)
		}
	}))
	defer mockServer.Close()

	app := config.App{
		ID:      "app-validate",
		Name:    "mytool",
		Version: "latest",
		AppType: "systemd",
		Artifact: &config.ArtifactSpec{
			URL: mockServer.URL + "/tool-${ARCH}.tar.gz",
			Files: []config.ArtifactFile{
				{
					Name:        "tool",
					ExtractPath: "tool",
				},
			},
		},
	}

	// 1. Initial cache
	res1, err := mgr.EnsureArtifact(context.Background(), app, "amd64", "linux", "", nil)
	if err != nil {
		t.Fatalf("Initial EnsureArtifact failed: %v", err)
	}
	initialHash := res1["tool"].SHA256

	// 2. Validate without changes: should report up to date
	val1, err := mgr.ValidateArtifact(context.Background(), app, "amd64", "linux", "", nil)
	if err != nil {
		t.Fatalf("ValidateArtifact failed: %v", err)
	}
	if val1.Updated {
		t.Errorf("expected Updated = false when unchanged, got true")
	}
	if val1.Status != "up_to_date" {
		t.Errorf("expected status 'up_to_date', got %s", val1.Status)
	}

	// 3. Update upstream file content
	atomic.StoreInt32(&currentVersion, 2)

	// 4. Validate with changes: should detect changed origin file, flush cache, and re-download!
	val2, err := mgr.ValidateArtifact(context.Background(), app, "amd64", "linux", "", nil)
	if err != nil {
		t.Fatalf("ValidateArtifact with change failed: %v", err)
	}
	if !val2.Updated {
		t.Errorf("expected Updated = true after upstream changed")
	}
	if val2.Status != "updated" {
		t.Errorf("expected status 'updated', got %s", val2.Status)
	}

	// Verify the cached extracted binary now has the new hash!
	filesUpdated, err := mgr.EnsureArtifact(context.Background(), app, "amd64", "linux", "", nil)
	if err != nil {
		t.Fatalf("EnsureArtifact after update failed: %v", err)
	}
	newHash := filesUpdated["tool"].SHA256
	if newHash == initialHash {
		t.Error("expected new hash after cache flush and re-download, but got old hash")
	}
}
