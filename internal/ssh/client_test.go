package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
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

func TestHostKeyVerificationRejectsUnknownAndMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	khPath := filepath.Join(tmpDir, "known_hosts")

	key1, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer1, err := ssh.NewSignerFromKey(key1)
	if err != nil {
		t.Fatal(err)
	}
	pub1 := signer1.PublicKey()

	key2, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer2, err := ssh.NewSignerFromKey(key2)
	if err != nil {
		t.Fatal(err)
	}
	pub2 := signer2.PublicKey()

	line := knownhosts.Line([]string{"known-host:22"}, pub1)
	if err := os.WriteFile(khPath, []byte(line+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cb, err := knownhosts.New(khPath)
	if err != nil {
		t.Fatalf("failed to parse known_hosts: %v", err)
	}

	remoteAddr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:22")

	// Known host with matching key should succeed
	if err := cb("known-host:22", remoteAddr, pub1); err != nil {
		t.Errorf("expected matching key to succeed, got: %v", err)
	}

	// Unknown host should fail
	if err := cb("unknown-host:22", remoteAddr, pub1); err == nil {
		t.Errorf("expected unknown host to be rejected, got nil error")
	}

	// Known host with mismatched key should fail
	if err := cb("known-host:22", remoteAddr, pub2); err == nil {
		t.Errorf("expected mismatched key to be rejected, got nil error")
	}
}

func TestHostKeyAlgorithmsPrioritization(t *testing.T) {
	// 1. ED25519 prioritized
	edAlgos := orderHostKeyAlgorithms([]string{ssh.KeyAlgoED25519}, "")
	if len(edAlgos) == 0 || edAlgos[0] != ssh.KeyAlgoED25519 {
		t.Errorf("expected ssh-ed25519 first, got %v", edAlgos)
	}

	// 2. RSA prioritized
	rsaAlgos := orderHostKeyAlgorithms([]string{ssh.KeyAlgoRSA}, "")
	if len(rsaAlgos) == 0 || rsaAlgos[0] != ssh.KeyAlgoRSASHA512 {
		t.Errorf("expected rsa-sha2-512 first, got %v", rsaAlgos)
	}

	// 3. Default fallback preference has ed25519 before ecdsa/rsa
	defaultAlgos := orderHostKeyAlgorithms(nil, "")
	var edIdx, ecdsaIdx, rsaIdx int = -1, -1, -1
	for i, a := range defaultAlgos {
		if a == ssh.KeyAlgoED25519 && edIdx == -1 {
			edIdx = i
		}
		if a == ssh.KeyAlgoECDSA256 && ecdsaIdx == -1 {
			ecdsaIdx = i
		}
		if a == ssh.KeyAlgoRSASHA512 && rsaIdx == -1 {
			rsaIdx = i
		}
	}
	if edIdx == -1 || ecdsaIdx == -1 || rsaIdx == -1 || edIdx > ecdsaIdx || ecdsaIdx > rsaIdx {
		t.Errorf("expected default preference ed25519 < ecdsa < rsa, got ed=%d, ecdsa=%d, rsa=%d in %v", edIdx, ecdsaIdx, rsaIdx, defaultAlgos)
	}

	// 4. ssh_config override
	cfgPrepend := orderHostKeyAlgorithms(nil, "^custom-algo")
	if len(cfgPrepend) == 0 || cfgPrepend[0] != "custom-algo" {
		t.Errorf("expected prepended custom-algo, got %v", cfgPrepend)
	}
}

func TestGetKnownKeyTypesWithCommaSeparatedKnownHosts(t *testing.T) {
	tmpDir := t.TempDir()
	khPath := filepath.Join(tmpDir, "known_hosts")

	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatal(err)
	}

	// Format: nbbug.s.sagan.me,16.5.55.232 ssh-ed25519 ...
	line := knownhosts.Line([]string{"nbbug.s.sagan.me,16.5.55.232"}, sshPub)
	if err := os.WriteFile(khPath, []byte(line+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cb, err := knownhosts.New(khPath)
	if err != nil {
		t.Fatalf("failed to create knownhosts callback: %v", err)
	}

	// Query hostname
	typesHost := getKnownKeyTypes(cb, "nbbug.s.sagan.me:22")
	if len(typesHost) != 1 || typesHost[0] != ssh.KeyAlgoED25519 {
		t.Fatalf("expected [ssh-ed25519] for hostname, got %v", typesHost)
	}

	// Query IP
	typesIP := getKnownKeyTypes(cb, "16.5.55.232:22")
	if len(typesIP) != 1 || typesIP[0] != ssh.KeyAlgoED25519 {
		t.Fatalf("expected [ssh-ed25519] for IP, got %v", typesIP)
	}

	// Query unknown
	typesUnknown := getKnownKeyTypes(cb, "other.example.com:22")
	if len(typesUnknown) != 0 {
		t.Fatalf("expected empty for unknown host, got %v", typesUnknown)
	}
}

func TestSSHMultiKeyHandshakeResolution(t *testing.T) {
	// Generate an RSA host key and an ED25519 host key for a test SSH server
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaSigner, err := ssh.NewSignerFromKey(rsaKey)
	if err != nil {
		t.Fatal(err)
	}

	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edSigner, err := ssh.NewSignerFromKey(edPriv)
	if err != nil {
		t.Fatal(err)
	}
	_ = edPub

	serverConfig := &ssh.ServerConfig{
		NoClientAuth: true,
	}
	// Server offers BOTH RSA and ED25519 (in default Go order RSA would take precedence)
	serverConfig.AddHostKey(rsaSigner)
	serverConfig.AddHostKey(edSigner)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				sConn, chans, reqs, err := ssh.NewServerConn(c, serverConfig)
				if err != nil {
					return
				}
				_ = sConn
				go ssh.DiscardRequests(reqs)
				for newChannel := range chans {
					_ = newChannel.Reject(ssh.UnknownChannelType, "test")
				}
			}(conn)
		}
	}()

	serverAddr := listener.Addr().String()

	// 1. Client known_hosts has ONLY the ED25519 host key
	tmpDir := t.TempDir()
	khPathED := filepath.Join(tmpDir, "known_hosts_ed")
	lineED := knownhosts.Line([]string{serverAddr}, edSigner.PublicKey())
	if err := os.WriteFile(khPathED, []byte(lineED+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cbED, err := knownhosts.New(khPathED)
	if err != nil {
		t.Fatal(err)
	}

	knownTypesED := getKnownKeyTypes(cbED, serverAddr)
	algosED := orderHostKeyAlgorithms(knownTypesED, "")

	clientConfigED := &ssh.ClientConfig{
		User:              "test",
		Auth:              []ssh.AuthMethod{ssh.Password("none")},
		HostKeyCallback:   cbED,
		HostKeyAlgorithms: algosED,
		Timeout:           2 * time.Second,
	}

	clientED, err := ssh.Dial("tcp", serverAddr, clientConfigED)
	if err != nil {
		t.Fatalf("Dial with ED25519 known_hosts failed (reproduced issue): %v", err)
	}
	_ = clientED.Close()

	// 2. Client known_hosts has ONLY the RSA host key
	khPathRSA := filepath.Join(tmpDir, "known_hosts_rsa")
	lineRSA := knownhosts.Line([]string{serverAddr}, rsaSigner.PublicKey())
	if err := os.WriteFile(khPathRSA, []byte(lineRSA+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cbRSA, err := knownhosts.New(khPathRSA)
	if err != nil {
		t.Fatal(err)
	}

	knownTypesRSA := getKnownKeyTypes(cbRSA, serverAddr)
	algosRSA := orderHostKeyAlgorithms(knownTypesRSA, "")

	clientConfigRSA := &ssh.ClientConfig{
		User:              "test",
		Auth:              []ssh.AuthMethod{ssh.Password("none")},
		HostKeyCallback:   cbRSA,
		HostKeyAlgorithms: algosRSA,
		Timeout:           2 * time.Second,
	}

	clientRSA, err := ssh.Dial("tcp", serverAddr, clientConfigRSA)
	if err != nil {
		t.Fatalf("Dial with RSA known_hosts failed: %v", err)
	}
	_ = clientRSA.Close()

	// 3. Client known_hosts has a DIFFERENT/MISMATCHED key
	badPub, _, _ := ed25519.GenerateKey(rand.Reader)
	badSSHPub, _ := ssh.NewPublicKey(badPub)
	khPathBad := filepath.Join(tmpDir, "known_hosts_bad")
	lineBad := knownhosts.Line([]string{serverAddr}, badSSHPub)
	if err := os.WriteFile(khPathBad, []byte(lineBad+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	cbBad, _ := knownhosts.New(khPathBad)
	knownTypesBad := getKnownKeyTypes(cbBad, serverAddr)
	algosBad := orderHostKeyAlgorithms(knownTypesBad, "")

	clientConfigBad := &ssh.ClientConfig{
		User:              "test",
		Auth:              []ssh.AuthMethod{ssh.Password("none")},
		HostKeyCallback:   cbBad,
		HostKeyAlgorithms: algosBad,
		Timeout:           2 * time.Second,
	}

	_, err = ssh.Dial("tcp", serverAddr, clientConfigBad)
	if err == nil {
		t.Fatalf("Expected key mismatch error for bad key, got nil")
	}
	if !strings.Contains(err.Error(), "knownhosts: key mismatch") {
		t.Fatalf("Expected 'knownhosts: key mismatch', got: %v", err)
	}
}


