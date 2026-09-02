package engine

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"easy42/internal/compiler"
	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/internal/ssh"
)

var (
	ErrNodeNotFound     = errors.New("node not found")
	ErrNodeAlreadyExist = errors.New("node with this name or IP already exists")
	ErrLinkAlreadyExist = errors.New("link between these nodes already exists")
	ErrLinkNotFound     = errors.New("link not found")
)

// Manager orchestrates all operations across config, crypto, ssh, and sync
type Manager struct {
	mu          sync.RWMutex
	store       *config.Store
	vault       *crypto.KeyVault
	pool        *ssh.ClientPool
	statuses    map[string]*config.NodeStatus
	lastSync    time.Time
	lastResults []config.SyncResult
}

// NewManager creates a new Manager instance
func NewManager(store *config.Store) *Manager {
	return &Manager{
		store:       store,
		vault:       crypto.NewKeyVault(),
		pool:        ssh.NewClientPool(),
		statuses:    make(map[string]*config.NodeStatus),
		lastResults: make([]config.SyncResult, 0),
	}
}

// Store returns the underlying store
func (m *Manager) Store() *config.Store {
	return m.store
}

// Vault returns the KeyVault
func (m *Manager) Vault() *crypto.KeyVault {
	return m.vault
}

// Unlock unlocks the master key vault using the user password
func (m *Manager) Unlock(password string) error {
	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return err
		}
	}

	valid, err := crypto.VerifyPassword(password, cfg.PasswordHash)
	if err != nil || !valid {
		return errors.New("invalid password")
	}

	dek, err := crypto.DecryptDEK(password, cfg.EncryptedDEK)
	if err != nil {
		return fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	return m.vault.Unlock(dek)
}

// Lock locks the vault
func (m *Manager) Lock() {
	m.vault.Lock()
}

// ChangePassword verifies oldPassword, re-encrypts the DEK with newPassword, updates config.json, and unlocks the vault
func (m *Manager) ChangePassword(oldPassword, newPassword string) error {
	newPassword = strings.TrimSpace(newPassword)
	if len(newPassword) < 6 {
		return errors.New("new password must be at least 6 characters")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return err
		}
	}

	valid, err := crypto.VerifyPassword(oldPassword, cfg.PasswordHash)
	if err != nil || !valid {
		return errors.New("current password is incorrect")
	}

	dek, err := crypto.DecryptDEK(oldPassword, cfg.EncryptedDEK)
	if err != nil {
		return fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	newHash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	newEncryptedDEK, err := crypto.EncryptDEK(newPassword, dek)
	if err != nil {
		return fmt.Errorf("failed to re-encrypt DEK: %w", err)
	}

	cfgCopy := *cfg
	cfgCopy.PasswordHash = newHash
	cfgCopy.EncryptedDEK = newEncryptedDEK

	if err := m.store.Save(&cfgCopy); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Update vault with DEK
	_ = m.vault.Unlock(dek)

	return nil
}

// ResetSessionSecret regenerates session_secret in config.json and locks the vault, terminating all sessions
func (m *Manager) ResetSessionSecret() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return err
		}
	}

	newSecret, err := crypto.GenerateSessionSecret()
	if err != nil {
		return fmt.Errorf("failed to generate session secret: %w", err)
	}

	cfgCopy := *cfg
	cfgCopy.SessionSecret = newSecret

	if err := m.store.Save(&cfgCopy); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	m.vault.Lock()
	return nil
}

// IsUnlocked checks if the vault is unlocked
func (m *Manager) IsUnlocked() bool {
	return m.vault.IsUnlocked()
}

// GetNodes returns all configured nodes
func (m *Manager) GetNodes() []config.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil
	}
	res := make([]config.Node, len(cfg.Nodes))
	copy(res, cfg.Nodes)
	return res
}

// FindNode finds a node by name
func (m *Manager) FindNode(name string) *config.Node {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil
	}
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == name {
			cp := cfg.Nodes[i]
			return &cp
		}
	}
	return nil
}

// AddNode adds a new node to the topology
func (m *Manager) AddNode(node config.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	node.Name = strings.TrimSpace(node.Name)
	if len(node.Name) == 0 || len(node.Name) > 11 {
		return errors.New("node name must be between 1 and 11 characters")
	}
	if node.Host == "" || node.IP == "" {
		return errors.New("host and IP are required")
	}

	cfg := m.store.Get()
	for _, existing := range cfg.Nodes {
		if strings.EqualFold(existing.Name, node.Name) {
			return fmt.Errorf("node with name %s already exists", node.Name)
		}
		if existing.IP == node.IP {
			return fmt.Errorf("node with IP %s already exists (%s)", node.IP, existing.Name)
		}
	}

	// Ensure "none" entrypoint exists at the end
	hasNone := false
	for _, ep := range node.Entrypoints {
		if ep.IsNone() {
			hasNone = true
			break
		}
	}
	if !hasNone {
		node.Entrypoints = append(node.Entrypoints, config.Entrypoint{
			IP:   "",
			Tags: []string{"nat"},
		})
	}

	cfg.Nodes = append(cfg.Nodes, node)
	return m.store.Save(cfg)
}

// UpdateNode updates an existing node
func (m *Manager) UpdateNode(name string, updated config.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	idx := -1
	for i, n := range cfg.Nodes {
		if n.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrNodeNotFound
	}

	// Check name uniqueness if changed
	if updated.Name != name {
		for _, n := range cfg.Nodes {
			if n.Name == updated.Name {
				return ErrNodeAlreadyExist
			}
		}
	}

	// Ensure "none" endpoint exists
	hasNone := false
	for _, ep := range updated.Entrypoints {
		if ep.IsNone() {
			hasNone = true
			break
		}
	}
	if !hasNone {
		updated.Entrypoints = append(updated.Entrypoints, config.Entrypoint{
			IP:   "",
			Tags: []string{"nat"},
		})
	}

	// Update links connected to this node if name or IP changed
	oldNode := cfg.Nodes[idx]
	if updated.Name != name {
		for i := range cfg.Links {
			if cfg.Links[i].From.Name == name {
				cfg.Links[i].From.Name = updated.Name
				cfg.Links[i].To.Interface = compiler.GetInterfaceName(updated.Name)
			}
			if cfg.Links[i].To.Name == name {
				cfg.Links[i].To.Name = updated.Name
				cfg.Links[i].From.Interface = compiler.GetInterfaceName(updated.Name)
			}
		}
	}

	if updated.IP != oldNode.IP {
		if newAddr, err := compiler.DeriveIPv6LinkLocal(updated.IP); err == nil {
			for i := range cfg.Links {
				if cfg.Links[i].From.Name == updated.Name {
					cfg.Links[i].From.Address = newAddr
				}
				if cfg.Links[i].To.Name == updated.Name {
					cfg.Links[i].To.Address = newAddr
				}
			}
		}
	}

	// Preserve coordinates if not specified in updated node
	if updated.X == nil && oldNode.X != nil {
		updated.X = oldNode.X
	}
	if updated.Y == nil && oldNode.Y != nil {
		updated.Y = oldNode.Y
	}

	cfg.Nodes[idx] = updated

	// Re-resolve endpoints for connected links
	for i := range cfg.Links {
		if cfg.Links[i].From.Name == updated.Name || cfg.Links[i].To.Name == updated.Name {
			var fromN, toN *config.Node
			if cfg.Links[i].From.Name == updated.Name {
				fromN = &cfg.Nodes[idx]
				for j := range cfg.Nodes {
					if cfg.Nodes[j].Name == cfg.Links[i].To.Name {
						toN = &cfg.Nodes[j]
						break
					}
				}
			} else {
				toN = &cfg.Nodes[idx]
				for j := range cfg.Nodes {
					if cfg.Nodes[j].Name == cfg.Links[i].From.Name {
						fromN = &cfg.Nodes[j]
						break
					}
				}
			}
			if fromN != nil && toN != nil {
				fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromN, toN, nil, cfg.Links[i].To.ListenPort)
				toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toN, fromN, nil, cfg.Links[i].From.ListenPort)
				cfg.Links[i].From.Endpoint = fromEP
				cfg.Links[i].To.Endpoint = toEP
				if toEP != "" {
					cfg.Links[i].From.PersistentKeepalive = 25
				} else {
					cfg.Links[i].From.PersistentKeepalive = 0
				}
				if fromEP != "" {
					cfg.Links[i].To.PersistentKeepalive = 25
				} else {
					cfg.Links[i].To.PersistentKeepalive = 0
				}
			}
		}
	}

	return m.store.Save(cfg)
}

// UpdateNodePosition updates the graph coordinates (x, y) of a node
func (m *Manager) UpdateNodePosition(name string, x float64, y float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return ErrNodeNotFound
	}
	idx := -1
	for i, n := range cfg.Nodes {
		if n.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrNodeNotFound
	}

	cfg.Nodes[idx].X = &x
	cfg.Nodes[idx].Y = &y

	return m.store.Save(cfg)
}

// DeleteNode removes a node and all its connected links
func (m *Manager) DeleteNode(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	newNodes := make([]config.Node, 0)
	for _, n := range cfg.Nodes {
		if n.Name != name {
			newNodes = append(newNodes, n)
		}
	}
	cfg.Nodes = newNodes

	// Remove links connected to this node
	newLinks := make([]config.Link, 0)
	for _, l := range cfg.Links {
		if l.From.Name != name && l.To.Name != name {
			newLinks = append(newLinks, l)
		}
	}
	cfg.Links = newLinks

	return m.store.Save(cfg)
}

// GetLinks returns all links
func (m *Manager) GetLinks() []config.Link {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil
	}
	res := make([]config.Link, len(cfg.Links))
	copy(res, cfg.Links)
	return res
}

// AddLink creates a new WireGuard link between two nodes with keypairs and parameters.
// Optional customMTU can specify [fromMTU, toMTU] (relative to node1Name, node2Name).
func (m *Manager) AddLink(node1Name, node2Name string, listenPort1, listenPort2 int, tags []string, customMTU ...int) (*config.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return nil, crypto.ErrVaultLocked
	}

	cfg := m.store.Get()
	var n1, n2 *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == node1Name {
			n1 = &cfg.Nodes[i]
		}
		if cfg.Nodes[i].Name == node2Name {
			n2 = &cfg.Nodes[i]
		}
	}
	if n1 == nil || n2 == nil {
		return nil, errors.New("one or both nodes not found")
	}

	customMTU1 := 0
	customMTU2 := 0
	if len(customMTU) >= 1 && customMTU[0] > 0 {
		customMTU1 = customMTU[0]
	}
	if len(customMTU) >= 2 && customMTU[1] > 0 {
		customMTU2 = customMTU[1]
	}

	// Lexicographical ordering: from.Name < to.Name
	fromNode, toNode := n1, n2
	fromPort, toPort := listenPort1, listenPort2
	fromCustomMTU, toCustomMTU := customMTU1, customMTU2
	if strings.Compare(fromNode.Name, toNode.Name) > 0 {
		fromNode, toNode = toNode, fromNode
		fromPort, toPort = toPort, fromPort
		fromCustomMTU, toCustomMTU = toCustomMTU, fromCustomMTU
	}

	// Check duplicate link
	for _, l := range cfg.Links {
		if l.From.Name == fromNode.Name && l.To.Name == toNode.Name {
			return nil, ErrLinkAlreadyExist
		}
	}

	// Generate WireGuard keypairs for both ends
	kpFrom, err := crypto.GenerateWgKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate keypair for %s: %w", fromNode.Name, err)
	}
	kpTo, err := crypto.GenerateWgKeyPair()
	if err != nil {
		return nil, fmt.Errorf("failed to generate keypair for %s: %w", toNode.Name, err)
	}

	encPrivFrom, err := m.vault.EncryptField(kpFrom.PrivateKey)
	if err != nil {
		return nil, err
	}
	encPrivTo, err := m.vault.EncryptField(kpTo.PrivateKey)
	if err != nil {
		return nil, err
	}

	fromAddr, err := compiler.DeriveIPv6LinkLocal(fromNode.IP)
	if err != nil {
		return nil, err
	}
	toAddr, err := compiler.DeriveIPv6LinkLocal(toNode.IP)
	if err != nil {
		return nil, err
	}

	if fromPort == 0 {
		fromPort = compiler.DerivePortFromASN(toNode.ASN)
	}
	if toPort == 0 {
		toPort = compiler.DerivePortFromASN(fromNode.ASN)
	}

	fromEP, _, epTo := compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, toPort)
	toEP, _, epFrom := compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, fromPort)

	fromKeepalive := 0
	if toEP != "" {
		fromKeepalive = 25
	}
	toKeepalive := 0
	if fromEP != "" {
		toKeepalive = 25
	}

	// Determine LinkEnd MTU: used entrypoint mtu minus 80 (wg overhead)
	resolveUsedMTU := func(primaryEP, fallbackEP *config.Entrypoint, selfNode, peerNode *config.Node) int {
		var ep *config.Entrypoint
		if primaryEP != nil && !primaryEP.IsNone() {
			ep = primaryEP
		} else if fallbackEP != nil && !fallbackEP.IsNone() {
			ep = fallbackEP
		} else {
			if selfNode != nil {
				for i := range selfNode.Entrypoints {
					if !selfNode.Entrypoints[i].IsNone() {
						ep = &selfNode.Entrypoints[i]
						break
					}
				}
			}
			if ep == nil && peerNode != nil {
				for i := range peerNode.Entrypoints {
					if !peerNode.Entrypoints[i].IsNone() {
						ep = &peerNode.Entrypoints[i]
						break
					}
				}
			}
		}

		baseMTU := 1500
		if ep != nil && ep.MTU > 0 {
			baseMTU = ep.MTU
		}
		return baseMTU - 80
	}

	fromMTU := resolveUsedMTU(epTo, epFrom, fromNode, toNode)
	toMTU := resolveUsedMTU(epFrom, epTo, toNode, fromNode)
	if fromCustomMTU > 0 {
		fromMTU = fromCustomMTU
	}
	if toCustomMTU > 0 {
		toMTU = toCustomMTU
	}

	link := config.Link{
		From: config.LinkEnd{
			Name:                fromNode.Name,
			Interface:           compiler.GetInterfaceName(toNode.Name),
			Address:             fromAddr,
			ListenPort:          fromPort,
			Endpoint:            fromEP,
			PrivateKey:          encPrivFrom,
			PublicKey:           kpFrom.PublicKey,
			PersistentKeepalive: fromKeepalive,
			MTU:                 fromMTU,
		},
		To: config.LinkEnd{
			Name:                toNode.Name,
			Interface:           compiler.GetInterfaceName(fromNode.Name),
			Address:             toAddr,
			ListenPort:          toPort,
			Endpoint:            toEP,
			PrivateKey:          encPrivTo,
			PublicKey:           kpTo.PublicKey,
			PersistentKeepalive: toKeepalive,
			MTU:                 toMTU,
		},
		Tags: tags,
	}

	cfg.Links = append(cfg.Links, link)
	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}

	return &link, nil
}

// UpdateLink updates parameters (listen ports, custom MTUs, tags) of an existing WireGuard link
func (m *Manager) UpdateLink(node1Name, node2Name string, listenPort1, listenPort2 int, tags []string, customMTU ...int) (*config.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	var n1, n2 *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == node1Name {
			n1 = &cfg.Nodes[i]
		}
		if cfg.Nodes[i].Name == node2Name {
			n2 = &cfg.Nodes[i]
		}
	}
	if n1 == nil || n2 == nil {
		return nil, errors.New("one or both nodes not found")
	}

	customMTU1 := 0
	customMTU2 := 0
	if len(customMTU) >= 1 && customMTU[0] > 0 {
		customMTU1 = customMTU[0]
	}
	if len(customMTU) >= 2 && customMTU[1] > 0 {
		customMTU2 = customMTU[1]
	}

	// Lexicographical ordering: from.Name < to.Name
	fromNode, toNode := n1, n2
	fromPort, toPort := listenPort1, listenPort2
	fromCustomMTU, toCustomMTU := customMTU1, customMTU2
	if strings.Compare(fromNode.Name, toNode.Name) > 0 {
		fromNode, toNode = toNode, fromNode
		fromPort, toPort = toPort, fromPort
		fromCustomMTU, toCustomMTU = toCustomMTU, fromCustomMTU
	}

	linkIdx := -1
	for i, l := range cfg.Links {
		if l.From.Name == fromNode.Name && l.To.Name == toNode.Name {
			linkIdx = i
			break
		}
	}
	if linkIdx == -1 {
		return nil, ErrLinkNotFound
	}

	link := &cfg.Links[linkIdx]

	if fromPort > 0 {
		link.From.ListenPort = fromPort
	}
	if toPort > 0 {
		link.To.ListenPort = toPort
	}
	if fromCustomMTU > 0 {
		link.From.MTU = fromCustomMTU
	}
	if toCustomMTU > 0 {
		link.To.MTU = toCustomMTU
	}
	if tags != nil {
		link.Tags = tags
	}

	// Re-resolve endpoints and keepalives with updated ports
	fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, link.To.ListenPort)
	toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, link.From.ListenPort)
	link.From.Endpoint = fromEP
	link.To.Endpoint = toEP
	if toEP != "" {
		link.From.PersistentKeepalive = 25
	} else {
		link.From.PersistentKeepalive = 0
	}
	if fromEP != "" {
		link.To.PersistentKeepalive = 25
	} else {
		link.To.PersistentKeepalive = 0
	}

	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}

	return link, nil
}

// DeleteLink removes a link between two nodes
func (m *Manager) DeleteLink(node1Name, node2Name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	from, to := node1Name, node2Name
	if strings.Compare(from, to) > 0 {
		from, to = to, from
	}

	cfg := m.store.Get()
	newLinks := make([]config.Link, 0)
	found := false
	for _, l := range cfg.Links {
		if l.From.Name == from && l.To.Name == to {
			found = true
			continue
		}
		newLinks = append(newLinks, l)
	}

	if !found {
		return ErrLinkNotFound
	}

	cfg.Links = newLinks
	return m.store.Save(cfg)
}

// ProbeHost probes a remote node via SSH
func (m *Manager) ProbeHost(host string) (*ssh.ProbeResult, error) {
	sshClient, _, err := m.pool.GetClient(host)
	if err != nil {
		return nil, err
	}

	nodes := m.GetNodes()
	return ssh.ProbeHost(sshClient, host, nodes)
}

// RefreshNodeStatus refreshes a node's live status via SSH
func (m *Manager) RefreshNodeStatus(nodeName string) (*config.NodeStatus, error) {
	node := m.FindNode(nodeName)
	if node == nil {
		return nil, ErrNodeNotFound
	}

	status := &config.NodeStatus{
		Name:      node.Name,
		Host:      node.Host,
		LastSeen:  time.Now(),
		Connected: false,
	}

	sshClient, _, err := m.pool.GetClient(node.Host)
	if err != nil {
		status.Error = err.Error()
		m.mu.Lock()
		m.statuses[nodeName] = status
		m.mu.Unlock()
		return status, nil
	}

	status.Connected = true
	hostname, _ := ssh.RunCommand(sshClient, "hostname")
	status.Hostname = strings.TrimSpace(hostname)

	wgStatus, err := ssh.QueryWireGuardStatus(sshClient)
	if err == nil {
		status.WgInterfaces = wgStatus
	}

	m.mu.Lock()
	m.statuses[nodeName] = status
	m.mu.Unlock()

	return status, nil
}

// GetNodeStatuses returns cached node statuses
func (m *Manager) GetNodeStatuses() map[string]config.NodeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make(map[string]config.NodeStatus)
	for k, v := range m.statuses {
		res[k] = *v
	}
	return res
}

// PlanSync computes the sync actions required to bring remote devices up to date
func (m *Manager) PlanSync() ([]config.SyncAction, error) {
	m.mu.RLock()
	if !m.vault.IsUnlocked() {
		m.mu.RUnlock()
		return nil, crypto.ErrVaultLocked
	}

	cfg := m.store.Get()
	nodeMap := make(map[string]*config.Node)
	for i := range cfg.Nodes {
		nodeMap[cfg.Nodes[i].Name] = &cfg.Nodes[i]
	}
	nodes := make([]config.Node, len(cfg.Nodes))
	copy(nodes, cfg.Nodes)
	links := make([]config.Link, len(cfg.Links))
	copy(links, cfg.Links)
	m.mu.RUnlock()

	actions := make([]config.SyncAction, 0)

	// 1. Detect unused/deleted wg42* interfaces on remote devices by running `wg`
	expectedIfacesPerNode := make(map[string]map[string]bool)
	for _, n := range nodes {
		expectedIfacesPerNode[n.Name] = make(map[string]bool)
	}
	for _, link := range links {
		if em, ok := expectedIfacesPerNode[link.From.Name]; ok {
			em[link.From.Interface] = true
		}
		if em, ok := expectedIfacesPerNode[link.To.Name]; ok {
			em[link.To.Interface] = true
		}
	}

	var wg sync.WaitGroup
	var cleanActionsMu sync.Mutex
	var cleanActions []config.SyncAction

	for _, n := range nodes {
		node := n
		wg.Add(1)
		go func() {
			defer wg.Done()
			sshClient, _, err := m.pool.GetClient(node.Host)
			if err != nil {
				return
			}
			runningIfaces, err := ssh.GetRunningWgInterfaces(sshClient)
			if err != nil {
				return
			}
			expected := expectedIfacesPerNode[node.Name]
			for _, iface := range runningIfaces {
				// We only manage wg42* prefix wireguard interfaces
				if strings.HasPrefix(iface, "wg42") {
					if !expected[iface] {
						targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", iface)
						cleanActionsMu.Lock()
						cleanActions = append(cleanActions, config.SyncAction{
							NodeName:    node.Name,
							Host:        node.Host,
							Type:        config.ActionDeleteConfig,
							Interface:   iface,
							TargetFile:  targetFile,
							Command:     fmt.Sprintf("wg-quick down %s && rm -f %s", iface, targetFile),
							Description: fmt.Sprintf("Remove deleted interface %s on %s", iface, node.Name),
							FileContent: fmt.Sprintf("# Deleted interface %s is no longer in graph.\n# Action: wg-quick down %s && rm -f %s\n", iface, iface, targetFile),
						})
						cleanActionsMu.Unlock()
					}
				}
			}
		}()
	}
	wg.Wait()

	// Sort clean actions for determinism
	sort.Slice(cleanActions, func(i, j int) bool {
		if cleanActions[i].NodeName != cleanActions[j].NodeName {
			return cleanActions[i].NodeName < cleanActions[j].NodeName
		}
		return cleanActions[i].Interface < cleanActions[j].Interface
	})
	actions = append(actions, cleanActions...)

	// 2. Active links configuration
	for _, link := range links {
		fromNode := nodeMap[link.From.Name]
		toNode := nodeMap[link.To.Name]
		if fromNode == nil || toNode == nil {
			continue
		}

		// 1. From node end
		fromConf, err := compiler.GenerateWgConfigContent(fromNode, toNode, &link.From, &link.To, m.vault)
		if err == nil {
			targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", link.From.Interface)
			actions = append(actions, config.SyncAction{
				NodeName:    fromNode.Name,
				Host:        fromNode.Host,
				Type:        config.ActionSyncConfig,
				Interface:   link.From.Interface,
				TargetFile:  targetFile,
				FileContent: fromConf,
				Description: fmt.Sprintf("Configure %s on %s (peer %s)", link.From.Interface, fromNode.Name, toNode.Name),
			})
		}

		// 2. To node end
		toConf, err := compiler.GenerateWgConfigContent(toNode, fromNode, &link.To, &link.From, m.vault)
		if err == nil {
			targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", link.To.Interface)
			actions = append(actions, config.SyncAction{
				NodeName:    toNode.Name,
				Host:        toNode.Host,
				Type:        config.ActionSyncConfig,
				Interface:   link.To.Interface,
				TargetFile:  targetFile,
				FileContent: toConf,
				Description: fmt.Sprintf("Configure %s on %s (peer %s)", link.To.Interface, toNode.Name, fromNode.Name),
			})
		}
	}

	if actions == nil {
		actions = []config.SyncAction{}
	}
	return actions, nil
}

// ExecuteSync executes planned actions across nodes
func (m *Manager) ExecuteSync() ([]config.SyncResult, error) {
	actions, err := m.PlanSync()
	if err != nil {
		return nil, err
	}

	results := make([]config.SyncResult, 0)
	for _, act := range actions {
		start := time.Now()
		res := config.SyncResult{
			NodeName: act.NodeName,
			Action:   act.Description,
		}

		sshClient, sftpClient, err := m.pool.GetClient(act.Host)
		if err != nil {
			res.Success = false
			res.Error = fmt.Sprintf("SSH connect failed: %v", err)
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)
			continue
		}

		// Handle deleted interface cleanup
		if act.Type == config.ActionDeleteConfig || act.Type == config.ActionDownInterface {
			if err := ssh.CleanWireGuardInterface(sshClient, sftpClient, act.Interface); err != nil {
				res.Success = false
				res.Error = fmt.Sprintf("Failed to clean interface %s: %v", act.Interface, err)
			} else {
				res.Success = true
			}
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)
			continue
		}

		// Check current remote file
		currentContent, _ := ssh.ReadRemoteFile(sftpClient, act.TargetFile)
		needsUpdate := compiler.NeedsUpdate(currentContent, act.FileContent)
		if needsUpdate {
			if err := ssh.AtomicWriteFile(sftpClient, act.TargetFile, []byte(act.FileContent), 0600); err != nil {
				res.Success = false
				res.Error = fmt.Sprintf("Failed to write config: %v", err)
				res.Duration = float64(time.Since(start).Milliseconds())
				results = append(results, res)
				continue
			}
		}

		// Start device wg42* interface if not in started state, or sync dynamically if already running
		if !ssh.IsInterfaceStarted(sshClient, act.Interface) {
			if err := ssh.StartWireGuardInterface(sshClient, act.Interface); err != nil {
				res.Success = false
				res.Error = fmt.Sprintf("Failed to start WireGuard interface %s: %v", act.Interface, err)
				res.Duration = float64(time.Since(start).Milliseconds())
				results = append(results, res)
				continue
			}
		} else if needsUpdate {
			// Interface is already started; sync/reload WireGuard dynamically
			if err := ssh.SyncWireGuard(sshClient, act.Interface, act.TargetFile); err != nil {
				res.Success = false
				res.Error = fmt.Sprintf("Failed to reload WireGuard interface %s: %v", act.Interface, err)
				res.Duration = float64(time.Since(start).Milliseconds())
				results = append(results, res)
				continue
			}
		}

		res.Success = true
		res.Duration = float64(time.Since(start).Milliseconds())
		results = append(results, res)
	}

	if results == nil {
		results = []config.SyncResult{}
	}

	m.mu.Lock()
	m.lastSync = time.Now()
	m.lastResults = results
	m.mu.Unlock()

	return results, nil
}

// GetLastSyncResults returns the latest sync execution results
func (m *Manager) GetLastSyncResults() (time.Time, []config.SyncResult) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := m.lastResults
	if res == nil {
		res = []config.SyncResult{}
	}
	return m.lastSync, res
}
