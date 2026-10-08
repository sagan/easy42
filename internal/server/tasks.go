package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type taskActionRequest struct {
	Nodes []string `json:"nodes"`
}

func (s *Server) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	tasksList, err := s.mgr.GetTasks()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tasksList)
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
	s.mgr.SetServerURL(fmt.Sprintf("%s://%s", scheme, r.Host))

	results, err := s.mgr.RunTask(r.Context(), taskID, req.Nodes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, results)
}
