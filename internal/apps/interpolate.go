package apps

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"easy42/internal/config"
)

var (
	// Matches ${VAR_NAME} or ${VAR_NAME:-default_value}
	dollarBraceRegex = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)(?::-([^}]*))?\}`)

	// Matches {{VAR_NAME}} or {{.VAR_NAME}}
	mustacheRegex = regexp.MustCompile(`\{\{\s*\.?([a-zA-Z_][a-zA-Z0-9_]*)\s*\}\}`)
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
// - NODE_DATA_*: Structured JSON data fields from node Note (up to 1 layer hierarchy)
// Also provides backward-compatible aliases:
// - NODE_IPV4 -> NODE_IP
// - NODE_IPV6 -> NODE_IP6
func BuildNodeContextVars(n config.Node) map[string]string {
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

	// Structured JSON data fields from Note (up to 1 layer hierarchy)
	noteData := config.ExtractNoteData(n.Note)
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

// InterpolateVariables substitutes variables into content using globalVars and contextVars
func InterpolateVariables(content string, globalVars map[string]string, contextVars map[string]string) string {
	if content == "" {
		return ""
	}

	lookup := func(key string) (string, bool) {
		if contextVars != nil {
			if val, ok := contextVars[key]; ok {
				return val, true
			}
		}
		if globalVars != nil {
			if val, ok := globalVars[key]; ok {
				return val, true
			}
		}
		return "", false
	}

	// 1. Substitute {{VAR}} or {{.VAR}}
	res := mustacheRegex.ReplaceAllStringFunc(content, func(match string) string {
		sub := mustacheRegex.FindStringSubmatch(match)
		if len(sub) > 1 {
			k := sub[1]
			if val, found := lookup(k); found {
				return val
			}
		}
		return match
	})

	// 2. Substitute ${VAR} or ${VAR:-default}
	res = dollarBraceRegex.ReplaceAllStringFunc(res, func(match string) string {
		sub := dollarBraceRegex.FindStringSubmatch(match)
		if len(sub) > 1 {
			k := sub[1]
			val, found := lookup(k)
			if found && val != "" {
				return val
			}
			// If not found or empty and default provided
			if len(sub) > 2 && sub[2] != "" {
				return sub[2]
			}
			if found {
				return val // empty string was explicitly set
			}
		}
		return match
	})

	return res
}

// VariablesToMap converts a slice of AppVariable into a key-value map
func VariablesToMap(vars []config.AppVariable) map[string]string {
	res := make(map[string]string, len(vars))
	for _, v := range vars {
		k := strings.TrimSpace(v.Key)
		if k != "" {
			res[k] = v.Value
		}
	}
	return res
}

// InterpolateApp creates a copy of the app with variables interpolated in MainContent, EnvContent, and Files
func InterpolateApp(app config.App, globalVars map[string]string, contextVars map[string]string) config.App {
	interpolated := app
	interpolated.MainContent = InterpolateVariables(app.MainContent, globalVars, contextVars)
	interpolated.EnvContent = InterpolateVariables(app.EnvContent, globalVars, contextVars)

	if len(app.Files) > 0 {
		interpolatedFiles := make(map[string]string, len(app.Files))
		for k, v := range app.Files {
			interpolatedFiles[k] = InterpolateVariables(v, globalVars, contextVars)
		}
		interpolated.Files = interpolatedFiles
	}

	return interpolated
}
