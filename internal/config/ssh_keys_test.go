package config

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
)

// testSSHKeyBlob builds a minimal SSH public key blob that embeds algorithm as
// its first SSH string, so it passes ValidateSSHPublicKey.
func testSSHKeyBlob(algorithm string, payload string) string {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(len(algorithm)))
	buf = append(buf, algorithm...)
	buf = append(buf, payload...)
	return base64.StdEncoding.EncodeToString(buf)
}

func TestValidateSSHPublicKey(t *testing.T) {
	ed := testSSHKeyBlob("ssh-ed25519", "payload")

	if err := ValidateSSHPublicKey("ssh-ed25519", ed); err != nil {
		t.Fatalf("expected valid ed25519 key, got %v", err)
	}
	if err := ValidateSSHPublicKey("ssh-rsa", ed); err == nil {
		t.Fatal("expected algorithm mismatch error")
	}
	if err := ValidateSSHPublicKey("totally-unknown", ed); err == nil {
		t.Fatal("expected unsupported algorithm error")
	}
	if err := ValidateSSHPublicKey("ssh-ed25519", "not base64!"); err == nil {
		t.Fatal("expected invalid base64 error")
	}
	if err := ValidateSSHPublicKey("", ""); err == nil {
		t.Fatal("expected empty algorithm error")
	}
}

func TestCompareAgentVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.18", "0.1.18", 0},
		{"0.1.18", "0.1.17", 1},
		{"v0.1.18", "0.1.18", 0},
		{"0.1.9", "0.1.18", -1},
		{"0.2", "0.1.18", 1},
		{"", "0.1.18", -1},
		{"unknown", "0.1.18", -1},
		{"0.1.19-beta", "0.1.18", 1},
	}
	for _, c := range cases {
		if got := CompareAgentVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareAgentVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}

	if !SSHKeySupportedByAgent("0.1.18") || !SSHKeySupportedByAgent("0.2.0") {
		t.Fatal("expected 0.1.18 and newer to support SSH keys")
	}
	if SSHKeySupportedByAgent("0.1.17") || SSHKeySupportedByAgent("") {
		t.Fatal("expected older / unknown agents not to support SSH keys")
	}
}

func TestBuildSSHKeysBlock(t *testing.T) {
	a := SSHPublicKey{Algorithm: "ssh-ed25519", Key: testSSHKeyBlob("ssh-ed25519", "a"), Comment: "node-a", Kind: SSHKeyKindHost, Hostnames: []string{"10.0.0.1", "node-a"}}
	b := SSHPublicKey{Algorithm: "ssh-rsa", Key: testSSHKeyBlob("ssh-rsa", "b"), Kind: SSHKeyKindHost, Hostnames: []string{"10.0.0.2"}}
	dup := SSHPublicKey{Algorithm: "ssh-ed25519", Key: testSSHKeyBlob("ssh-ed25519", "a"), Comment: "duplicate"}
	invalid := SSHPublicKey{Algorithm: "ssh-ed25519", Key: "garbage"}

	block := BuildSSHKeysBlock([]SSHPublicKey{a, b, dup, invalid}, false)
	want := "### EASY42_START\n" +
		"10.0.0.1,node-a ssh-ed25519 " + a.Key + " node-a\n" +
		"10.0.0.2 ssh-rsa " + b.Key + "\n" +
		"### EASY42_END\n"
	if block != want {
		t.Fatalf("unexpected block:\n%q\nwant:\n%q", block, want)
	}

	// Order of input keys must not affect the block/hash.
	block2 := BuildSSHKeysBlock([]SSHPublicKey{b, a}, false)
	if HashSSHKeysBlock(block) != HashSSHKeysBlock(block2) {
		t.Fatal("expected identical hash regardless of key order")
	}

	// authorized_keys only accepts root login identities (user keys); host keys
	// belong in known_hosts.
	user := SSHPublicKey{Algorithm: "ssh-ed25519", Key: testSSHKeyBlob("ssh-ed25519", "a"), Comment: "node-a", Kind: SSHKeyKindUser}
	legacy := SSHPublicKey{Algorithm: "ssh-rsa", Key: testSSHKeyBlob("ssh-rsa", "b")}
	authBlock := BuildSSHKeysBlock([]SSHPublicKey{a, user, legacy}, true)
	if !strings.Contains(authBlock, "ssh-ed25519 "+a.Key+" easy42 node-a\n") {
		t.Fatalf("authorized_keys entries must carry the easy42 comment, got:\n%s", authBlock)
	}
	if !strings.Contains(authBlock, "ssh-rsa "+b.Key+" easy42\n") {
		t.Fatalf("keyless-role keys must be treated as user keys, got:\n%s", authBlock)
	}
	// A host key must never be installed into authorized_keys.
	if got := BuildSSHKeysBlock([]SSHPublicKey{a}, true); !strings.Contains(got, "### EASY42_START\n### EASY42_END\n") {
		t.Fatalf("host keys must be excluded from authorized_keys, got:\n%s", got)
	}
	// authorized_keys entries never carry host names.
	if strings.Contains(authBlock, "10.0.0.1") {
		t.Fatalf("authorized_keys entries must not carry host names, got:\n%s", authBlock)
	}
}

func TestSSHKeyKindRouting(t *testing.T) {
	user := SSHPublicKey{Kind: SSHKeyKindUser}
	host := SSHPublicKey{Kind: SSHKeyKindHost}
	legacy := SSHPublicKey{}
	if user.IsHostKey() || legacy.IsHostKey() || !host.IsHostKey() {
		t.Fatal("host key detection is wrong")
	}
	if !user.ForFile(true) || host.ForFile(true) {
		t.Fatal("authorized_keys must only receive user keys")
	}
	if !host.ForFile(false) || user.ForFile(false) || legacy.ForFile(false) {
		t.Fatal("known_hosts must only receive host keys")
	}
	if got := host.Normalize().Kind; got != SSHKeyKindHost {
		t.Fatalf("expected normalized host kind, got %q", got)
	}
	if got := legacy.Normalize().Kind; got != SSHKeyKindUser {
		t.Fatalf("expected legacy keys to normalize to user, got %q", got)
	}
}

func TestBuildSSHKeysBlockKnownHostsRequiresHostnames(t *testing.T) {
	noHosts := SSHPublicKey{Algorithm: "ssh-ed25519", Key: testSSHKeyBlob("ssh-ed25519", "a"), Comment: "node-a"}
	block := BuildSSHKeysBlock([]SSHPublicKey{noHosts}, false)
	want := "### EASY42_START\n### EASY42_END\n"
	if block != want {
		t.Fatalf("known_hosts keys without host names must be skipped, got:\n%q", block)
	}
	// authorized_keys still accepts them.
	if authBlock := BuildSSHKeysBlock([]SSHPublicKey{noHosts}, true); !strings.Contains(authBlock, noHosts.Key) {
		t.Fatalf("authorized_keys must keep keys without host names, got:\n%s", authBlock)
	}
}

func TestNodeKnownHostsHostnames(t *testing.T) {
	node := Node{
		Name:        "njrlubuntu",
		IP:          "192.168.100.1",
		IP6:         "fd42::1",
		ExternalIP:  "172.20.0.1",
		ExternalIP6: "fd42:1::1/64",
	}

	got := NodeKnownHostsHostnames(node, "")
	want := []string{"192.168.100.1", "fd42::1", "172.20.0.1", "fd42:1::1", "njrlubuntu", "njrlubuntu.internal"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected host names: %v, want %v", got, want)
	}

	withDomain := NodeKnownHostsHostnames(node, " https://Easy42.Example.com/ ")
	last := withDomain[len(withDomain)-1]
	if last != "njrlubuntu.easy42.example.com" {
		t.Fatalf("expected base domain host name, got %v", withDomain)
	}

	// Missing values are skipped and duplicates removed.
	sparse := NodeKnownHostsHostnames(Node{Name: "n1", IP: "10.0.0.1", IP6: "10.0.0.1"}, "example.com")
	wantSparse := []string{"10.0.0.1", "n1", "n1.internal", "n1.example.com"}
	if strings.Join(sparse, ",") != strings.Join(wantSparse, ",") {
		t.Fatalf("unexpected sparse host names: %v, want %v", sparse, wantSparse)
	}
}

func TestValidateSSHKeyRule(t *testing.T) {
	valid := SSHKeyRule{
		Type:        "all",
		SrcSelector: map[string]string{"tag": "core"},
		DstSelector: map[string]string{"name": "node-a,node-b"},
	}
	if err := ValidateSSHKeyRule(valid); err != nil {
		t.Fatalf("expected valid rule, got %v", err)
	}

	badType := valid
	badType.Type = "unknown"
	if err := ValidateSSHKeyRule(badType); err == nil {
		t.Fatal("expected invalid type error")
	}

	noDst := valid
	noDst.DstSelector = nil
	if err := ValidateSSHKeyRule(noDst); err == nil {
		t.Fatal("expected empty destination selector to be rejected")
	}

	badSrc := valid
	badSrc.SrcSelector = map[string]string{"bogus": "x"}
	if err := ValidateSSHKeyRule(badSrc); err == nil {
		t.Fatal("expected invalid source selector key error")
	}
}

func TestSSHKeyRuleTargetFiles(t *testing.T) {
	cases := map[string][]string{
		"known_hosts":     {SSHKnownHostsPath},
		"authorized_keys": {SSHAuthorizedKeysPath},
		"all":             {SSHKnownHostsPath, SSHAuthorizedKeysPath},
		"":                {SSHKnownHostsPath, SSHAuthorizedKeysPath},
	}
	for ruleType, want := range cases {
		got := SSHKeyRule{Type: ruleType}.TargetFiles()
		if len(got) != len(want) {
			t.Fatalf("type %q: got %v, want %v", ruleType, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("type %q: got %v, want %v", ruleType, got, want)
			}
		}
	}
	if !IsSSHKeyTargetPath(SSHAuthorizedKeysPath) || !IsAuthorizedKeysPath(SSHAuthorizedKeysPath) {
		t.Fatal("expected authorized_keys path to be recognized")
	}
	if IsSSHKeyTargetPath("/root/.ssh/id_ed25519") {
		t.Fatal("identity files are not managed targets")
	}
}

func TestSSHKeyRuleAllNodesSelector(t *testing.T) {
	allNodes := SSHKeyRule{
		Type:        SSHKeyFileAuthorizedKeys,
		SrcSelector: AllNodesSelector(),
		DstSelector: AllNodesSelector(),
	}
	if err := ValidateSSHKeyRule(allNodes); err != nil {
		t.Fatalf("all-nodes selector must be valid: %v", err)
	}
	if !IsAllNodesSelector(allNodes.DstSelector) {
		t.Fatal("expected the all-nodes selector to be recognized")
	}

	// The marker must not be combined with other conditions.
	combined := allNodes
	combined.DstSelector = map[string]string{AllNodesSelectorKey: AllNodesSelectorValue, "tag": "edge"}
	if err := ValidateSSHKeyRule(combined); err == nil {
		t.Fatal("expected combined all-nodes selector to be rejected")
	}

	// A legacy "all" value is not the all-nodes selector anymore.
	legacy := allNodes
	legacy.DstSelector = map[string]string{AllNodesSelectorKey: "true"}
	if err := ValidateSSHKeyRule(legacy); err == nil {
		t.Fatal("expected legacy all-nodes value to be rejected")
	}

	// An empty selector matches no node and is rejected on both sides.
	for _, side := range []string{"source", "destination"} {
		empty := allNodes
		if side == "source" {
			empty.SrcSelector = nil
		} else {
			empty.DstSelector = nil
		}
		if err := ValidateSSHKeyRule(empty); err == nil {
			t.Fatalf("expected empty %s selector to be rejected", side)
		}
	}
}
