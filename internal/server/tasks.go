package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"easy42/tasks"

	"github.com/go-chi/chi/v5"
)

type taskActionRequest struct {
	Nodes []string `json:"nodes"`
}

type installOnlineTaskRequest struct {
	Source string   `json:"source"`
	TaskID string   `json:"task_id"`
	Files  []string `json:"files"`
}

func (s *Server) handleGetOnlineSources(w http.ResponseWriter, r *http.Request) {
	sources := s.mgr.GetOnlineSources()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"default_source": tasks.DefaultOnlineSource,
		"sources":        sources,
	})
}

func (s *Server) handleGetOnlineManifest(w http.ResponseWriter, r *http.Request) {
	sourceURL := r.URL.Query().Get("source")
	manifest, err := s.mgr.GetOnlineManifest(r.Context(), sourceURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("Failed to fetch online manifest: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, manifest)
}

func (s *Server) handleInstallOnlineTask(w http.ResponseWriter, r *http.Request) {
	var req installOnlineTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	if err := s.mgr.InstallOnlineTask(r.Context(), req.Source, req.TaskID, req.Files); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to install task: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Task '%s' installed successfully", req.TaskID),
		"task_id": req.TaskID,
	})
}

func (s *Server) handleDeleteCustomTask(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	if err := s.mgr.DeleteCustomTask(r.Context(), taskID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Custom task '%s' deleted successfully", taskID),
		"task_id": taskID,
	})
}

func (s *Server) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	tasksList, err := s.mgr.GetTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tasksList)
}

func (s *Server) handleGetTaskSources(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	sources, err := s.mgr.GetTaskSources(taskID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sources)
}

func (s *Server) handleTaskStatus(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	var req taskActionRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	results, err := s.mgr.CheckTaskStatus(r.Context(), taskID, req.Nodes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, results)
}

func (s *Server) handleTaskRun(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "id")
	if taskID == "" {
		writeError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	var req taskActionRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	scheme := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "http"
	}
	if isSafeHost(r.Host) {
		s.mgr.SetServerURL(fmt.Sprintf("%s://%s", scheme, r.Host))
	}

	results, err := s.mgr.RunTask(r.Context(), taskID, req.Nodes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, results)
}
