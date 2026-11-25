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

// AllNodesSelectorKey marks a selector as "every managed node". The value must
// be exactly AllNodesSelectorValue and the key cannot be combined with other
// conditions. An empty or missing selector matches no node.
const (
	AllNodesSelectorKey   = "all"
	AllNodesSelectorValue = "1"
)

// AllNodesSelector returns the canonical selector that targets every managed
// node: {"all": "1"}.
func AllNodesSelector() map[string]string {
	return map[string]string{AllNodesSelectorKey: AllNodesSelectorValue}
}

// IsAllNodesSelector reports whether the selector is exactly the canonical
// all-nodes selector {"all": "1"}.
func IsAllNodesSelector(selector map[string]string) bool {
	if len(selector) != 1 {
		return false
	}
	for key, value := range selector {
		return strings.EqualFold(strings.TrimSpace(key), AllNodesSelectorKey) &&
			strings.TrimSpace(value) == AllNodesSelectorValue
	}
	return false
}

// ValidateSelector validates a node selector. The empty (or nil) selector is
// valid and matches no node; every non-empty selector must be either the
// all-nodes selector or use the supported keys. Values are not validated here
// because unknown node/tag names simply match nothing.
func ValidateSelector(selector map[string]string) error {
	if len(selector) == 0 {
		return nil
	}
	if IsAllNodesSelector(selector) {
		return nil
	}
	for rawKey, rawValue := range selector {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			return fmt.Errorf("selector key cannot be empty")
		}
		if strings.EqualFold(key, AllNodesSelectorKey) {
			// The all-nodes key is only valid on its own (handled above).
			return fmt.Errorf("invalid '%s' selector: it must be {\"%s\": \"%s\"} and cannot be combined with other conditions",
				AllNodesSelectorKey, AllNodesSelectorKey, AllNodesSelectorValue)
		}

		switch {
		case strings.EqualFold(key, "name"), strings.EqualFold(key, "tag"):
			// Supported built-in selector.
		case strings.HasPrefix(key, "env:"):
			if strings.TrimSpace(key[len("env:"):]) == "" {
				return fmt.Errorf("invalid selector key '%s': environment variable name cannot be empty", rawKey)
			}
			if strings.Contains(rawValue, ",") {
				return fmt.Errorf("invalid '%s' value: environment conditions match one exact value", rawKey)
			}
		default:
			return fmt.Errorf("invalid selector key '%s': must be 'name', 'tag', 'env:<NAME>', or the all-nodes selector {\"all\": \"1\"}", rawKey)
		}
	}
	return nil
}

// NodeMatchesSelector reports whether a node satisfies every condition in a
// selector. A nil or empty selector matches no node; the all-nodes selector
// {"all": "1"} matches every node.
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
		// An empty selector means "no node"; targeting every node requires an
		// explicit all-nodes selector.
		return false
	}
	if IsAllNodesSelector(selector) {
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

		case strings.HasPrefix(key, "env:"):
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
