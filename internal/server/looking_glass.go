package server

import (
	"encoding/json"
	"net/http"

	"easy42/internal/config"
	"easy42/internal/lookingglass"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetLookingGlassTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.mgr.GetLookingGlassTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleSaveLookingGlassTask(w http.ResponseWriter, r *http.Request) {
	var task config.LookingGlassTask
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	urlID := chi.URLParam(r, "id")
	if urlID != "" {
		task.ID = urlID
	}

	saved, err := s.mgr.SaveLookingGlassTask(task)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteLookingGlassTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	if err := s.mgr.DeleteLookingGlassTask(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleRunLookingGlass(w http.ResponseWriter, r *http.Request) {
	var req lookingglass.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	resp, err := s.mgr.RunLookingGlass(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
