package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// fetchFAMetrics issues an HTTP GET to the FlapAlerted metrics endpoint and returns compacted JSON.
func fetchFAMetrics(ctx context.Context, client *http.Client, metricsURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var compactBuf bytes.Buffer
	if err := json.Compact(&compactBuf, body); err == nil {
		return compactBuf.String(), nil
	}
	return strings.TrimSpace(string(body)), nil
}

// resolveFAMetricsURL constructs the FlapAlerted /flaps/metrics/json endpoint from base URL
func resolveFAMetricsURL(faBaseURL string) string {
	baseURL := strings.TrimRight(faBaseURL, "/")
	if strings.HasSuffix(baseURL, "/flaps/metrics/json") {
		return baseURL
	}
	return baseURL + "/flaps/metrics/json"
}

// handleFlapAlertedMetrics handles one-shot JSON metrics or delegates to stream if requested
func (s *Server) handleFlapAlertedMetrics(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		s.handleFlapAlertedMetricsStream(w, r)
		return
	}

	cfg := s.mgr.Store().Get()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "Config store unavailable")
		return
	}

	faBaseURL := cfg.NetworkSettings.GetFlapAlertedURL()
	if faBaseURL == "" {
		writeError(w, http.StatusBadRequest, "FlapAlerted is not configured")
		return
	}

	metricsURL := resolveFAMetricsURL(faBaseURL)
	httpClient := &http.Client{
		Timeout: 3 * time.Second,
	}

	data, err := fetchFAMetrics(r.Context(), httpClient, metricsURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("FlapAlerted is inaccessible: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(data))
}

// handleFlapAlertedMetricsStream exposes a Server-Sent Events (SSE) text/event-stream API for FlapAlerted live metrics
func (s *Server) handleFlapAlertedMetricsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming unsupported")
		return
	}

	cfg := s.mgr.Store().Get()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "Config store unavailable")
		return
	}

	faBaseURL := cfg.NetworkSettings.GetFlapAlertedURL()
	if faBaseURL == "" {
		writeError(w, http.StatusBadRequest, "FlapAlerted is not configured")
		return
	}

	metricsURL := resolveFAMetricsURL(faBaseURL)
	httpClient := &http.Client{
		Timeout: 3 * time.Second,
	}

	// Initial probe to verify FlapAlerted accessibility
	initialData, err := fetchFAMetrics(r.Context(), httpClient, metricsURL)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("FlapAlerted is inaccessible: %v", err))
		return
	}

	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial metrics event
	if _, err := fmt.Fprintf(w, "data: %s\n\n", initialData); err != nil {
		return
	}
	flusher.Flush()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			data, err := fetchFAMetrics(r.Context(), httpClient, metricsURL)
			if err != nil {
				// Temporary failure during streaming, keep retry loop alive
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
