package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"easy42/internal/config"
	"github.com/go-chi/chi/v5"
)

// handleGetServerTasks returns all predefined server tasks with settings and state
func (s *Server) handleGetServerTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.mgr.GetServerTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

// handleUpdateServerTask updates the cron schedule and enabled flag for a task
func (s *Server) handleUpdateServerTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !config.IsPredefinedServerTask(id) {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	var req struct {
		CronExpr string `json:"cron_expr"`
		Enabled  bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	req.CronExpr = strings.TrimSpace(req.CronExpr)
	if err := config.ValidateCronExpr(req.CronExpr); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := s.mgr.UpdateServerTask(id, req.CronExpr, req.Enabled)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleRunServerTask triggers immediate execution of a server background task
func (s *Server) handleRunServerTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !config.IsPredefinedServerTask(id) {
		writeError(w, http.StatusNotFound, "Task not found")
		return
	}

	task, runErr := s.mgr.ExecuteServerTask(r.Context(), id)
	if task == nil && runErr != nil {
		writeError(w, http.StatusInternalServerError, runErr.Error())
		return
	}

	// Return updated task state (even if execution encountered an error, so the UI can display it)
	writeJSON(w, http.StatusOK, task)
}
