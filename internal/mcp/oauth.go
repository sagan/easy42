package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"easy42/internal/engine"
)

// OAuthTokenResponse represents standard RFC 6749 OAuth 2.0 token response
type OAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// AuthCode stores temporary state during Authorization Code exchange
type AuthCode struct {
	Code                string
	ClientID            string
	RedirectURI         string
	CodeChallenge       string
	CodeChallengeMethod string
	Role                string
	ExpiresAt           time.Time
}

// AccessTokenInfo holds runtime state of an issued access token
type AccessTokenInfo struct {
	Token     string
	Role      string
	ClientID  string
	ExpiresAt time.Time
}

// RefreshTokenInfo holds runtime state of a refresh token
type RefreshTokenInfo struct {
	Token     string
	Role      string
	ClientID  string
	ExpiresAt time.Time
}

// ClientRegistration represents a dynamic or static OAuth client
type ClientRegistration struct {
	ClientID     string    `json:"client_id"`
	ClientSecret string    `json:"client_secret"`
	ClientName   string    `json:"client_name,omitempty"`
	RedirectURIs []string  `json:"redirect_uris,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// OAuthServer manages OAuth 2.0 / 2.1 token issuance, validation, and PKCE verification
type OAuthServer struct {
	mu            sync.RWMutex
	mgr           *engine.Manager
	authCodes     map[string]*AuthCode
	accessTokens  map[string]*AccessTokenInfo
	refreshTokens map[string]*RefreshTokenInfo
	dynClients    map[string]*ClientRegistration
}

// NewOAuthServer initializes an in-memory OAuth server
func NewOAuthServer(mgr *engine.Manager) *OAuthServer {
	return &OAuthServer{
		mgr:           mgr,
		authCodes:     make(map[string]*AuthCode),
		accessTokens:  make(map[string]*AccessTokenInfo),
		refreshTokens: make(map[string]*RefreshTokenInfo),
		dynClients:    make(map[string]*ClientRegistration),
	}
}

// ValidateAccessToken checks if token is valid and returns the assigned role
func (s *OAuthServer) ValidateAccessToken(token string) (string, bool) {
	if token == "" {
		return "", false
	}

	s.mu.RLock()
	info, exists := s.accessTokens[token]
	s.mu.RUnlock()

	if !exists {
		return "", false
	}

	if time.Now().After(info.ExpiresAt) {
		s.mu.Lock()
		delete(s.accessTokens, token)
		s.mu.Unlock()
		return "", false
	}

	return info.Role, true
}

// CreateAuthCode stores a pending authorization code
func (s *OAuthServer) CreateAuthCode(clientID, redirectURI, codeChallenge, method, role string) (string, error) {
	if role == "" {
		role = RoleFullControl
	}

	codeBytes := make([]byte, 24)
	if _, err := rand.Read(codeBytes); err != nil {
		return "", err
	}
	code := fmt.Sprintf("e42_code_%s", hex.EncodeToString(codeBytes))

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean up expired codes
	now := time.Now()
	for k, v := range s.authCodes {
		if now.After(v.ExpiresAt) {
			delete(s.authCodes, k)
		}
	}

	s.authCodes[code] = &AuthCode{
		Code:                code,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: method,
		Role:                role,
		ExpiresAt:           now.Add(10 * time.Minute),
	}

	return code, nil
}

// ExchangeAuthCode verifies code and PKCE, issuing an access token
func (s *OAuthServer) ExchangeAuthCode(clientID, clientSecret, code, codeVerifier, redirectURI string) (*OAuthTokenResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	authCode, exists := s.authCodes[code]
	if !exists {
		return nil, errors.New("invalid or expired authorization code")
	}

	// Single use
	delete(s.authCodes, code)

	if time.Now().After(authCode.ExpiresAt) {
		return nil, errors.New("authorization code has expired")
	}

	// Verify client_id
	if authCode.ClientID != "" && clientID != "" && authCode.ClientID != clientID {
		return nil, errors.New("client_id mismatch")
	}

	// Verify redirect_uri if specified in authorization request
	if authCode.RedirectURI != "" && redirectURI != "" && authCode.RedirectURI != redirectURI {
		return nil, errors.New("redirect_uri mismatch")
	}

	// Verify PKCE code_challenge if present
	if authCode.CodeChallenge != "" {
		if codeVerifier == "" {
			return nil, errors.New("code_verifier required for PKCE")
		}
		if !verifyPKCE(authCode.CodeChallenge, authCode.CodeChallengeMethod, codeVerifier) {
			return nil, errors.New("invalid code_verifier PKCE verification failed")
		}
	} else if clientSecret != "" {
		// If no PKCE, verify client credentials
		if !s.validateClientCredentials(clientID, clientSecret) {
			return nil, errors.New("invalid client_id or client_secret")
		}
	}

	return s.issueTokensLocked(authCode.Role, clientID, 86400*30) // 30-day token
}

// ClientCredentialsToken handles grant_type=client_credentials
func (s *OAuthServer) ClientCredentialsToken(clientID, clientSecret, scope string) (*OAuthTokenResponse, error) {
	if !s.validateClientCredentials(clientID, clientSecret) {
		return nil, errors.New("invalid_client: client authentication failed")
	}

	role := RoleFullControl
	if strings.Contains(strings.ToLower(scope), "read") && !strings.Contains(strings.ToLower(scope), "write") {
		role = RoleReadOnly
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.issueTokensLocked(role, clientID, 86400*30)
}

// RefreshToken handles grant_type=refresh_token
func (s *OAuthServer) RefreshToken(refreshToken, clientID, clientSecret string) (*OAuthTokenResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	refInfo, exists := s.refreshTokens[refreshToken]
	if !exists {
		return nil, errors.New("invalid_grant: refresh token not found")
	}

	if time.Now().After(refInfo.ExpiresAt) {
		delete(s.refreshTokens, refreshToken)
		return nil, errors.New("invalid_grant: refresh token expired")
	}

	if clientSecret != "" && !s.validateClientCredentials(clientID, clientSecret) {
		return nil, errors.New("invalid_client: client authentication failed")
	}

	// Rotate refresh token
	delete(s.refreshTokens, refreshToken)
	return s.issueTokensLocked(refInfo.Role, refInfo.ClientID, 86400*30)
}

// RegisterClient supports Dynamic Client Registration (RFC 7591)
func (s *OAuthServer) RegisterClient(name string, redirectURIs []string) *ClientRegistration {
	s.mu.Lock()
	defer s.mu.Unlock()

	idBytes := make([]byte, 16)
	secBytes := make([]byte, 28)
	_, _ = rand.Read(idBytes)
	_, _ = rand.Read(secBytes)

	reg := &ClientRegistration{
		ClientID:     fmt.Sprintf("e42_cid_%s", hex.EncodeToString(idBytes)),
		ClientSecret: fmt.Sprintf("e42_sec_%s", hex.EncodeToString(secBytes)),
		ClientName:   name,
		RedirectURIs: redirectURIs,
		CreatedAt:    time.Now(),
	}

	s.dynClients[reg.ClientID] = reg
	return reg
}

// validateClientCredentials checks clientID/secret against config and dynamic clients
func (s *OAuthServer) validateClientCredentials(clientID, clientSecret string) bool {
	settings := s.mgr.GetMCPSettings()

	// 1. Check primary configured OAuth Client ID and Secret
	cfgID := settings.OAuthClientID
	if cfgID == "" {
		cfgID = "easy42-mcp"
	}

	if (clientID == "" || clientID == cfgID) && settings.OAuthClientSecret != "" {
		if subtle.ConstantTimeCompare([]byte(clientSecret), []byte(settings.OAuthClientSecret)) == 1 {
			return true
		}
	}

	// 2. Also accept direct matching against FullControlToken for seamless setup
	if settings.FullControlToken != "" && subtle.ConstantTimeCompare([]byte(clientSecret), []byte(settings.FullControlToken)) == 1 {
		return true
	}

	// 3. Check dynamic clients
	if clientID != "" {
		if client, ok := s.dynClients[clientID]; ok {
			if subtle.ConstantTimeCompare([]byte(clientSecret), []byte(client.ClientSecret)) == 1 {
				return true
			}
		}
	}

	return false
}

// issueTokensLocked creates access and refresh tokens (caller must hold s.mu)
func (s *OAuthServer) issueTokensLocked(role, clientID string, durationSec int) (*OAuthTokenResponse, error) {
	atBytes := make([]byte, 28)
	rtBytes := make([]byte, 28)
	if _, err := rand.Read(atBytes); err != nil {
		return nil, err
	}
	if _, err := rand.Read(rtBytes); err != nil {
		return nil, err
	}

	accessToken := fmt.Sprintf("e42_at_%s", hex.EncodeToString(atBytes))
	refreshToken := fmt.Sprintf("e42_rt_%s", hex.EncodeToString(rtBytes))

	now := time.Now()
	expiresAt := now.Add(time.Duration(durationSec) * time.Second)

	s.accessTokens[accessToken] = &AccessTokenInfo{
		Token:     accessToken,
		Role:      role,
		ClientID:  clientID,
		ExpiresAt: expiresAt,
	}

	s.refreshTokens[refreshToken] = &RefreshTokenInfo{
		Token:     refreshToken,
		Role:      role,
		ClientID:  clientID,
		ExpiresAt: now.Add(90 * 24 * time.Hour), // 90 days refresh token
	}

	scope := "mcp:all"
	if role == RoleReadOnly {
		scope = "mcp:read"
	}

	return &OAuthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    durationSec,
		RefreshToken: refreshToken,
		Scope:        scope,
	}, nil
}

// verifyPKCE validates code_verifier against code_challenge using S256 or plain
func verifyPKCE(challenge, method, verifier string) bool {
	switch strings.ToUpper(method) {
	case "S256", "":
		h := sha256.Sum256([]byte(verifier))
		computed := base64.RawURLEncoding.EncodeToString(h[:])
		return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
	case "PLAIN":
		return subtle.ConstantTimeCompare([]byte(verifier), []byte(challenge)) == 1
	default:
		return false
	}
}
