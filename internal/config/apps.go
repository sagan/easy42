package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var appNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// App represents a workload deployed and managed on Easy42 cluster nodes
type App struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	URL         string            `json:"url,omitempty" yaml:"url,omitempty"`
	AppType     string            `json:"app_type"` // "docker" or "systemd"
	MainContent string            `json:"main_content"` // compose.yaml or service unit
	EnvContent  string            `json:"env_content,omitempty"` // .env content KEY=VAL
	Files       map[string]string `json:"files,omitempty"` // relative_filename -> content
	TargetType  string            `json:"target_type,omitempty"` // "manual" (default), "all", or "tag"
	TargetTag   string            `json:"target_tag,omitempty"` // Tag name when TargetType is "tag"
	Nodes       []string          `json:"nodes"` // Node names assigned when TargetType is "manual" (can be empty)
	Published   bool              `json:"published"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ValidateAppName validates that an app name contains only safe alphanumeric, dash, and underscore characters
func ValidateAppName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("app name cannot be empty")
	}
	if !appNameRegex.MatchString(name) {
		return fmt.Errorf("app name '%s' is invalid: must be 1-64 characters matching ^[a-zA-Z0-9_-]+$", name)
	}
	return nil
}

// ValidateApp validates an App definition
func ValidateApp(app App) error {
	if err := ValidateAppName(app.Name); err != nil {
		return err
	}

	appType := strings.TrimSpace(strings.ToLower(app.AppType))
	if appType != "docker" && appType != "systemd" {
		return fmt.Errorf("unsupported app type '%s': must be 'docker' or 'systemd'", app.AppType)
	}

	targetType := strings.ToLower(strings.TrimSpace(app.TargetType))
	if targetType != "" && targetType != "manual" && targetType != "all" && targetType != "tag" {
		return fmt.Errorf("invalid target_type '%s': must be 'manual', 'all', or 'tag'", app.TargetType)
	}
	if targetType == "tag" && strings.TrimSpace(app.TargetTag) == "" {
		return fmt.Errorf("target_tag cannot be empty when target_type is 'tag'")
	}

	if strings.TrimSpace(app.MainContent) == "" {
		if appType == "docker" {
			return fmt.Errorf("docker compose file content cannot be empty")
		}
		return fmt.Errorf("systemd service unit content cannot be empty")
	}

	for filename := range app.Files {
		filename = strings.TrimSpace(filename)
		if filename == "" {
			return fmt.Errorf("config filename cannot be empty")
		}
		if strings.Contains(filename, "..") || strings.HasPrefix(filename, "/") || strings.HasPrefix(filename, "\\") {
			return fmt.Errorf("invalid config file path '%s': must be a relative path without '..'", filename)
		}
	}

	return nil
}

// AppTargetsNode checks if an app targets a given node.
// If node is nil but nodeName is provided, manual and all target types are supported.
func AppTargetsNode(app App, node *Node, nodeName string) bool {
	if node != nil && nodeName == "" {
		nodeName = node.Name
	}
	targetType := strings.ToLower(strings.TrimSpace(app.TargetType))
	switch targetType {
	case "all":
		return true
	case "tag":
		if node == nil {
			return false
		}
		targetTag := strings.TrimSpace(app.TargetTag)
		if targetTag == "" {
			return false
		}
		for _, tag := range node.Tags {
			if strings.EqualFold(strings.TrimSpace(tag), targetTag) {
				return true
			}
		}
		return false
	case "manual", "":
		for _, n := range app.Nodes {
			if n == nodeName {
				return true
			}
		}
		return false
	default:
		return false
	}
}

var variableKeyRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// AppVariable represents a global key-value variable accessible across app manifests, env files, and configs
type AppVariable struct {
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Description string    `json:"description,omitempty"`
	IsSecret    bool      `json:"is_secret,omitempty"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ValidateVariableKey validates that a variable key is a valid identifier
func ValidateVariableKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("variable key cannot be empty")
	}
	if !variableKeyRegex.MatchString(key) {
		return fmt.Errorf("variable key '%s' is invalid: must match ^[a-zA-Z_][a-zA-Z0-9_]*$", key)
	}
	return nil
}

