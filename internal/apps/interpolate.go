package apps

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/fluxcd/pkg/envsubst"
	"github.com/fluxcd/pkg/envsubst/parse"

	"easy42/internal/config"
)

var (
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
// - NODE_TAGS: The comma separated and sorted (alphabetical order) node tags list
// - NODE_TAG_*: The flag env which is set for each node tag (e.g. NODE_TAG_VPS=1)
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

// escapeDollarInText preserves escaped dollars in literal text that would otherwise be parsed as substitutions.
func escapeDollarInText(s string) string {
	return strings.ReplaceAll(s, "${", "$${")
}

// prepareTemplate inspects the AST from envsubst/parse and escapes any bare undefined variables
// so that envsubst preserves them (e.g. ${UNKNOWN_TAG}) instead of expanding them to empty or failing.
func prepareTemplate(n parse.Node, lookup func(string) (string, bool), b *strings.Builder) {
	switch node := n.(type) {
	case *parse.TextNode:
		b.WriteString(escapeDollarInText(node.Value))
	case *parse.ListNode:
		for _, child := range node.Nodes {
			prepareTemplate(child, lookup, b)
		}
	case *parse.FuncNode:
		_, exists := lookup(node.Param)
		if node.Name == "" && !exists {
			// Bare undefined variable -> escape so envsubst outputs literal ${PARAM}
			b.WriteString("$${" + node.Param + "}")
			return
		}

		b.WriteString("${" + node.Param)
		if node.Name != "" {
			b.WriteString(node.Name)
			for i, arg := range node.Args {
				if node.Name == ":" && i > 0 {
					b.WriteString(":")
				} else if (node.Name == "/" || node.Name == "//") && i > 0 {
					b.WriteString("/")
				}
				prepareTemplate(arg, lookup, b)
			}
		}
		b.WriteString("}")
	}
}

// interpolateEnvsubst expands ${VAR} and bash parameter expansions (nested variables, defaults,
// case conversion, slicing, pattern substitution) using github.com/fluxcd/pkg/envsubst.
func interpolateEnvsubst(content string, lookup func(string) (string, bool)) string {
	if content == "" {
		return ""
	}

	tree, err := parse.Parse(content)
	if err != nil {
		return content
	}

	var b strings.Builder
	prepareTemplate(tree.Root, lookup, &b)
	prepared := b.String()

	res, err := envsubst.Eval(prepared, lookup)
	if err != nil {
		return content
	}
	return res
}

// InterpolateVariables substitutes variables into content using globalVars and contextVars.
// It supports {{VAR}} mustache syntax and full bash-compatible ${VAR} expansion via github.com/fluxcd/pkg/envsubst
// (including default fallbacks, nested expressions like ${VAR:-${DEFAULT}}, slicing, and pattern replacements).
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

	// 2. Substitute ${VAR} via fluxcd/pkg/envsubst (supports nested variables and bash expansions)
	return interpolateEnvsubst(res, lookup)
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

// SplitCronLine splits a single cron line into schedule and command.
// Supports standard 5-field cron, @reboot/@daily special schedules, and commented lines (#).
func SplitCronLine(line string) (schedule string, cmd string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", ""
	}

	prefix := ""
	if strings.HasPrefix(trimmed, "#") {
		prefix = "#"
		trimmed = strings.TrimSpace(trimmed[1:])
		if trimmed == "" {
			return prefix, ""
		}
	}

	if strings.HasPrefix(trimmed, "@") {
		idx := strings.IndexAny(trimmed, " \t")
		if idx == -1 {
			return prefix + trimmed, ""
		}
		schedule = prefix + trimmed[:idx]
		cmd = strings.TrimSpace(trimmed[idx+1:])
		return schedule, cmd
	}

	fields := strings.Fields(trimmed)
	if len(fields) <= 5 {
		return prefix + trimmed, ""
	}

	count := 0
	inField := false
	cmdStartIdx := -1
	for i, r := range trimmed {
		isSpace := r == ' ' || r == '\t'
		if !isSpace && !inField {
			inField = true
			count++
			if count == 6 {
				cmdStartIdx = i
				break
			}
		} else if isSpace && inField {
			inField = false
		}
	}

	if cmdStartIdx != -1 {
		schedule = prefix + strings.TrimSpace(trimmed[:cmdStartIdx])
		cmd = trimmed[cmdStartIdx:]
	} else {
		schedule = prefix + strings.Join(fields[:5], " ")
		cmd = strings.Join(fields[5:], " ")
	}
	return schedule, cmd
}

// InterpolateCronLine interpolates variables into the cmdline part of a cron job line,
// preserving the cron schedule.
func InterpolateCronLine(line string, globalVars map[string]string, contextVars map[string]string) string {
	schedule, cmd := SplitCronLine(line)
	if cmd == "" {
		return line
	}
	interpolatedCmd := InterpolateVariables(cmd, globalVars, contextVars)
	if schedule == "" {
		return interpolatedCmd
	}
	return schedule + " " + interpolatedCmd
}

// InterpolateCronMainContent interpolates variables into the cmdline of each cron line in content.
func InterpolateCronMainContent(content string, globalVars map[string]string, contextVars map[string]string) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			lines[i] = InterpolateCronLine(line, globalVars, contextVars)
		}
	}
	return strings.Join(lines, "\n")
}

// InterpolateApp creates a copy of the app with variables interpolated in EnvContent, Files, and for cron apps, the command line.
// Systemd unit and Docker Compose manifests (MainContent) are excluded to prevent conflicts with their native ${VAR} variable interpolation syntax.
func InterpolateApp(app config.App, globalVars map[string]string, contextVars map[string]string) config.App {
	interpolated := app
	if app.URL != "" {
		interpolated.URL = InterpolateVariables(app.URL, globalVars, contextVars)
	}
	interpolated.EnvContent = InterpolateVariables(app.EnvContent, globalVars, contextVars)

	if len(app.Files) > 0 {
		interpolatedFiles := make(map[string]string, len(app.Files))
		for k, v := range app.Files {
			interpolatedFiles[k] = InterpolateVariables(v, globalVars, contextVars)
		}
		interpolated.Files = interpolatedFiles
	}

	if strings.ToLower(app.AppType) == "cron" {
		interpolated.MainContent = InterpolateCronMainContent(app.MainContent, globalVars, contextVars)
	}

	return interpolated
}
