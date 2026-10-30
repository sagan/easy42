package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type runPingTableRequest struct {
	LatencyType string `json:"latency_type"` // "e2e" or "link"
}

func (s *Server) handleRunPingTable(w http.ResponseWriter, r *http.Request) {
	var req runPingTableRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	task, err := s.mgr.RunPingTable(req.LatencyType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleCancelPingTable(w http.ResponseWriter, r *http.Request) {
	if err := s.mgr.CancelPingTable(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
}

func (s *Server) handleGetCurrentPingTable(w http.ResponseWriter, r *http.Request) {
	task := s.mgr.GetActivePingTable()
	if task == nil {
		// Try to get latest from history
		hist, err := s.mgr.GetPingTableHistory(1)
		if err == nil && len(hist) > 0 {
			task, _ = s.mgr.GetPingTableTask(hist[0].ID)
		}
	}

	if task == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}

	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleGetPingTableHistory(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	history, err := s.mgr.GetPingTableHistory(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, history)
}

func (s *Server) handleGetPingTableTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	task, err := s.mgr.GetPingTableTask(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if task == nil {
		writeError(w, http.StatusNotFound, "Ping table task not found")
		return
	}

	writeJSON(w, http.StatusOK, task)
}

func (s *Server) handleDeletePingTableTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "Task ID is required")
		return
	}

	if err := s.mgr.DeletePingTableTask(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleStreamPingTable(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := s.mgr.SubscribePingTable()
	defer unsub()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case task, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(task)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
