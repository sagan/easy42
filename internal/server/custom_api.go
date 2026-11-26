package server

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/internal/engine"

	"github.com/go-chi/chi/v5"
)

// handleExecuteCustomAPI is the public endpoint for invoking a programmable API
func (s *Server) handleExecuteCustomAPI(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if strings.TrimSpace(id) == "" {
		writeError(w, http.StatusBadRequest, "Missing API ID")
		return
	}

	customAPI, err := s.mgr.GetCustomAPI(id)
	if err != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("Custom API %q not found", id))
		return
	}

	if customAPI.Disabled {
		writeError(w, http.StatusForbidden, fmt.Sprintf("Custom API %q is disabled", id))
		return
	}

	// Token authentication
	if token := strings.TrimSpace(customAPI.Token); token != "" {
		reqToken := ""
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			reqToken = strings.TrimSpace(authHeader[7:])
		} else if authHeader != "" {
			reqToken = authHeader
		}

		if reqToken == "" {
			reqToken = r.URL.Query().Get("token")
		}

		if subtle.ConstantTimeCompare([]byte(reqToken), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="easy42-custom-api"`)
			writeError(w, http.StatusUnauthorized, "Unauthorized: invalid or missing bearer token")
			return
		}
	}

	output, err := s.mgr.ExecuteCustomAPI(customAPI, r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Template execution error: %v", err))
		return
	}

	contentType := strings.TrimSpace(customAPI.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}
	w.Header().Set("Content-Type", contentType)

	for k, v := range customAPI.ExtraHeaders {
		w.Header().Set(k, v)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(output))
}

// handleGetCustomAPIs returns the list of all configured custom APIs
func (s *Server) handleGetCustomAPIs(w http.ResponseWriter, r *http.Request) {
	apis := s.mgr.GetCustomAPIs()
	writeJSON(w, http.StatusOK, apis)
}

// handleGetCustomAPI returns a single custom API by ID
func (s *Server) handleGetCustomAPI(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	api, err := s.mgr.GetCustomAPI(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, api)
}

// handleCreateCustomAPI creates a new programmable custom API
func (s *Server) handleCreateCustomAPI(w http.ResponseWriter, r *http.Request) {
	var api config.CustomAPI
	if err := json.NewDecoder(r.Body).Decode(&api); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid API payload")
		return
	}

	created, err := s.mgr.CreateCustomAPI(api)
	if err != nil {
		if err == crypto.ErrVaultLocked {
			writeError(w, http.StatusLocked, "Vault is locked. Unlock with password first.")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

// handleUpdateCustomAPI updates an existing custom API
func (s *Server) handleUpdateCustomAPI(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var api config.CustomAPI
	if err := json.NewDecoder(r.Body).Decode(&api); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid API payload")
		return
	}

	updated, err := s.mgr.UpdateCustomAPI(id, api)
	if err != nil {
		if err == crypto.ErrVaultLocked {
			writeError(w, http.StatusLocked, "Vault is locked. Unlock with password first.")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteCustomAPI deletes a custom API by ID
func (s *Server) handleDeleteCustomAPI(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := s.mgr.DeleteCustomAPI(id)
	if err != nil {
		if err == crypto.ErrVaultLocked {
			writeError(w, http.StatusLocked, "Vault is locked. Unlock with password first.")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": "Custom API deleted successfully"})
}

// TestCustomAPIPayload represents the request body for the test endpoint
type TestCustomAPIPayload struct {
	API     config.CustomAPI                    `json:"api"`
	Request *engine.CustomAPITestRequestPayload `json:"request,omitempty"`
}

// TestCustomAPIResponse represents the response body for the test endpoint
type TestCustomAPIResponse struct {
	Output      string            `json:"output"`
	ContentType string            `json:"content_type"`
	Headers     map[string]string `json:"headers,omitempty"`
}

// handleTestCustomAPI tests rendering an API template without saving
func (s *Server) handleTestCustomAPI(w http.ResponseWriter, r *http.Request) {
	var payload TestCustomAPIPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid test payload")
		return
	}

	output, err := s.mgr.TestCustomAPI(payload.API, payload.Request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	contentType := strings.TrimSpace(payload.API.ContentType)
	if contentType == "" {
		contentType = "application/json"
	}

	headers := make(map[string]string)
	for k, v := range payload.API.ExtraHeaders {
		headers[k] = v
	}

	writeJSON(w, http.StatusOK, TestCustomAPIResponse{
		Output:      output,
		ContentType: contentType,
		Headers:     headers,
	})
}
