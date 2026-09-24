package config

import (
	"strings"
	"time"
)

// Template type constants
const (
	TemplateTypeWg   = "wg"
	TemplateTypeBird = "bird"
	TemplateTypeNft  = "nft"
)

// Builtin template ID constants
const (
	DefaultWgTemplateID   = "default_wg"
	DefaultBirdTemplateID = "default_bird"
	DefaultNftTemplateID  = "default_nft"
)

// IsValidTemplateType returns true if t is a recognized template type
func IsValidTemplateType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case TemplateTypeWg, TemplateTypeBird, TemplateTypeNft:
		return true
	default:
		return false
	}
}

// ConfigTemplate represents a custom or system Go text template for generating node configs
type ConfigTemplate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"` // "wg" | "bird" | "nft"
	Description string    `json:"description,omitempty"`
	Content     string    `json:"content"`
	IsBuiltin   bool      `json:"is_builtin,omitempty"` // true for system default templates (read-only)
	ModifiedAt  time.Time `json:"modified_at,omitempty"`
}

// FindCustomTemplate finds a template in the user configuration by ID or Name
func (c *Config) FindCustomTemplate(idOrName string) *ConfigTemplate {
	if c == nil || idOrName == "" {
		return nil
	}
	clean := strings.TrimSpace(idOrName)
	for i := range c.Templates {
		if c.Templates[i].ID == clean || c.Templates[i].Name == clean {
			return &c.Templates[i]
		}
	}
	return nil
}
