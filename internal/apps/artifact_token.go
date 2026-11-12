package apps

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// ArtifactTokenInfo stores information associated with an artifact download token
type ArtifactTokenInfo struct {
	FilePath  string
	FileName  string
	SHA256    string
	NodeName  string
	ExpiresAt time.Time
}

// ArtifactTokenManager securely issues and validates temporary download tokens
type ArtifactTokenManager struct {
	mu     sync.RWMutex
	tokens map[string]ArtifactTokenInfo
}

// NewArtifactTokenManager creates a new token manager
func NewArtifactTokenManager() *ArtifactTokenManager {
	return &ArtifactTokenManager{
		tokens: make(map[string]ArtifactTokenInfo),
	}
}

// GenerateToken creates an ephemeral token for downloading an artifact file
func (m *ArtifactTokenManager) GenerateToken(filePath, fileName, sha256Hash, nodeName string, ttl time.Duration) string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based if crypto/rand fails
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	token := hex.EncodeToString(b)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Clean up expired tokens if map grows large
	now := time.Now()
	if len(m.tokens) > 200 {
		for k, v := range m.tokens {
			if now.After(v.ExpiresAt) {
				delete(m.tokens, k)
			}
		}
	}

	m.tokens[token] = ArtifactTokenInfo{
		FilePath:  filePath,
		FileName:  fileName,
		SHA256:    sha256Hash,
		NodeName:  nodeName,
		ExpiresAt: now.Add(ttl),
	}

	return token
}

// ValidateToken validates a download token and returns its info
func (m *ArtifactTokenManager) ValidateToken(token string) (*ArtifactTokenInfo, bool) {
	if token == "" {
		return nil, false
	}

	m.mu.RLock()
	info, ok := m.tokens[token]
	m.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if time.Now().After(info.ExpiresAt) {
		m.mu.Lock()
		delete(m.tokens, token)
		m.mu.Unlock()
		return nil, false
	}

	return &info, true
}
