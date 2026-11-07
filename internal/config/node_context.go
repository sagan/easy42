package config

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// CleanCIDR strips the CIDR suffix from an IP address.
func CleanCIDR(ip string) string {
	ip = strings.TrimSpace(ip)
	if idx := strings.Index(ip, "/"); idx != -1 {
		return strings.TrimSpace(ip[:idx])
	}
	return ip
}

// sanitizeEnvKey converts a key string to an uppercase alphanumeric + underscore env name.
func sanitizeEnvKey(k string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(k) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// formatEnvVal formats primitive JSON values (string, bool, numbers) for variable substitution.
func formatEnvVal(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case bool:
		if val {
			return "true", true
		}
		return "false", true
	case float64:
		if val == math.Trunc(val) && !math.IsNaN(val) && !math.IsInf(val, 0) {
			return strconv.FormatInt(int64(val), 10), true
		}
		return strconv.FormatFloat(val, 'f', -1, 64), true
	case int:
		return strconv.Itoa(val), true
	case int64:
		return strconv.FormatInt(val, 10), true
	case uint64:
		return strconv.FormatUint(val, 10), true
	default:
		return "", false
	}
}

// BuildNodeContextVars constructs the complete map of node-specific internal variables
// consistent with the device helper feature (docs/device-helper.md:L153-L160):
// - NODE_NAME: node.name
// - NODE_INTERFACE: node.interface
// - NODE_IP: node.ip (CIDR suffixes are stripped before use)
// - NODE_IP6: node.ip6 (CIDR suffixes are stripped before use)
// - NODE_EXTERNAL_IP: node.external_ip (CIDR suffixes stripped, added only when different from NODE_IP)
// - NODE_EXTERNAL_IP6: node.external_ip6 (CIDR suffixes stripped, added only when different from NODE_IP6)
// - HAS_CUSTOM_SCRIPT: "1" when node.custom_network_script is non-empty, otherwise "0"
// - NODE_TAGS: The comma separated and sorted (alphabetical order) node tags list
// - NODE_TAG_*: The flag env which is set for each node tag (e.g. NODE_TAG_VPS=1)
// - NODE_DATA_*: Structured JSON data fields from node Note (up to 1 layer hierarchy)
// Also provides backward-compatible aliases:
// - NODE_IPV4 -> NODE_IP
// - NODE_IPV6 -> NODE_IP6
func BuildNodeContextVars(n Node) map[string]string {
	res := make(map[string]string)

	name := strings.TrimSpace(n.Name)
	iface := strings.TrimSpace(n.Interface)
	nodeIP := CleanCIDR(n.IP)
	nodeIP6 := CleanCIDR(n.IP6)
	nodeExtIP := CleanCIDR(n.ExternalIP)
	nodeExtIP6 := CleanCIDR(n.ExternalIP6)
	customScript := strings.TrimSpace(n.CustomNetworkScript)

	res["NODE_NAME"] = name
	res["NODE_INTERFACE"] = iface
	res["NODE_IP"] = nodeIP
	res["NODE_IP6"] = nodeIP6

	// Backward-compatible aliases
	res["NODE_IPV4"] = nodeIP
	res["NODE_IPV6"] = nodeIP6

	if nodeExtIP != "" && nodeExtIP != nodeIP {
		res["NODE_EXTERNAL_IP"] = nodeExtIP
	}

	if nodeExtIP6 != "" && nodeExtIP6 != nodeIP6 {
		res["NODE_EXTERNAL_IP6"] = nodeExtIP6
	}

	if customScript != "" {
		res["HAS_CUSTOM_SCRIPT"] = "1"
	} else {
		res["HAS_CUSTOM_SCRIPT"] = "0"
	}

	// Comma-separated sorted tags and NODE_TAG_* flag envs
	var cleanTags []string
	seenTags := make(map[string]bool)
	for _, t := range n.Tags {
		trimmed := strings.TrimSpace(t)
		if trimmed != "" && !seenTags[trimmed] {
			seenTags[trimmed] = true
			cleanTags = append(cleanTags, trimmed)
		}
	}
	sort.Strings(cleanTags)
	res["NODE_TAGS"] = strings.Join(cleanTags, ",")
	for _, t := range cleanTags {
		tagKey := sanitizeEnvKey(t)
		if tagKey != "" {
			res["NODE_TAG_"+tagKey] = "1"
		}
	}

	// Structured JSON data fields from Note (up to 1 layer hierarchy)
	noteData := ExtractNoteData(n.Note)
	if len(noteData) > 0 {
		// 1. Process Layer 0 primitive fields
		for k0, v0 := range noteData {
			s0 := sanitizeEnvKey(k0)
			if s0 == "" {
				continue
			}
			if formatted, ok := formatEnvVal(v0); ok {
				res["NODE_DATA_"+s0] = formatted
			}
		}

		// 2. Process Layer 1 nested object fields
		for k0, v0 := range noteData {
			s0 := sanitizeEnvKey(k0)
			if s0 == "" {
				continue
			}
			if subMap, ok := v0.(map[string]any); ok {
				for k1, v1 := range subMap {
					s1 := sanitizeEnvKey(k1)
					if s1 == "" {
						continue
					}
					if subFormatted, subOk := formatEnvVal(v1); subOk {
						res["NODE_DATA_"+s0+"_"+s1] = subFormatted
						aliasKey := "NODE_DATA_" + s1
						if _, exists := res[aliasKey]; !exists {
							res[aliasKey] = subFormatted
						}
					}
				}
			}
		}
	}

	return res
}
