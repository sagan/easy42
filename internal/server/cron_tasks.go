package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"easy42/internal/config"
	"github.com/go-chi/chi/v5"
)

func (s *Server) handleGetCronTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := s.mgr.GetCronTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleCreateCronTask(w http.ResponseWriter, r *http.Request) {
	var task config.CronTask
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	saved, err := s.mgr.SaveCronTask(task)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, saved)
}

func (s *Server) handleUpdateCronTask(w http.ResponseWriter, r *http.Request) {
	var task config.CronTask
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON body: "+err.Error())
		return
	}

	id := chi.URLParam(r, "id")
	if id != "" {
		task.ID = id
	}

	saved, err := s.mgr.SaveCronTask(task)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleDeleteCronTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	if err := s.mgr.DeleteCronTask(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleToggleCronTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	task, err := s.mgr.ToggleCronTask(id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleGetNodeCronTasksResults(w http.ResponseWriter, r *http.Request) {
	nodeName := chi.URLParam(r, "name")
	if nodeName == "" {
		writeError(w, http.StatusBadRequest, "Node name is required")
		return
	}

	taskID := r.URL.Query().Get("task_id")
	rangeStr := r.URL.Query().Get("range")
	if rangeStr == "" {
		rangeStr = "24h"
	}

	limit := 100
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	resp, err := s.mgr.GetNodeCronTasksResults(nodeName, taskID, rangeStr, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, resp)
}
