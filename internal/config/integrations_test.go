package config

import (
	"os"
	"strings"
	"testing"
)

func TestGeneratePrometheusToken(t *testing.T) {
	tok1 := GeneratePrometheusToken()
	tok2 := GeneratePrometheusToken()

	if !strings.HasPrefix(tok1, "e42_prom_") {
		t.Fatalf("expected token prefix e42_prom_, got %s", tok1)
	}
	if !strings.HasPrefix(tok2, "e42_prom_") {
		t.Fatalf("expected token prefix e42_prom_, got %s", tok2)
	}
	if tok1 == tok2 {
		t.Fatalf("tokens must be unique, got duplicate: %s", tok1)
	}
	if len(tok1) < 20 {
		t.Fatalf("token is too short: %s", tok1)
	}
}

func TestStoreIntegrationsPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "e42-store-integrations-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)
	_, err = store.Initialize()
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}

	cfg := store.Get()
	if cfg == nil {
		t.Fatal("config is nil")
	}
	if !cfg.Integrations.PrometheusEnabled {
		t.Errorf("expected PrometheusEnabled to be true by default")
	}
	if !strings.HasPrefix(cfg.Integrations.PrometheusAuthToken, "e42_prom_") {
		t.Errorf("expected token prefix e42_prom_, got %s", cfg.Integrations.PrometheusAuthToken)
	}

	// Reload from disk
	store2 := NewStore(tmpDir)
	cfg2, err := store2.Load()
	if err != nil {
		t.Fatalf("failed to reload store: %v", err)
	}
	if cfg2.Integrations.PrometheusAuthToken != cfg.Integrations.PrometheusAuthToken {
		t.Errorf("token changed across reload: %s vs %s", cfg2.Integrations.PrometheusAuthToken, cfg.Integrations.PrometheusAuthToken)
	}
}
