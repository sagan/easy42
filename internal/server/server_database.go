package server

import (
	"encoding/json"
	"net/http"

	"easy42/internal/config"
)

// handleGetDatabaseInfo returns current sqlite database metrics and retention settings
func (s *Server) handleGetDatabaseInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.mgr.GetDatabaseInfo()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleUpdateDatabaseSettings updates database retention days configuration
func (s *Server) handleUpdateDatabaseSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RetentionDays    int `json:"retention_days"`
		RawRetentionDays int `json:"raw_retention_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if req.RetentionDays <= 0 {
		req.RetentionDays = config.DefaultRetentionDays
	}
	if req.RetentionDays > 3650 {
		writeError(w, http.StatusBadRequest, "Retention days must be between 1 and 3650")
		return
	}
	if req.RawRetentionDays <= 0 {
		req.RawRetentionDays = config.DefaultRawRetentionDays
	}
	if req.RawRetentionDays > 30 {
		writeError(w, http.StatusBadRequest, "Raw retention days must be between 1 and 30")
		return
	}
	if req.RawRetentionDays > req.RetentionDays {
		req.RawRetentionDays = req.RetentionDays
	}

	info, err := s.mgr.UpdateDatabaseSettings(config.DatabaseSettings{
		RetentionDays:    req.RetentionDays,
		RawRetentionDays: req.RawRetentionDays,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// handleVacuumDatabase triggers SQLite VACUUM command to reclaim unused space
func (s *Server) handleVacuumDatabase(w http.ResponseWriter, r *http.Request) {
	info, err := s.mgr.VacuumDatabase()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}
