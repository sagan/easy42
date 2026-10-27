package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MCPSettings holds configuration and credentials for the Model Context Protocol server
type MCPSettings struct {
	Enabled          bool   `json:"enabled"`
	ReadOnlyToken    string `json:"read_only_token"`
	FullControlToken string `json:"full_control_token"`
}

// GenerateMCPToken creates a cryptographically secure random token for MCP authentication
func GenerateMCPToken(tokenType string) string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("e42_mcp_%s_%d", tokenType, time.Now().UnixNano())
	}
	prefix := "ro"
	lower := strings.ToLower(tokenType)
	if strings.Contains(lower, "full") || strings.Contains(lower, "admin") || strings.Contains(lower, "rw") {
		prefix = "full"
	}
	return fmt.Sprintf("e42_mcp_%s_%s", prefix, hex.EncodeToString(b))
}

// ValidateMCPSettings validates the MCP configuration
func ValidateMCPSettings(s *MCPSettings) error {
	if s == nil {
		return nil
	}
	if s.Enabled {
		if strings.TrimSpace(s.ReadOnlyToken) == "" {
			return errors.New("read-only token cannot be empty when MCP is enabled")
		}
		if strings.TrimSpace(s.FullControlToken) == "" {
			return errors.New("full-control token cannot be empty when MCP is enabled")
		}
		if s.ReadOnlyToken == s.FullControlToken {
			return errors.New("read-only token and full-control token must be distinct")
		}
	}
	return nil
}
