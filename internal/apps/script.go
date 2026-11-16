package apps

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
)

var scriptEnvBlockRegex = regexp.MustCompile(`(?s)\n*### EASY42_ENV_START\n.*?\n### EASY42_ENV_END\n*`)

type scriptEnvVar struct {
	key string
	val string
}

// ParseScriptEnvVars extracts key-value pairs from .env content preserving order
func ParseScriptEnvVars(envContent string) []scriptEnvVar {
	var vars []scriptEnvVar
	scanner := bufio.NewScanner(strings.NewReader(envContent))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(line[7:])
		}
		eqIdx := strings.Index(line, "=")
		if eqIdx == -1 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])
		if (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) ||
			(strings.HasPrefix(val, `'`) && strings.HasSuffix(val, `'`)) {
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
		}
		if key != "" {
			vars = append(vars, scriptEnvVar{key: key, val: val})
		}
	}
	return vars
}

// InjectScriptEnvironment injects the rendered environment block into the script content.
// It inserts after the initial shebang line (#!) or prepends #!/bin/sh if missing.
// If the shebang invokes python (e.g. #!/usr/bin/env python / python3), python os.environ syntax is used;
// otherwise standard shell export syntax is used.
func InjectScriptEnvironment(mainContent string, envContent string) string {
	// 1. Strip any existing EASY42_ENV block
	cleaned := scriptEnvBlockRegex.ReplaceAllString(mainContent, "\n")
	cleaned = strings.TrimRight(cleaned, " \t\r\n")

	envVars := ParseScriptEnvVars(envContent)
	if len(envVars) == 0 {
		if !strings.HasPrefix(strings.TrimSpace(cleaned), "#!") {
			return "#!/bin/sh\n\n" + strings.TrimLeft(cleaned, "\r\n") + "\n"
		}
		return cleaned + "\n"
	}

	lines := strings.Split(cleaned, "\n")
	firstLine := ""
	if len(lines) > 0 {
		firstLine = strings.TrimSpace(lines[0])
	}

	hasShebang := strings.HasPrefix(firstLine, "#!")
	isPython := hasShebang && (strings.HasPrefix(firstLine, "#!/usr/bin/env python") ||
		strings.HasPrefix(firstLine, "#!/usr/bin/python"))

	var envBlock strings.Builder
	if isPython {
		envBlock.WriteString("### EASY42_ENV_START\nimport os\n")
		for _, v := range envVars {
			escaped := strings.ReplaceAll(v.val, `\`, `\\`)
			escaped = strings.ReplaceAll(escaped, `'`, `\'`)
			envBlock.WriteString(fmt.Sprintf("os.environ['%s'] = '%s'\n", v.key, escaped))
		}
		envBlock.WriteString("### EASY42_ENV_END")
	} else {
		envBlock.WriteString("### EASY42_ENV_START\n")
		for _, v := range envVars {
			if strings.ContainsAny(v.val, " \t\n\"'$`\\") {
				escaped := strings.ReplaceAll(v.val, "'", `'\''`)
				envBlock.WriteString(fmt.Sprintf("export %s='%s'\n", v.key, escaped))
			} else {
				envBlock.WriteString(fmt.Sprintf("export %s=%s\n", v.key, v.val))
			}
		}
		envBlock.WriteString("### EASY42_ENV_END")
	}

	blockStr := envBlock.String()

	if hasShebang {
		shebang := lines[0]
		remaining := strings.TrimLeft(strings.Join(lines[1:], "\n"), "\r\n")
		if remaining == "" {
			return fmt.Sprintf("%s\n\n%s\n", shebang, blockStr)
		}
		return fmt.Sprintf("%s\n\n%s\n\n%s\n", shebang, blockStr, remaining)
	}

	// No shebang, default to #!/bin/sh
	remaining := strings.TrimLeft(cleaned, "\r\n")
	if remaining == "" {
		return fmt.Sprintf("#!/bin/sh\n\n%s\n", blockStr)
	}
	return fmt.Sprintf("#!/bin/sh\n\n%s\n\n%s\n", blockStr, remaining)
}
