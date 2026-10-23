package config

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ValidInterfaceNameRegex enforces valid Linux network interface naming (max 15 chars)
	ValidInterfaceNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,15}$`)

	// RFC 1035 / RFC 1123 domain name regex
	domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

	// OpenSSH host alias regex (alphanumeric, dots, hyphens, underscores)
	sshAliasRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

	// Unix username regex for SSH
	sshUserRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

	// Telegram Bot Token regex: <bot_id>:<token>
	telegramBotTokenRegex = regexp.MustCompile(`^[0-9]{5,16}:[a-zA-Z0-9_-]{10,64}$`)

	// BIRD prefix range regex: {min,max}
	birdPrefixRangeRegex = regexp.MustCompile(`^\{[0-9]{1,3},[0-9]{1,3}\}$`)
)

// IsValidNodeName validates node names (1-64 characters, [a-zA-Z0-9_-])
func IsValidNodeName(name string) bool {
	n := strings.TrimSpace(name)
	return len(n) >= 1 && len(n) <= 64 && ValidNodeNameRegex.MatchString(n)
}

// IsValidInterfaceName validates interface names (1-15 characters, [a-zA-Z0-9_-])
func IsValidInterfaceName(iface string) bool {
	i := strings.TrimSpace(iface)
	return len(i) >= 1 && len(i) <= 15 && ValidInterfaceNameRegex.MatchString(i)
}

// IsValidIPv4 checks if string is a valid IPv4 address (supports optional /prefix suffix)
func IsValidIPv4(ipStr string) bool {
	s := strings.TrimSpace(ipStr)
	if s == "" {
		return false
	}
	if idx := strings.Index(s, "/"); idx != -1 {
		s = strings.TrimSpace(s[:idx])
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}

// IsValidIPv6 checks if string is a valid IPv6 address (supports optional /prefix suffix)
func IsValidIPv6(ipStr string) bool {
	s := strings.TrimSpace(ipStr)
	if s == "" {
		return false
	}
	if idx := strings.Index(s, "/"); idx != -1 {
		s = strings.TrimSpace(s[:idx])
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() == nil
}

// IsValidIP checks if string is a valid IPv4 or IPv6 address
func IsValidIP(ipStr string) bool {
	return IsValidIPv4(ipStr) || IsValidIPv6(ipStr)
}

// IsValidCIDR checks if string is a valid IPv4 or IPv6 CIDR prefix or BIRD prefix pattern (e.g. 172.20.0.0/14{21,29} or 172.20.0.0/14+)
func IsValidCIDR(cidrStr string) bool {
	s := strings.TrimSpace(cidrStr)
	if s == "" {
		return false
	}
	base := s
	if idx := strings.IndexAny(s, "{+-"); idx != -1 {
		suffix := s[idx:]
		base = s[:idx]
		if suffix != "+" && suffix != "-" {
			if !birdPrefixRangeRegex.MatchString(suffix) {
				return false
			}
		}
	}
	_, _, err := net.ParseCIDR(base)
	return err == nil
}

// IsValidDomain validates whether a string conforms to a valid domain / hostname
func IsValidDomain(domain string) bool {
	d := strings.TrimSpace(domain)
	if d == "" || len(d) > 253 {
		return false
	}
	d = strings.TrimSuffix(d, ".")
	return domainRegex.MatchString(d)
}

// IsValidHost checks if a string is a valid IP address or domain name
func IsValidHost(host string) bool {
	h := strings.TrimSpace(host)
	if h == "" {
		return false
	}
	return IsValidIP(h) || IsValidDomain(h)
}

// IsValidHostPort validates host:port, [ipv6]:port, or domain:port
func IsValidHostPort(hp string) bool {
	s := strings.TrimSpace(hp)
	if s == "" {
		return false
	}
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return false
	}
	return IsValidHost(host)
}

// IsValidSSHHost validates an SSH connection destination:
// can be host, user@host, host:port, user@host:port, or [ipv6]:port.
// Ensures no shell metacharacters, spaces, or dangerous characters.
func IsValidSSHHost(raw string) bool {
	s := strings.TrimSpace(raw)
	if s == "" || len(s) > 255 {
		return false
	}

	// Reject whitespace, quotes, and shell metacharacters
	if strings.ContainsAny(s, " \t\r\n;'\"`$<>|&\\{}()") {
		return false
	}

	// Extract optional user
	hostPart := s
	if idx := strings.LastIndex(s, "@"); idx != -1 {
		user := s[:idx]
		if !sshUserRegex.MatchString(user) {
			return false
		}
		hostPart = s[idx+1:]
	}

	if hostPart == "" {
		return false
	}

	// Check bracketed IPv6: [2001:db8::1] or [2001:db8::1]:22
	if strings.HasPrefix(hostPart, "[") {
		h, p, err := net.SplitHostPort(hostPart)
		if err == nil {
			port, err := strconv.Atoi(p)
			if err != nil || port < 1 || port > 65535 {
				return false
			}
			return IsValidIPv6(h)
		}
		if strings.HasSuffix(hostPart, "]") {
			return IsValidIPv6(hostPart[1 : len(hostPart)-1])
		}
		return false
	}

	// Check host:port
	if h, p, err := net.SplitHostPort(hostPart); err == nil {
		port, err := strconv.Atoi(p)
		if err != nil || port < 1 || port > 65535 {
			return false
		}
		return IsValidIP(h) || IsValidDomain(h) || sshAliasRegex.MatchString(h)
	}

	// Plain IP, domain, or SSH host alias
	return IsValidIP(hostPart) || IsValidDomain(hostPart) || sshAliasRegex.MatchString(hostPart)
}

// IsValidPort checks if an integer is a valid network port (1-65535)
func IsValidPort(port int) bool {
	return port >= 1 && port <= 65535
}

// IsValidASN checks if an ASN is a valid 32-bit autonomous system number
func IsValidASN(asn uint64) bool {
	return asn >= 1 && asn <= 4294967295
}

// IsValidTelegramBotToken validates Telegram bot token format (<id>:<secret>)
func IsValidTelegramBotToken(token string) bool {
	t := strings.TrimSpace(token)
	return telegramBotTokenRegex.MatchString(t)
}

// ValidateNode validates all properties of a Node configuration object
func ValidateNode(node *Node) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}

	node.Name = strings.TrimSpace(node.Name)
	if !IsValidNodeName(node.Name) {
		return fmt.Errorf("invalid node name '%s': must be 1-64 characters [a-zA-Z0-9_-]", node.Name)
	}

	if node.Interface != "" && !IsValidInterfaceName(node.Interface) {
		return fmt.Errorf("invalid interface name '%s' for node %s", node.Interface, node.Name)
	}

	if !node.IsExternal {
		// Managed node requirements
		if strings.TrimSpace(node.Host) == "" {
			return fmt.Errorf("host is required for managed node %s", node.Name)
		}
		if !IsValidSSHHost(node.Host) {
			return fmt.Errorf("invalid host '%s' for managed node %s", node.Host, node.Name)
		}

		if strings.TrimSpace(node.IP) == "" {
			return fmt.Errorf("IPv4 address is required for managed node %s", node.Name)
		}
		if !IsValidIPv4(node.IP) {
			return fmt.Errorf("invalid IPv4 address '%s' for node %s", node.IP, node.Name)
		}

		if node.IP6 != "" && !IsValidIPv6(node.IP6) {
			return fmt.Errorf("invalid IPv6 address '%s' for node %s", node.IP6, node.Name)
		}
	} else {
		// External node requirements
		if node.ExternalIP != "" && !IsValidIPv4(node.ExternalIP) {
			return fmt.Errorf("invalid external IPv4 address '%s' for node %s", node.ExternalIP, node.Name)
		}
		if node.ExternalIP6 != "" && !IsValidIPv6(node.ExternalIP6) {
			return fmt.Errorf("invalid external IPv6 address '%s' for node %s", node.ExternalIP6, node.Name)
		}
		if node.Host != "" && !IsValidSSHHost(node.Host) {
			return fmt.Errorf("invalid host '%s' for external node %s", node.Host, node.Name)
		}
	}

	if node.ASN > 0 && !IsValidASN(node.ASN) {
		return fmt.Errorf("invalid ASN %d for node %s", node.ASN, node.Name)
	}

	if node.Table < 0 || node.Table > 65535 {
		return fmt.Errorf("invalid routing table %d for node %s (must be 0-65535)", node.Table, node.Name)
	}

	for i, ep := range node.Entrypoints {
		if ep.IsNone() {
			continue
		}
		cleanIP := strings.TrimSpace(ep.IP)
		if cleanIP != "" && !IsValidHost(cleanIP) {
			return fmt.Errorf("entrypoint[%d]: invalid address '%s' for node %s", i, cleanIP, node.Name)
		}
		if ep.MTU < 0 || ep.MTU > 9000 {
			return fmt.Errorf("entrypoint[%d]: invalid MTU %d for node %s (must be 0-9000)", i, ep.MTU, node.Name)
		}
	}

	return nil
}

// ValidateNetworkSettings validates global network configuration settings
func ValidateNetworkSettings(settings *NetworkSettings) error {
	if settings == nil {
		return nil
	}

	for _, p := range settings.Prefixes {
		if !IsValidCIDR(p) {
			return fmt.Errorf("invalid network prefix CIDR: '%s'", p)
		}
	}
	for _, p := range settings.DisallowedDN42Networks {
		if !IsValidCIDR(p) {
			return fmt.Errorf("invalid disallowed network CIDR: '%s'", p)
		}
	}

	if faAddr := strings.TrimSpace(settings.FlapAlertedAddress); faAddr != "" {
		if !IsValidIP(faAddr) {
			return fmt.Errorf("invalid flapalerted_address: '%s'", faAddr)
		}
	}

	if settings.FlapAlertedPort != 0 && !IsValidPort(settings.FlapAlertedPort) {
		return fmt.Errorf("invalid flapalerted_port: %d (must be 1-65535)", settings.FlapAlertedPort)
	}

	if faURL := strings.TrimSpace(settings.FlapAlertedURL); faURL != "" {
		u, err := url.ParseRequestURI(faURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("invalid flapalerted_url: must be a valid http or https URL")
		}
	}

	return nil
}

// ValidateDNSConfig validates Cloudflare DNS integration settings
func ValidateDNSConfig(cfg *DNSConfig) error {
	if cfg == nil {
		return nil
	}
	if domain := strings.TrimSpace(cfg.BaseDomain); domain != "" {
		if !IsValidDomain(domain) {
			return fmt.Errorf("invalid DNS base_domain: '%s'", domain)
		}
	}
	return nil
}

// ValidateIntegrationsSettings validates third-party integrations (Telegram, Prometheus, Server URL)
func ValidateIntegrationsSettings(settings *IntegrationsSettings) error {
	if settings == nil {
		return nil
	}
	if settings.TelegramEnabled {
		tok := strings.TrimSpace(settings.TelegramBotToken)
		if tok != "" && !IsValidTelegramBotToken(tok) {
			return fmt.Errorf("invalid telegram_bot_token format")
		}
	}
	if sURL := strings.TrimSpace(settings.ServerURL); sURL != "" {
		u, err := url.ParseRequestURI(sURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("invalid server_url: must be a valid http or https URL")
		}
	}
	return nil
}
