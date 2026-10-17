package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"easy42/internal/config"
	"github.com/go-chi/chi/v5"
)

// handleGetConfigApplySchema returns the official strict JSON schema for config change objects
func (s *Server) handleGetConfigApplySchema(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(config.ConfigChangeJSONSchema))
}

type validateApplyRequest struct {
	Content string `json:"content"`
}

type validateApplyResponse struct {
	Valid  bool   `json:"valid"`
	Format string `json:"format,omitempty"`
	Error  string `json:"error,omitempty"`
}

// handleValidateConfigChange strictly validates snippet syntax, fields, and schema
func (s *Server) handleValidateConfigChange(w http.ResponseWriter, r *http.Request) {
	var req validateApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	_, format, err := config.ParseAndValidateConfigChange([]byte(req.Content))
	if err != nil {
		writeJSON(w, http.StatusOK, validateApplyResponse{
			Valid: false,
			Error: err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, validateApplyResponse{
		Valid:  true,
		Format: format,
	})
}

type previewApplyRequest struct {
	Content string `json:"content"`
}

// handlePreviewConfigApply parses and simulates applying the changes, returning diffs
func (s *Server) handlePreviewConfigApply(w http.ResponseWriter, r *http.Request) {
	var req previewApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	preview, err := s.mgr.PreviewApply(req.Content)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":   err.Error(),
			"valid":   false,
			"preview": preview,
		})
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

type executeApplyRequest struct {
	Content  string `json:"content"`
	AutoSync bool   `json:"auto_sync,omitempty"`
}

// handleExecuteConfigApply applies the patch atomically and optionally triggers sync
func (s *Server) handleExecuteConfigApply(w http.ResponseWriter, r *http.Request) {
	var req executeApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	result, err := s.mgr.ApplyConfigChange(req.Content)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.AutoSync && len(result.AffectedNodes) > 0 {
		go func(nodes []string) {
			_, _ = s.mgr.ExecuteSyncNodesWithScope(false, "auto", nodes...)
		}(result.AffectedNodes)
	}

	writeJSON(w, http.StatusOK, result)
}

// handleListConfigChanges returns all saved config change snippets
func (s *Server) handleListConfigChanges(w http.ResponseWriter, r *http.Request) {
	changes := s.mgr.ListConfigChanges()
	writeJSON(w, http.StatusOK, changes)
}

// handleGetConfigChange retrieves a saved config change snippet by ID
func (s *Server) handleGetConfigChange(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cc, err := s.mgr.GetConfigChange(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cc)
}

// handleSaveConfigChange creates or updates a saved config change snippet
func (s *Server) handleSaveConfigChange(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var cc config.ConfigChange
	if err := json.NewDecoder(r.Body).Decode(&cc); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if id != "" {
		cc.ID = id
	}
	cc.Name = strings.TrimSpace(cc.Name)
	if cc.Name == "" {
		writeError(w, http.StatusBadRequest, "Snippet name is required")
		return
	}

	saved, err := s.mgr.SaveConfigChange(cc)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, saved)
}

// handleDeleteConfigChange deletes a saved config change snippet
func (s *Server) handleDeleteConfigChange(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.mgr.DeleteConfigChange(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleApplySavedConfigChange applies a saved config change snippet by ID
func (s *Server) handleApplySavedConfigChange(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req struct {
		AutoSync bool `json:"auto_sync,omitempty"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	result, err := s.mgr.ApplySavedConfigChange(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if req.AutoSync && len(result.AffectedNodes) > 0 {
		go func(nodes []string) {
			_, _ = s.mgr.ExecuteSyncNodesWithScope(false, "auto", nodes...)
		}(result.AffectedNodes)
	}

	writeJSON(w, http.StatusOK, result)
}
