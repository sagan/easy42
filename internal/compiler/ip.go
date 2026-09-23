package compiler

import (
	"fmt"
	"hash/fnv"
	"net"
	"strconv"
	"strings"
)

// DeriveIPv6LinkLocal converts an IPv4 address like "192.168.100.10" to "fe80::192:168:100:10/64"
func DeriveIPv6LinkLocal(ipv4Str string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(ipv4Str))
	if ip == nil {
		return "", fmt.Errorf("invalid IPv4 address: %s", ipv4Str)
	}

	ipv4 := ip.To4()
	if ipv4 == nil {
		return "", fmt.Errorf("not an IPv4 address: %s", ipv4Str)
	}

	// Format as fe80::<a1>:<a2>:<a3>:<a4>/64 using hex or decimal standard
	// README explicitly uses fe80::192:168:100:10/64 or fe80::c0a8:640a/64
	// In IPv6 notation: fe80::<hex1>:<hex2>/64 or dotted-quad fe80::192.168.100.10/64
	// WireGuard / Linux supports standard IPv6: fe80::c0a8:640a/64 or fe80::192.168.100.10/64
	// We format as fe80::%x:%x/64 which is 100% standard RFC IPv6
	part1 := uint16(ipv4[0])<<8 | uint16(ipv4[1])
	part2 := uint16(ipv4[2])<<8 | uint16(ipv4[3])

	return fmt.Sprintf("fe80::%x:%x/64", part1, part2), nil
}

// DeriveIPv6LinkLocalAddressOnly returns the IPv6 link-local address without prefix
func DeriveIPv6LinkLocalAddressOnly(ipv4Str string) (string, error) {
	cidr, err := DeriveIPv6LinkLocal(ipv4Str)
	if err != nil {
		return "", err
	}
	parts := strings.Split(cidr, "/")
	return parts[0], nil
}

// DeriveIPv4LinkLocal derives a 169.254.X.X/32 link-local address from a peer's main IP and link index using a deterministic hash function.
// If multiple links exist between the local and remote node, linkIndex (0, 1, 2...) ensures distinct addresses.
// The X.X octets are safely within RFC 3927 link-local range (169.254.1.1 to 169.254.254.254).
func DeriveIPv4LinkLocal(peerIP string, linkIndex ...int) (string, error) {
	clean := strings.TrimSpace(peerIP)
	if idx := strings.Index(clean, "/"); idx != -1 {
		clean = clean[:idx]
	}
	if clean == "" {
		return "", fmt.Errorf("empty IP address")
	}

	idx := 0
	if len(linkIndex) > 0 && linkIndex[0] > 0 {
		idx = linkIndex[0]
	}

	h := fnv.New32a()
	if idx > 0 {
		fmt.Fprintf(h, "%s#%d", clean, idx)
	} else {
		h.Write([]byte(clean))
	}
	sum := h.Sum32()

	// RFC 3927 allocates 169.254.0.0/16, reserving 169.254.0.x and 169.254.255.x.
	// Host addresses range from 169.254.1.1 to 169.254.254.254.
	x1 := 1 + int((sum>>8)%254)
	x2 := 1 + int(sum%254)

	return fmt.Sprintf("169.254.%d.%d/32", x1, x2), nil
}

// DeriveIPv4LinkLocalAddressOnly returns the IPv4 link-local address without prefix
func DeriveIPv4LinkLocalAddressOnly(peerIP string, linkIndex ...int) (string, error) {
	cidr, err := DeriveIPv4LinkLocal(peerIP, linkIndex...)
	if err != nil {
		return "", err
	}
	parts := strings.Split(cidr, "/")
	return parts[0], nil
}

// DerivePortFromIP derives a default WireGuard listen port based on peer IP:
// 20000 + hash(other_end_peer_ip) % 10000 (range 20000-29999).
// It uses FNV-1a 32-bit hash for an even, random, and deterministic distribution.
func DerivePortFromIP(ip string) int {
	clean := strings.TrimSpace(ip)
	if clean == "" {
		return 20000
	}
	h := fnv.New32a()
	h.Write([]byte(clean))
	return 20000 + int(h.Sum32()%10000)
}

// DerivePortFromASN extracts the last 5 digits of an ASN (e.g. 4224420001 -> 20001).
// Deprecated: use DerivePortFromIP instead.
func DerivePortFromASN(asn uint64) int {
	last5 := int(asn % 100000)
	if last5 >= 1024 && last5 <= 65535 {
		return last5
	}
	// Fallback in 20000-29999 range
	return 20000 + int(asn%10000)
}

// FormatHostPort formats an IP (v4 or v6) or domain with a port
func FormatHostPort(host string, port int) string {
	host = strings.TrimSpace(host)
	// Check if IPv6
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return fmt.Sprintf("[%s]:%d", host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// ParsePortRange parses a range string like "2000-2999" into start and end
func ParsePortRange(rangeStr string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(rangeStr), "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid port range: %s", rangeStr)
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	return start, end, nil
}
