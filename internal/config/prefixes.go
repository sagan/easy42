package config

import (
	"strings"
)

// CleanPrefixes normalizes and repairs prefix lists.
// It repairs prefixes that were improperly split at commas within BIRD brace syntax (such as "172.20.0.0/14{21" and "29}"),
// splits items that contain multiple prefixes separated by commas or newlines (outside of braces),
// strips extraneous spaces within braces (e.g. {21, 29} -> {21,29}), and removes empty entries.
func CleanPrefixes(prefixes []string) []string {
	if len(prefixes) == 0 {
		return nil
	}

	// 1. Repair fragments where braces were split across multiple array elements.
	var repaired []string
	var current strings.Builder
	inBrace := false

	for _, p := range prefixes {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}

		if inBrace {
			current.WriteByte(',')
			current.WriteString(trimmed)
			if strings.Contains(trimmed, "}") {
				inBrace = false
				repaired = append(repaired, current.String())
				current.Reset()
			}
		} else {
			hasOpen := strings.Contains(trimmed, "{")
			hasClose := strings.Contains(trimmed, "}")
			if hasOpen && !hasClose {
				inBrace = true
				current.WriteString(trimmed)
			} else {
				repaired = append(repaired, trimmed)
			}
		}
	}
	if current.Len() > 0 {
		repaired = append(repaired, current.String())
	}

	// 2. For each repaired item, split if it contains comma or newline outside of braces,
	// and remove spaces inside braces.
	var result []string
	for _, item := range repaired {
		var token strings.Builder
		tokenInBrace := false
		for i := 0; i < len(item); i++ {
			c := item[i]
			switch c {
			case '{':
				tokenInBrace = true
				token.WriteByte(c)
			case '}':
				tokenInBrace = false
				token.WriteByte(c)
			case ' ', '\t':
				if !tokenInBrace {
					token.WriteByte(c)
				}
				// Skip space inside { ... } to normalize {21, 29} -> {21,29}
			case ',', '\n', '\r':
				if !tokenInBrace {
					tok := strings.TrimSpace(token.String())
					if tok != "" {
						result = append(result, tok)
					}
					token.Reset()
				} else {
					token.WriteByte(c)
				}
			default:
				token.WriteByte(c)
			}
		}
		tok := strings.TrimSpace(token.String())
		if tok != "" {
			result = append(result, tok)
		}
	}

	return result
}
