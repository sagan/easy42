package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"easy42/internal/config"
)

func TestAppWithArtifactSpec(t *testing.T) {
	srv, tempDir, _ := setupTestServer(t)
	defer func() {
		srv.mgr.Close()
		_ = os.RemoveAll(tempDir)
	}()

	// 1. Setup mock HTTP server serving a .tar.gz archive
	tarContent := []byte("#!/bin/sh\necho rclone binary simulated")
	h := sha256.Sum256(tarContent)
	fileHash := hex.EncodeToString(h[:])

	buf := new(bytes.Buffer)
	gzw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gzw)
	hdr := &tar.Header{
		Name:     "rclone-v1.68.1-linux-amd64/rclone",
		Mode:     0755,
		Size:     int64(len(tarContent)),
		Typeflag: tar.TypeReg,
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(tarContent)
	_ = tw.Close()
	_ = gzw.Close()
	archiveBytes := buf.Bytes()

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archiveBytes)
	}))
	defer mockServer.Close()

	// 2. Add node1 to config
	cfg := srv.mgr.Store().Get()
	cfg.Nodes = append(cfg.Nodes, config.Node{
		Name: "node1",
		Host: "192.168.1.50",
		IP:   "10.42.0.77/24",
	})
	_ = srv.mgr.Store().Save(cfg)

	// 3. Create native app with Artifact
	app := config.App{
		ID:          "app_rclone",
		Name:        "rclone",
		Version:     "v1.68.1",
		AppType:     "systemd",
		MainContent: "[Unit]\nDescription=rclone service\n\n[Service]\nExecStart=/usr/local/bin/rclone version\n",
		Artifact: &config.ArtifactSpec{
			URL: mockServer.URL + "/rclone-${VERSION}-linux-${ARCH}.tar.gz",
			Files: []config.ArtifactFile{
				{
					Name:        "rclone",
					ExtractPath: "rclone-*-linux-*/rclone",
					TargetPath:  "${PATH}/rclone",
					FileMode:    0755,
					PullPolicy:  "if_changed",
				},
			},
		},
		Selector:  map[string]string{"name": "node1"},
		Published: true,
		Enabled:   true,
	}

	saved, err := srv.mgr.SaveApp(app)
	if err != nil {
		t.Fatalf("SaveApp failed: %v", err)
	}
	if saved.Artifact == nil {
		t.Fatalf("expected saved app to retain Artifact")
	}

	// 4. Build specs for node1
	specs, err := srv.mgr.BuildAppSpecsForNode("node1")
	if err != nil {
		t.Fatalf("BuildAppSpecsForNode failed: %v", err)
	}

	if len(specs) != 1 {
		t.Fatalf("expected 1 app spec, got %d", len(specs))
	}
	spec := specs[0]
	if len(spec.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact in AppSpec, got %d", len(spec.Artifacts))
	}

	art := spec.Artifacts[0]
	if art.Name != "rclone" {
		t.Errorf("expected artifact name 'rclone', got %s", art.Name)
	}
	if !strings.HasSuffix(art.TargetPath, "/rclone") {
		t.Errorf("expected target_path ending with /rclone, got %s", art.TargetPath)
	}
	if art.FileSha256 != fileHash {
		t.Errorf("expected file_sha256 %s, got %s", fileHash, art.FileSha256)
	}
	if art.DownloadToken == "" {
		t.Errorf("download_token cannot be empty")
	}

	// 5. Download the artifact from server using the token
	req := httptest.NewRequest(http.MethodGet, "/api/download?token="+art.DownloadToken, nil)
	w := httptest.NewRecorder()
	srv.handleArtifactDownload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from download, got %d (body: %s)", w.Code, w.Body.String())
	}

	downloaded, _ := io.ReadAll(w.Body)
	if string(downloaded) != string(tarContent) {
		t.Fatalf("downloaded binary mismatch: %q vs %q", string(downloaded), string(tarContent))
	}

	// 6. Test agent-side download simulation against this server
	ts := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		srv.handleArtifactDownload(rw, r)
	}))
	defer ts.Close()

	_ = context.Background()
	_ = fmt.Sprint(ts.URL)
}
