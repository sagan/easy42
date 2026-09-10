package roa

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestROAManager_LocalFile(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(tmpDir)

	testFilePath := filepath.Join(tmpDir, "local_roa.conf")
	sampleContent := "roa 172.20.0.0/16 max 28 as 4242420000;\n"
	if err := os.WriteFile(testFilePath, []byte(sampleContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	content, err := mgr.GetROAContent(context.Background(), testFilePath, false)
	if err != nil {
		t.Fatalf("GetROAContent failed: %v", err)
	}
	if content != sampleContent {
		t.Errorf("Expected content %q, got %q", sampleContent, content)
	}
}

func TestROAManager_URLFetchAndCache(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(tmpDir)

	fetchCount := 0
	sampleContent := "roa fd00::/8 max 64 as 4242420001;\n"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sampleContent))
	}))
	defer ts.Close()

	// First fetch should hit HTTP server
	content, err := mgr.GetROAContent(context.Background(), ts.URL, false)
	if err != nil {
		t.Fatalf("GetROAContent failed: %v", err)
	}
	if content != sampleContent {
		t.Errorf("Expected content %q, got %q", sampleContent, content)
	}
	if fetchCount != 1 {
		t.Errorf("Expected 1 fetch, got %d", fetchCount)
	}

	// Second fetch should use cache without hitting server
	content2, err := mgr.GetROAContent(context.Background(), ts.URL, false)
	if err != nil {
		t.Fatalf("GetROAContent from cache failed: %v", err)
	}
	if content2 != sampleContent {
		t.Errorf("Expected cached content %q, got %q", sampleContent, content2)
	}
	if fetchCount != 1 {
		t.Errorf("Expected cache hit (fetchCount still 1), got %d", fetchCount)
	}

	// Force refresh should hit server again
	_, err = mgr.GetROAContent(context.Background(), ts.URL, true)
	if err != nil {
		t.Fatalf("GetROAContent with force refresh failed: %v", err)
	}
	if fetchCount != 2 {
		t.Errorf("Expected 2 fetches after force refresh, got %d", fetchCount)
	}

	// Clear cache test
	if err := mgr.ClearCache(); err != nil {
		t.Fatalf("ClearCache failed: %v", err)
	}
}

func TestROAManager_CacheFallbackOnError(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManager(tmpDir)

	statusCode := http.StatusOK
	sampleContent := "roa 172.20.100.0/24 max 24 as 4242421234;\n"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			w.WriteHeader(statusCode)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sampleContent))
	}))
	defer ts.Close()

	// Initial successful fetch
	_, err := mgr.GetROAContent(context.Background(), ts.URL, false)
	if err != nil {
		t.Fatalf("Initial fetch failed: %v", err)
	}

	// Upstream goes down (500 Internal Server Error)
	statusCode = http.StatusInternalServerError

	// Force refresh should fallback to stale cache instead of failing completely
	content, err := mgr.GetROAContent(context.Background(), ts.URL, true)
	if err != nil {
		t.Fatalf("Expected fallback to stale cache, got error: %v", err)
	}
	if !strings.Contains(content, "172.20.100.0/24") {
		t.Errorf("Expected fallback content, got %q", content)
	}
}
