package config

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

const (
	// ObfuscatorTunNameMaxLen is the maximum length of a Linux TUN interface name.
	ObfuscatorTunNameMaxLen = 15
	// ObfuscatorServiceSuffix is appended to the WireGuard interface name to form
	// the system service name (e.g. wg42peer-obfuscator).
	ObfuscatorServiceSuffix = "-obfuscator"
)

var (
	obfuscatorTunNameRegex = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)
)

// ObfuscatorServiceName returns the system service name for a WireGuard interface.
func ObfuscatorServiceName(iface string) string {
	iface = strings.TrimSpace(iface)
	if iface == "" {
		return ""
	}
	if strings.HasSuffix(iface, ObfuscatorServiceSuffix) {
		return iface
	}
	return iface + ObfuscatorServiceSuffix
}

// ValidateObfuscatorRole validates a normalized obfuscator role.
func ValidateObfuscatorRole(role string) error {
	switch NormalizeObfuscatorRole(role) {
	case ObfuscatorNone, ObfuscatorPhantunClient, ObfuscatorPhantunServer:
		return nil
	default:
		return fmt.Errorf("invalid obfuscator role %q (expected none, %s, or %s)", role, ObfuscatorPhantunClient, ObfuscatorPhantunServer)
	}
}

// ValidateObfuscatorParameterToken validates a single additional Phantun CLI argument.
// Tokens must not contain whitespace or shell metacharacters so that they remain safe
// when embedded in generated init scripts.
func ValidateObfuscatorParameterToken(token string) error {
	if token == "" {
		return fmt.Errorf("extra obfuscator argument must not be empty")
	}
	if len(token) > 256 {
		return fmt.Errorf("extra obfuscator argument is too long")
	}
	if strings.ContainsAny(token, " \t\r\n;|&$<>`\"'\\(){}[]*?!#~") {
		return fmt.Errorf("extra obfuscator argument %q contains whitespace or unsafe characters", token)
	}
	return nil
}

// ValidateObfuscatorParameters validates parameters for the given role.
func ValidateObfuscatorParameters(role string, p *ObfuscatorParameters) error {
	role = NormalizeObfuscatorRole(role)
	if role == ObfuscatorNone {
		return nil
	}
	if p == nil {
		return fmt.Errorf("%s obfuscator parameters are required", role)
	}

	validPort := func(name string, port int) error {
		if port < 1 || port > 65535 {
			return fmt.Errorf("%s must be between 1 and 65535", name)
		}
		return nil
	}

	if strings.TrimSpace(p.TunName) == "" {
		return fmt.Errorf("obfuscator tun_name is required")
	}
	if !obfuscatorTunNameRegex.MatchString(strings.TrimSpace(p.TunName)) {
		return fmt.Errorf("obfuscator tun_name %q must be 1-%d characters of letters, digits, '_', '-', or '.'", p.TunName, ObfuscatorTunNameMaxLen)
	}

	tunLocal := strings.TrimSpace(p.TunLocal)
	tunPeer := strings.TrimSpace(p.TunPeer)
	if tunLocal == "" || tunPeer == "" {
		return fmt.Errorf("obfuscator tun_local and tun_peer are required")
	}
	localIP := net.ParseIP(strings.TrimSuffix(tunLocal, "/32"))
	if localIP == nil || localIP.To4() == nil {
		return fmt.Errorf("obfuscator tun_local %q must be a valid IPv4 address", tunLocal)
	}
	peerIP := net.ParseIP(strings.TrimSuffix(tunPeer, "/32"))
	if peerIP == nil || peerIP.To4() == nil {
		return fmt.Errorf("obfuscator tun_peer %q must be a valid IPv4 address", tunPeer)
	}
	if localIP.Equal(peerIP) {
		return fmt.Errorf("obfuscator tun_local and tun_peer must differ")
	}

	for _, arg := range p.ExtraArgs {
		if err := ValidateObfuscatorParameterToken(arg); err != nil {
			return err
		}
	}

	switch role {
	case ObfuscatorPhantunClient:
		if err := validPort("obfuscator local_port", p.LocalPort); err != nil {
			return err
		}
		if err := validPort("obfuscator server_port", p.ServerPort); err != nil {
			return err
		}
	case ObfuscatorPhantunServer:
		if err := validPort("obfuscator server_port", p.ServerPort); err != nil {
			return err
		}
		if err := validPort("obfuscator wireguard_port", p.WireGuardPort); err != nil {
			return err
		}
	}
	return nil
}

// ValidateLinkObfuscator validates the obfuscator configuration of a link.
// It enforces complementary roles and agent-mode availability for both endpoints.
func ValidateLinkObfuscator(link *Link, fromNode, toNode *Node) error {
	if link == nil {
		return nil
	}
	fromRole := link.From.ObfuscatorRole()
	toRole := link.To.ObfuscatorRole()
	if fromRole == ObfuscatorNone && toRole == ObfuscatorNone {
		return nil
	}
	if err := ValidateObfuscatorRole(fromRole); err != nil {
		return fmt.Errorf("from endpoint: %w", err)
	}
	if err := ValidateObfuscatorRole(toRole); err != nil {
		return fmt.Errorf("to endpoint: %w", err)
	}
	if fromRole == ObfuscatorNone || toRole == ObfuscatorNone {
		return fmt.Errorf("obfuscator must be enabled on both link endpoints")
	}
	if ComplementObfuscatorRole(fromRole) != toRole {
		return fmt.Errorf("obfuscator roles must be complementary (%s <-> %s)", ObfuscatorPhantunClient, ObfuscatorPhantunServer)
	}
	if link.IsManual() {
		return fmt.Errorf("obfuscator is only supported for WireGuard links")
	}
	if link.From.IsManual() || link.To.IsManual() {
		return fmt.Errorf("obfuscator is only supported for WireGuard links")
	}
	if fromNode == nil || toNode == nil {
		return fmt.Errorf("obfuscator link endpoints must reference existing nodes")
	}
	if fromNode.IsExternal || toNode.IsExternal {
		return fmt.Errorf("obfuscator is only supported for links between managed nodes")
	}
	if !fromNode.IsAgentMode() || !toNode.IsAgentMode() {
		return fmt.Errorf("obfuscator requires agent mode on both nodes (%s and %s)", fromNode.Name, toNode.Name)
	}
	if strings.TrimSpace(link.From.Interface) == "" || strings.TrimSpace(link.To.Interface) == "" {
		return fmt.Errorf("obfuscator link endpoints must have interface names")
	}
	if err := ValidateObfuscatorParameters(fromRole, link.From.ObfuscatorParameters); err != nil {
		return fmt.Errorf("from endpoint: %w", err)
	}
	if err := ValidateObfuscatorParameters(toRole, link.To.ObfuscatorParameters); err != nil {
		return fmt.Errorf("to endpoint: %w", err)
	}
	return nil
}

// SanitizeObfuscatorTunName converts a node name into a safe TUN interface name.
func SanitizeObfuscatorTunName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "peer"
	}
	return out
}

// FormatObfuscatorTunName builds "tunpt<peer>" truncated to the Linux TUN name limit.
// A caller-supplied collision suffix is applied before truncation.
func FormatObfuscatorTunName(peerName string, suffix int) string {
	const prefix = "tunpt"
	base := SanitizeObfuscatorTunName(peerName)
	if base == "" {
		base = "peer"
	}
	suffixStr := ""
	if suffix > 0 {
		suffixStr = fmt.Sprintf("%d", suffix)
	}
	maxBase := ObfuscatorTunNameMaxLen - len(prefix) - len(suffixStr)
	if maxBase < 1 {
		maxBase = 1
	}
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return prefix + base + suffixStr
}
