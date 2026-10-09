package agent

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed embedded/*
var EmbeddedAgentFS embed.FS

// ArchDetectCommand is a shell command that accurately detects machine architecture,
// resolving MIPS endianness (little-endian mipsel vs big-endian mips) on OpenWrt and Linux.
const ArchDetectCommand = `ARCH=$(uname -m); case "$ARCH" in x86_64|amd64) echo "x86_64" ;; aarch64|arm64) echo "aarch64" ;; armv7*|armhf) echo "armv7" ;; mips*el|mipsel) echo "mipsel" ;; mips*) if (printf '\001\000' 2>/dev/null || echo -n -e '\x01\x00' 2>/dev/null) | hexdump -x 2>/dev/null | grep -q "0001"; then echo "mipsel"; elif printf '\001\000' 2>/dev/null | od -An -t x2 2>/dev/null | grep -q "0001"; then echo "mipsel"; elif [ -f /etc/openwrt_release ] && grep -qi "mipsel" /etc/openwrt_release; then echo "mipsel"; elif [ -f /etc/os-release ] && grep -qi "mipsel" /etc/os-release; then echo "mipsel"; elif od -j 5 -N 1 -An -t x1 /bin/sh 2>/dev/null | grep -q "01"; then echo "mipsel"; elif command -v lscpu >/dev/null 2>&1 && lscpu 2>/dev/null | grep -qi "Little Endian"; then echo "mipsel"; else echo "$ARCH"; fi ;; *) echo "$ARCH" ;; esac`

// NormalizeArch normalizes arch strings like amd64, arm64, armv7l, etc.
func NormalizeArch(arch string) string {
	a := strings.ToLower(strings.TrimSpace(arch))
	switch a {
	case "x86_64", "amd64", "x64":
		return "x86_64"
	case "aarch64", "arm64", "arm64v8":
		return "aarch64"
	case "armv7", "armv7l", "armhf", "arm":
		return "armv7"
	case "mipsel", "mipsle", "mips64el", "mips64le":
		return "mipsel"
	default:
		return a
	}
}

// GetAgentBinary returns the embedded prebuilt agent binary for the given architecture
func GetAgentBinary(arch string) ([]byte, error) {
	norm := NormalizeArch(arch)
	filename := fmt.Sprintf("embedded/easy42-agent-%s", norm)
	data, err := EmbeddedAgentFS.ReadFile(filename)
	if err != nil {
		if norm == "mips" {
			return nil, fmt.Errorf("unsupported agent architecture \"mips\" (big-endian MIPS is not supported; available: x86_64, aarch64, armv7, mipsel)")
		}
		return nil, fmt.Errorf("unsupported agent architecture %q (available: x86_64, aarch64, armv7, mipsel)", arch)
	}
	return data, nil
}
