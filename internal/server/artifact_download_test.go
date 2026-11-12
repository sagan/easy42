package server

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArtifactDownloadEndpoint(t *testing.T) {
	srv, tempDir, _ := setupTestServer(t)
	defer func() {
		srv.mgr.Close()
		_ = os.RemoveAll(tempDir)
	}()

	// Create a dummy binary file in tempDir
	testBinPath := filepath.Join(tempDir, "rclone-bin")
	content := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	if err := os.WriteFile(testBinPath, content, 0755); err != nil {
		t.Fatalf("failed to write test bin: %v", err)
	}

	h := sha256.Sum256(content)
	contentHash := hex.EncodeToString(h[:])

	tokenMgr := srv.mgr.GetArtifactTokenManager()
	token := tokenMgr.GenerateToken(testBinPath, "rclone", contentHash, "node-1", 1*time.Hour)

	// 1. Missing token -> 401
	req := httptest.NewRequest(http.MethodGet, "/api/download", nil)
	w := httptest.NewRecorder()
	srv.handleArtifactDownload(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got: %d", w.Code)
	}

	// 2. Invalid token -> 404
	req = httptest.NewRequest(http.MethodGet, "/api/download?token=invalid_token", nil)
	w = httptest.NewRecorder()
	srv.handleArtifactDownload(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid token, got: %d", w.Code)
	}

	// 3. Valid download -> 200 OK
	req = httptest.NewRequest(http.MethodGet, "/api/download?token="+token, nil)
	w = httptest.NewRecorder()
	srv.handleArtifactDownload(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got: %d", w.Code)
	}
	body, _ := io.ReadAll(w.Body)
	if string(body) != string(content) {
		t.Fatalf("downloaded content mismatch: %q vs %q", string(body), string(content))
	}
	if w.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("expected Accept-Ranges: bytes, got: %s", w.Header().Get("Accept-Ranges"))
	}

	// 4. Resumable Range request -> 206 Partial Content
	req = httptest.NewRequest(http.MethodGet, "/api/download?token="+token, nil)
	req.Header.Set("Range", "bytes=10-")
	w = httptest.NewRecorder()
	srv.handleArtifactDownload(w, req)
	if w.Code != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got: %d", w.Code)
	}
	rangeBody, _ := io.ReadAll(w.Body)
	expectedRange := content[10:]
	if string(rangeBody) != string(expectedRange) {
		t.Fatalf("range body mismatch: %q vs %q", string(rangeBody), string(expectedRange))
	}
}
