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
	AppType     string            `json:"app_type"`              // "docker", "systemd", or "cron"
	MainContent string            `json:"main_content"`          // compose.yaml or service unit
	EnvContent  string            `json:"env_content,omitempty"` // .env content KEY=VAL
	Files       map[string]string `json:"files,omitempty"`       // relative_filename -> content
	// Selector optionally restricts which nodes the app is deployed to.
	// Supported keys are "name", "tag" and "env:<NAME>". All conditions must match (AND).
	// An empty selector matches every node.
	Selector  map[string]string `json:"selector,omitempty"`
	Published bool              `json:"published"`
	Enabled   bool              `json:"enabled"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
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
	if appType != "docker" && appType != "systemd" && appType != "cron" {
		return fmt.Errorf("unsupported app type '%s': must be 'docker', 'systemd', or 'cron'", app.AppType)
	}

	if err := ValidateSelector(app.Selector); err != nil {
		return err
	}

	if strings.TrimSpace(app.MainContent) == "" {
		if appType == "docker" {
			return fmt.Errorf("docker compose file content cannot be empty")
		} else if appType == "systemd" {
			return fmt.Errorf("systemd service unit content cannot be empty")
		}
		return fmt.Errorf("cron expression and command cannot be empty")
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
// If node is nil but nodeName is provided, name-only selectors can still be
// evaluated. Selectors relying on node tags or environment variables require
// the full node definition.
func AppTargetsNode(app App, node *Node, nodeName string) bool {
	if len(app.Selector) == 0 {
		return true
	}
	if node == nil {
		return nodeMatchesSelector(nil, nodeName, app.Selector)
	}
	if nodeName == "" {
		nodeName = node.Name
	}
	return nodeMatchesSelector(node, nodeName, app.Selector)
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
