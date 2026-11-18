package lookingglass

import (
	"testing"

	"easy42/internal/config"
)

func TestSanitizeParam(t *testing.T) {
	// IP/CIDR test
	ipParam := config.TaskParam{
		Key:      "target",
		Label:    "Target",
		Type:     config.ParamTypeIPOrCIDR,
		Required: true,
	}

	validIPs := []string{"1.1.1.1", "172.20.0.53", "2001:4860:4860::8888", "172.20.0.0/14", "example.com", "dn42.us"}
	for _, ip := range validIPs {
		res, err := SanitizeParam(ipParam, ip)
		if err != nil {
			t.Errorf("expected '%s' to be valid, got error: %v", ip, err)
		}
		if res != ip {
			t.Errorf("expected '%s', got '%s'", ip, res)
		}
	}

	invalidIPs := []string{"1.1.1.1; rm -rf /", "1.1.1.1 | cat /etc/passwd", "1.1.1.1`id`", "$(whoami)"}
	for _, ip := range invalidIPs {
		_, err := SanitizeParam(ipParam, ip)
		if err == nil {
			t.Errorf("expected '%s' to be rejected, but it passed", ip)
		}
	}

	// Number test
	numParam := config.TaskParam{
		Key:      "count",
		Label:    "Count",
		Type:     config.ParamTypeNumber,
		Required: true,
	}
	res, err := SanitizeParam(numParam, "5")
	if err != nil || res != "5" {
		t.Errorf("expected '5', got '%s' (err: %v)", res, err)
	}
	// Test capping count to 30
	res, err = SanitizeParam(numParam, "100")
	if err != nil || res != "30" {
		t.Errorf("expected '30', got '%s' (err: %v)", res, err)
	}

	// Select test
	selParam := config.TaskParam{
		Key:      "scope",
		Label:    "Scope",
		Type:     config.ParamTypeSelect,
		Options:  []string{"all", "primary", "filtered"},
		Required: true,
	}
	res, err = SanitizeParam(selParam, "primary")
	if err != nil || res != "primary" {
		t.Errorf("expected 'primary', got '%s' (err: %v)", res, err)
	}
	_, err = SanitizeParam(selParam, "malicious_scope")
	if err == nil {
		t.Errorf("expected error for invalid select option, got nil")
	}
	// HostPort test
	hpParam := config.TaskParam{
		Key:      "target",
		Label:    "Target",
		Type:     config.ParamTypeHostPort,
		Required: true,
	}
	validHostPorts := []string{"1.1.1.1:80", "[2001:db8::1]:443", "example.com:8080"}
	for _, hp := range validHostPorts {
		res, err := SanitizeParam(hpParam, hp)
		if err != nil {
			t.Errorf("expected '%s' to be valid, got: %v", hp, err)
		}
		if res != hp {
			t.Errorf("expected '%s', got '%s'", hp, res)
		}
	}

	invalidHostPorts := []string{"1.1.1.1", "[2001:db8::1]", "1.1.1.1:99999", "1.1.1.1:0", "1.1.1.1:80; ls"}
	for _, hp := range invalidHostPorts {
		if _, err := SanitizeParam(hpParam, hp); err == nil {
			t.Errorf("expected '%s' to be rejected, but it passed", hp)
		}
	}
}

func TestBuildCommand(t *testing.T) {
	tmpl := "ping -c {{count}} -W 2 {{target}}"
	params := []config.TaskParam{
		{
			Key:          "target",
			Label:        "Target",
			Type:         config.ParamTypeIPOrCIDR,
			Required:     true,
			DefaultValue: "172.20.0.53",
		},
		{
			Key:          "count",
			Label:        "Count",
			Type:         config.ParamTypeNumber,
			Required:     false,
			DefaultValue: "4",
		},
	}

	cmd, err := BuildCommand(tmpl, params, map[string]string{
		"target": "1.1.1.1",
		"count":  "3",
	})
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}
	expected := "ping -c 3 -W 2 1.1.1.1"
	if cmd != expected {
		t.Errorf("expected '%s', got '%s'", expected, cmd)
	}
}

func TestValidateAdHocCommand(t *testing.T) {
	valid := []string{
		"ping -c 4 1.1.1.1",
		"traceroute 1.0.0.1",
		"birdc show protocols",
		"ip route get 1.1.1.1",
		"whois -h whois.dn42 AS4242420000",
		"tcp_ping 1.1.1.1:80 4",
		"http https://example.com/ 1",
		"http http://1.1.1.1:8080/path?q=1",
	}
	for _, c := range valid {
		if err := ValidateAdHocCommand(c); err != nil {
			t.Errorf("expected '%s' to be valid, got: %v", c, err)
		}
	}

	invalid := []string{
		"rm -rf /",
		"cat /etc/passwd",
		"ping 1.1.1.1; ls",
		"birdc show protocols | grep bgp",
		"whois `id`",
		"birdc down",
		"birdc configure",
		"ip route del default",
		"ip route flush table main",
		"ip link set dev eth0 down",
		"dig -f /etc/shadow @localhost",
		"dig -b 1.2.3.4 example.com",
		"ping -f 1.1.1.1",
		"ping -c 50 1.1.1.1",
		"ping '1.1.1.1'",
		"ping \\1.1.1.1",
		"http ftp://example.com",
		"http example.com",
		"http",
	}
	for _, c := range invalid {
		if err := ValidateAdHocCommand(c); err == nil {
			t.Errorf("expected '%s' to be rejected, but it passed", c)
		}
	}
}

func TestSanitizeURLParam(t *testing.T) {
	spec := config.TaskParam{
		Key:          "target",
		Label:        "Target URL",
		Type:         config.ParamTypeURL,
		Required:     true,
		DefaultValue: "https://example.com/",
	}

	valid := []string{
		"https://example.com/",
		"http://1.1.1.1:8080/path?q=1",
		"HTTPS://example.com/path#frag",
		"http://[2001:db8::1]:80/",
	}
	for _, v := range valid {
		if _, err := SanitizeParam(spec, v); err != nil {
			t.Errorf("expected %q to be valid, got: %v", v, err)
		}
	}

	invalid := []string{
		"ftp://example.com",
		"example.com",
		"https://",
		"http:///path",
		"https://exa mple.com/",
	}
	for _, v := range invalid {
		if _, err := SanitizeParam(spec, v); err == nil {
			t.Errorf("expected %q to be rejected, but it passed", v)
		}
	}
}

func TestBuildCommandHTTPTemplate(t *testing.T) {
	tmpl := "http {{target}} {{count}}"
	params := []config.TaskParam{
		{Key: "target", Label: "Target URL", Type: config.ParamTypeURL, Required: true, DefaultValue: "https://example.com/"},
		{Key: "count", Label: "Probe Count", Type: config.ParamTypeNumber, Required: false, DefaultValue: "1"},
	}

	cmd, err := BuildCommand(tmpl, params, map[string]string{"target": "https://example.com/api", "count": "2"})
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}
	if cmd != "http https://example.com/api 2" {
		t.Errorf("unexpected command: %s", cmd)
	}

	// Defaults are applied when params are absent
	cmd, err = BuildCommand(tmpl, params, nil)
	if err != nil {
		t.Fatalf("BuildCommand with defaults failed: %v", err)
	}
	if cmd != "http https://example.com/ 1" {
		t.Errorf("unexpected command with defaults: %s", cmd)
	}

	// Invalid URL is rejected
	if _, err := BuildCommand(tmpl, params, map[string]string{"target": "example.com"}); err == nil {
		t.Errorf("expected invalid URL to be rejected")
	}
}

func TestParseHTTPCommand(t *testing.T) {
	target, count, err := ParseHTTPCommand("http https://example.com/ 3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != "https://example.com/" || count != 3 {
		t.Errorf("unexpected result: target=%q count=%d", target, count)
	}

	// Count defaults to 1 and is clamped to the HTTP maximum
	if _, count, err := ParseHTTPCommand("http https://example.com/"); err != nil || count != 1 {
		t.Errorf("expected default count 1, got %d (err=%v)", count, err)
	}
	if _, count, err := ParseHTTPCommand("http https://example.com/ 50"); err != nil || count != config.HTTPProbeMaxCount {
		t.Errorf("expected count clamped to %d, got %d (err=%v)", config.HTTPProbeMaxCount, count, err)
	}

	// Invalid inputs
	for _, c := range []string{"http", "http example.com", "http https://example.com/ abc"} {
		if _, _, err := ParseHTTPCommand(c); err == nil {
			t.Errorf("expected %q to be rejected", c)
		}
	}
}
