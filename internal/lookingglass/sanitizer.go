package lookingglass

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"easy42/internal/config"
)

var (
	// Regex for valid hostname/domain
	hostnameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	// Regex for safe alphanumeric + common network symbols
	safeStringRegex = regexp.MustCompile(`^[a-zA-Z0-9_.:/=\[\]~ ,+-]+$`)
	// Disallowed shell metacharacters for safety
	dangerousCharsRegex = regexp.MustCompile(`[;&|` + "`" + `$\n\r><]`)

	// Whitelisted commands for ad-hoc execution
	whitelistedAdHocPrefixes = []string{
		"ping",
		"traceroute",
		"mtr",
		"birdc",
		"ip route",
		"ip -4 route",
		"ip -6 route",
		"ip link show",
		"ip addr show",
		"whois",
		"dig",
	}
)

// SanitizeParam validates and returns a clean parameter string according to TaskParam spec
func SanitizeParam(spec config.TaskParam, val string) (string, error) {
	val = strings.TrimSpace(val)
	if val == "" {
		if spec.Required {
			return "", fmt.Errorf("parameter '%s' is required", spec.Label)
		}
		return spec.DefaultValue, nil
	}

	// Always reject dangerous characters
	if dangerousCharsRegex.MatchString(val) {
		return "", fmt.Errorf("parameter '%s' contains invalid characters", spec.Label)
	}

	switch spec.Type {
	case config.ParamTypeNumber:
		num, err := strconv.Atoi(val)
		if err != nil {
			return "", fmt.Errorf("parameter '%s' must be an integer: %v", spec.Label, err)
		}
		if num < 0 {
			return "", fmt.Errorf("parameter '%s' cannot be negative", spec.Label)
		}
		if spec.Key == "count" && num > 30 {
			return "30", nil
		}
		if spec.Key == "max_ttl" && num > 64 {
			return "64", nil
		}
		if spec.Key == "cycles" && num > 20 {
			return "20", nil
		}
		return strconv.Itoa(num), nil

	case config.ParamTypeIPOrCIDR:
		// Check single IP
		if ip := net.ParseIP(val); ip != nil {
			return ip.String(), nil
		}
		// Check CIDR
		if _, _, err := net.ParseCIDR(val); err == nil {
			return val, nil
		}
		// Check hostname
		if hostnameRegex.MatchString(val) {
			return val, nil
		}
		return "", fmt.Errorf("parameter '%s' must be a valid IP address, CIDR, or hostname", spec.Label)

	case config.ParamTypeSelect:
		found := false
		for _, opt := range spec.Options {
			if strings.EqualFold(opt, val) {
				val = opt
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("parameter '%s' must be one of: %s", spec.Label, strings.Join(spec.Options, ", "))
		}
		return val, nil

	case config.ParamTypeBoolean:
		low := strings.ToLower(val)
		if low == "true" || low == "1" || low == "yes" {
			return "true", nil
		}
		return "false", nil

	case config.ParamTypeString:
		fallthrough
	default:
		if spec.Regex != "" {
			matched, err := regexp.MatchString(spec.Regex, val)
			if err != nil || !matched {
				return "", fmt.Errorf("parameter '%s' failed validation pattern", spec.Label)
			}
		}
		if !safeStringRegex.MatchString(val) {
			return "", fmt.Errorf("parameter '%s' contains disallowed characters", spec.Label)
		}
		return val, nil
	}
}

// BuildCommand interpolates parameters into a command template
func BuildCommand(tmpl string, params []config.TaskParam, userParams map[string]string) (string, error) {
	cmd := tmpl
	for _, p := range params {
		userVal, exists := userParams[p.Key]
		if !exists || userVal == "" {
			userVal = p.DefaultValue
		}

		cleanVal, err := SanitizeParam(p, userVal)
		if err != nil {
			return "", err
		}

		placeholder := fmt.Sprintf("{{%s}}", p.Key)
		cmd = strings.ReplaceAll(cmd, placeholder, cleanVal)
	}

	// Any leftover unpopulated {{...}} placeholders: replace with empty string if optional, or error
	re := regexp.MustCompile(`\{\{[a-zA-Z0-9_]+\}\}`)
	matches := re.FindAllString(cmd, -1)
	if len(matches) > 0 {
		for _, m := range matches {
			cmd = strings.ReplaceAll(cmd, m, "")
		}
	}

	cmd = strings.TrimSpace(cmd)
	// Collapse multiple consecutive spaces
	spaceRe := regexp.MustCompile(`\s+`)
	cmd = spaceRe.ReplaceAllString(cmd, " ")

	return cmd, nil
}

// ValidateAdHocCommand checks if an ad-hoc custom command is permissible and safe
func ValidateAdHocCommand(cmd string) error {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return fmt.Errorf("command cannot be empty")
	}

	// Must not contain shell metacharacters
	if dangerousCharsRegex.MatchString(cmd) {
		return fmt.Errorf("command contains dangerous shell metacharacters")
	}

	// Check whitelist prefixes
	matched := false
	for _, prefix := range whitelistedAdHocPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			matched = true
			break
		}
	}

	if !matched {
		return fmt.Errorf("command must start with one of: %s", strings.Join(whitelistedAdHocPrefixes, ", "))
	}

	return nil
}
