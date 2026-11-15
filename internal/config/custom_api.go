package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var validAPIIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// CustomAPI represents a user-defined programmable API endpoint
type CustomAPI struct {
	ID           string            `json:"id"`                      // Unique slug used in URL: /api/custom/{id}
	Name         string            `json:"name,omitempty"`          // Optional display name / label
	Description  string            `json:"description,omitempty"`   // Optional description
	ContentType  string            `json:"content_type"`            // e.g. "application/json", "text/plain"
	Token        string            `json:"token,omitempty"`         // Optional Authorization Bearer token
	ExtraHeaders map[string]string `json:"extra_headers,omitempty"` // Extra response headers
	Template     string            `json:"template"`                // Go text/template string
	Disabled     bool              `json:"disabled,omitempty"`      // Whether this custom API is disabled
	CreatedAt    time.Time         `json:"created_at,omitempty"`
	ModifiedAt   time.Time         `json:"modified_at,omitempty"`
}

// ValidateCustomAPI validates the programmable API fields
func ValidateCustomAPI(api *CustomAPI) error {
	if api == nil {
		return errors.New("custom API cannot be nil")
	}
	id := strings.TrimSpace(api.ID)
	if id == "" {
		return errors.New("API ID is required")
	}
	if !validAPIIDRegex.MatchString(id) {
		return fmt.Errorf("API ID %q is invalid: must only contain letters, numbers, hyphens, and underscores", id)
	}
	if strings.TrimSpace(api.ContentType) == "" {
		api.ContentType = "application/json"
	}
	if strings.TrimSpace(api.Template) == "" {
		return errors.New("API template content cannot be empty")
	}
	return nil
}

// FindCustomAPI finds a custom API in the configuration by ID
func (c *Config) FindCustomAPI(id string) *CustomAPI {
	if c == nil || id == "" {
		return nil
	}
	clean := strings.TrimSpace(id)
	for i := range c.CustomAPIs {
		if c.CustomAPIs[i].ID == clean {
			return &c.CustomAPIs[i]
		}
	}
	return nil
}
