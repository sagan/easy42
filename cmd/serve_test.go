package cmd

import (
	"bytes"
	"testing"
)

func TestServeFlagsMultipleAddresses(t *testing.T) {
	// 1. Test default value
	v := &listenAddrValue{addrs: []string{"127.0.0.1:4242"}}
	if v.String() != "127.0.0.1:4242" {
		t.Fatalf("Expected default 127.0.0.1:4242, got %s", v.String())
	}
	if v.Type() != "string" {
		t.Fatalf("Expected type string, got %s", v.Type())
	}

	// 2. Test comma-separated parsing
	v = &listenAddrValue{addrs: []string{"127.0.0.1:4242"}}
	if err := v.Set("127.0.0.1:4242,192.168.100.1:4242"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if len(v.addrs) != 2 {
		t.Fatalf("Expected 2 addresses, got %d: %v", len(v.addrs), v.addrs)
	}
	if v.addrs[0] != "127.0.0.1:4242" || v.addrs[1] != "192.168.100.1:4242" {
		t.Fatalf("Unexpected addresses: %v", v.addrs)
	}

	// 3. Test comma-separated parsing with spaces
	v = &listenAddrValue{addrs: []string{"127.0.0.1:4242"}}
	if err := v.Set("127.0.0.1:4242, 192.168.100.1:4242"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if len(v.addrs) != 2 {
		t.Fatalf("Expected 2 addresses, got %d: %v", len(v.addrs), v.addrs)
	}
	if v.addrs[0] != "127.0.0.1:4242" || v.addrs[1] != "192.168.100.1:4242" {
		t.Fatalf("Unexpected addresses: %v", v.addrs)
	}

	// 4. Test repeated flag calls (e.g. -l addr1 -l addr2)
	v = &listenAddrValue{addrs: []string{"127.0.0.1:4242"}}
	if err := v.Set("127.0.0.1:4242"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if err := v.Set("192.168.100.1:4242"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}
	if len(v.addrs) != 2 {
		t.Fatalf("Expected 2 addresses, got %d: %v", len(v.addrs), v.addrs)
	}
	if v.addrs[0] != "127.0.0.1:4242" || v.addrs[1] != "192.168.100.1:4242" {
		t.Fatalf("Unexpected addresses: %v", v.addrs)
	}
}

func TestServeHelpOutput(t *testing.T) {
	out := serveCmd.UsageString()
	if !bytes.Contains([]byte(out), []byte("-l, --listen string")) {
		t.Fatalf("Expected '-l, --listen string' in help output, got: %s", out)
	}
	if !bytes.Contains([]byte(out), []byte("default \"127.0.0.1:4242\"")) {
		t.Fatalf("Expected default \"127.0.0.1:4242\" in help output, got: %s", out)
	}
}
