package compiler

import (
	"fmt"
	"strings"

	"easy42/internal/config"
	"easy42/util/tplutil"
)

// GetBuiltinTemplates returns the system default configuration templates
func GetBuiltinTemplates() []config.ConfigTemplate {
	wgContent, _ := GetDefaultWgTemplate()
	birdContent, _ := GetDefaultBirdTemplate()
	nftContent, _ := GetDefaultNftablesTemplate()

	return []config.ConfigTemplate{
		{
			ID:          config.DefaultWgTemplateID,
			Name:        "Default WireGuard Template",
			Type:        config.TemplateTypeWg,
			Description: "System default WireGuard interface configuration template",
			Content:     wgContent,
			IsBuiltin:   true,
		},
		{
			ID:          config.DefaultBirdTemplateID,
			Name:        "Default BIRD Template",
			Type:        config.TemplateTypeBird,
			Description: "System default BIRD 2 routing daemon configuration template",
			Content:     birdContent,
			IsBuiltin:   true,
		},
		{
			ID:          config.DefaultNftTemplateID,
			Name:        "Default nftables Template",
			Type:        config.TemplateTypeNft,
			Description: "System default nftables firewall rules configuration template",
			Content:     nftContent,
			IsBuiltin:   true,
		},
	}
}

// GetBuiltinTemplate returns a specific system default template by type
func GetBuiltinTemplate(tmplType string) (config.ConfigTemplate, bool) {
	for _, t := range GetBuiltinTemplates() {
		if t.Type == tmplType {
			return t, true
		}
	}
	return config.ConfigTemplate{}, false
}

// GetAllTemplates returns both system default templates and custom templates
func GetAllTemplates(customTemplates []config.ConfigTemplate) []config.ConfigTemplate {
	builtins := GetBuiltinTemplates()
	res := make([]config.ConfigTemplate, 0, len(builtins)+len(customTemplates))
	res = append(res, builtins...)
	res = append(res, customTemplates...)
	return res
}

// FindTemplate finds a template by ID or Name for a given template type.
// If idOrName is empty or "default", the built-in default template content for that type is returned.
func FindTemplate(tmplType, idOrName string, customTemplates []config.ConfigTemplate) (string, error) {
	cleanID := strings.TrimSpace(idOrName)
	if cleanID == "" || strings.EqualFold(cleanID, "default") {
		switch tmplType {
		case config.TemplateTypeWg:
			return GetDefaultWgTemplate()
		case config.TemplateTypeBird:
			return GetDefaultBirdTemplate()
		case config.TemplateTypeNft:
			return GetDefaultNftablesTemplate()
		default:
			return "", fmt.Errorf("unknown template type: %s", tmplType)
		}
	}

	// 1. Search custom templates matching type and (ID or Name)
	for _, t := range customTemplates {
		if t.Type == tmplType && (t.ID == cleanID || t.Name == cleanID) {
			return t.Content, nil
		}
	}

	// 2. Search built-in templates
	for _, t := range GetBuiltinTemplates() {
		if t.Type == tmplType && (t.ID == cleanID || t.Name == cleanID) {
			return t.Content, nil
		}
	}

	return "", fmt.Errorf("template %q (type: %s) not found", idOrName, tmplType)
}

// ValidateTemplate validates that the template type is known and that the Go text template syntax is valid
func ValidateTemplate(tmplType, content string) error {
	if !config.IsValidTemplateType(tmplType) {
		return fmt.Errorf("invalid template type %q: must be wg, bird, or nft", tmplType)
	}
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("template content cannot be empty")
	}
	_, err := tplutil.GetTemplate(content, false)
	if err != nil {
		return fmt.Errorf("template syntax error: %w", err)
	}
	return nil
}
