package ssh

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kevinburke/ssh_config"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ClientPool manages SSH and SFTP connections to remote hosts
type ClientPool struct {
	mu        sync.RWMutex
	hostMu    sync.Mutex
	hostLocks map[string]*sync.Mutex
	clients   map[string]*PooledClient
}

// PooledClient wraps an active SSH client and SFTP client
type PooledClient struct {
	SSHClient  *ssh.Client
	SFTPClient *sftp.Client
	LastUsed   time.Time
}

// NewClientPool creates a new SSH connection pool
func NewClientPool() *ClientPool {
	return &ClientPool{
		hostLocks: make(map[string]*sync.Mutex),
		clients:   make(map[string]*PooledClient),
	}
}

func (p *ClientPool) getHostLock(host string) *sync.Mutex {
	p.hostMu.Lock()
	defer p.hostMu.Unlock()
	if p.hostLocks == nil {
		p.hostLocks = make(map[string]*sync.Mutex)
	}
	l, exists := p.hostLocks[host]
	if !exists {
		l = &sync.Mutex{}
		p.hostLocks[host] = l
	}
	return l
}

// CloseHost closes and removes the cached SSH and SFTP connections for a specific host
func (p *ClientPool) CloseHost(hostAliasOrIP string) {
	hostAliasOrIP = strings.TrimSpace(hostAliasOrIP)
	hostLock := p.getHostLock(hostAliasOrIP)
	hostLock.Lock()
	defer hostLock.Unlock()

	p.mu.Lock()
	pc, exists := p.clients[hostAliasOrIP]
	if exists {
		delete(p.clients, hostAliasOrIP)
	}
	p.mu.Unlock()

	if exists && pc != nil {
		if pc.SFTPClient != nil {
			_ = pc.SFTPClient.Close()
		}
		if pc.SSHClient != nil {
			_ = pc.SSHClient.Close()
		}
	}
}

// CloseAll closes all cached SSH and SFTP connections
func (p *ClientPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, pc := range p.clients {
		if pc.SFTPClient != nil {
			_ = pc.SFTPClient.Close()
		}
		if pc.SSHClient != nil {
			_ = pc.SSHClient.Close()
		}
	}
	p.clients = make(map[string]*PooledClient)
}

// GetClient returns an active SSH and SFTP client for a host.
// It uses host-level locking so dials to different hosts execute concurrently.
func (p *ClientPool) GetClient(hostAliasOrIP string) (*ssh.Client, *sftp.Client, error) {
	return p.GetClientWithTimeout(hostAliasOrIP, 5*time.Second)
}

// GetClientWithTimeout returns an active SSH and SFTP client with a custom dial timeout.
func (p *ClientPool) GetClientWithTimeout(hostAliasOrIP string, dialTimeout time.Duration) (*ssh.Client, *sftp.Client, error) {
	hostAliasOrIP = strings.TrimSpace(hostAliasOrIP)
	hostLock := p.getHostLock(hostAliasOrIP)
	hostLock.Lock()
	defer hostLock.Unlock()

	p.mu.RLock()
	pc, exists := p.clients[hostAliasOrIP]
	p.mu.RUnlock()

	if exists {
		// Test connection health without holding the global pool lock
		if _, _, err := pc.SSHClient.SendRequest("keepalive@easy42", true, nil); err == nil {
			pc.LastUsed = time.Now()
			return pc.SSHClient, pc.SFTPClient, nil
		}
		// Connection dead, close it
		if pc.SFTPClient != nil {
			_ = pc.SFTPClient.Close()
		}
		if pc.SSHClient != nil {
			_ = pc.SSHClient.Close()
		}
		p.mu.Lock()
		delete(p.clients, hostAliasOrIP)
		p.mu.Unlock()
	}

	sshClient, err := DialSSHWithTimeout(hostAliasOrIP, dialTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to dial ssh for %s: %w", hostAliasOrIP, err)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, nil, fmt.Errorf("failed to create sftp client for %s: %w", hostAliasOrIP, err)
	}

	pc = &PooledClient{
		SSHClient:  sshClient,
		SFTPClient: sftpClient,
		LastUsed:   time.Now(),
	}

	p.mu.Lock()
	p.clients[hostAliasOrIP] = pc
	p.mu.Unlock()

	return sshClient, sftpClient, nil
}

// ParseSSHHost parses an SSH host string which may contain user, host/IP, and port.
// Supported formats:
//   - host
//   - user@host
//   - host:port
//   - user@host:port
//   - [ipv6]
//   - [ipv6]:port
//   - user@[ipv6]
//   - user@[ipv6]:port
//   - raw ipv6 (e.g. 2001:db8::1)
//   - user@ipv6 (e.g. root@2001:db8::1)
func ParseSSHHost(raw string) (user, host, port string) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", "", ""
	}

	// 1. Extract user if present (user@...)
	if idx := strings.LastIndex(s, "@"); idx != -1 {
		user = s[:idx]
		s = s[idx+1:]
	}

	// 2. Check if bracketed IPv6 without port: [2001:db8::1]
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return user, s[1 : len(s)-1], ""
	}

	// 3. Try net.SplitHostPort (handles host:port and [ipv6]:port)
	if h, p, err := net.SplitHostPort(s); err == nil {
		if _, err := strconv.Atoi(p); err == nil {
			return user, h, p
		}
	}

	// 4. Otherwise, it is a plain host, IPv4, or raw IPv6 (e.g. 2001:db8::1)
	return user, s, ""
}

// DialSSH connects to a host using OpenSSH config, agent, and standard keys with default 5s timeout
func DialSSH(hostAliasOrIP string) (*ssh.Client, error) {
	return DialSSHWithTimeout(hostAliasOrIP, 5*time.Second)
}

// DialSSHWithTimeout connects to a host with a specified connection timeout
func DialSSHWithTimeout(hostAliasOrIP string, timeout time.Duration) (*ssh.Client, error) {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	explicitUser, hostPart, explicitPort := ParseSSHHost(hostAliasOrIP)

	// 1. Resolve OpenSSH config (Host alias, User, Port, HostName, IdentityFile, HostKeyAlgorithms)
	var realHost, user, portStr, identityFile, hostKeyAlgosConfig string

	sshConfigPath := filepath.Join(home, ".ssh", "config")
	if f, err := os.Open(sshConfigPath); err == nil {
		cfg, err := ssh_config.Decode(f)
		_ = f.Close()
		if err == nil {
			realHost, _ = cfg.Get(hostAliasOrIP, "HostName")
			if (realHost == "" || realHost == "%h") && hostPart != hostAliasOrIP {
				realHost, _ = cfg.Get(hostPart, "HostName")
			}
			user, _ = cfg.Get(hostAliasOrIP, "User")
			if user == "" && hostPart != hostAliasOrIP {
				user, _ = cfg.Get(hostPart, "User")
			}
			portStr, _ = cfg.Get(hostAliasOrIP, "Port")
			if portStr == "" && hostPart != hostAliasOrIP {
				portStr, _ = cfg.Get(hostPart, "Port")
			}
			identityFile, _ = cfg.Get(hostAliasOrIP, "IdentityFile")
			if identityFile == "" && hostPart != hostAliasOrIP {
				identityFile, _ = cfg.Get(hostPart, "IdentityFile")
			}
			hostKeyAlgosConfig, _ = cfg.Get(hostAliasOrIP, "HostKeyAlgorithms")
			if hostKeyAlgosConfig == "" && hostPart != hostAliasOrIP {
				hostKeyAlgosConfig, _ = cfg.Get(hostPart, "HostKeyAlgorithms")
			}
		}
	}

	if realHost == "" || realHost == "%h" {
		realHost = hostPart
	} else {
		cfgUser, cleanRealHost, cfgPort := ParseSSHHost(realHost)
		if cleanRealHost != "" {
			realHost = cleanRealHost
		}
		if user == "" && cfgUser != "" {
			user = cfgUser
		}
		if portStr == "" && cfgPort != "" {
			portStr = cfgPort
		}
	}

	if explicitUser != "" {
		user = explicitUser
	} else if user == "" {
		user = os.Getenv("USER")
		if user == "" {
			user = "root"
		}
	}

	port := 22
	if explicitPort != "" {
		if p, err := strconv.Atoi(explicitPort); err == nil && p > 0 {
			port = p
		}
	} else if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
			port = p
		}
	}

	// 2. Discover Auth Methods
	var authMethods []ssh.AuthMethod

	// SSH Agent
	if authSock := os.Getenv("SSH_AUTH_SOCK"); authSock != "" {
		if conn, err := net.Dial("unix", authSock); err == nil {
			agentClient := agent.NewClient(conn)
			authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
		}
	}

	// Key files
	keyCandidates := []string{
		identityFile,
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_rsa"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
	}

	for _, kPath := range keyCandidates {
		if kPath == "" {
			continue
		}
		// Expand ~ if needed
		if strings.HasPrefix(kPath, "~/") {
			kPath = filepath.Join(home, kPath[2:])
		}
		if keyBytes, err := os.ReadFile(kPath); err == nil {
			if signer, err := ssh.ParsePrivateKey(keyBytes); err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH authentication methods (keys or agent) found for %s", hostAliasOrIP)
	}

	// 3. Host Key Callback and Algorithm Discovery
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	var khFiles []string
	for _, p := range []string{
		knownHostsPath,
		filepath.Join(home, ".ssh", "known_hosts2"),
		"/etc/ssh/ssh_known_hosts",
		"/etc/ssh/ssh_known_hosts2",
	} {
		if _, err := os.Stat(p); err == nil {
			khFiles = append(khFiles, p)
		}
	}
	if len(khFiles) == 0 {
		khFiles = []string{knownHostsPath}
	}

	khCallback, err := knownhosts.New(khFiles...)
	if err != nil {
		return nil, fmt.Errorf("failed to read known_hosts (%s): %w", knownHostsPath, err)
	}

	targetAddr := net.JoinHostPort(realHost, strconv.Itoa(port))

	// Probe known key types from known_hosts to prioritize matching algorithms
	probeAddrs := []string{targetAddr}
	if hostPart != "" && hostPart != realHost {
		probeAddrs = append(probeAddrs, net.JoinHostPort(hostPart, strconv.Itoa(port)))
	}
	if net.ParseIP(realHost) == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		var r net.Resolver
		if ips, err := r.LookupIP(ctx, "ip", realHost); err == nil {
			for _, ip := range ips {
				probeAddrs = append(probeAddrs, net.JoinHostPort(ip.String(), strconv.Itoa(port)))
			}
		}
		cancel()
	}

	knownTypes := getKnownKeyTypes(khCallback, probeAddrs...)
	hostKeyAlgorithms := orderHostKeyAlgorithms(knownTypes, hostKeyAlgosConfig)

	hostKeyCallback := func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := khCallback(hostname, remote, key)
		if err == nil {
			return nil
		}
		// 1. Try host alias if different from realHost
		if hostPart != "" && hostPart != realHost {
			aliasAddr := net.JoinHostPort(hostPart, strconv.Itoa(port))
			if err2 := khCallback(aliasAddr, remote, key); err2 == nil {
				return nil
			}
		}
		// 2. Try remote address if different from hostname
		if remote != nil && remote.String() != "" && remote.String() != hostname {
			if err2 := khCallback(remote.String(), remote, key); err2 == nil {
				return nil
			}
		}
		return err
	}

	clientConfig := &ssh.ClientConfig{
		User:              user,
		Auth:              authMethods,
		HostKeyCallback:   hostKeyCallback,
		HostKeyAlgorithms: hostKeyAlgorithms,
		Timeout:           timeout,
	}

	client, err := ssh.Dial("tcp", targetAddr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial to %s (%s) failed: %w", hostAliasOrIP, targetAddr, err)
	}

	return client, nil
}

// defaultHostKeyAlgorithms specifies OpenSSH-standard host key algorithm preference order
var defaultHostKeyAlgorithms = []string{
	ssh.CertAlgoED25519v01,
	ssh.CertAlgoECDSA256v01,
	ssh.CertAlgoECDSA384v01,
	ssh.CertAlgoECDSA521v01,
	ssh.CertAlgoRSASHA512v01,
	ssh.CertAlgoRSASHA256v01,
	ssh.CertAlgoRSAv01,
	ssh.KeyAlgoED25519,
	ssh.KeyAlgoECDSA256,
	ssh.KeyAlgoECDSA384,
	ssh.KeyAlgoECDSA521,
	ssh.KeyAlgoRSASHA512,
	ssh.KeyAlgoRSASHA256,
	ssh.KeyAlgoRSA,
}

var (
	probeKeyOnce sync.Once
	probePubKey  ssh.PublicKey
	probeAddr    = &net.TCPAddr{IP: net.IPv4zero, Port: 22}
)

func getProbePublicKey() ssh.PublicKey {
	probeKeyOnce.Do(func() {
		pub, err := ssh.NewPublicKey(ed25519.PublicKey(make([]byte, ed25519.PublicKeySize)))
		if err == nil {
			probePubKey = pub
		}
	})
	return probePubKey
}

// getKnownKeyTypes queries known_hosts callbacks with probe addresses to identify recorded key types
func getKnownKeyTypes(khCallback ssh.HostKeyCallback, addrs ...string) []string {
	probeKey := getProbePublicKey()
	if probeKey == nil {
		return nil
	}

	var keyTypes []string
	seen := make(map[string]bool)

	for _, addr := range addrs {
		if addr == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, "22")
		}

		err := khCallback(addr, probeAddr, probeKey)
		if err == nil {
			t := probeKey.Type()
			if !seen[t] {
				seen[t] = true
				keyTypes = append(keyTypes, t)
			}
			continue
		}

		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			for _, want := range keyErr.Want {
				t := want.Key.Type()
				if !seen[t] {
					seen[t] = true
					keyTypes = append(keyTypes, t)
				}
			}
		}
	}
	return keyTypes
}

// algosForKeyType maps an OpenSSH public key type to corresponding host key algorithms
func algosForKeyType(keyType string) []string {
	switch keyType {
	case ssh.KeyAlgoED25519:
		return []string{ssh.KeyAlgoED25519, ssh.CertAlgoED25519v01}
	case ssh.KeyAlgoECDSA256:
		return []string{ssh.KeyAlgoECDSA256, ssh.CertAlgoECDSA256v01}
	case ssh.KeyAlgoECDSA384:
		return []string{ssh.KeyAlgoECDSA384, ssh.CertAlgoECDSA384v01}
	case ssh.KeyAlgoECDSA521:
		return []string{ssh.KeyAlgoECDSA521, ssh.CertAlgoECDSA521v01}
	case ssh.KeyAlgoRSA:
		return []string{
			ssh.KeyAlgoRSASHA512,
			ssh.KeyAlgoRSASHA256,
			ssh.KeyAlgoRSA,
			ssh.CertAlgoRSASHA512v01,
			ssh.CertAlgoRSASHA256v01,
			ssh.CertAlgoRSAv01,
		}
	case "sk-ssh-ed25519@openssh.com":
		return []string{"sk-ssh-ed25519@openssh.com", "sk-ssh-ed25519-cert-v01@openssh.com"}
	case "sk-ecdsa-sha2-nistp256@openssh.com":
		return []string{"sk-ecdsa-sha2-nistp256@openssh.com", "sk-ecdsa-sha2-nistp256-cert-v01@openssh.com"}
	default:
		return []string{keyType}
	}
}

// orderHostKeyAlgorithms builds the ordered list of host key algorithms, prioritizing known host keys
func orderHostKeyAlgorithms(knownKeyTypes []string, configAlgos string) []string {
	if configAlgos != "" {
		fields := strings.Split(configAlgos, ",")
		var clean []string
		for _, f := range fields {
			f = strings.TrimSpace(f)
			if f != "" {
				clean = append(clean, f)
			}
		}
		if len(clean) > 0 {
			first := clean[0]
			if strings.HasPrefix(first, "+") {
				clean[0] = strings.TrimPrefix(first, "+")
				return mergeAlgorithms(defaultHostKeyAlgorithms, clean)
			} else if strings.HasPrefix(first, "^") {
				clean[0] = strings.TrimPrefix(first, "^")
				return mergeAlgorithms(clean, defaultHostKeyAlgorithms)
			} else if strings.HasPrefix(first, "-") {
				clean[0] = strings.TrimPrefix(first, "-")
				toRemove := make(map[string]bool)
				for _, r := range clean {
					toRemove[r] = true
				}
				var filtered []string
				for _, d := range defaultHostKeyAlgorithms {
					if !toRemove[d] {
						filtered = append(filtered, d)
					}
				}
				return filtered
			}
			return clean
		}
	}

	var prioritized []string
	for _, kt := range knownKeyTypes {
		prioritized = append(prioritized, algosForKeyType(kt)...)
	}

	return mergeAlgorithms(prioritized, defaultHostKeyAlgorithms)
}

func mergeAlgorithms(lists ...[]string) []string {
	var result []string
	seen := make(map[string]bool)
	for _, list := range lists {
		for _, algo := range list {
			if !seen[algo] && algo != "" {
				seen[algo] = true
				result = append(result, algo)
			}
		}
	}
	return result
}
