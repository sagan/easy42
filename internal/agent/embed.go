package agent

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed embedded/*
var EmbeddedAgentFS embed.FS

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
	case "mipsel", "mipsle":
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
		return nil, fmt.Errorf("unsupported agent architecture %q (available: x86_64, aarch64, armv7, mipsel)", arch)
	}
	return data, nil
}
