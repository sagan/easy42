package compiler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// NormalizeConfig strips comments and extraneous whitespace for comparison
func NormalizeConfig(content string) string {
	lines := strings.Split(content, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Ignore comment lines (# or ;) and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		// Normalize case for keys and values
		cleaned = append(cleaned, trimmed)
	}
	return strings.Join(cleaned, "\n")
}

// NeedsUpdate compares current remote content with desired content ignoring comments and whitespace
func NeedsUpdate(currentContent, desiredContent string) bool {
	return NormalizeConfig(currentContent) != NormalizeConfig(desiredContent)
}

// GenerateDiff creates a simple line-by-line diff representation for display
func GenerateDiff(currentContent, desiredContent string) string {
	currentLines := strings.Split(strings.TrimSpace(currentContent), "\n")
	desiredLines := strings.Split(strings.TrimSpace(desiredContent), "\n")

	if len(currentLines) == 1 && currentLines[0] == "" {
		currentLines = nil
	}

	var sb strings.Builder
	if len(currentLines) == 0 {
		sb.WriteString("--- /dev/null\n+++ desired\n")
		for _, l := range desiredLines {
			sb.WriteString(fmt.Sprintf("+ %s\n", l))
		}
		return sb.String()
	}

	sb.WriteString("--- current\n+++ desired\n")
	currMap := make(map[string]bool)
	for _, l := range currentLines {
		currMap[strings.TrimSpace(l)] = true
	}
	desMap := make(map[string]bool)
	for _, l := range desiredLines {
		desMap[strings.TrimSpace(l)] = true
	}

	for _, l := range currentLines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !desMap[trimmed] {
			sb.WriteString(fmt.Sprintf("- %s\n", l))
		}
	}

	for _, l := range desiredLines {
		trimmed := strings.TrimSpace(l)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !currMap[trimmed] {
			sb.WriteString(fmt.Sprintf("+ %s\n", l))
		}
	}

	return sb.String()
}

// extractWgRestartDirectives extracts directives from the [Interface] section of a WireGuard config
// that cannot be dynamically applied via wg syncconf and require an interface restart (wg-quick down/up).
func extractWgRestartDirectives(content string) map[string][]string {
	result := make(map[string][]string)
	restartKeys := map[string]bool{
		"mtu":        true,
		"address":    true,
		"table":      true,
		"dns":        true,
		"preup":      true,
		"postup":     true,
		"predown":    true,
		"postdown":   true,
		"saveconfig": true,
	}

	lines := strings.Split(content, "\n")
	currentSection := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			currentSection = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
			continue
		}
		if currentSection != "interface" {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			k := strings.ToLower(strings.TrimSpace(parts[0]))
			v := strings.TrimSpace(parts[1])
			if restartKeys[k] {
				// Normalize comma-separated addresses or dns
				if k == "address" || k == "dns" {
					items := strings.Split(v, ",")
					for _, item := range items {
						trimmedItem := strings.TrimSpace(item)
						if trimmedItem != "" {
							result[k] = append(result[k], trimmedItem)
						}
					}
				} else {
					result[k] = append(result[k], v)
				}
			}
		}
	}

	// Sort address and dns slices for stable comparison
	for _, k := range []string{"address", "dns"} {
		if slice, ok := result[k]; ok && len(slice) > 1 {
			sort.Strings(slice)
			result[k] = slice
		}
	}

	return result
}

// RequiresWgRestart determines whether changes between the current remote WireGuard config
// and the desired WireGuard config require restarting the interface (wg-quick down <iface>; wg-quick up <iface>)
// rather than dynamic reconfiguration via wg syncconf.
// Attributes like MTU, Address, Table, PreUp, PostUp cannot be applied by wg syncconf.
func RequiresWgRestart(currentContent, desiredContent string) bool {
	if strings.TrimSpace(currentContent) == "" {
		// If current content is empty or unknown, restart to ensure correct initialization
		return true
	}

	currDirectives := extractWgRestartDirectives(currentContent)
	desDirectives := extractWgRestartDirectives(desiredContent)

	allKeys := make(map[string]bool)
	for k := range currDirectives {
		allKeys[k] = true
	}
	for k := range desDirectives {
		allKeys[k] = true
	}

	for k := range allKeys {
		cVals := currDirectives[k]
		dVals := desDirectives[k]
		if len(cVals) != len(dVals) {
			return true
		}
		for i := range cVals {
			if cVals[i] != dVals[i] {
				return true
			}
		}
	}

	return false
}

// ExtractWgMTU extracts the MTU integer value from the [Interface] section of a WireGuard config.
// Returns 0 if not specified or invalid.
func ExtractWgMTU(content string) int {
	lines := strings.Split(content, "\n")
	currentSection := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			currentSection = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
			continue
		}
		if currentSection != "interface" {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			k := strings.ToLower(strings.TrimSpace(parts[0]))
			if k == "mtu" {
				val := strings.TrimSpace(parts[1])
				if mtu, err := strconv.Atoi(val); err == nil {
					return mtu
				}
			}
		}
	}
	return 0
}
