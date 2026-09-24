package compiler

import (
	"bytes"
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

// compareIPs orders two IPs deterministically (by IP byte values if parseable, or lexicographically).
func compareIPs(ip1, ip2 string) int {
	p1 := net.ParseIP(ip1)
	p2 := net.ParseIP(ip2)
	if p1 != nil && p2 != nil {
		p1v4 := p1.To4()
		p2v4 := p2.To4()
		if p1v4 != nil && p2v4 != nil {
			if c := bytes.Compare(p1v4, p2v4); c != 0 {
				return c
			}
		} else if p1v4 == nil && p2v4 == nil {
			if c := bytes.Compare(p1.To16(), p2.To16()); c != 0 {
				return c
			}
		}
	}
	return strings.Compare(ip1, ip2)
}

// DeriveIPv4LinkLocal derives a deterministic pair of 169.254.X.X/32 link-local addresses
// from both nodes' main IPs and an optional link index.
// The derivation is deterministic: the two nodes' main IPs are sorted, and the hash material
// is the sorted two nodes' main IP and link index (e.g. "<ipLow>-<ipHigh>#<linkIndex>").
// The node corresponding to node1IP receives the first address in the returned pair,
// and the node corresponding to node2IP receives the second address.
// The returned addresses form a consecutive /30 pair (4*k + 1 and 4*k + 2) safely within RFC 3927.
func DeriveIPv4LinkLocal(node1IP, node2IP string, linkIndex ...int) (string, string, error) {
	clean1 := strings.TrimSpace(node1IP)
	if idx := strings.Index(clean1, "/"); idx != -1 {
		clean1 = clean1[:idx]
	}
	clean2 := strings.TrimSpace(node2IP)
	if idx := strings.Index(clean2, "/"); idx != -1 {
		clean2 = clean2[:idx]
	}
	if clean1 == "" || clean2 == "" {
		return "", "", fmt.Errorf("empty IP address")
	}

	cmp := compareIPs(clean1, clean2)
	var sorted1, sorted2 string
	isFirstLow := true
	if cmp <= 0 {
		sorted1, sorted2 = clean1, clean2
		isFirstLow = true
	} else {
		sorted1, sorted2 = clean2, clean1
		isFirstLow = false
	}

	idx := 0
	if len(linkIndex) > 0 && linkIndex[0] > 0 {
		idx = linkIndex[0]
	}

	h := fnv.New32a()
	fmt.Fprintf(h, "%s-%s#%d", sorted1, sorted2, idx)
	sum := h.Sum32()

	// RFC 3927 allocates 169.254.0.0/16, reserving 169.254.0.x and 169.254.255.x.
	// Host addresses range from 169.254.1.1 to 169.254.254.254.
	// We allocate a /30 subnet pair (4*k + 1 and 4*k + 2) in 169.254.x1.0/24.
	x1 := 1 + int((sum>>8)%254)
	base := int(sum%64) * 4
	host1 := base + 1
	host2 := base + 2

	addrLow := fmt.Sprintf("169.254.%d.%d/32", x1, host1)
	addrHigh := fmt.Sprintf("169.254.%d.%d/32", x1, host2)

	if isFirstLow {
		return addrLow, addrHigh, nil
	}
	return addrHigh, addrLow, nil
}

// DeriveIPv4LinkLocalPair is an alias for DeriveIPv4LinkLocal returning the pair of addresses.
func DeriveIPv4LinkLocalPair(node1IP, node2IP string, linkIndex ...int) (string, string, error) {
	return DeriveIPv4LinkLocal(node1IP, node2IP, linkIndex...)
}

// DeriveIPv4LinkLocalAddressOnly returns the pair of IPv4 link-local addresses without CIDR prefix.
func DeriveIPv4LinkLocalAddressOnly(node1IP, node2IP string, linkIndex ...int) (string, string, error) {
	c1, c2, err := DeriveIPv4LinkLocal(node1IP, node2IP, linkIndex...)
	if err != nil {
		return "", "", err
	}
	strip := func(cidr string) string {
		parts := strings.Split(cidr, "/")
		return parts[0]
	}
	return strip(c1), strip(c2), nil
}

// DeriveIPv4LinkLocalAddressOnlyPair is an alias for DeriveIPv4LinkLocalAddressOnly.
func DeriveIPv4LinkLocalAddressOnlyPair(node1IP, node2IP string, linkIndex ...int) (string, string, error) {
	return DeriveIPv4LinkLocalAddressOnly(node1IP, node2IP, linkIndex...)
}

// DeriveNodeIPv4LinkLocal derives the 169.254.X.X/32 link-local address for selfIP in the link with peerIP.
func DeriveNodeIPv4LinkLocal(selfIP, peerIP string, linkIndex ...int) (string, error) {
	selfAddr, _, err := DeriveIPv4LinkLocal(selfIP, peerIP, linkIndex...)
	return selfAddr, err
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
