package config

import (
	"testing"
)

func TestValidationFunctions(t *testing.T) {
	// Node Name
	if !IsValidNodeName("node-1") || !IsValidNodeName("node_a") || !IsValidNodeName("DN42EXT") {
		t.Errorf("Expected valid node names to pass")
	}
	if IsValidNodeName("") || IsValidNodeName("bad name") || IsValidNodeName("node;rm") || IsValidNodeName("a/b") {
		t.Errorf("Expected invalid node names to fail")
	}

	// Interface Name
	if !IsValidInterfaceName("wg42_a") || !IsValidInterfaceName("eth0") || !IsValidInterfaceName("lo") {
		t.Errorf("Expected valid interface names to pass")
	}
	if IsValidInterfaceName("") || IsValidInterfaceName("very_long_interface_name") || IsValidInterfaceName("wg;reboot") {
		t.Errorf("Expected invalid interface names to fail")
	}

	// IPv4
	if !IsValidIPv4("192.168.1.1") || !IsValidIPv4("1.1.1.1") || !IsValidIPv4("10.0.0.1") {
		t.Errorf("Expected valid IPv4 to pass")
	}
	if IsValidIPv4("256.0.0.1") || IsValidIPv4("1.2.3") || IsValidIPv4("fe80::1") || IsValidIPv4("192.168.1.1/24") {
		t.Errorf("Expected invalid IPv4 to fail")
	}

	// IPv6
	if !IsValidIPv6("2001:db8::1") || !IsValidIPv6("fe80::1") || !IsValidIPv6("fd42::1/64") {
		t.Errorf("Expected valid IPv6 to pass")
	}
	if IsValidIPv6("192.168.1.1") || IsValidIPv6("invalid:ipv6:xyz") {
		t.Errorf("Expected invalid IPv6 to fail")
	}

	// Domain
	if !IsValidDomain("sagan.me") || !IsValidDomain("node.dn42.dev") || !IsValidDomain("localhost") {
		t.Errorf("Expected valid domains to pass")
	}
	if IsValidDomain("-bad.domain") || IsValidDomain("bad..domain") || IsValidDomain("domain with space") {
		t.Errorf("Expected invalid domains to fail")
	}

	// SSH Host
	if !IsValidSSHHost("192.168.1.1") || !IsValidSSHHost("root@192.168.1.1") || !IsValidSSHHost("root@node.dn42.dev:2222") ||
		!IsValidSSHHost("node.dn42.dev:22") || !IsValidSSHHost("my-alias") || !IsValidSSHHost("root@[2001:db8::1]:22") {
		t.Errorf("Expected valid SSH hosts to pass")
	}
	if IsValidSSHHost("192.168.1.1; rm -rf /") || IsValidSSHHost("host$(reboot)") || IsValidSSHHost("host`id`") ||
		IsValidSSHHost("host with space") || IsValidSSHHost("user@") {
		t.Errorf("Expected dangerous/invalid SSH hosts to fail")
	}

	// CIDR
	if !IsValidCIDR("10.0.0.0/8") || !IsValidCIDR("fd42::/48") {
		t.Errorf("Expected valid CIDRs to pass")
	}
	if IsValidCIDR("10.0.0.0") || IsValidCIDR("10.0.0.0/33") {
		t.Errorf("Expected invalid CIDRs to fail")
	}

	// Telegram Bot Token
	if !IsValidTelegramBotToken("123456789:ABCdefGhIJKlmNoPQRsTUVwxyZ_12345678") {
		t.Errorf("Expected valid Telegram bot token to pass")
	}
	if IsValidTelegramBotToken("invalid-token") || IsValidTelegramBotToken("12345:short") {
		t.Errorf("Expected invalid Telegram bot token to fail")
	}
}

func TestValidateNode(t *testing.T) {
	validManaged := &Node{
		Name: "node-1",
		Host: "192.168.1.10",
		IP:   "172.20.1.1",
		IP6:  "fd42::1",
		ASN:  4224420001,
	}
	if err := ValidateNode(validManaged); err != nil {
		t.Errorf("Expected valid managed node to pass, got: %v", err)
	}

	invalidManaged := &Node{
		Name: "node-1",
		Host: "host;reboot",
		IP:   "bad-ip",
	}
	if err := ValidateNode(invalidManaged); err == nil {
		t.Errorf("Expected invalid managed node to fail")
	}

	validExt := &Node{
		Name:       "dn42ext",
		IsExternal: true,
		ExternalIP: "1.2.3.4",
	}
	if err := ValidateNode(validExt); err != nil {
		t.Errorf("Expected valid external node to pass, got: %v", err)
	}
}
