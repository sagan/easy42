package compiler

import "testing"

func TestStripHostPort(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"127.0.0.1:22", "127.0.0.1"},
		{"user@127.0.0.1:22", "127.0.0.1"},
		{"user@127.0.0.1", "127.0.0.1"},
		{"127.0.0.1", "127.0.0.1"},
		{"[2001:db8::1]:51820", "2001:db8::1"},
		{"[2001:db8::1]", "2001:db8::1"},
		{"user@[2001:db8::1]:51820", "2001:db8::1"},
		{"2001:db8::1", "2001:db8::1"},
		{"router1:2222", "router1"},
		{"user@router1", "router1"},
	}

	for _, tt := range tests {
		got := StripHostPort(tt.input)
		if got != tt.expected {
			t.Errorf("StripHostPort(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatHostPort(t *testing.T) {
	tests := []struct {
		host     string
		port     int
		expected string
	}{
		{"127.0.0.1", 51820, "127.0.0.1:51820"},
		{"127.0.0.1:22", 51820, "127.0.0.1:51820"},
		{"user@127.0.0.1:22", 51820, "127.0.0.1:51820"},
		{"2001:db8::1", 51820, "[2001:db8::1]:51820"},
		{"[2001:db8::1]", 51820, "[2001:db8::1]:51820"},
		{"[2001:db8::1]:2222", 51820, "[2001:db8::1]:51820"},
		{"my-host.domain", 51820, "my-host.domain:51820"},
		{"my-host.domain:22", 51820, "my-host.domain:51820"},
	}

	for _, tt := range tests {
		got := FormatHostPort(tt.host, tt.port)
		if got != tt.expected {
			t.Errorf("FormatHostPort(%q, %d) = %q; want %q", tt.host, tt.port, got, tt.expected)
		}
	}
}
