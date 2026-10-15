package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"easy42/internal/config"
)

func TestResolveScope(t *testing.T) {
	tests := []struct {
		name        string
		scope       string
		isTerminal  bool
		expected    string
		expectError bool
	}{
		{
			name:       "auto in pty terminal resolves to full",
			scope:      "auto",
			isTerminal: true,
			expected:   "full",
		},
		{
			name:       "auto in non-pty terminal resolves to public",
			scope:      "auto",
			isTerminal: false,
			expected:   "public",
		},
		{
			name:       "public in pty terminal resolves to public",
			scope:      "public",
			isTerminal: true,
			expected:   "public",
		},
		{
			name:       "public in non-pty terminal resolves to public",
			scope:      "public",
			isTerminal: false,
			expected:   "public",
		},
		{
			name:       "full in pty terminal resolves to full",
			scope:      "full",
			isTerminal: true,
			expected:   "full",
		},
		{
			name:       "full in non-pty terminal resolves to full",
			scope:      "full",
			isTerminal: false,
			expected:   "full",
		},
		{
			name:       "auto case insensitive with spaces",
			scope:      "  AuTo  ",
			isTerminal: true,
			expected:   "full",
		},
		{
			name:       "public case insensitive",
			scope:      "PUBLIC",
			isTerminal: false,
			expected:   "public",
		},
		{
			name:        "invalid scope returns error",
			scope:       "unknown",
			isTerminal:  true,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := resolveScope(tt.scope, tt.isTerminal)
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error for scope %q, got nil", tt.scope)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if actual != tt.expected {
				t.Fatalf("expected scope %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestSyncFlags(t *testing.T) {
	flag := syncCmd.Flags().Lookup("scope")
	if flag == nil {
		t.Fatal("expected 'scope' flag to be registered on syncCmd")
	}
	if flag.Shorthand != "s" {
		t.Fatalf("expected shorthand 's', got %q", flag.Shorthand)
	}
	if flag.DefValue != "auto" {
		t.Fatalf("expected default value 'auto', got %q", flag.DefValue)
	}

	usage := syncCmd.UsageString()
	if !bytes.Contains([]byte(usage), []byte("-s, --scope")) {
		t.Fatalf("expected '-s, --scope' in usage string, got:\n%s", usage)
	}
}

func TestSyncScopePublicDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := dataDir
	dataDir = tmpDir
	defer func() { dataDir = origDir }()

	store := config.NewStore(tmpDir)
	if _, err := store.Initialize(); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	// Reset flags
	syncScope = "public"
	dryRun = true
	syncPassword = ""
	syncNode = ""
	os.Unsetenv("EASY42_PASSWORD")

	// Execute RunE directly with public scope; it should NOT prompt for password or fail auth
	err := syncCmd.RunE(syncCmd, []string{})
	if err != nil {
		t.Fatalf("expected public dry-run to succeed without password, got error: %v", err)
	}
}

func TestSyncScopeInvalidScope(t *testing.T) {
	syncScope = "invalid-scope"
	err := syncCmd.RunE(syncCmd, []string{})
	if err == nil {
		t.Fatal("expected error with invalid scope, got nil")
	}
}

func TestSyncDelegationToRunningServer(t *testing.T) {
	tmpDir := t.TempDir()
	origDir := dataDir
	dataDir = tmpDir
	defer func() { dataDir = origDir }()

	store := config.NewStore(tmpDir)
	if _, err := store.Initialize(); err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	syncCalled := false
	serverCalledScope := ""

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/status":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"has_config":    true,
				"authenticated": true,
				"unlocked":      false,
			})
		case "/api/sync":
			syncCalled = true
			serverCalledScope = r.URL.Query().Get("scope")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]config.SyncResult{
				{
					NodeName: "berohost",
					Action:   "Deploy ROA6 table",
					Success:  true,
					Duration: 5,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Write server.url in tmpDir
	_ = os.WriteFile(filepath.Join(tmpDir, "server.url"), []byte(ts.URL), 0644)

	syncScope = "public"
	dryRun = false
	syncPassword = ""
	syncNode = ""

	err := syncCmd.RunE(syncCmd, []string{})
	if err != nil {
		t.Fatalf("expected sync to succeed via running server, got error: %v", err)
	}

	if !syncCalled {
		t.Fatal("expected /api/sync on running server to be called, but was not")
	}
	if serverCalledScope != "public" {
		t.Fatalf("expected scope 'public' to be passed to server, got %q", serverCalledScope)
	}
}
