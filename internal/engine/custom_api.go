package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/util/tplutil"

	"github.com/go-chi/chi/v5"
)

// CustomAPITestRequestPayload defines simulated request properties when testing an API
type CustomAPITestRequestPayload struct {
	Method  string            `json:"method,omitempty"`
	Path    string            `json:"path,omitempty"`
	Query   map[string]string `json:"query,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// GetCustomAPIs returns all programmable custom APIs
func (m *Manager) GetCustomAPIs() []config.CustomAPI {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.Get()
	if cfg == nil || len(cfg.CustomAPIs) == 0 {
		return []config.CustomAPI{}
	}
	result := make([]config.CustomAPI, len(cfg.CustomAPIs))
	copy(result, cfg.CustomAPIs)
	return result
}

// GetCustomAPI returns a single custom API by ID
func (m *Manager) GetCustomAPI(id string) (*config.CustomAPI, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.Get()
	if cfg == nil {
		return nil, config.ErrConfigNotFound
	}
	api := cfg.FindCustomAPI(id)
	if api == nil {
		return nil, fmt.Errorf("custom API %q not found", id)
	}
	apiCopy := *api
	return &apiCopy, nil
}

// CreateCustomAPI creates a new programmable custom API
func (m *Manager) CreateCustomAPI(api config.CustomAPI) (*config.CustomAPI, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return nil, crypto.ErrVaultLocked
	}

	api.ID = strings.TrimSpace(api.ID)
	api.Name = strings.TrimSpace(api.Name)
	api.Description = strings.TrimSpace(api.Description)
	api.ContentType = strings.TrimSpace(api.ContentType)
	api.Token = strings.TrimSpace(api.Token)

	if err := config.ValidateCustomAPI(&api); err != nil {
		return nil, err
	}

	// Verify template compiles
	if _, err := tplutil.GetTemplate(api.Template, false); err != nil {
		return nil, fmt.Errorf("invalid template syntax: %w", err)
	}

	cfg := m.store.Get()
	if cfg == nil {
		return nil, config.ErrConfigNotFound
	}

	for _, existing := range cfg.CustomAPIs {
		if strings.EqualFold(existing.ID, api.ID) {
			return nil, fmt.Errorf("API with ID %q already exists", api.ID)
		}
	}

	now := time.Now()
	api.CreatedAt = now
	api.ModifiedAt = now

	cfg.CustomAPIs = append(cfg.CustomAPIs, api)
	if err := m.store.Save(cfg); err != nil {
		return nil, fmt.Errorf("failed to save custom API: %w", err)
	}

	return &api, nil
}

// UpdateCustomAPI updates an existing custom API
func (m *Manager) UpdateCustomAPI(id string, api config.CustomAPI) (*config.CustomAPI, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return nil, crypto.ErrVaultLocked
	}

	targetID := strings.TrimSpace(id)
	newID := strings.TrimSpace(api.ID)
	if newID == "" {
		newID = targetID
	}
	api.ID = newID
	api.Name = strings.TrimSpace(api.Name)
	api.Description = strings.TrimSpace(api.Description)
	api.ContentType = strings.TrimSpace(api.ContentType)
	api.Token = strings.TrimSpace(api.Token)

	if err := config.ValidateCustomAPI(&api); err != nil {
		return nil, err
	}

	// Verify template compiles
	if _, err := tplutil.GetTemplate(api.Template, false); err != nil {
		return nil, fmt.Errorf("invalid template syntax: %w", err)
	}

	cfg := m.store.Get()
	if cfg == nil {
		return nil, config.ErrConfigNotFound
	}

	foundIdx := -1
	for i, existing := range cfg.CustomAPIs {
		if existing.ID == targetID {
			foundIdx = i
		} else if strings.EqualFold(existing.ID, newID) {
			return nil, fmt.Errorf("another API with ID %q already exists", newID)
		}
	}

	if foundIdx == -1 {
		return nil, fmt.Errorf("custom API %q not found", targetID)
	}

	api.CreatedAt = cfg.CustomAPIs[foundIdx].CreatedAt
	api.ModifiedAt = time.Now()

	cfg.CustomAPIs[foundIdx] = api
	if err := m.store.Save(cfg); err != nil {
		return nil, fmt.Errorf("failed to save custom API: %w", err)
	}

	return &api, nil
}

// DeleteCustomAPI deletes a custom API by ID
func (m *Manager) DeleteCustomAPI(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return crypto.ErrVaultLocked
	}

	targetID := strings.TrimSpace(id)
	cfg := m.store.Get()
	if cfg == nil {
		return config.ErrConfigNotFound
	}

	foundIdx := -1
	for i, existing := range cfg.CustomAPIs {
		if existing.ID == targetID {
			foundIdx = i
			break
		}
	}

	if foundIdx == -1 {
		return fmt.Errorf("custom API %q not found", targetID)
	}

	cfg.CustomAPIs = append(cfg.CustomAPIs[:foundIdx], cfg.CustomAPIs[foundIdx+1:]...)
	if err := m.store.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	return nil
}

// BuildAPIContext constructs the template context with cluster config, current state,
// and the current request parsed as a recursive map[string]any in the similar format of JavaScript Request object.
func (m *Manager) BuildAPIContext(r *http.Request) (map[string]any, error) {
	m.mu.RLock()
	cfg := m.store.Get()
	m.mu.RUnlock()

	var cfgMap map[string]any
	var cfgCopy config.Config
	if cfg != nil {
		cfgCopy = *cfg
		// Sanitize sensitive credentials and cluster secrets
		cfgCopy.PasswordHash = ""
		cfgCopy.EncryptedDEK = ""
		cfgCopy.SessionSecret = ""
		cfgCopy.MCP = config.MCPSettings{}
		cfgCopy.Integrations.PrometheusAuthToken = ""
		cfgCopy.Integrations.TelegramBotToken = ""
		cfgCopy.Integrations.TelegramSecretToken = ""
		cfgCopy.DNS.APIToken = ""
		cfgCopy.ConfigChanges = nil
		cfgCopy.AppVariables = nil
		if len(cfg.CustomAPIs) > 0 {
			apisCopy := make([]config.CustomAPI, len(cfg.CustomAPIs))
			copy(apisCopy, cfg.CustomAPIs)
			for i := range apisCopy {
				apisCopy[i].Token = ""
			}
			cfgCopy.CustomAPIs = apisCopy
		}
		if len(cfg.Apps) > 0 {
			appsCopy := make([]config.App, len(cfg.Apps))
			copy(appsCopy, cfg.Apps)
			for i := range appsCopy {
				appsCopy[i].EnvContent = ""
			}
			cfgCopy.Apps = appsCopy
		}
		if len(cfg.Nodes) > 0 {
			nodesCopy := make([]config.Node, len(cfg.Nodes))
			copy(nodesCopy, cfg.Nodes)
			for i := range nodesCopy {
				nodesCopy[i].AgentToken = ""
			}
			cfgCopy.Nodes = nodesCopy
		}
		if len(cfg.Links) > 0 {
			linksCopy := make([]config.Link, len(cfg.Links))
			copy(linksCopy, cfg.Links)
			for i := range linksCopy {
				linksCopy[i].Psk = ""
				linksCopy[i].From.Psk = ""
				linksCopy[i].To.Psk = ""
				linksCopy[i].From.PrivateKey = ""
				linksCopy[i].To.PrivateKey = ""
			}
			cfgCopy.Links = linksCopy
		}

		if cfgBytes, err := json.Marshal(cfgCopy); err == nil {
			decoder := json.NewDecoder(bytes.NewReader(cfgBytes))
			decoder.UseNumber()
			_ = decoder.Decode(&cfgMap)
		}
	}

	// Runtime state
	state := m.GetNetworkState()
	var stateMap map[string]any
	if state != nil {
		if stateBytes, err := json.Marshal(state); err == nil {
			decoder := json.NewDecoder(bytes.NewReader(stateBytes))
			decoder.UseNumber()
			_ = decoder.Decode(&stateMap)
		}
	}

	// Request object
	reqMap := BuildRequestMap(r)

	ctx := map[string]any{
		"config":  cfgMap,
		"state":   stateMap,
		"request": reqMap,
	}

	// Merge top-level config keys for convenience so both {{ .nodes }} and {{ .config.nodes }} work
	for k, v := range cfgMap {
		if _, exists := ctx[k]; !exists {
			ctx[k] = v
		}
	}

	return ctx, nil
}

// BuildRequestMap parses an HTTP request into a recursive map[string]any in the style of JavaScript Request object
func BuildRequestMap(r *http.Request) map[string]any {
	if r == nil {
		return map[string]any{
			"url":       "",
			"method":    "GET",
			"headers":   map[string]any{},
			"path":      "",
			"query":     map[string]any{},
			"search":    "",
			"params":    map[string]string{},
			"body":      nil,
			"raw_body":  "",
			"client_ip": "",
			"host":      "",
			"protocol":  "HTTP/1.1",
		}
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	reqURI := r.RequestURI
	if reqURI == "" {
		reqURI = r.URL.RequestURI()
	}
	fullURL := fmt.Sprintf("%s://%s%s", scheme, r.Host, reqURI)

	// Headers: normalized lowercase keys
	headers := make(map[string]any)
	for k, v := range r.Header {
		lowerK := strings.ToLower(k)
		if len(v) == 1 {
			headers[lowerK] = v[0]
		} else {
			headers[lowerK] = v
		}
	}

	// Query parameters
	query := make(map[string]any)
	for k, v := range r.URL.Query() {
		if len(v) == 1 {
			query[k] = v[0]
		} else {
			query[k] = v
		}
	}

	// Route parameters from chi context
	params := make(map[string]string)
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		for i, key := range rctx.URLParams.Keys {
			if i < len(rctx.URLParams.Values) {
				params[key] = rctx.URLParams.Values[i]
			}
		}
	}

	// Body parsing
	var parsedBody any
	var rawBody string
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 10*1024*1024))
		if err == nil && len(bodyBytes) > 0 {
			rawBody = string(bodyBytes)
			var jsonVal any
			if err := json.Unmarshal(bodyBytes, &jsonVal); err == nil {
				parsedBody = jsonVal
			} else {
				parsedBody = rawBody
			}
		}
	}

	// Client IP
	clientIP := r.RemoteAddr
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		clientIP = strings.TrimSpace(parts[0])
	} else if xrip := r.Header.Get("X-Real-IP"); xrip != "" {
		clientIP = strings.TrimSpace(xrip)
	}

	return map[string]any{
		"url":       fullURL,
		"method":    r.Method,
		"headers":   headers,
		"path":      r.URL.Path,
		"query":     query,
		"search":    r.URL.RawQuery,
		"params":    params,
		"body":      parsedBody,
		"raw_body":  rawBody,
		"client_ip": clientIP,
		"host":      r.Host,
		"protocol":  r.Proto,
	}
}

// ExecuteCustomAPI renders the Go text template of a custom API
func (m *Manager) ExecuteCustomAPI(api *config.CustomAPI, r *http.Request) (string, error) {
	if api == nil {
		return "", errors.New("custom API is nil")
	}
	if api.Disabled {
		return "", fmt.Errorf("custom API %q is disabled", api.ID)
	}

	ctx, err := m.BuildAPIContext(r)
	if err != nil {
		return "", fmt.Errorf("failed to build API context: %w", err)
	}

	return tplutil.RenderTemplate(api.Template, ctx)
}

// TestCustomAPI tests rendering a template with an optional mock/simulated request
func (m *Manager) TestCustomAPI(api config.CustomAPI, testReq *CustomAPITestRequestPayload) (string, error) {
	// Verify template compiles first
	if _, err := tplutil.GetTemplate(api.Template, false); err != nil {
		return "", fmt.Errorf("invalid template syntax: %w", err)
	}

	var httpReq *http.Request
	if testReq != nil {
		method := testReq.Method
		if method == "" {
			method = "GET"
		}
		path := testReq.Path
		if path == "" {
			path = fmt.Sprintf("/api/custom/%s", api.ID)
		}
		if len(testReq.Query) > 0 {
			var qParts []string
			for k, v := range testReq.Query {
				qParts = append(qParts, fmt.Sprintf("%s=%s", k, v))
			}
			path = fmt.Sprintf("%s?%s", path, strings.Join(qParts, "&"))
		}
		var bodyReader io.Reader
		if testReq.Body != "" {
			bodyReader = strings.NewReader(testReq.Body)
		}
		r, err := http.NewRequest(method, path, bodyReader)
		if err == nil {
			r.Host = "easy42.local"
			for k, v := range testReq.Headers {
				r.Header.Set(k, v)
			}
			httpReq = r
		}
	}

	if httpReq == nil {
		r, _ := http.NewRequest("GET", fmt.Sprintf("/api/custom/%s", api.ID), nil)
		if r != nil {
			r.Host = "easy42.local"
		}
		httpReq = r
	}

	ctx, err := m.BuildAPIContext(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to build API context: %w", err)
	}

	return tplutil.RenderTemplate(api.Template, ctx)
}
