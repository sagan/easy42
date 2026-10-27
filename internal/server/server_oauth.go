package server

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"easy42/internal/crypto"
	"easy42/internal/mcp"
)

// getBaseURL determines the public scheme and host for metadata endpoints
func getBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if xfh := r.Header.Get("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func setOAuthCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Accept, X-MCP-Token, X-Requested-With")
	w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate, Authorization")
}

// handleOAuthProtectedResource serves RFC 9728 Protected Resource Metadata
func (s *Server) handleOAuthProtectedResource(w http.ResponseWriter, r *http.Request) {
	setOAuthCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	baseURL := getBaseURL(r)
	meta := map[string]any{
		"resource":              baseURL + "/mcp",
		"authorization_servers": []string{baseURL},
		"scopes_supported":      []string{"read", "write", "mcp:all"},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(meta)
}

// handleOAuthAuthorizationServerMetadata serves RFC 8414 Authorization Server Metadata
func (s *Server) handleOAuthAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	setOAuthCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	baseURL := getBaseURL(r)
	meta := map[string]any{
		"issuer":                                baseURL,
		"authorization_endpoint":                baseURL + "/oauth/authorize",
		"token_endpoint":                        baseURL + "/oauth/token",
		"registration_endpoint":                 baseURL + "/oauth/register",
		"response_types_supported":             []string{"code"},
		"grant_types_supported":                []string{"authorization_code", "client_credentials", "refresh_token"},
		"code_challenge_methods_supported":     []string{"S256", "plain"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"scopes_supported":                      []string{"read", "write", "mcp:all"},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(meta)
}

// handleOpenIDConfiguration serves OpenID Connect discovery fallback
func (s *Server) handleOpenIDConfiguration(w http.ResponseWriter, r *http.Request) {
	s.handleOAuthAuthorizationServerMetadata(w, r)
}

// handleOAuthToken exchanges authorization codes or client credentials for access tokens
func (s *Server) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	setOAuthCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if s.mcpServer == nil || s.mcpServer.OAuthServer() == nil {
		http.Error(w, `{"error":"server_error","error_description":"MCP OAuth server not available"}`, http.StatusInternalServerError)
		return
	}

	// Parse parameters from query, form-data, or JSON body
	_ = r.ParseForm()

	grantType := r.FormValue("grant_type")
	clientID := r.FormValue("client_id")
	clientSecret := r.FormValue("client_secret")
	code := r.FormValue("code")
	codeVerifier := r.FormValue("code_verifier")
	redirectURI := r.FormValue("redirect_uri")
	refreshToken := r.FormValue("refresh_token")
	scope := r.FormValue("scope")

	// Support HTTP Basic Authentication for client_id and client_secret
	if authHdr := r.Header.Get("Authorization"); strings.HasPrefix(authHdr, "Basic ") {
		encoded := strings.TrimPrefix(authHdr, "Basic ")
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err == nil {
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				clientID = parts[0]
				clientSecret = parts[1]
			}
		}
	}

	// Also support JSON body if form is empty
	if grantType == "" && strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var jsonBody struct {
			GrantType    string `json:"grant_type"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			Code         string `json:"code"`
			CodeVerifier string `json:"code_verifier"`
			RedirectURI  string `json:"redirect_uri"`
			RefreshToken string `json:"refresh_token"`
			Scope        string `json:"scope"`
		}
		if err := json.NewDecoder(r.Body).Decode(&jsonBody); err == nil {
			if jsonBody.GrantType != "" {
				grantType = jsonBody.GrantType
			}
			if jsonBody.ClientID != "" {
				clientID = jsonBody.ClientID
			}
			if jsonBody.ClientSecret != "" {
				clientSecret = jsonBody.ClientSecret
			}
			if jsonBody.Code != "" {
				code = jsonBody.Code
			}
			if jsonBody.CodeVerifier != "" {
				codeVerifier = jsonBody.CodeVerifier
			}
			if jsonBody.RedirectURI != "" {
				redirectURI = jsonBody.RedirectURI
			}
			if jsonBody.RefreshToken != "" {
				refreshToken = jsonBody.RefreshToken
			}
			if jsonBody.Scope != "" {
				scope = jsonBody.Scope
			}
		}
	}

	oauthSrv := s.mcpServer.OAuthServer()
	var (
		resp *mcp.OAuthTokenResponse
		err  error
	)

	switch grantType {
	case "authorization_code":
		if code == "" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "code parameter is required")
			return
		}
		resp, err = oauthSrv.ExchangeAuthCode(clientID, clientSecret, code, codeVerifier, redirectURI)
	case "client_credentials":
		if clientSecret == "" {
			writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "client_secret is required")
			return
		}
		resp, err = oauthSrv.ClientCredentialsToken(clientID, clientSecret, scope)
	case "refresh_token":
		if refreshToken == "" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", "refresh_token parameter is required")
			return
		}
		resp, err = oauthSrv.RefreshToken(refreshToken, clientID, clientSecret)
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code, client_credentials, or refresh_token")
		return
	}

	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeOAuthError(w http.ResponseWriter, status int, errCode, errDesc string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             errCode,
		"error_description": errDesc,
	})
}

// handleOAuthAuthorize handles GET and POST for the user consent flow
func (s *Server) handleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	setOAuthCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	_ = r.ParseForm()

	responseType := r.FormValue("response_type")
	clientID := r.FormValue("client_id")
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")
	scope := r.FormValue("scope")
	codeChallenge := r.FormValue("code_challenge")
	codeChallengeMethod := r.FormValue("code_challenge_method")

	if responseType != "code" && r.Method == http.MethodGet {
		http.Error(w, "response_type must be 'code'", http.StatusBadRequest)
		return
	}
	if redirectURI == "" {
		http.Error(w, "redirect_uri is required", http.StatusBadRequest)
		return
	}

	// Verify redirect_uri is a valid URL
	parsedRedirect, err := url.Parse(redirectURI)
	if err != nil || (!parsedRedirect.IsAbs() && !strings.HasPrefix(redirectURI, "http://") && !strings.HasPrefix(redirectURI, "https://")) {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}

	// Check if already authenticated via session cookie
	isAuthenticated := false
	cfg := s.mgr.Store().Get()
	if cfg != nil {
		if cookie, err := r.Cookie("easy42_session"); err == nil && cookie.Value != "" {
			if crypto.VerifySessionToken(cookie.Value, cfg.SessionSecret, cfg.PasswordHash) {
				isAuthenticated = true
			}
		}
	}

	// POST: user submitted the consent form (Authorize or Deny)
	if r.Method == http.MethodPost {
		action := r.FormValue("action")
		if action == "deny" {
			redirectWithOAuthError(w, redirectURI, "access_denied", "The user denied the authorization request", state)
			return
		}

		// Verify authentication: either session cookie or password submitted
		if !isAuthenticated {
			password := r.FormValue("password")
			valid, _ := crypto.VerifyPassword(password, cfg.PasswordHash)
			if cfg == nil || !valid {
				renderConsentPage(w, r, clientID, redirectURI, state, scope, codeChallenge, codeChallengeMethod, false, "Invalid master password. Please enter the correct password to authorize.")
				return
			}
		}

		role := r.FormValue("role")
		if role != mcp.RoleReadOnly {
			role = mcp.RoleFullControl
		}

		code, err := s.mcpServer.OAuthServer().CreateAuthCode(clientID, redirectURI, codeChallenge, codeChallengeMethod, role)
		if err != nil {
			http.Error(w, "Failed to create authorization code: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Redirect back to client with code and state
		redirectURL, _ := url.Parse(redirectURI)
		q := redirectURL.Query()
		q.Set("code", code)
		if state != "" {
			q.Set("state", state)
		}
		redirectURL.RawQuery = q.Encode()

		http.Redirect(w, r, redirectURL.String(), http.StatusFound)
		return
	}

	// GET: render consent page
	renderConsentPage(w, r, clientID, redirectURI, state, scope, codeChallenge, codeChallengeMethod, isAuthenticated, "")
}

func redirectWithOAuthError(w http.ResponseWriter, redirectURI, errCode, errDesc, state string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, errDesc, http.StatusBadRequest)
		return
	}
	q := u.Query()
	q.Set("error", errCode)
	q.Set("error_description", errDesc)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, nil, u.String(), http.StatusFound)
}

func renderConsentPage(w http.ResponseWriter, r *http.Request, clientID, redirectURI, state, scope, codeChallenge, method string, isAuth bool, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	clientName := clientID
	if clientName == "" || clientName == "easy42-mcp" {
		clientName = "Google Gemini / AI Agent"
	}

	tmpl := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Authorize AI Agent - Easy42 MCP</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; }
    body { background-color: #F8FAFC; color: #0F172A; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 20px; }
    .card { background: #FFFFFF; border: 1px solid #E2E8F0; border-radius: 16px; box-shadow: 0 10px 25px -5px rgba(15, 23, 42, 0.08); max-width: 480px; width: 100%; padding: 32px; }
    .header { display: flex; align-items: center; gap: 14px; margin-bottom: 20px; }
    .logo-badge { width: 48px; height: 48px; border-radius: 12px; background: #EEF2FF; color: #4F46E5; display: flex; align-items: center; justify-content: center; font-size: 24px; font-weight: bold; border: 1px solid #E0E7FF; }
    h1 { font-size: 1.25rem; font-weight: 700; color: #0F172A; }
    .subtitle { color: #64748B; font-size: 0.88rem; margin-top: 4px; }
    .app-box { background: #F1F5F9; border-radius: 10px; padding: 14px 16px; margin-bottom: 20px; display: flex; align-items: center; justify-content: space-between; }
    .app-name { font-weight: 600; color: #1E293B; font-size: 0.95rem; }
    .permissions { margin-bottom: 24px; }
    .permissions h2 { font-size: 0.8rem; text-transform: uppercase; letter-spacing: 0.05em; color: #64748B; margin-bottom: 10px; }
    .perm-item { display: flex; align-items: flex-start; gap: 10px; margin-bottom: 8px; font-size: 0.86rem; color: #334155; }
    .perm-check { color: #16A34A; font-weight: bold; }
    .role-select { margin-bottom: 20px; }
    .role-select label { display: block; font-size: 0.82rem; font-weight: 600; color: #475569; margin-bottom: 6px; }
    .role-select select, .pwd-input { width: 100%; padding: 10px 12px; border-radius: 8px; border: 1px solid #CBD5E1; font-size: 0.9rem; background: #FFFFFF; outline: none; }
    .pwd-input:focus, .role-select select:focus { border-color: #4F46E5; ring: 2px solid #EEF2FF; }
    .actions { display: flex; gap: 12px; margin-top: 24px; }
    .btn { flex: 1; padding: 12px; border-radius: 8px; font-weight: 600; font-size: 0.9rem; cursor: pointer; text-align: center; border: none; text-decoration: none; }
    .btn-primary { background: #4F46E5; color: #FFFFFF; }
    .btn-primary:hover { background: #4338CA; }
    .btn-secondary { background: #F1F5F9; color: #475569; border: 1px solid #CBD5E1; }
    .btn-secondary:hover { background: #E2E8F0; }
    .alert-error { background: #FEF2F2; border: 1px solid #FCA5A5; color: #991B1B; padding: 10px 14px; border-radius: 8px; font-size: 0.85rem; margin-bottom: 16px; }
    .alert-auth { background: #F0FDF4; border: 1px solid #BBF7D0; color: #166534; padding: 8px 12px; border-radius: 8px; font-size: 0.82rem; margin-bottom: 16px; display: flex; align-items: center; gap: 8px; }
  </style>
</head>
<body>
  <div class="card">
    <div class="header">
      <div class="logo-badge">🤖</div>
      <div>
        <h1>Authorize AI Agent</h1>
        <div class="subtitle">Easy42 Mesh Control Plane</div>
      </div>
    </div>

    {{if .ErrorMsg}}
    <div class="alert-error">{{.ErrorMsg}}</div>
    {{end}}

    <div class="app-box">
      <div>
        <div style="font-size:0.75rem; color:#64748B; font-weight:600; text-transform:uppercase;">Application</div>
        <div class="app-name">{{.ClientName}}</div>
      </div>
      <div style="font-size:0.75rem; background:#E0E7FF; color:#4338CA; font-weight:700; padding:4px 8px; border-radius:6px;">MCP Client</div>
    </div>

    <div class="permissions">
      <h2>Permissions Requested</h2>
      <div class="perm-item">
        <span class="perm-check">✓</span>
        <div>Query mesh cluster state, active nodes, and WireGuard links</div>
      </div>
      <div class="perm-item">
        <span class="perm-check">✓</span>
        <div>Run real-time latency probes, Looking Glass, and telemetry</div>
      </div>
      <div class="perm-item">
        <span class="perm-check">✓</span>
        <div>Apply mesh network routing policies & synchronize fleet config</div>
      </div>
    </div>

    <form method="POST" action="/oauth/authorize">
      <input type="hidden" name="response_type" value="code">
      <input type="hidden" name="client_id" value="{{.ClientID}}">
      <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
      <input type="hidden" name="state" value="{{.State}}">
      <input type="hidden" name="scope" value="{{.Scope}}">
      <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
      <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">

      <div class="role-select">
        <label for="role">Permission Scope</label>
        <select id="role" name="role">
          <option value="full_control" selected>Full Control (Manage & Configure)</option>
          <option value="read_only">Read-Only (Query & Inspect Only)</option>
        </select>
      </div>

      {{if .IsAuth}}
      <div class="alert-auth">
        <span>✓</span> Authenticated as Easy42 Administrator
      </div>
      {{else}}
      <div style="margin-bottom: 20px;">
        <label for="password" style="display:block; font-size:0.82rem; font-weight:600; color:#475569; margin-bottom:6px;">Admin Master Password</label>
        <input type="password" id="password" name="password" class="pwd-input" placeholder="Enter Easy42 password" required autofocus>
      </div>
      {{end}}

      <div class="actions">
        <button type="submit" name="action" value="deny" class="btn btn-secondary">Deny</button>
        <button type="submit" name="action" value="approve" class="btn btn-primary">Authorize</button>
      </div>
    </form>
  </div>
</body>
</html>`

	t, err := template.New("consent").Parse(tmpl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := struct {
		ClientID            string
		ClientName          string
		RedirectURI         string
		State               string
		Scope               string
		CodeChallenge       string
		CodeChallengeMethod string
		IsAuth              bool
		ErrorMsg            string
	}{
		ClientID:            clientID,
		ClientName:          clientName,
		RedirectURI:         redirectURI,
		State:               state,
		Scope:               scope,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: method,
		IsAuth:              isAuth,
		ErrorMsg:            errorMsg,
	}

	_ = t.Execute(w, data)
}

// handleOAuthRegister handles RFC 7591 Dynamic Client Registration
func (s *Server) handleOAuthRegister(w http.ResponseWriter, r *http.Request) {
	setOAuthCORS(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.mcpServer == nil || s.mcpServer.OAuthServer() == nil {
		http.Error(w, "OAuth server not available", http.StatusInternalServerError)
		return
	}

	var req struct {
		ClientName   string   `json:"client_name"`
		RedirectURIs []string `json:"redirect_uris"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.ClientName == "" {
		req.ClientName = "AI Client"
	}

	reg := s.mcpServer.OAuthServer().RegisterClient(req.ClientName, req.RedirectURIs)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"client_id":     reg.ClientID,
		"client_secret": reg.ClientSecret,
		"client_name":   reg.ClientName,
		"redirect_uris": reg.RedirectURIs,
	})
}
