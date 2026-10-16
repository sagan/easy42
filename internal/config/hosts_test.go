package config

import (
	"strings"
	"testing"
)

func TestBuildHostsBlockAndHash(t *testing.T) {
	records := []HostRecord{
		{Name: "foo", IP: "192.168.100.1"},
		{Name: "bar", IP: "192.168.100.2"},
	}

	block := BuildHostsBlock(records)
	expected := "### EASY42_START\n192.168.100.2 bar bar.internal\n192.168.100.1 foo foo.internal\n### EASY42_END\n"
	if block != expected {
		t.Fatalf("expected:\n%q\ngot:\n%q", expected, block)
	}

	hash := HashHostsBlock(block)
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	// Extract from full /etc/hosts content
	fullContent := "127.0.0.1 localhost\n::1 localhost ip6-localhost\n" + block + "# extra comment\n"
	extractedBlock, extractedHash := ExtractHostsBlock(fullContent)
	if extractedHash != hash {
		t.Fatalf("expected hash %q, got %q", hash, extractedHash)
	}
	if !strings.Contains(extractedBlock, "192.168.100.2 bar bar.internal") {
		t.Fatalf("extracted block missing content: %s", extractedBlock)
	}
}
