package apps

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"easy42/internal/crypto"
)

// ArtifactTokenInfo stores information associated with an artifact download token
type ArtifactTokenInfo struct {
	AppName   string    `json:"app_name,omitempty"`
	Version   string    `json:"version,omitempty"`
	Arch      string    `json:"arch,omitempty"`
	OSName    string    `json:"os_name,omitempty"`
	Endian    string    `json:"endian,omitempty"`
	FilePath  string    `json:"file_path"`
	FileName  string    `json:"file_name"`
	SHA256    string    `json:"sha256"`
	NodeName  string    `json:"node_name,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

type artifactTokenClaims struct {
	ArtifactTokenInfo
	HashPrefix string `json:"pwh"`
}

// SecretProvider supplies the session secret and password hash for artifact token signing
type SecretProvider func() (sessionSecret, passwordHash string)

// ArtifactTokenManager securely issues and validates stateless temporary download tokens
type ArtifactTokenManager struct {
	mu             sync.RWMutex
	secretProvider SecretProvider
	fallbackSecret string
	fallbackHash   string
}

// NewArtifactTokenManager creates a new token manager
func NewArtifactTokenManager(providers ...SecretProvider) *ArtifactTokenManager {
	fallbackSecret, _ := crypto.GenerateSessionSecret()
	m := &ArtifactTokenManager{
		fallbackSecret: fallbackSecret,
		fallbackHash:   "easy42-default-hash",
	}
	if len(providers) > 0 && providers[0] != nil {
		m.secretProvider = providers[0]
	}
	return m
}

// SetSecretProvider sets or updates the secret provider callback
func (m *ArtifactTokenManager) SetSecretProvider(provider SecretProvider) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secretProvider = provider
}

func (m *ArtifactTokenManager) getSecrets() (string, string) {
	m.mu.RLock()
	provider := m.secretProvider
	m.mu.RUnlock()

	if provider != nil {
		secret, hash := provider()
		if secret != "" {
			return secret, hash
		}
	}
	return m.fallbackSecret, m.fallbackHash
}

// GenerateToken creates a stateless token for downloading an artifact file
func (m *ArtifactTokenManager) GenerateToken(filePath, fileName, sha256Hash, nodeName string, ttl time.Duration) string {
	return m.GenerateTokenWithApp("", "", "", "", "", filePath, fileName, sha256Hash, nodeName, ttl)
}

// GenerateTokenWithApp creates a stateless token including app context for on-demand re-download
func (m *ArtifactTokenManager) GenerateTokenWithApp(appName, version, arch, osName, endian, filePath, fileName, sha256Hash, nodeName string, ttl time.Duration) string {
	sessionSecret, passwordHash := m.getSecrets()
	info := ArtifactTokenInfo{
		AppName:   appName,
		Version:   version,
		Arch:      arch,
		OSName:    osName,
		Endian:    endian,
		FilePath:  filePath,
		FileName:  fileName,
		SHA256:    sha256Hash,
		NodeName:  nodeName,
		ExpiresAt: time.Now().Add(ttl),
	}
	return CreateArtifactToken(sessionSecret, passwordHash, info)
}

// ValidateToken validates a download token and returns its info
func (m *ArtifactTokenManager) ValidateToken(token string) (*ArtifactTokenInfo, bool) {
	sessionSecret, passwordHash := m.getSecrets()
	return VerifyArtifactToken(token, sessionSecret, passwordHash)
}

// CreateArtifactToken creates an HMAC signed stateless token containing artifact metadata and expiration
func CreateArtifactToken(sessionSecret, passwordHash string, info ArtifactTokenInfo) string {
	claims := artifactTokenClaims{
		ArtifactTokenInfo: info,
		HashPrefix:        crypto.HashFingerprint(passwordHash),
	}

	payloadBytes, err := json.Marshal(claims)
	if err != nil {
		return ""
	}

	mac := hmac.New(sha256.New, []byte(sessionSecret))
	mac.Write(payloadBytes)
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	b64Payload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	return fmt.Sprintf("%s.%s", b64Payload, signature)
}

// VerifyArtifactToken verifies the HMAC signature, expiration timestamp, and password hash fingerprint of an artifact token
func VerifyArtifactToken(token, sessionSecret, passwordHash string) (*ArtifactTokenInfo, bool) {
	if token == "" || sessionSecret == "" {
		return nil, false
	}

	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, false
	}

	b64Payload, signature := parts[0], parts[1]
	payloadBytes, err := base64.RawURLEncoding.DecodeString(b64Payload)
	if err != nil {
		return nil, false
	}

	mac := hmac.New(sha256.New, []byte(sessionSecret))
	mac.Write(payloadBytes)
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return nil, false
	}

	var claims artifactTokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, false
	}

	if !claims.ExpiresAt.IsZero() && time.Now().After(claims.ExpiresAt) {
		return nil, false
	}

	expectedPrefix := crypto.HashFingerprint(passwordHash)
	if claims.HashPrefix != expectedPrefix {
		return nil, false
	}

	return &claims.ArtifactTokenInfo, true
}
