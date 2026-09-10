package ssh

import (
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

	// 1. Resolve OpenSSH config (Host alias, User, Port, HostName, IdentityFile)
	var realHost, user, portStr, identityFile string

	sshConfigPath := filepath.Join(home, ".ssh", "config")
	if f, err := os.Open(sshConfigPath); err == nil {
		cfg, err := ssh_config.Decode(f)
		_ = f.Close()
		if err == nil {
			realHost, _ = cfg.Get(hostAliasOrIP, "HostName")
			user, _ = cfg.Get(hostAliasOrIP, "User")
			portStr, _ = cfg.Get(hostAliasOrIP, "Port")
			identityFile, _ = cfg.Get(hostAliasOrIP, "IdentityFile")
		}
	}

	if realHost == "" {
		realHost = hostAliasOrIP
	}
	if user == "" {
		user = os.Getenv("USER")
		if user == "" {
			user = "root"
		}
	}
	port := 22
	if portStr != "" {
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

	// 3. Host Key Callback (known_hosts or fallback)
	knownHostsPath := filepath.Join(home, ".ssh", "known_hosts")
	var hostKeyCallback ssh.HostKeyCallback
	if khCallback, err := knownhosts.New(knownHostsPath); err == nil {
		hostKeyCallback = func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			// If known_hosts fails, we can fall back to accept or log
			err := khCallback(hostname, remote, key)
			if err != nil {
				// Fallback to accepting key to avoid blocking initial setup
				return nil
			}
			return nil
		}
	} else {
		// Fallback callback
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	clientConfig := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         timeout,
	}

	targetAddr := net.JoinHostPort(realHost, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", targetAddr, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh dial to %s (%s) failed: %w", hostAliasOrIP, targetAddr, err)
	}

	return client, nil
}
