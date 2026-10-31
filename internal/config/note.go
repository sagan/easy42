package config

import (
	"encoding/json"
	"regexp"
	"strings"
)

// noteCodeBlockRegex matches markdown fenced code blocks:
// ``` (optional lang) content ```
// Group 1 captures the optional language identifier, group 2 captures the inner content.
var noteCodeBlockRegex = regexp.MustCompile("(?i)```+([a-zA-Z0-9_-]*)?[ \\t]*(?:\\r?\\n)?([\\s\\S]*?)```+")

// ExtractNoteData extracts structured JSON information from the first found
// "```" or "```json" block in the Note content.
// Parsing is best effort: any error is ignored and an empty map is returned.
func ExtractNoteData(note string) map[string]any {
	trimmed := strings.TrimSpace(note)
	if trimmed == "" {
		return map[string]any{}
	}

	matches := noteCodeBlockRegex.FindAllStringSubmatch(note, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		lang := strings.ToLower(strings.TrimSpace(match[1]))
		// Only consider blocks that have no language ("```") or "json" ("```json")
		if lang != "" && lang != "json" {
			continue
		}

		body := strings.TrimSpace(match[2])
		if body == "" {
			return map[string]any{}
		}

		var data map[string]any
		if err := json.Unmarshal([]byte(body), &data); err == nil && data != nil {
			return data
		}
		// The first found ``` or ```json block was encountered; best-effort attempt finished.
		return map[string]any{}
	}

	return map[string]any{}
}

// Data returns the structured JSON data extracted from Note.
func (n *Node) Data() map[string]any {
	if n == nil {
		return map[string]any{}
	}
	return ExtractNoteData(n.Note)
}

// Data returns the structured JSON data extracted from Note.
func (l *Link) Data() map[string]any {
	if l == nil {
		return map[string]any{}
	}
	return ExtractNoteData(l.Note)
}

// Data returns the structured JSON data extracted from Note.
func (e *LinkEnd) Data() map[string]any {
	if e == nil {
		return map[string]any{}
	}
	return ExtractNoteData(e.Note)
}
