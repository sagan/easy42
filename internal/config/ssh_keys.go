package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SSH key file management constants.
const (
	// SSHKeyFileKnownHosts is the OpenSSH known_hosts file managed on target nodes.
	SSHKeyFileKnownHosts = "known_hosts"
	// SSHKeyFileAuthorizedKeys is the OpenSSH authorized_keys file managed on target nodes.
	SSHKeyFileAuthorizedKeys = "authorized_keys"
	// SSHKeyRuleTypeAll manages both files.
	SSHKeyRuleTypeAll = "all"

	// SSHKnownHostsPath is the absolute path of the managed known_hosts file.
	SSHKnownHostsPath = "/root/.ssh/known_hosts"
	// SSHAuthorizedKeysPath is the absolute path of the managed authorized_keys file.
	SSHAuthorizedKeysPath = "/root/.ssh/authorized_keys"

	// SSHKeysStartMarker starts the Easy42-managed block in SSH key files.
	SSHKeysStartMarker = "### EASY42_START"
	// SSHKeysEndMarker ends the Easy42-managed block in SSH key files.
	SSHKeysEndMarker = "### EASY42_END"
)

// MinAgentVersionForSSHKeys is the first bundled agent version that supports
// root SSH public key collection and SSH key file management.
const MinAgentVersionForSSHKeys = "0.1.18"

// SSHKeyFilePaths maps a rule file type to the target paths it manages.
var SSHKeyFilePaths = map[string][]string{
	SSHKeyFileKnownHosts:     {SSHKnownHostsPath},
	SSHKeyFileAuthorizedKeys: {SSHAuthorizedKeysPath},
	SSHKeyRuleTypeAll:        {SSHKnownHostsPath, SSHAuthorizedKeysPath},
}

// SSHPublicKey is a root user public key reported by a node agent.
type SSHPublicKey struct {
	Algorithm string `json:"algorithm"`         // e.g. "ssh-ed25519"
	Key       string `json:"key"`               // Base64 key blob
	Comment   string `json:"comment,omitempty"` // Optional trailing comment
	Path      string `json:"path,omitempty"`    // Source *.pub file on the node
}

// Normalize trims all fields and returns the normalized key.
func (k SSHPublicKey) Normalize() SSHPublicKey {
	return SSHPublicKey{
		Algorithm: strings.TrimSpace(k.Algorithm),
		Key:       strings.TrimSpace(k.Key),
		Comment:   strings.TrimSpace(k.Comment),
		Path:      strings.TrimSpace(k.Path),
	}
}

// IsValid reports whether the key has a supported algorithm and a well formed
// key blob that embeds the algorithm name.
func (k SSHPublicKey) IsValid() bool {
	return ValidateSSHPublicKey(k.Algorithm, k.Key) == nil
}

// fingerprint identifies the key material (algorithm + blob).
func (k SSHPublicKey) fingerprint() string {
	return k.Algorithm + "\x00" + k.Key
}

// SSHKeyRule maps a set of source nodes' public keys onto the SSH key files of
// a set of destination nodes.
type SSHKeyRule struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"` // "known_hosts" | "authorized_keys" | "all"
	SrcSelector map[string]string `json:"src_selector"`
	DstSelector map[string]string `json:"dst_selector"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// normalizeSSHKeyRuleType returns the canonical rule type or an error.
func normalizeSSHKeyRuleType(t string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case SSHKeyFileKnownHosts:
		return SSHKeyFileKnownHosts, nil
	case SSHKeyFileAuthorizedKeys:
		return SSHKeyFileAuthorizedKeys, nil
	case SSHKeyRuleTypeAll, "":
		return SSHKeyRuleTypeAll, nil
	default:
		return "", fmt.Errorf("invalid SSH key rule type '%s': must be '%s', '%s' or '%s'", t, SSHKeyFileKnownHosts, SSHKeyFileAuthorizedKeys, SSHKeyRuleTypeAll)
	}
}

// ValidateSSHKeyRule validates a rule definition. Both selectors must be
// explicit: an empty selector matches no node, so a rule can never be saved or
// applied without an intentional target.
func ValidateSSHKeyRule(rule SSHKeyRule) error {
	if _, err := normalizeSSHKeyRuleType(rule.Type); err != nil {
		return err
	}
	if err := ValidateSelector(rule.SrcSelector); err != nil {
		return fmt.Errorf("source node selector: %w", err)
	}
	if err := ValidateSelector(rule.DstSelector); err != nil {
		return fmt.Errorf("destination node selector: %w", err)
	}
	if len(rule.SrcSelector) == 0 {
		return fmt.Errorf("source node selector cannot be empty: choose the source nodes explicitly or select all nodes")
	}
	if len(rule.DstSelector) == 0 {
		return fmt.Errorf("destination node selector cannot be empty: choose the destination nodes explicitly or select all nodes")
	}
	return nil
}

// NormalizedType returns the canonical rule type.
func (r SSHKeyRule) NormalizedType() string {
	t, err := normalizeSSHKeyRuleType(r.Type)
	if err != nil {
		return SSHKeyFileKnownHosts
	}
	return t
}

// TargetFiles returns the absolute SSH key file paths this rule manages.
func (r SSHKeyRule) TargetFiles() []string {
	paths := SSHKeyFilePaths[r.NormalizedType()]
	res := make([]string, len(paths))
	copy(res, paths)
	return res
}

// sshKeyAlgorithms lists the OpenSSH public key algorithms accepted from agents.
var sshKeyAlgorithms = map[string]bool{
	"ssh-rsa":                            true,
	"ssh-dss":                            true,
	"ssh-ed25519":                        true,
	"ssh-ed25519-sk":                     true,
	"ecdsa-sha2-nistp256":                true,
	"ecdsa-sha2-nistp384":                true,
	"ecdsa-sha2-nistp521":                true,
	"sk-ssh-ed25519@openssh.com":         true,
	"sk-ecdsa-sha2-nistp256@openssh.com": true,
}

var sshKeyBase64Regex = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)

// IsSupportedSSHKeyAlgorithm reports whether algorithm is a known OpenSSH key type.
func IsSupportedSSHKeyAlgorithm(algorithm string) bool {
	return sshKeyAlgorithms[strings.TrimSpace(algorithm)]
}

// ValidateSSHPublicKey validates an algorithm + base64 key blob pair. The blob
// must be valid base64 and start with its own SSH string-encoded algorithm.
func ValidateSSHPublicKey(algorithm, key string) error {
	algorithm = strings.TrimSpace(algorithm)
	key = strings.TrimSpace(key)
	if algorithm == "" {
		return fmt.Errorf("SSH key algorithm cannot be empty")
	}
	if !IsSupportedSSHKeyAlgorithm(algorithm) {
		return fmt.Errorf("unsupported SSH key algorithm '%s'", algorithm)
	}
	if key == "" || len(key) > 16*1024 || !sshKeyBase64Regex.MatchString(key) {
		return fmt.Errorf("invalid SSH public key blob")
	}
	blob, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return fmt.Errorf("invalid SSH public key blob: %w", err)
	}
	if len(blob) < 4 {
		return fmt.Errorf("invalid SSH public key blob: too short")
	}
	nameLen := int(binary.BigEndian.Uint32(blob[:4]))
	if nameLen != len(algorithm) || nameLen > len(blob)-4 || string(blob[4:4+nameLen]) != algorithm {
		return fmt.Errorf("invalid SSH public key blob: algorithm mismatch")
	}
	return nil
}

// SSHKeySupportedByAgent reports whether an agent version supports SSH key
// management. Unknown (empty/unparseable) versions are treated as unsupported
// because only newer agents report collected keys.
func SSHKeySupportedByAgent(agentVersion string) bool {
	return CompareAgentVersions(agentVersion, MinAgentVersionForSSHKeys) >= 0
}

// CompareAgentVersions compares two semver-like version strings. A leading "v"
// is ignored and missing numeric components count as zero. A non-numeric or
// empty version sorts before any numeric version.
func CompareAgentVersions(a, b string) int {
	pa := parseAgentVersion(a)
	pb := parseAgentVersion(b)
	if len(pa) == 0 || len(pb) == 0 {
		switch {
		case len(pa) == 0 && len(pb) == 0:
			return 0
		case len(pa) == 0:
			return -1
		default:
			return 1
		}
	}
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parseAgentVersion(v string) []int {
	v = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "v"))
	v = strings.TrimPrefix(v, "V")
	if v == "" {
		return nil
	}
	// Drop pre-release / build metadata.
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	res := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return nil
		}
		res = append(res, n)
	}
	return res
}

// SSHKeyLine builds the single line written for a key. Entries installed into
// authorized_keys get a fixed comment prefix so cluster managed keys can be
// told apart from manually added ones.
func SSHKeyLine(key SSHPublicKey, authorizedKeys bool) string {
	key = key.Normalize()
	comment := key.Comment
	if authorizedKeys {
		if comment == "" {
			comment = "easy42"
		} else {
			comment = "easy42 " + comment
		}
	}
	if comment == "" {
		return fmt.Sprintf("%s %s", key.Algorithm, key.Key)
	}
	return fmt.Sprintf("%s %s %s", key.Algorithm, key.Key, comment)
}

// BuildSSHKeysBlock assembles the deterministic Easy42 block for the given keys.
// Invalid keys and duplicate key material are skipped. Lines are sorted so the
// resulting block hash only changes when the effective key set changes.
func BuildSSHKeysBlock(keys []SSHPublicKey, authorizedKeys bool) string {
	lines := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		k = k.Normalize()
		if !k.IsValid() {
			continue
		}
		fp := k.fingerprint()
		if seen[fp] {
			continue
		}
		seen[fp] = true
		lines = append(lines, SSHKeyLine(k, authorizedKeys))
	}
	sort.Strings(lines)
	// Defensive dedupe for keys that only differ by comment.
	uniq := lines[:0]
	for i, l := range lines {
		if i > 0 && lines[i-1] == l {
			continue
		}
		uniq = append(uniq, l)
	}

	var sb strings.Builder
	sb.WriteString(SSHKeysStartMarker)
	sb.WriteString("\n")
	for _, l := range uniq {
		sb.WriteString(l)
		sb.WriteString("\n")
	}
	sb.WriteString(SSHKeysEndMarker)
	sb.WriteString("\n")
	return sb.String()
}

// HashSSHKeysBlock computes the SHA-256 hash of the normalized block content.
func HashSSHKeysBlock(block string) string {
	norm := strings.ReplaceAll(block, "\r\n", "\n")
	trimmed := strings.TrimSpace(norm)
	if trimmed == "" {
		return ""
	}
	h := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(h[:])
}

// IsSSHKeyTargetPath reports whether path is one of the managed SSH key files.
func IsSSHKeyTargetPath(path string) bool {
	path = strings.TrimSpace(path)
	return path == SSHKnownHostsPath || path == SSHAuthorizedKeysPath
}

// IsAuthorizedKeysPath reports whether path is the managed authorized_keys file.
func IsAuthorizedKeysPath(path string) bool {
	return strings.TrimSpace(path) == SSHAuthorizedKeysPath
}
