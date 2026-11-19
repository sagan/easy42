package config

import (
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"
)

// testSSHKeyBlob builds a well-formed SSH public key blob whose public fields
// are derived from payload, so it passes ValidateSSHPublicKey (which requires
// exactly the public fields and nothing else).
func testSSHKeyBlob(algorithm string, payload string) string {
	return base64.StdEncoding.EncodeToString(sshTestPublicBlob(algorithm, []byte(payload)))
}

// sshTestPublicBlob returns algorithm followed by exactly the public fields the
// algorithm defines, filled from seed so distinct payloads give distinct keys.
func sshTestPublicBlob(algorithm string, seed []byte) []byte {
	appendField := func(buf []byte, field []byte) []byte {
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(field)))
		buf = append(buf, l[:]...)
		return append(buf, field...)
	}
	derive := func(n int) []byte {
		out := make([]byte, n)
		for i := range out {
			if len(seed) == 0 {
				out[i] = byte(i)
			} else {
				out[i] = seed[i%len(seed)]
			}
		}
		return out
	}

	buf := appendField(nil, []byte(algorithm))
	switch algorithm {
	case "ssh-rsa":
		buf = appendField(buf, []byte{0x01, 0x00, 0x01})
		modulus := append([]byte{0x01}, derive(32)...)
		buf = appendField(buf, modulus)
	default: // ssh-ed25519
		buf = appendField(buf, derive(32))
	}
	return buf
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

// Regression for the Dropbear Ed25519 bug: the agent used to publish the whole
// `private scalar || public point` field (64 bytes), producing an 83-byte blob.
// An OpenSSH client compares the 32-byte point it receives with the stored
// blob, so such a key never matched and every connection prompted.
func TestValidateSSHPublicKeyRejectsOverlongDropbearRecord(t *testing.T) {
	// The exact blob Easy42 used to write for a real OpenWrt node.
	overlong := "AAAAC3NzaC1lZDI1NTE5AAAAQCFU8KveP2LWnDvSSUn9ZMnV+SBhmY0AXwev2GgLz0kPEIUCoG9TXXZ/vFp17idehMxMWyxdWNCUJS0pyVyOxXU="
	if err := ValidateSSHPublicKey("ssh-ed25519", overlong); err == nil {
		t.Fatal("expected the over-long Dropbear ed25519 record to be rejected")
	}
	if IsExactSSHPublicKeyBlob("ssh-ed25519", mustBase64(t, overlong)) {
		t.Fatal("IsExactSSHPublicKeyBlob must reject the 64-byte-point blob")
	}

	// The correct key for the same node, as `ssh-keyscan` reports it.
	exact := "AAAAC3NzaC1lZDI1NTE5AAAAIBCFAqBvU112f7xade4nXoTMTFssXVjQlCUtKclcjsV1"
	if err := ValidateSSHPublicKey("ssh-ed25519", exact); err != nil {
		t.Fatalf("expected the exact 32-byte key to be valid, got %v", err)
	}

	// The current home2 keys must survive the round trip through the block
	// builder, or the fix would be cosmetic.
	node := SSHPublicKey{
		Algorithm: "ssh-ed25519",
		Key:       exact,
		Kind:      SSHKeyKindHost,
		Hostnames: []string{"192.168.2.1", "home2"},
	}
	block := BuildSSHKeysBlock([]SSHPublicKey{node}, false)
	if !strings.Contains(block, exact) {
		t.Fatalf("known_hosts block must contain the exact key, got:\n%s", block)
	}
}

func TestIsExactSSHPublicKeyBlob(t *testing.T) {
	ed := mustBase64(t, testSSHKeyBlob("ssh-ed25519", "node"))
	if !IsExactSSHPublicKeyBlob("ssh-ed25519", ed) {
		t.Fatal("well-formed ed25519 key must be exact")
	}
	if IsExactSSHPublicKeyBlob("ssh-ed25519", append(append([]byte{}, ed...), 0x00)) {
		t.Fatal("trailing byte must be rejected")
	}

	rsa := mustBase64(t, testSSHKeyBlob("ssh-rsa", "node"))
	if !IsExactSSHPublicKeyBlob("ssh-rsa", rsa) {
		t.Fatal("well-formed rsa key must be exact")
	}
	// RSA records carry d/p/q after n; they must be rejected too.
	if IsExactSSHPublicKeyBlob("ssh-rsa", append(append([]byte{}, rsa...), 0, 0, 0, 1, 9)) {
		t.Fatal("rsa key with private fields appended must be rejected")
	}
}

func mustBase64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("invalid base64 fixture: %v", err)
	}
	return b
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

func TestBuildSSHKeysBlockKnownHostsMergesSharedKeys(t *testing.T) {
	// Cloned images share their host key; both nodes' host names must survive.
	key := testSSHKeyBlob("ssh-ed25519", "shared")
	a := SSHPublicKey{Algorithm: "ssh-ed25519", Key: key, Kind: SSHKeyKindHost, Hostnames: []string{"10.0.0.1", "node-a"}}
	b := SSHPublicKey{Algorithm: "ssh-ed25519", Key: key, Kind: SSHKeyKindHost, Hostnames: []string{"10.0.0.2", "node-b"}}
	block := BuildSSHKeysBlock([]SSHPublicKey{a, b}, false)
	want := "### EASY42_START\n10.0.0.1,node-a,10.0.0.2,node-b ssh-ed25519 " + key + "\n### EASY42_END\n"
	if block != want {
		t.Fatalf("shared host key names must be merged, got:\n%q\nwant:\n%q", block, want)
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
