package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"sort"
	"strings"
	"unicode"
)

const (
	HostsStartMarker = "### EASY42_START"
	HostsEndMarker   = "### EASY42_END"
)

// HostRecord represents a cluster node or extra host entry with IP, name, and optional aliases
type HostRecord struct {
	Name    string   `json:"name"`
	IP      string   `json:"ip"`
	Aliases []string `json:"aliases,omitempty"`
}

// IsValidHostname validates whether a string conforms to a valid RFC 1123 / DNS hostname
func IsValidHostname(name string) bool {
	s := strings.TrimSpace(name)
	if s == "" || len(s) > 253 {
		return false
	}
	// Strip trailing dot if present
	s = strings.TrimSuffix(s, ".")
	labels := strings.Split(s, ".")
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '_' {
				return false
			}
		}
	}
	return true
}

// ParsedHostEntry represents a line parsed from extra hosts
type ParsedHostEntry struct {
	IP        string
	Hostnames []string
}

// ParseExtraHosts parses and validates raw extra hosts text (lines of "/etc/hosts" format)
func ParseExtraHosts(text string) ([]ParsedHostEntry, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var entries []ParsedHostEntry

	for lineIdx, rawLine := range lines {
		lineNum := lineIdx + 1
		line := strings.TrimSpace(rawLine)
		if idx := strings.Index(line, "#"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}

		// Split by whitespace or commas
		tokens := strings.FieldsFunc(line, func(r rune) bool {
			return unicode.IsSpace(r) || r == ','
		})
		if len(tokens) == 0 {
			continue
		}

		var ip string
		var hostnames []string

		// Check if first token is IP
		if net.ParseIP(tokens[0]) != nil {
			ip = tokens[0]
			hostnames = tokens[1:]
		} else if net.ParseIP(tokens[len(tokens)-1]) != nil {
			// In case formatted as hostname... IP
			ip = tokens[len(tokens)-1]
			hostnames = tokens[:len(tokens)-1]
		} else {
			return nil, fmt.Errorf("line %d: invalid IP address %q", lineNum, tokens[0])
		}

		if len(hostnames) == 0 {
			return nil, fmt.Errorf("line %d: missing hostname for IP %q", lineNum, ip)
		}

		var cleanHostnames []string
		seenInLine := make(map[string]bool)
		for _, h := range hostnames {
			hTrim := strings.TrimSpace(strings.Trim(h, ":,"))
			if hTrim == "" {
				continue
			}
			if !IsValidHostname(hTrim) {
				return nil, fmt.Errorf("line %d: invalid hostname %q", lineNum, hTrim)
			}
			hLower := strings.ToLower(hTrim)
			if !seenInLine[hLower] {
				seenInLine[hLower] = true
				cleanHostnames = append(cleanHostnames, hTrim)
			}
		}

		if len(cleanHostnames) == 0 {
			return nil, fmt.Errorf("line %d: no valid hostnames provided for IP %q", lineNum, ip)
		}

		entries = append(entries, ParsedHostEntry{
			IP:        ip,
			Hostnames: cleanHostnames,
		})
	}

	return entries, nil
}

// MergeHostRecords merges node extra hosts, global extra hosts, and easy42 cluster node IP hosts.
// Priority: node extra hosts (1) > global extra hosts (2) > easy42 node IP hosts (3).
// If a same hostname has different IPs from different sources only the IP of highest priority wins.
func MergeHostRecords(nodeExtraHostsText, globalExtraHostsText string, clusterNodes []Node) ([]HostRecord, error) {
	nodeEntries, err := ParseExtraHosts(nodeExtraHostsText)
	if err != nil {
		return nil, fmt.Errorf("node extra hosts: %w", err)
	}

	globalEntries, err := ParseExtraHosts(globalExtraHostsText)
	if err != nil {
		return nil, fmt.Errorf("global extra hosts: %w", err)
	}

	// Hostname -> winning IP
	type winInfo struct {
		ip       string
		priority int // 1: node extra, 2: global extra, 3: cluster node
		origName string
	}
	hostWin := make(map[string]winInfo)

	// 1. Process node extra hosts (priority 1)
	for _, entry := range nodeEntries {
		for _, h := range entry.Hostnames {
			hl := strings.ToLower(h)
			hostWin[hl] = winInfo{
				ip:       entry.IP,
				priority: 1,
				origName: h,
			}
		}
	}

	// 2. Process global extra hosts (priority 2)
	for _, entry := range globalEntries {
		for _, h := range entry.Hostnames {
			hl := strings.ToLower(h)
			if cur, exists := hostWin[hl]; !exists || cur.priority > 2 {
				hostWin[hl] = winInfo{
					ip:       entry.IP,
					priority: 2,
					origName: h,
				}
			}
		}
	}

	// 3. Process easy42 cluster nodes (priority 3)
	for _, n := range clusterNodes {
		if n.IsExternal {
			continue
		}
		ip := strings.TrimSpace(n.IP)
		name := strings.TrimSpace(n.Name)
		if ip == "" || name == "" {
			continue
		}

		// Node name
		hl := strings.ToLower(name)
		if cur, exists := hostWin[hl]; !exists || cur.priority > 3 {
			hostWin[hl] = winInfo{
				ip:       ip,
				priority: 3,
				origName: name,
			}
		}

		// Node name.internal
		internalName := name + ".internal"
		hlInt := strings.ToLower(internalName)
		if cur, exists := hostWin[hlInt]; !exists || cur.priority > 3 {
			hostWin[hlInt] = winInfo{
				ip:       ip,
				priority: 3,
				origName: internalName,
			}
		}
	}

	// Group by winning IP
	ipMap := make(map[string][]string)
	var ipOrder []string
	seenIP := make(map[string]bool)

	var allHostnames []string
	for h := range hostWin {
		allHostnames = append(allHostnames, h)
	}
	sort.Strings(allHostnames)

	for _, hl := range allHostnames {
		info := hostWin[hl]
		if !seenIP[info.ip] {
			seenIP[info.ip] = true
			ipOrder = append(ipOrder, info.ip)
		}
		ipMap[info.ip] = append(ipMap[info.ip], info.origName)
	}

	var result []HostRecord
	for _, ip := range ipOrder {
		hosts := ipMap[ip]
		if len(hosts) == 0 {
			continue
		}
		// Sort hostnames: non-internal / shorter / alphabetical
		sort.Slice(hosts, func(i, j int) bool {
			if strings.HasSuffix(hosts[i], ".internal") && !strings.HasSuffix(hosts[j], ".internal") {
				return false
			}
			if !strings.HasSuffix(hosts[i], ".internal") && strings.HasSuffix(hosts[j], ".internal") {
				return true
			}
			return hosts[i] < hosts[j]
		})

		primary := hosts[0]
		var aliases []string
		if len(hosts) > 1 {
			aliases = hosts[1:]
		}

		result = append(result, HostRecord{
			Name:    primary,
			IP:      ip,
			Aliases: aliases,
		})
	}

	// Sort result by primary Name ascending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// BuildHostsBlock assembles the easy42 hosts block from records, sorted by node name ascending.
// Format:
// ### EASY42_START
// 192.168.100.2 bar bar.internal
// 192.168.100.1 foo foo.internal
// ### EASY42_END
func BuildHostsBlock(records []HostRecord) string {
	sorted := make([]HostRecord, len(records))
	copy(sorted, records)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	var sb strings.Builder
	sb.WriteString(HostsStartMarker)
	sb.WriteString("\n")
	for _, r := range sorted {
		ip := strings.TrimSpace(r.IP)
		name := strings.TrimSpace(r.Name)
		if ip != "" && name != "" {
			parts := []string{name}
			for _, a := range r.Aliases {
				at := strings.TrimSpace(a)
				if at != "" && at != name {
					parts = append(parts, at)
				}
			}
			// Backward compatibility: if Aliases is nil and name has no dot, add name.internal
			if r.Aliases == nil && len(parts) == 1 && !strings.Contains(name, ".") {
				parts = append(parts, name+".internal")
			}
			sb.WriteString(fmt.Sprintf("%s %s\n", ip, strings.Join(parts, " ")))
		}
	}
	sb.WriteString(HostsEndMarker)
	sb.WriteString("\n")
	return sb.String()
}

// HashHostsBlock computes the SHA-256 hash of the normalized hosts block content.
func HashHostsBlock(block string) string {
	norm := strings.ReplaceAll(block, "\r\n", "\n")
	trimmed := strings.TrimSpace(norm)
	if trimmed == "" {
		return ""
	}
	h := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(h[:])
}

// ExtractHostsBlock extracts the easy42 block and its SHA-256 hash from hosts file content.
func ExtractHostsBlock(content string) (string, string) {
	norm := strings.ReplaceAll(content, "\r\n", "\n")
	startIdx := strings.Index(norm, HostsStartMarker)
	if startIdx == -1 {
		return "", ""
	}
	endIdx := strings.Index(norm[startIdx:], HostsEndMarker)
	if endIdx == -1 {
		return "", ""
	}
	endIdx += startIdx + len(HostsEndMarker)
	block := norm[startIdx:endIdx]
	return block, HashHostsBlock(block)
}
