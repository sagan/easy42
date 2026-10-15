package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"easy42/internal/config"
	"easy42/internal/engine"
)

func TestDatabaseSettingsAPI(t *testing.T) {
	tempDir := t.TempDir()
	store := config.NewStore(tempDir)
	_, err := store.Initialize()
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	mgr := engine.NewManager(store)
	defer mgr.Close()
	stateStore := mgr.StateStore()

	srv := New(Config{
		Manager: mgr,
	})

	// 1. Record dummy metric to populate database
	if err := stateStore.RecordMetrics("test-node", config.NodeMetricInput{
		Timestamp:  time.Now(),
		CPUPercent: 12.5,
	}); err != nil {
		t.Fatalf("failed to record metrics: %v", err)
	}

	// 2. Test GET /api/settings/database
	req := httptest.NewRequest("GET", "/api/settings/database", nil)
	w := httptest.NewRecorder()
	srv.handleGetDatabaseInfo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var info engine.DatabaseInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("failed to parse database info: %v", err)
	}

	if info.RetentionDays != config.DefaultRetentionDays {
		t.Errorf("expected default retention days %d, got %d", config.DefaultRetentionDays, info.RetentionDays)
	}
	if info.TotalSizeBytes <= 0 {
		t.Errorf("expected positive total size bytes, got %d", info.TotalSizeBytes)
	}
	if info.TotalSizeHuman == "" {
		t.Errorf("expected non-empty total size human string")
	}
	if info.MetricsCount < 1 {
		t.Errorf("expected at least 1 metrics record, got %d", info.MetricsCount)
	}

	// 3. Test PUT /api/settings/database
	payload := []byte(`{"retention_days": 60}`)
	putReq := httptest.NewRequest("PUT", "/api/settings/database", bytes.NewReader(payload))
	putReq.Header.Set("Content-Type", "application/json")
	putW := httptest.NewRecorder()
	srv.handleUpdateDatabaseSettings(putW, putReq)

	if putW.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", putW.Code, putW.Body.String())
	}

	var updated engine.DatabaseInfo
	if err := json.Unmarshal(putW.Body.Bytes(), &updated); err != nil {
		t.Fatalf("failed to parse updated info: %v", err)
	}
	if updated.RetentionDays != 60 {
		t.Errorf("expected retention days 60, got %d", updated.RetentionDays)
	}

	// 4. Test POST /api/settings/database/vacuum
	vacReq := httptest.NewRequest("POST", "/api/settings/database/vacuum", nil)
	vacW := httptest.NewRecorder()
	srv.handleVacuumDatabase(vacW, vacReq)

	if vacW.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", vacW.Code, vacW.Body.String())
	}

	var vacInfo engine.DatabaseInfo
	if err := json.Unmarshal(vacW.Body.Bytes(), &vacInfo); err != nil {
		t.Fatalf("failed to parse vacuumed info: %v", err)
	}
	if vacInfo.TotalSizeBytes <= 0 {
		t.Errorf("expected valid size after vacuum, got %d", vacInfo.TotalSizeBytes)
	}
}
