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

func TestBuildHostsBlockWithAliases(t *testing.T) {
	records := []HostRecord{
		{Name: "db", IP: "10.0.0.1", Aliases: []string{"db.lan", "postgres"}},
		{Name: "gitlab.example.com", IP: "10.0.0.50", Aliases: []string{}},
	}

	block := BuildHostsBlock(records)
	expected := "### EASY42_START\n10.0.0.1 db db.lan postgres\n10.0.0.50 gitlab.example.com\n### EASY42_END\n"
	if block != expected {
		t.Fatalf("expected:\n%q\ngot:\n%q", expected, block)
	}
}

func TestParseExtraHosts(t *testing.T) {
	raw := `
# Header comment
192.168.1.50 gitlab.example.com gitlab # inline comment
10.0.0.1 db.lan, postgres

fd42:a159:f9f0:1::1 node1.dn42 node1
`
	entries, err := ParseExtraHosts(raw)
	if err != nil {
		t.Fatalf("ParseExtraHosts failed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].IP != "192.168.1.50" || len(entries[0].Hostnames) != 2 || entries[0].Hostnames[0] != "gitlab.example.com" {
		t.Fatalf("unexpected entry 0: %+v", entries[0])
	}
	if entries[1].IP != "10.0.0.1" || len(entries[1].Hostnames) != 2 || entries[1].Hostnames[1] != "postgres" {
		t.Fatalf("unexpected entry 1: %+v", entries[1])
	}
	if entries[2].IP != "fd42:a159:f9f0:1::1" || len(entries[2].Hostnames) != 2 {
		t.Fatalf("unexpected entry 2: %+v", entries[2])
	}

	// Test invalid cases
	invalidCases := []struct {
		input       string
		errContains string
	}{
		{"invalid_ip host.lan", "invalid IP address"},
		{"192.168.1.1", "missing hostname"},
		{"192.168.1.1 -bad-hostname", "invalid hostname"},
		{"192.168.1.1 host@bad", "invalid hostname"},
	}

	for _, tc := range invalidCases {
		_, err := ParseExtraHosts(tc.input)
		if err == nil {
			t.Errorf("expected error for %q, got nil", tc.input)
		} else if !strings.Contains(err.Error(), tc.errContains) {
			t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
		}
	}
}

func TestMergeHostRecordsPriority(t *testing.T) {
	clusterNodes := []Node{
		{Name: "foo", IP: "192.168.100.1"},
		{Name: "bar", IP: "192.168.100.2"},
		{Name: "baz", IP: "192.168.100.3"},
	}

	// Global overrides bar to 10.0.0.2, and adds gitlab.lan
	globalExtra := `
10.0.0.2 bar
10.0.0.50 gitlab.lan
`
	// Node extra overrides foo to 172.16.0.1, and overrides bar to 172.16.0.2
	nodeExtra := `
172.16.0.1 foo
172.16.0.2 bar
`

	records, err := MergeHostRecords(nodeExtra, globalExtra, clusterNodes)
	if err != nil {
		t.Fatalf("MergeHostRecords failed: %v", err)
	}

	// Verify priority:
	// bar: node extra (172.16.0.2) > global extra (10.0.0.2) > easy42 (192.168.100.2) -> 172.16.0.2
	// foo: node extra (172.16.0.1) > easy42 (192.168.100.1) -> 172.16.0.1
	// gitlab.lan: global extra (10.0.0.50)
	// baz: easy42 (192.168.100.3)
	// bar.internal: easy42 (192.168.100.2) since not overridden in node or global
	// foo.internal: easy42 (192.168.100.1) since not overridden in node or global

	lookup := make(map[string]string)
	for _, r := range records {
		lookup[r.Name] = r.IP
		for _, a := range r.Aliases {
			lookup[a] = r.IP
		}
	}

	if lookup["bar"] != "172.16.0.2" {
		t.Errorf("expected bar -> 172.16.0.2, got %s", lookup["bar"])
	}
	if lookup["foo"] != "172.16.0.1" {
		t.Errorf("expected foo -> 172.16.0.1, got %s", lookup["foo"])
	}
	if lookup["gitlab.lan"] != "10.0.0.50" {
		t.Errorf("expected gitlab.lan -> 10.0.0.50, got %s", lookup["gitlab.lan"])
	}
	if lookup["baz"] != "192.168.100.3" {
		t.Errorf("expected baz -> 192.168.100.3, got %s", lookup["baz"])
	}
	if lookup["baz.internal"] != "192.168.100.3" {
		t.Errorf("expected baz.internal -> 192.168.100.3, got %s", lookup["baz.internal"])
	}

	// Verify block generation from merged records
	block := BuildHostsBlock(records)
	if !strings.Contains(block, "172.16.0.2 bar") {
		t.Errorf("expected '172.16.0.2 bar' in block, got:\n%s", block)
	}
	if !strings.Contains(block, "172.16.0.1 foo") {
		t.Errorf("expected '172.16.0.1 foo' in block, got:\n%s", block)
	}
	if !strings.Contains(block, "10.0.0.50 gitlab.lan") {
		t.Errorf("expected '10.0.0.50 gitlab.lan' in block, got:\n%s", block)
	}
	if !strings.Contains(block, "192.168.100.3 baz baz.internal") {
		t.Errorf("expected '192.168.100.3 baz baz.internal' in block, got:\n%s", block)
	}
}
