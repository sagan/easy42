package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const (
	HostsStartMarker = "### EASY42_START"
	HostsEndMarker   = "### EASY42_END"
)

// HostRecord represents a cluster node's name and main IPv4 address
type HostRecord struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
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
			sb.WriteString(fmt.Sprintf("%s %s %s.internal\n", ip, name, name))
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
