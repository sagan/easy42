package agent

import (
	"bytes"
	"testing"
)

func TestEmbeddedAgentBinaries(t *testing.T) {
	targets := []struct {
		arch     string
		normArch string
	}{
		{"x86_64", "x86_64"},
		{"amd64", "x86_64"},
		{"aarch64", "aarch64"},
		{"arm64", "aarch64"},
		{"armv7", "armv7"},
		{"armhf", "armv7"},
		{"mipsel", "mipsel"},
	}

	for _, tt := range targets {
		t.Run(tt.arch, func(t *testing.T) {
			data, err := GetAgentBinary(tt.arch)
			if err != nil {
				t.Fatalf("failed to get agent binary for %s: %v", tt.arch, err)
			}
			if len(data) == 0 {
				t.Fatalf("agent binary for %s is empty", tt.arch)
			}

			// Verify ELF magic header: 0x7F, 'E', 'L', 'F'
			elfMagic := []byte{0x7f, 'E', 'L', 'F'}
			if !bytes.HasPrefix(data, elfMagic) {
				t.Fatalf("agent binary for %s does not have valid ELF header", tt.arch)
			}
		})
	}
}

func TestNormalizeArch(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"x86_64", "x86_64"},
		{"amd64", "x86_64"},
		{"aarch64", "aarch64"},
		{"arm64", "aarch64"},
		{"armv7l", "armv7"},
		{"armhf", "armv7"},
		{"mipsel", "mipsel"},
		{"mipsle", "mipsel"},
		{"mips64el", "mipsel"},
		{"mips", "mips"},
	}

	for _, c := range cases {
		if got := NormalizeArch(c.input); got != c.expected {
			t.Errorf("NormalizeArch(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestUnsupportedArch(t *testing.T) {
	_, err := GetAgentBinary("unknown_arch_123")
	if err == nil {
		t.Fatalf("expected error for unsupported arch, got nil")
	}

	// Test MIPS big-endian specific error message
	_, errMips := GetAgentBinary("mips")
	if errMips == nil {
		t.Fatalf("expected error for mips (big-endian), got nil")
	}
	if !bytes.Contains([]byte(errMips.Error()), []byte("big-endian MIPS is not supported")) {
		t.Errorf("expected error message to mention big-endian MIPS, got: %v", errMips)
	}
}
