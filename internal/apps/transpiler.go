package apps

import (
	"bufio"
	"fmt"
	"strings"
)

// ParsedServiceUnit holds the extracted fields from a systemd service unit
type ParsedServiceUnit struct {
	Description      string
	ExecStart        string
	ExecStop         string
	WorkingDirectory string
	User             string
	Group            string
	Restart          string
	Environment      map[string]string
	EnvironmentFiles []string
	RawDirectives    map[string]string
}

// ParseSystemdService parses key systemd service unit directives
func ParseSystemdService(content string) (*ParsedServiceUnit, error) {
	unit := &ParsedServiceUnit{
		Environment:   make(map[string]string),
		RawDirectives: make(map[string]string),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	currentSection := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}

		idx := strings.Index(line, "=")
		if idx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])

		unit.RawDirectives[fmt.Sprintf("%s.%s", currentSection, key)] = val

		switch strings.ToLower(key) {
		case "description":
			unit.Description = val
		case "execstart":
			unit.ExecStart = val
		case "execstop":
			unit.ExecStop = val
		case "workingdirectory":
			unit.WorkingDirectory = val
		case "user":
			unit.User = val
		case "group":
			unit.Group = val
		case "restart":
			unit.Restart = strings.ToLower(val)
		case "environment":
			// Handle Environment="FOO=bar" or Environment=FOO=bar
			clean := strings.Trim(val, `"'`)
			eqIdx := strings.Index(clean, "=")
			if eqIdx != -1 {
				k := strings.TrimSpace(clean[:eqIdx])
				v := strings.TrimSpace(clean[eqIdx+1:])
				unit.Environment[k] = v
			}
		case "environmentfile":
			unit.EnvironmentFiles = append(unit.EnvironmentFiles, strings.Trim(val, `"'`))
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading systemd unit: %w", err)
	}

	return unit, nil
}

// ParseEnvFile parses a standard KEY=VALUE .env file
func ParseEnvFile(envContent string) map[string]string {
	res := make(map[string]string)
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
		// Remove wrapping quotes if present
		if (strings.HasPrefix(val, `"`) && strings.HasSuffix(val, `"`)) ||
			(strings.HasPrefix(val, `'`) && strings.HasSuffix(val, `'`)) {
			if len(val) >= 2 {
				val = val[1 : len(val)-1]
			}
		}
		res[key] = val
	}
	return res
}

// ShellEscape quotes a string safely for POSIX shell
func ShellEscape(s string) string {
	// If it contains single quote, replace ' with '\''
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// TranspileToOpenRC generates an OpenRC init script (/etc/init.d/ezapp-<name>)
func TranspileToOpenRC(appName string, unit *ParsedServiceUnit) (string, error) {
	if unit.ExecStart == "" {
		return "", fmt.Errorf("cannot transpile to OpenRC: ExecStart is missing")
	}

	cmdParts := splitCommand(unit.ExecStart)
	if len(cmdParts) == 0 {
		return "", fmt.Errorf("invalid ExecStart directive")
	}

	cmdBin := cmdParts[0]
	cmdArgs := ""
	if len(cmdParts) > 1 {
		var escapedArgs []string
		for _, arg := range cmdParts[1:] {
			escapedArgs = append(escapedArgs, ShellEscape(arg))
		}
		cmdArgs = strings.Join(escapedArgs, " ")
	}

	var sb strings.Builder
	sb.WriteString("#!/sbin/openrc-run\n")
	sb.WriteString(fmt.Sprintf("# Easy42 OpenRC Service: ezapp-%s\n", appName))
	if unit.Description != "" {
		sb.WriteString(fmt.Sprintf("description=%s\n", ShellEscape(unit.Description)))
	} else {
		sb.WriteString(fmt.Sprintf("description=\"Easy42 managed app %s\"\n", appName))
	}

	sb.WriteString(fmt.Sprintf("command=%s\n", ShellEscape(cmdBin)))
	if cmdArgs != "" {
		sb.WriteString(fmt.Sprintf("command_args=\"%s\"\n", cmdArgs))
	}

	userGroup := ""
	if unit.User != "" && unit.Group != "" {
		userGroup = fmt.Sprintf("%s:%s", unit.User, unit.Group)
	} else if unit.User != "" {
		userGroup = unit.User
	}
	if userGroup != "" {
		sb.WriteString(fmt.Sprintf("command_user=%s\n", ShellEscape(userGroup)))
	}

	if unit.WorkingDirectory != "" {
		sb.WriteString(fmt.Sprintf("directory=%s\n", ShellEscape(unit.WorkingDirectory)))
	}

	sb.WriteString("command_background=\"yes\"\n")
	sb.WriteString(fmt.Sprintf("pidfile=\"/run/ezapp-%s.pid\"\n\n", appName))

	sb.WriteString("depend() {\n")
	sb.WriteString("    need net\n")
	sb.WriteString("    use dns logger\n")
	sb.WriteString("}\n\n")

	sb.WriteString("start_pre() {\n")
	sb.WriteString(fmt.Sprintf("    if [ -f \"/etc/conf.d/ezapp-%s\" ]; then\n", appName))
	sb.WriteString(fmt.Sprintf("        . \"/etc/conf.d/ezapp-%s\"\n", appName))
	sb.WriteString("    fi\n")
	sb.WriteString("}\n")

	return sb.String(), nil
}

// GenerateOpenRCConfD generates /etc/conf.d/ezapp-<name> with explicit export directives
func GenerateOpenRCConfD(appName string, envVars map[string]string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Easy42 environment configuration for ezapp-%s\n", appName))
	for k, v := range envVars {
		sb.WriteString(fmt.Sprintf("export %s=%s\n", k, ShellEscape(v)))
	}
	return sb.String()
}

// TranspileToProcd generates an OpenWrt procd init script (/etc/init.d/ezapp-<name>)
func TranspileToProcd(appName string, unit *ParsedServiceUnit, envVars map[string]string) (string, error) {
	if unit.ExecStart == "" {
		return "", fmt.Errorf("cannot transpile to Procd: ExecStart is missing")
	}

	cmdParts := splitCommand(unit.ExecStart)
	if len(cmdParts) == 0 {
		return "", fmt.Errorf("invalid ExecStart directive")
	}

	var sb strings.Builder
	sb.WriteString("#!/bin/sh /etc/rc.common\n")
	sb.WriteString(fmt.Sprintf("# Easy42 OpenWrt Procd Service: ezapp-%s\n\n", appName))
	sb.WriteString("USE_PROCD=1\n")
	sb.WriteString("START=95\n")
	sb.WriteString("STOP=10\n\n")

	sb.WriteString("start_service() {\n")
	sb.WriteString(fmt.Sprintf("    procd_open_instance \"ezapp-%s\"\n", appName))

	// procd_set_param command bin arg1 arg2
	var escapedCommand []string
	for _, p := range cmdParts {
		escapedCommand = append(escapedCommand, ShellEscape(p))
	}
	sb.WriteString(fmt.Sprintf("    procd_set_param command %s\n", strings.Join(escapedCommand, " ")))

	// respawn policy
	if unit.Restart == "always" || unit.Restart == "on-failure" {
		sb.WriteString("    procd_set_param respawn 3600 5 0\n")
	}

	// user
	if unit.User != "" {
		sb.WriteString(fmt.Sprintf("    procd_set_param user %s\n", ShellEscape(unit.User)))
	}

	// env vars
	if len(envVars) > 0 {
		var envParts []string
		for k, v := range envVars {
			envParts = append(envParts, fmt.Sprintf("%s=%s", k, ShellEscape(v)))
		}
		sb.WriteString(fmt.Sprintf("    procd_set_param env %s\n", strings.Join(envParts, " ")))
	}

	sb.WriteString("    procd_close_instance\n")
	sb.WriteString("}\n")

	return sb.String(), nil
}

// splitCommand parses command line taking basic quotes into account
func splitCommand(cmd string) []string {
	var res []string
	var cur strings.Builder
	inQuotes := false
	quoteChar := byte(0)

	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if inQuotes {
			if c == quoteChar {
				inQuotes = false
			} else {
				cur.WriteByte(c)
			}
		} else {
			if c == '"' || c == '\'' {
				inQuotes = true
				quoteChar = c
			} else if c == ' ' || c == '\t' {
				if cur.Len() > 0 {
					res = append(res, cur.String())
					cur.Reset()
				}
			} else {
				cur.WriteByte(c)
			}
		}
	}
	if cur.Len() > 0 {
		res = append(res, cur.String())
	}
	return res
}

// EnsureSystemdEnvironmentFile ensures that the systemd service content contains
// EnvironmentFile=/etc/systemd/system/ezapp-<appName>.env under [Service].
func EnsureSystemdEnvironmentFile(appName string, content string) string {
	appName = strings.TrimSpace(appName)
	if appName == "" {
		appName = "sample"
	}
	expectedPath := fmt.Sprintf("/etc/systemd/system/ezapp-%s.env", appName)

	if strings.Contains(content, expectedPath) {
		return content
	}

	samplePath := "/etc/systemd/system/ezapp-sample.env"
	if strings.Contains(content, samplePath) {
		return strings.Replace(content, samplePath, expectedPath, 1)
	}

	expectedDirective := fmt.Sprintf("EnvironmentFile=%s", expectedPath)

	lines := strings.Split(content, "\n")
	serviceIdx := -1
	for i, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "[Service]") {
			serviceIdx = i
			break
		}
	}

	if serviceIdx != -1 {
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:serviceIdx+1]...)
		newLines = append(newLines, expectedDirective)
		newLines = append(newLines, lines[serviceIdx+1:]...)
		return strings.Join(newLines, "\n")
	}

	trimmedContent := strings.TrimRight(content, "\r\n")
	if trimmedContent == "" {
		return fmt.Sprintf("[Service]\n%s\n", expectedDirective)
	}
	return fmt.Sprintf("%s\n\n[Service]\n%s\n", trimmedContent, expectedDirective)
}

