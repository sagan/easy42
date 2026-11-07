package engine

import (
	"strings"

	"easy42/internal/apps"
	"easy42/internal/config"
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

// NodeMatchesTaskSelector reports whether a node satisfies every condition in a
// task selector. An empty selector matches every node.
//
// Supported selector keys:
//   - name: comma-separated whitelist of node names.
//   - tag: comma-separated whitelist of node tags; the node must have at least
//     one of them.
//   - env:<NAME>: the node metadata/environment variable must equal the value.
//     This includes built-ins such as NODE_IP/NODE_IP6 and NODE_DATA_* values
//     extracted from the node Note JSON block.
func NodeMatchesTaskSelector(n config.Node, selector map[string]string) bool {
	if len(selector) == 0 {
		return true
	}

	contextVars := apps.BuildNodeContextVars(n)
	nodeName := strings.TrimSpace(n.Name)

	nodeTags := make([]string, 0, len(n.Tags))
	for _, t := range n.Tags {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			nodeTags = append(nodeTags, trimmed)
		}
	}

	for rawKey, rawValue := range selector {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return false
		}

		switch {
		case strings.EqualFold(key, "name"):
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
			// accidentally widens the task scope.
			return false
		}
	}

	return true
}
