package compiler

import (
	"fmt"
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
