package apps

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"easy42/internal/config"
)

func TestDecompressAndExtractZip(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.zip")

	// Create a zip archive containing nested file
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	content := []byte("#!/bin/sh\necho rclone")
	w, err := zw.Create("rclone-v1.68.1-linux-amd64/rclone")
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("failed to write zip content: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}
	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write archive file: %v", err)
	}

	destDir := filepath.Join(tmpDir, "extracted")
	files := []config.ArtifactFile{
		{
			Name:        "rclone",
			ExtractPath: "rclone-*-linux-*/rclone",
			TargetPath:  "${PATH}/rclone",
		},
	}

	res, err := DecompressAndExtract(archivePath, files, destDir)
	if err != nil {
		t.Fatalf("DecompressAndExtract failed: %v", err)
	}

	info, ok := res["rclone"]
	if !ok || info == nil {
		t.Fatalf("expected rclone in result, got: %+v", res)
	}

	expectedHash := sha256Hex(content)
	if info.SHA256 != expectedHash {
		t.Errorf("SHA256 mismatch: expected %s, got %s", expectedHash, info.SHA256)
	}

	extractedContent, err := os.ReadFile(filepath.Join(destDir, "rclone"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(extractedContent) != string(content) {
		t.Errorf("content mismatch: got %q, expected %q", string(extractedContent), string(content))
	}
}

func TestDecompressAndExtractTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")

	buf := new(bytes.Buffer)
	gzw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gzw)
	content := []byte("binary payload for vproxy")
	hdr := &tar.Header{
		Name:     "vproxy-linux-amd64/vproxy",
		Mode:     0755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("failed to write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("failed to close gzip: %v", err)
	}
	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write archive: %v", err)
	}

	destDir := filepath.Join(tmpDir, "extracted")
	files := []config.ArtifactFile{
		{
			Name:        "vproxy",
			ExtractPath: "vproxy", // Match basename
			TargetPath:  "${PATH}/vproxy",
		},
	}

	res, err := DecompressAndExtract(archivePath, files, destDir)
	if err != nil {
		t.Fatalf("DecompressAndExtract tar.gz failed: %v", err)
	}

	info, ok := res["vproxy"]
	if !ok || info == nil {
		t.Fatalf("expected vproxy in result")
	}
	expectedHash := sha256Hex(content)
	if info.SHA256 != expectedHash {
		t.Errorf("SHA256 mismatch: expected %s, got %s", expectedHash, info.SHA256)
	}
}

func TestResolveArtifactURLTemplate(t *testing.T) {
	spec := &config.ArtifactSpec{
		URL: "https://downloads.rclone.org/${VERSION}/rclone-${VERSION}-linux-${ARCH}.zip",
	}

	res, err := ResolveArtifactURL(context.Background(), spec, "rclone", "v1.68.1", "amd64", "linux", "", nil, http.DefaultClient)
	if err != nil {
		t.Fatalf("ResolveArtifactURL failed: %v", err)
	}

	expected := "https://downloads.rclone.org/v1.68.1/rclone-v1.68.1-linux-amd64.zip"
	if res != expected {
		t.Errorf("expected %s, got %s", expected, res)
	}
}

func TestArchConversions(t *testing.T) {
	// Test ToGNUArch
	if got := ToGNUArch("amd64"); got != "x86_64" {
		t.Errorf("ToGNUArch(amd64) = %s, want x86_64", got)
	}
	if got := ToGNUArch("arm64"); got != "aarch64" {
		t.Errorf("ToGNUArch(arm64) = %s, want aarch64", got)
	}
	if got := ToGNUArch("armv7"); got != "armv7l" {
		t.Errorf("ToGNUArch(armv7) = %s, want armv7l", got)
	}
	if got := ToGNUArch("386"); got != "i386" {
		t.Errorf("ToGNUArch(386) = %s, want i386", got)
	}
	if got := ToGNUArch("x86_64"); got != "x86_64" {
		t.Errorf("ToGNUArch(x86_64) = %s, want x86_64", got)
	}

	// Test ToRustArch
	if got := ToRustArch("amd64"); got != "x86_64" {
		t.Errorf("ToRustArch(amd64) = %s, want x86_64", got)
	}
	if got := ToRustArch("arm64"); got != "aarch64" {
		t.Errorf("ToRustArch(arm64) = %s, want aarch64", got)
	}
	if got := ToRustArch("armv7"); got != "armv7" {
		t.Errorf("ToRustArch(armv7) = %s, want armv7", got)
	}

	// Test ToDebArch
	if got := ToDebArch("amd64"); got != "amd64" {
		t.Errorf("ToDebArch(amd64) = %s, want amd64", got)
	}
	if got := ToDebArch("armv7"); got != "armhf" {
		t.Errorf("ToDebArch(armv7) = %s, want armhf", got)
	}
}

func TestResolveArtifactURLTemplateWithARCH_GNU(t *testing.T) {
	spec := &config.ArtifactSpec{
		URL: "https://github.com/fastfetch-cli/fastfetch/releases/download/${VERSION}/fastfetch-linux-${ARCH_GNU}.tar.gz",
	}

	res, err := ResolveArtifactURL(context.Background(), spec, "fastfetch", "2.28.0", "amd64", "linux", "", nil, http.DefaultClient)
	if err != nil {
		t.Fatalf("ResolveArtifactURL failed: %v", err)
	}

	expected := "https://github.com/fastfetch-cli/fastfetch/releases/download/2.28.0/fastfetch-linux-x86_64.tar.gz"
	if res != expected {
		t.Errorf("expected %s, got %s", expected, res)
	}

	// Test arm64
	resArm, err := ResolveArtifactURL(context.Background(), spec, "fastfetch", "2.28.0", "arm64", "linux", "", nil, http.DefaultClient)
	if err != nil {
		t.Fatalf("ResolveArtifactURL failed: %v", err)
	}
	expectedArm := "https://github.com/fastfetch-cli/fastfetch/releases/download/2.28.0/fastfetch-linux-aarch64.tar.gz"
	if resArm != expectedArm {
		t.Errorf("expected %s, got %s", expectedArm, resArm)
	}
}

func TestResolveArtifactURLGitHub(t *testing.T) {
	// Mock GitHub API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `{
			"tag_name": "v1.68.1",
			"assets": [
				{
					"name": "rclone-v1.68.1-linux-amd64.zip",
					"browser_download_url": "https://example.com/rclone-linux-amd64.zip",
					"size": 12345
				},
				{
					"name": "rclone-v1.68.1-linux-arm64.zip",
					"browser_download_url": "https://example.com/rclone-linux-arm64.zip",
					"size": 12345
				}
			]
		}`)
	}))
	defer mockServer.Close()

	// Direct test of GitHubRelease parsing and asset pattern matching
	rel := GitHubRelease{
		TagName: "v1.68.1",
		Assets: []GitHubReleaseAsset{
			{
				Name:               "rclone-v1.68.1-linux-amd64.zip",
				BrowserDownloadURL: "https://example.com/rclone-linux-amd64.zip",
				Size:               12345,
			},
			{
				Name:               "rclone-v1.68.1-linux-arm64.zip",
				BrowserDownloadURL: "https://example.com/rclone-linux-arm64.zip",
				Size:               12345,
			},
		},
	}

	pattern := "rclone-*-linux-amd64.zip"
	matched := ""
	for _, a := range rel.Assets {
		if m, _ := filepath.Match(pattern, a.Name); m {
			matched = a.BrowserDownloadURL
			break
		}
	}
	if matched != "https://example.com/rclone-linux-amd64.zip" {
		t.Fatalf("asset matching failed, got: %s", matched)
	}
}

func TestArtifactTokenManager(t *testing.T) {
	mgr := NewArtifactTokenManager()
	token := mgr.GenerateToken("/var/lib/easy42/cache/test.bin", "test.bin", "hash123", "node-1", 1*time.Hour)
	if token == "" {
		t.Fatalf("token cannot be empty")
	}

	info, ok := mgr.ValidateToken(token)
	if !ok || info == nil {
		t.Fatalf("failed to validate valid token")
	}
	if info.FileName != "test.bin" || info.NodeName != "node-1" || info.SHA256 != "hash123" {
		t.Errorf("token info mismatch: %+v", info)
	}

	// Non-existent token
	_, ok = mgr.ValidateToken("invalid-token")
	if ok {
		t.Errorf("expected invalid token to fail")
	}

	// Expired token
	expToken := mgr.GenerateToken("/var/lib/test", "test", "hash", "node-1", -1*time.Minute)
	_, ok = mgr.ValidateToken(expToken)
	if ok {
		t.Errorf("expected expired token to fail")
	}
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func TestNormalizeEndianness(t *testing.T) {
	cases := map[string]string{
		"little":        "little",
		"LE":            "little",
		"little-endian": "little",
		"big":           "big",
		"be":            "big",
		"big-endian":    "big",
		"":              "",
		"unknown-value": "",
	}
	for in, want := range cases {
		if got := NormalizeEndianness(in); got != want {
			t.Errorf("NormalizeEndianness(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveMipsEndianness(t *testing.T) {
	cases := []struct{ arch, endian, want string }{
		// Kernel reports a bare "mips" on both endiannesses (OpenWrt MT7621).
		{"mips", "little", "mipsel"},
		{"mips", "big", "mips"},
		{"mips", "", "mips"},
		{"mips64", "little", "mips64le"},
		{"mips64", "big", "mips64"},
		// Already-disambiguated names trust the reported byte order.
		{"mipsel", "big", "mips"},
		{"mipsel", "little", "mipsel"},
		{"mips64le", "big", "mips64"},
		// Non-MIPS architectures pass through untouched.
		{"amd64", "little", "amd64"},
		{"arm64", "big", "arm64"},
	}
	for _, c := range cases {
		if got := ResolveMipsEndianness(c.arch, c.endian); got != c.want {
			t.Errorf("ResolveMipsEndianness(%q, %q) = %q, want %q", c.arch, c.endian, got, c.want)
		}
	}
}

func TestNodeArchAndEndianFromOSInfo(t *testing.T) {
	cases := []struct {
		osInfo     string
		wantArch   string
		wantEndian string
	}{
		{"linux mips little", "mips", "little"},
		{"linux mips big", "mips", "big"},
		{"linux x86_64", "x86_64", ""},
		{"linux aarch64", "aarch64", ""},
		// Legacy agents report a bare architecture.
		{"mipsel", "mipsel", ""},
		{"mips", "mips", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := NodeArchFromOSInfo(c.osInfo); got != c.wantArch {
			t.Errorf("NodeArchFromOSInfo(%q) = %q, want %q", c.osInfo, got, c.wantArch)
		}
		if got := NodeEndianFromOSInfo(c.osInfo); got != c.wantEndian {
			t.Errorf("NodeEndianFromOSInfo(%q) = %q, want %q", c.osInfo, got, c.wantEndian)
		}
	}
}

// TestResolveArtifactURLMipsRustTriple reproduces the wg-watcher report: an
// MT7621 router reports "mips" (little-endian), and the upstream asset is named
// after the Rust target triple "mipsel-unknown-linux-musl".
func TestResolveArtifactURLMipsRustTriple(t *testing.T) {
	spec := &config.ArtifactSpec{
		URL: "https://github.com/sagan/wg-watcher/releases/download/${VERSION}/wg-watcher-${ARCH_RUST}-unknown-linux-musl",
	}

	cases := []struct {
		arch, endian, want string
	}{
		{"mips", "little", "https://github.com/sagan/wg-watcher/releases/download/v0.1.6/wg-watcher-mipsel-unknown-linux-musl"},
		{"mips", "big", "https://github.com/sagan/wg-watcher/releases/download/v0.1.6/wg-watcher-mips-unknown-linux-musl"},
		{"amd64", "", "https://github.com/sagan/wg-watcher/releases/download/v0.1.6/wg-watcher-x86_64-unknown-linux-musl"},
		{"arm64", "", "https://github.com/sagan/wg-watcher/releases/download/v0.1.6/wg-watcher-aarch64-unknown-linux-musl"},
	}
	for _, c := range cases {
		got, err := ResolveArtifactURL(context.Background(), spec, "wg-watcher", "v0.1.6", c.arch, "linux", c.endian, nil, http.DefaultClient)
		if err != nil {
			t.Fatalf("ResolveArtifactURL(%s/%s) failed: %v", c.arch, c.endian, err)
		}
		if got != c.want {
			t.Errorf("ResolveArtifactURL(%s/%s) = %q, want %q", c.arch, c.endian, got, c.want)
		}
	}
}

func TestAssetMatchesArch(t *testing.T) {
	cases := []struct {
		asset    string
		variants []string
		want     bool
	}{
		{"wg-watcher-mipsel-unknown-linux-musl", []string{"mips", "mipsel"}, true},
		// "mips" must not match inside "mipsel".
		{"wg-watcher-mipsel-unknown-linux-musl", []string{"mips"}, false},
		{"wg-watcher-mips-unknown-linux-musl", []string{"mips"}, true},
		{"wg-watcher-mips-unknown-linux-musl", []string{"mipsel"}, false},
		// "arm" must not match inside "arm64".
		{"tool-linux-arm64.tar.gz", []string{"arm"}, false},
		{"tool-linux-arm64.tar.gz", []string{"aarch64", "arm64"}, true},
		// "arm" is only a token prefix of "armv7l", which is a legitimate match.
		{"tool-linux-armv7l.tar.gz", []string{"arm"}, false},
		{"tool-linux-armv7l.tar.gz", []string{"armv7l"}, true},
	}
	for _, c := range cases {
		if got := assetMatchesArch(c.asset, c.variants); got != c.want {
			t.Errorf("assetMatchesArch(%q, %v) = %v, want %v", c.asset, c.variants, got, c.want)
		}
	}
}

func TestResolveGitHubReleaseAutoSelectsMipsEndianness(t *testing.T) {
	// Mirrors the wg-watcher release: only little-endian MIPS is published.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"tag_name": "v0.1.6",
			"assets": [
				{"name": "wg-watcher-aarch64-unknown-linux-musl", "browser_download_url": "https://example.com/aarch64", "size": 1},
				{"name": "wg-watcher-mipsel-unknown-linux-musl", "browser_download_url": "https://example.com/mipsel", "size": 1},
				{"name": "wg-watcher-x86_64-unknown-linux-musl", "browser_download_url": "https://example.com/x86_64", "size": 1}
			]
		}`)
	}))
	defer mockServer.Close()

	origBase := githubAPIBase
	githubAPIBase = mockServer.URL
	defer func() { githubAPIBase = origBase }()

	spec := &config.ArtifactSpec{
		Resolver: &config.ArtifactResolver{
			Type:    "github_release",
			Repo:    "sagan/wg-watcher",
			Version: "latest",
		},
	}

	// Little-endian MIPS host: automatic selection must pick the mipsel asset.
	got, err := ResolveArtifactURL(context.Background(), spec, "wg-watcher", "latest", "mips", "linux", "little", nil, mockServer.Client())
	if err != nil {
		t.Fatalf("automatic selection failed: %v", err)
	}
	if got != "https://example.com/mipsel" {
		t.Errorf("got %q, want https://example.com/mipsel", got)
	}

	// Big-endian MIPS host: no matching asset, and it must not fall back to mipsel.
	if _, err := ResolveArtifactURL(context.Background(), spec, "wg-watcher", "latest", "mips", "linux", "big", nil, mockServer.Client()); err == nil {
		t.Errorf("expected an error for big-endian MIPS with no matching asset")
	}
}
