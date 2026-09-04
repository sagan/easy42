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
