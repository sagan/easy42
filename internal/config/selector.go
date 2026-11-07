package config

import (
	"fmt"
	"strings"
)

// splitSelectorList splits a comma-separated selector value into trimmed,
// non-empty items.
func splitSelectorList(value string) []string {
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

// ValidateSelector validates the keys of a node selector. Values are not
// validated here because empty or unknown node/tag names simply match nothing.
func ValidateSelector(selector map[string]string) error {
	for rawKey := range selector {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return fmt.Errorf("selector key cannot be empty")
		}

		switch {
		case strings.EqualFold(key, "name"), strings.EqualFold(key, "tag"):
			// Supported built-in selector.
		case strings.HasPrefix(strings.ToLower(key), "env:"):
			if strings.TrimSpace(key[len("env:"):]) == "" {
				return fmt.Errorf("invalid selector key '%s': environment variable name cannot be empty", rawKey)
			}
		default:
			return fmt.Errorf("invalid selector key '%s': must be 'name', 'tag', or 'env:<NAME>'", rawKey)
		}
	}
	return nil
}

// NodeMatchesSelector reports whether a node satisfies every condition in a
// selector. An empty selector matches every node.
//
// Supported selector keys:
//   - name: comma-separated whitelist of node names.
//   - tag: comma-separated whitelist of node tags; the node must have at least
//     one of them.
//   - env:<NAME>: the node metadata/environment variable must equal the value.
//     This includes built-ins such as NODE_IP/NODE_IP6 and NODE_DATA_* values
//     extracted from the node Note JSON block.
func NodeMatchesSelector(n Node, selector map[string]string) bool {
	return nodeMatchesSelector(&n, n.Name, selector)
}

func nodeMatchesSelector(n *Node, nodeName string, selector map[string]string) bool {
	if len(selector) == 0 {
		return true
	}

	var contextVars map[string]string
	var nodeTags []string
	if n != nil {
		contextVars = BuildNodeContextVars(*n)
		nodeTags = make([]string, 0, len(n.Tags))
		for _, t := range n.Tags {
			if trimmed := strings.TrimSpace(t); trimmed != "" {
				nodeTags = append(nodeTags, trimmed)
			}
		}
	}
	nodeName = strings.TrimSpace(nodeName)

	for rawKey, rawValue := range selector {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return false
		}

		switch {
		case strings.EqualFold(key, "name"):
			if n == nil && nodeName == "" {
				return false
			}
			items := splitSelectorList(rawValue)
			if len(items) == 0 {
				return false
			}
			matched := false
			for _, item := range items {
				if item == nodeName {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}

		case strings.EqualFold(key, "tag"):
			if n == nil {
				return false
			}
			items := splitSelectorList(rawValue)
			if len(items) == 0 {
				return false
			}
			matched := false
			for _, item := range items {
				for _, tag := range nodeTags {
					if item == tag {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			if !matched {
				return false
			}

		case strings.HasPrefix(strings.ToLower(key), "env:"):
			if n == nil {
				return false
			}
			envKey := strings.ToUpper(strings.TrimSpace(key[len("env:"):]))
			if envKey == "" {
				return false
			}
			actual, ok := contextVars[envKey]
			if !ok || actual != rawValue {
				return false
			}

		default:
			// Unknown selector keys are rejected (fail closed) so a typo never
			// accidentally widens the target scope.
			return false
		}
	}

	return true
}
