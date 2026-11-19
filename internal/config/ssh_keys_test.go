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
	a := SSHPublicKey{Algorithm: "ssh-ed25519", Key: testSSHKeyBlob("ssh-ed25519", "a"), Comment: "node-a"}
	b := SSHPublicKey{Algorithm: "ssh-rsa", Key: testSSHKeyBlob("ssh-rsa", "b")}
	dup := SSHPublicKey{Algorithm: "ssh-ed25519", Key: testSSHKeyBlob("ssh-ed25519", "a"), Comment: "duplicate"}
	invalid := SSHPublicKey{Algorithm: "ssh-ed25519", Key: "garbage"}

	block := BuildSSHKeysBlock([]SSHPublicKey{a, b, dup, invalid}, false)
	want := "### EASY42_START\n" +
		"ssh-ed25519 " + a.Key + " node-a\n" +
		"ssh-rsa " + b.Key + "\n" +
		"### EASY42_END\n"
	if block != want {
		t.Fatalf("unexpected block:\n%q\nwant:\n%q", block, want)
	}

	// Order of input keys must not affect the block/hash.
	block2 := BuildSSHKeysBlock([]SSHPublicKey{b, a}, false)
	if HashSSHKeysBlock(block) != HashSSHKeysBlock(block2) {
		t.Fatal("expected identical hash regardless of key order")
	}

	authBlock := BuildSSHKeysBlock([]SSHPublicKey{a, b}, true)
	if !strings.Contains(authBlock, "ssh-ed25519 "+a.Key+" easy42 node-a\n") {
		t.Fatalf("authorized_keys entries must carry the easy42 comment, got:\n%s", authBlock)
	}
	if !strings.Contains(authBlock, "ssh-rsa "+b.Key+" easy42\n") {
		t.Fatalf("authorized_keys entries without comment must get the easy42 comment, got:\n%s", authBlock)
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
		SrcSelector: map[string]string{AllNodesSelectorKey: "true"},
		DstSelector: map[string]string{AllNodesSelectorKey: "true"},
	}
	if err := ValidateSSHKeyRule(allNodes); err != nil {
		t.Fatalf("all-nodes selector must be valid: %v", err)
	}
	if !IsAllNodesSelector(allNodes.DstSelector) {
		t.Fatal("expected the all-nodes marker to be recognized")
	}
	if got := ExpandAllNodesSelector(allNodes.DstSelector); len(got) != 0 {
		t.Fatalf("expected the marker to expand to an empty (match-all) selector, got %v", got)
	}

	// The marker must not be combined with other conditions.
	combined := allNodes
	combined.DstSelector = map[string]string{AllNodesSelectorKey: "true", "tag": "edge"}
	if err := ValidateSSHKeyRule(combined); err == nil {
		t.Fatal("expected combined all-nodes selector to be rejected")
	}

	// An empty destination selector is still rejected (never implicit "all").
	empty := allNodes
	empty.DstSelector = nil
	if err := ValidateSSHKeyRule(empty); err == nil {
		t.Fatal("expected empty destination selector to be rejected")
	}
}
