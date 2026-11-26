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

// SSH key roles. A node publishes two kinds of public keys: the root login
// identities used by its SSH *client* and the SSH *server* host keys its sshd
// presents.
const (
	// SSHKeyKindUser is a root login identity (an agent's ~/.ssh/id_*.pub file).
	// It is installed into authorized_keys, which lets this node log in.
	SSHKeyKindUser = "user"
	// SSHKeyKindHost is an SSH server host key (an agent's /etc/ssh/ssh_host_*_key.pub
	// file). It is installed into known_hosts, which lets other nodes verify this
	// node's server identity when connecting to it.
	SSHKeyKindHost = "host"
)

// SSHPublicKey is a public key reported by a node agent.
type SSHPublicKey struct {
	Algorithm string `json:"algorithm"`         // e.g. "ssh-ed25519"
	Key       string `json:"key"`               // Base64 key blob
	Comment   string `json:"comment,omitempty"` // Optional trailing comment
	Path      string `json:"path,omitempty"`    // Source *.pub file on the node
	// Kind is the key role: SSHKeyKindUser (root login identity, default) or
	// SSHKeyKindHost (SSH server host key). It decides which managed file the
	// key belongs to: user keys go to authorized_keys, host keys to known_hosts.
	Kind string `json:"kind,omitempty"`
	// Hostnames are the host names this key is valid for. They are only used
	// in the known_hosts block, where OpenSSH requires a comma separated host
	// list (or a hashed host) before the key. Entries in authorized_keys ignore
	// them. They are never reported by an agent; the controller fills them in
	// from the owning node definition.
	Hostnames []string `json:"hostnames,omitempty"`
}

// Normalize trims all fields and returns the normalized key.
func (k SSHPublicKey) Normalize() SSHPublicKey {
	return SSHPublicKey{
		Algorithm: strings.TrimSpace(k.Algorithm),
		Key:       strings.TrimSpace(k.Key),
		Comment:   strings.TrimSpace(k.Comment),
		Path:      strings.TrimSpace(k.Path),
		Kind:      k.NormalizedKind(),
		Hostnames: normalizeSSHHostnames(k.Hostnames),
	}
}

// NormalizedKind returns the key role, defaulting to a user key when the agent
// did not report one (agents that predate host key collection).
func (k SSHPublicKey) NormalizedKind() string {
	if strings.EqualFold(strings.TrimSpace(k.Kind), SSHKeyKindHost) {
		return SSHKeyKindHost
	}
	return SSHKeyKindUser
}

// IsHostKey reports whether the key is an SSH server host key.
func (k SSHPublicKey) IsHostKey() bool {
	return k.NormalizedKind() == SSHKeyKindHost
}

// ForFile reports whether the key belongs in the given managed file: known_hosts
// holds SSH server host keys, authorized_keys holds root login identities.
func (k SSHPublicKey) ForFile(authorizedKeys bool) bool {
	if authorizedKeys {
		return !k.IsHostKey()
	}
	return k.IsHostKey()
}

// normalizeSSHHostnames trims, lowercases and deduplicates host names while
// preserving their order.
func normalizeSSHHostnames(hostnames []string) []string {
	if len(hostnames) == 0 {
		return nil
	}
	res := make([]string, 0, len(hostnames))
	seen := make(map[string]bool, len(hostnames))
	for _, h := range hostnames {
		h = strings.TrimSpace(h)
		if !strings.HasPrefix(h, "|") {
			h = strings.ToLower(h)
		}
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		res = append(res, h)
	}
	if len(res) == 0 {
		return nil
	}
	return res
}

// NormalizeBaseDomain cleans a Cloudflare "Base Domain" setting so it can be
// appended to a node name (scheme, path and surrounding dots are dropped).
func NormalizeBaseDomain(domain string) string {
	d := strings.ToLower(strings.TrimSpace(domain))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	if idx := strings.Index(d, "/"); idx != -1 {
		d = d[:idx]
	}
	return strings.Trim(d, ".")
}

// NodeKnownHostsHostnames returns the host names a node is reachable under and
// therefore which identities it may be known as in known_hosts: the main and
// external IP addresses, the node name, <name>.internal and, when a DNS base
// domain is configured, <name>.<base domain>.
func NodeKnownHostsHostnames(node Node, baseDomain string) []string {
	var raw []string
	if ip := CleanCIDR(node.IP); ip != "" {
		raw = append(raw, ip)
	}
	if ip6 := CleanCIDR(node.IP6); ip6 != "" {
		raw = append(raw, ip6)
	}
	if ip := CleanCIDR(node.ExternalIP); ip != "" {
		raw = append(raw, ip)
	}
	if ip6 := CleanCIDR(node.ExternalIP6); ip6 != "" {
		raw = append(raw, ip6)
	}
	name := strings.TrimSpace(node.Name)
	if name != "" {
		raw = append(raw, name, name+".internal")
		if base := NormalizeBaseDomain(baseDomain); base != "" {
			raw = append(raw, name+"."+base)
		}
	}
	return normalizeSSHHostnames(raw)
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

// SSHKeyRule maps a set of source nodes' public keys (or direct source key
// lines) onto the SSH key files of a set of destination nodes.
type SSHKeyRule struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"` // "known_hosts" | "authorized_keys" | "all"
	SrcSelector map[string]string `json:"src_selector,omitempty"`
	SourceKeys  string            `json:"source_keys,omitempty"`
	DstSelector map[string]string `json:"dst_selector"`
	Enabled     bool              `json:"enabled"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// HasSourceKeys reports whether the rule specifies direct source key lines
// instead of a source node selector.
func (r SSHKeyRule) HasSourceKeys() bool {
	return strings.TrimSpace(r.SourceKeys) != ""
}

// NormalizeSourceKeys normalizes line endings and trims surrounding whitespace.
func NormalizeSourceKeys(raw string) string {
	norm := strings.ReplaceAll(raw, "\r\n", "\n")
	norm = strings.ReplaceAll(norm, "\r", "\n")
	return strings.TrimSpace(norm)
}

// ParseSSHSourceKeys parses direct source key lines in authorized_keys
// (`<algorithm> <base64-key> [comment]`) or known_hosts
// (`<hostnames> <algorithm> <base64-key> [comment]`) format.
// Blank lines and `#` comments are ignored.
func ParseSSHSourceKeys(raw string) ([]SSHPublicKey, error) {
	norm := NormalizeSourceKeys(raw)
	if norm == "" {
		return nil, fmt.Errorf("cannot be empty: provide at least one valid SSH public key line")
	}

	lines := strings.Split(norm, "\n")
	keys := make([]SSHPublicKey, 0, len(lines))
	for i, rawLine := range lines {
		lineNum := i + 1
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if hasControlOrNewline(line) {
			return nil, fmt.Errorf("line %d: contains invalid control characters", lineNum)
		}
		fields := strings.Fields(line)
		switch {
		case len(fields) >= 2 && IsSupportedSSHKeyAlgorithm(fields[0]):
			alg, blob := fields[0], fields[1]
			if err := ValidateSSHPublicKey(alg, blob); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			keys = append(keys, SSHPublicKey{
				Algorithm: alg,
				Key:       blob,
				Comment:   trailingSSHKeyComment(line, 2),
				Kind:      SSHKeyKindUser,
			})
		case len(fields) >= 3 && IsSupportedSSHKeyAlgorithm(fields[1]):
			hosts := normalizeSSHHostnames(strings.Split(fields[0], ","))
			if len(hosts) == 0 {
				return nil, fmt.Errorf("line %d: known_hosts entry must include at least one hostname", lineNum)
			}
			alg, blob := fields[1], fields[2]
			if err := ValidateSSHPublicKey(alg, blob); err != nil {
				return nil, fmt.Errorf("line %d: %w", lineNum, err)
			}
			keys = append(keys, SSHPublicKey{
				Algorithm: alg,
				Key:       blob,
				Comment:   trailingSSHKeyComment(line, 3),
				Kind:      SSHKeyKindHost,
				Hostnames: hosts,
			})
		default:
			return nil, fmt.Errorf("line %d: invalid SSH key line (expected '<algorithm> <key> [comment]' or '<hostnames> <algorithm> <key> [comment]')", lineNum)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("cannot be empty: provide at least one valid SSH public key line")
	}
	return keys, nil
}

// trailingSSHKeyComment returns the remainder of line after skipping n whitespace-delimited fields.
func trailingSSHKeyComment(line string, skipFields int) string {
	idx := 0
	n := len(line)
	for f := 0; f < skipFields; f++ {
		for idx < n && (line[idx] == ' ' || line[idx] == '\t') {
			idx++
		}
		for idx < n && line[idx] != ' ' && line[idx] != '\t' {
			idx++
		}
	}
	return strings.TrimSpace(line[idx:])
}

// ValidateSSHSourceKeysForType parses raw source keys and verifies that every
// key matches the rule type (user keys for authorized_keys, host keys for
// known_hosts, or either for all).
func ValidateSSHSourceKeysForType(raw, ruleType string) ([]SSHPublicKey, error) {
	keys, err := ParseSSHSourceKeys(raw)
	if err != nil {
		return nil, err
	}
	switch ruleType {
	case SSHKeyFileAuthorizedKeys:
		for _, k := range keys {
			if k.IsHostKey() {
				return nil, fmt.Errorf("known_hosts entry is not valid for rule type '%s' (expected '<algorithm> <key> [comment]')", SSHKeyFileAuthorizedKeys)
			}
		}
	case SSHKeyFileKnownHosts:
		for _, k := range keys {
			if !k.IsHostKey() {
				return nil, fmt.Errorf("authorized_keys entry is not valid for rule type '%s' (expected '<hostnames> <algorithm> <key> [comment]')", SSHKeyFileKnownHosts)
			}
		}
	}
	return keys, nil
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

// ValidateSSHKeyRule validates a rule definition. Either a source node selector
// or direct source keys must be provided (never both), and the destination
// selector must be explicit.
func ValidateSSHKeyRule(rule SSHKeyRule) error {
	if strings.TrimSpace(rule.ID) != "" {
		if err := ValidateSSHKeyRuleID(rule.ID); err != nil {
			return err
		}
	}
	if _, err := normalizeSSHKeyRuleType(rule.Type); err != nil {
		return err
	}
	hasSrcSelector := len(rule.SrcSelector) > 0
	hasSourceKeys := rule.HasSourceKeys()
	if hasSrcSelector && hasSourceKeys {
		return fmt.Errorf("cannot specify both source node selector and source keys")
	}
	if !hasSrcSelector && !hasSourceKeys {
		return fmt.Errorf("source node selector or source keys cannot be empty: choose source nodes or provide source key lines")
	}
	if hasSrcSelector {
		if err := ValidateSelector(rule.SrcSelector); err != nil {
			return fmt.Errorf("source node selector: %w", err)
		}
	}
	if hasSourceKeys {
		if _, err := ValidateSSHSourceKeysForType(rule.SourceKeys, rule.NormalizedType()); err != nil {
			return fmt.Errorf("source keys: %w", err)
		}
	}
	if err := ValidateSelector(rule.DstSelector); err != nil {
		return fmt.Errorf("destination node selector: %w", err)
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
var sshKeyRuleIDRegex = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidateSSHKeyRuleID validates a custom SSH key rule ID.
func ValidateSSHKeyRuleID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("rule ID cannot be empty")
	}
	if !sshKeyRuleIDRegex.MatchString(id) {
		return fmt.Errorf("invalid rule ID '%s': must be 1-64 characters, start with a letter or digit, and contain only letters, digits, '.', '_' or '-'", id)
	}
	return nil
}

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
	// The blob must contain exactly the public fields. A longer blob (for
	// example a Dropbear Ed25519 record with the private scalar left in front of
	// the 32-byte point) parses as a valid key here but never matches the key an
	// OpenSSH client compares against, so reject it instead of publishing a
	// known_hosts entry that cannot work.
	if !IsExactSSHPublicKeyBlob(algorithm, blob) {
		return fmt.Errorf("invalid SSH public key blob: unexpected trailing data")
	}
	return nil
}

// IsExactSSHPublicKeyBlob reports whether blob is exactly one SSH public key of
// the given algorithm: the algorithm string followed by just its public fields
// and nothing else. The caller has already checked the algorithm string.
func IsExactSSHPublicKeyBlob(algorithm string, blob []byte) bool {
	fields := blob[4+len(algorithm):]
	pos := 0
	// next consumes one length-prefixed field and returns its bytes.
	next := func() ([]byte, bool) {
		if len(fields)-pos < 4 {
			return nil, false
		}
		n := int(binary.BigEndian.Uint32(fields[pos : pos+4]))
		start := pos + 4
		if n < 0 || n > len(fields)-start {
			return nil, false
		}
		pos = start + n
		return fields[start:pos], true
	}

	switch algorithm {
	case "ssh-rsa":
		// e, n
		if e, ok := next(); !ok || len(e) == 0 {
			return false
		}
		if n, ok := next(); !ok || len(n) == 0 {
			return false
		}
	case "ssh-ed25519":
		// Q: exactly one 32-byte point.
		if q, ok := next(); !ok || len(q) != 32 {
			return false
		}
	case "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521":
		// curve name, Q
		curve, ok := next()
		if !ok || string(curve) != strings.TrimPrefix(algorithm, "ecdsa-sha2-") {
			return false
		}
		q, ok := next()
		if !ok || len(q) == 0 || q[0] != 0x04 {
			return false
		}
		want := map[string]int{
			"ecdsa-sha2-nistp256": 65,
			"ecdsa-sha2-nistp384": 97,
			"ecdsa-sha2-nistp521": 133,
		}[algorithm]
		if len(q) != want {
			return false
		}
	case "ssh-dss":
		// p, q, g, y
		for i := 0; i < 4; i++ {
			if f, ok := next(); !ok || len(f) == 0 {
				return false
			}
		}
	default:
		return false
	}
	return pos == len(fields)
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
// told apart from manually added ones. Entries installed into known_hosts are
// prefixed with the comma separated host name list, which OpenSSH requires
// before the key type; keys without host names are skipped there because such
// a line is invalid and would make the whole file unparsable.
func sanitizeSSHComment(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '\r' || r == '\n' {
			break
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func hasControlOrNewline(s string) bool {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}

func SSHKeyLine(key SSHPublicKey, authorizedKeys bool) string {
	key = key.Normalize()
	if hasControlOrNewline(key.Algorithm) || hasControlOrNewline(key.Key) {
		return ""
	}
	comment := sanitizeSSHComment(key.Comment)
	prefix := ""
	if authorizedKeys {
		if comment == "" {
			comment = "easy42"
		} else {
			comment = "easy42 " + comment
		}
	} else {
		prefix = strings.Join(key.Hostnames, ",")
		if prefix == "" || hasControlOrNewline(prefix) {
			return ""
		}
	}
	line := key.Algorithm + " " + key.Key
	if prefix != "" {
		line = prefix + " " + line
	}
	if comment == "" {
		return line
	}
	return line + " " + comment
}

// BuildSSHKeysLines assembles the deterministic Easy42 lines for the given keys.
// Invalid keys, duplicate key material and keys that do not belong in the target
// file (user identities in known_hosts, host keys in authorized_keys) are
// skipped. Lines are sorted so the resulting output only changes when the
// effective key set changes.
func BuildSSHKeysLines(keys []SSHPublicKey, authorizedKeys bool) []string {
	lines := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))

	// Identical key material can be published by several nodes (cloned VM images
	// share their SSH host key). known_hosts needs one line per key, so for host
	// keys the host name lists of all publishers are merged; dropping later nodes
	// would silently leave those hosts untrusted.
	type hostKeyEntry struct {
		key   SSHPublicKey
		names []string
	}
	merged := make(map[string]*hostKeyEntry, len(keys))
	order := make([]string, 0, len(keys))

	for _, k := range keys {
		k = k.Normalize()
		if !k.IsValid() || !k.ForFile(authorizedKeys) {
			continue
		}
		fp := k.fingerprint()
		if authorizedKeys {
			if seen[fp] {
				continue
			}
			seen[fp] = true
			lines = append(lines, SSHKeyLine(k, authorizedKeys))
			continue
		}
		if len(k.Hostnames) == 0 {
			continue
		}
		if e, ok := merged[fp]; ok {
			e.names = append(e.names, k.Hostnames...)
			continue
		}
		merged[fp] = &hostKeyEntry{key: k, names: append([]string(nil), k.Hostnames...)}
		order = append(order, fp)
	}
	for _, fp := range order {
		e := merged[fp]
		e.key.Hostnames = normalizeSSHHostnames(e.names)
		lines = append(lines, SSHKeyLine(e.key, false))
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
	return uniq
}

// BuildSSHKeysBlock assembles the deterministic Easy42 block for the given keys.
// Invalid keys, duplicate key material and keys that do not belong in the target
// file (user identities in known_hosts, host keys in authorized_keys) are
// skipped. Lines are sorted so the resulting block hash only changes when the
// effective key set changes.
func BuildSSHKeysBlock(keys []SSHPublicKey, authorizedKeys bool) string {
	uniq := BuildSSHKeysLines(keys, authorizedKeys)

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
