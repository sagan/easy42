package ssh

import (
	"strings"
	"testing"
	"time"
)

func TestParseSSHHost(t *testing.T) {
	tests := []struct {
		input    string
		wantUser string
		wantHost string
		wantPort string
	}{
		// Basic hostnames / IPs
		{"192.168.1.1", "", "192.168.1.1", ""},
		{"example.com", "", "example.com", ""},
		{"node-router", "", "node-router", ""},

		// Host:Port
		{"127.0.0.1:22", "", "127.0.0.1", "22"},
		{"192.168.1.1:2222", "", "192.168.1.1", "2222"},
		{"example.com:22022", "", "example.com", "22022"},

		// User@Host
		{"root@192.168.1.1", "root", "192.168.1.1", ""},
		{"admin@example.com", "admin", "example.com", ""},

		// User@Host:Port
		{"root@127.0.0.1:22", "root", "127.0.0.1", "22"},
		{"ubuntu@router-gw:2222", "ubuntu", "router-gw", "2222"},

		// IPv6 bracketed
		{"[::1]", "", "::1", ""},
		{"[2001:db8::1]", "", "2001:db8::1", ""},
		{"[::1]:22", "", "::1", "22"},
		{"[2001:db8::1]:2222", "", "2001:db8::1", "2222"},

		// IPv6 bracketed with User
		{"root@[::1]", "root", "::1", ""},
		{"root@[::1]:22", "root", "::1", "22"},
		{"admin@[2001:db8::1]:2222", "admin", "2001:db8::1", "2222"},

		// Raw IPv6 without brackets
		{"::1", "", "::1", ""},
		{"2001:db8::1", "", "2001:db8::1", ""},
		{"root@::1", "root", "::1", ""},
		{"root@2001:db8::1", "root", "2001:db8::1", ""},

		// Whitespace handling
		{"  127.0.0.1:22  ", "", "127.0.0.1", "22"},
		{"  user@host:2222  ", "user", "host", "2222"},

		// Empty
		{"", "", "", ""},
	}

	for _, tt := range tests {
		user, host, port := ParseSSHHost(tt.input)
		if user != tt.wantUser || host != tt.wantHost || port != tt.wantPort {
			t.Errorf("ParseSSHHost(%q) = (%q, %q, %q); want (%q, %q, %q)",
				tt.input, user, host, port, tt.wantUser, tt.wantHost, tt.wantPort)
		}
	}
}

func TestDialSSHWithTimeoutPortHandling(t *testing.T) {
	// Attempt dialing a loopback port that is closed or has no SSH daemon
	// We want to verify that it does NOT fail with "[127.0.0.1:22]:22: no such host"
	_, err := DialSSHWithTimeout("127.0.0.1:54321", 500*time.Millisecond)
	if err == nil {
		t.Fatal("Expected dial error for port 54321, got nil")
	}

	errMsg := err.Error()
	// Should NOT contain malformed double-port brackets like [127.0.0.1:54321]:22 or lookup: no such host
	if strings.Contains(errMsg, "[127.0.0.1:54321]:22") {
		t.Errorf("Dial error contains double port formatting: %s", errMsg)
	}
	if strings.Contains(errMsg, "lookup 127.0.0.1:54321: no such host") {
		t.Errorf("Dial error treats host:port as hostname to lookup: %s", errMsg)
	}
	// Target address in error message should be 127.0.0.1:54321
	if !strings.Contains(errMsg, "127.0.0.1:54321") {
		t.Errorf("Dial error expected to mention 127.0.0.1:54321, got: %s", errMsg)
	}
}
