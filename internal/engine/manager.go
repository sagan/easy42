package engine

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"easy42/internal/compiler"
	"easy42/internal/config"
	"easy42/internal/crypto"
	"easy42/internal/roa"
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
	stateStore  *config.StateStore
	vault       *crypto.KeyVault
	pool        *ssh.ClientPool
	roaManager  *roa.Manager
	statuses    map[string]*config.NodeStatus
	lastSync    time.Time
	lastResults []config.SyncResult
}

// NewManager creates a new Manager instance
func NewManager(store *config.Store) *Manager {
	stateStore := config.NewStateStore(store.DataDir())
	_, _ = stateStore.Load()
	roaManager := roa.NewManager(filepath.Join(store.DataDir(), "cache", "roa"))
	return &Manager{
		store:       store,
		stateStore:  stateStore,
		vault:       crypto.NewKeyVault(),
		pool:        ssh.NewClientPool(),
		roaManager:  roaManager,
		statuses:    make(map[string]*config.NodeStatus),
		lastResults: make([]config.SyncResult, 0),
	}
}

// ROAManager returns the underlying ROA cache manager
func (m *Manager) ROAManager() *roa.Manager {
	return m.roaManager
}

// Store returns the underlying store
func (m *Manager) Store() *config.Store {
	return m.store
}

// StateStore returns the underlying state store
func (m *Manager) StateStore() *config.StateStore {
	return m.stateStore
}

// GetNetworkState returns the current recorded state
func (m *Manager) GetNetworkState() *config.NetworkState {
	return m.stateStore.Get()
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

// GetNode finds a node by name (alias to FindNode)
func (m *Manager) GetNode(name string) *config.Node {
	return m.FindNode(name)
}

// AddNode adds a new node to the topology
func (m *Manager) AddNode(node config.Node) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	node.Name = strings.TrimSpace(node.Name)
	if node.IsExternal {
		if len(node.Name) == 0 || len(node.Name) > 10 {
			return errors.New("external peer name must be between 1 and 10 characters")
		}
	} else {
		if len(node.Name) == 0 || len(node.Name) > 11 {
			return errors.New("node name must be between 1 and 11 characters")
		}
	}
	if !node.IsExternal && (node.Host == "" || node.IP == "") {
		return errors.New("host and IP are required for managed nodes")
	}

	node.IP = strings.TrimSpace(node.IP)
	node.IP6 = strings.TrimSpace(node.IP6)
	if idx := strings.Index(node.IP6, "/"); idx != -1 {
		node.IP6 = strings.TrimSpace(node.IP6[:idx])
	}

	cfg := m.store.Get()
	for _, existing := range cfg.Nodes {
		if strings.EqualFold(existing.Name, node.Name) {
			return fmt.Errorf("node with name %s already exists", node.Name)
		}
		if node.IP != "" && !node.IsExternal && !existing.IsExternal && existing.IP == node.IP {
			return fmt.Errorf("node with IP %s already exists (%s)", node.IP, existing.Name)
		}
		if node.IP6 != "" && !node.IsExternal && !existing.IsExternal && existing.IP6 != "" && existing.IP6 == node.IP6 {
			return fmt.Errorf("node with IPv6 %s already exists (%s)", node.IP6, existing.Name)
		}
	}

	if !node.IsExternal {
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
	}

	if node.Table <= 0 {
		node.Table = 254
	}

	node.ModifiedAt = time.Now().UTC()
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

	updated.Name = strings.TrimSpace(updated.Name)
	if updated.IsExternal {
		if len(updated.Name) == 0 || len(updated.Name) > 10 {
			return errors.New("external peer name must be between 1 and 10 characters")
		}
	} else {
		if len(updated.Name) == 0 || len(updated.Name) > 11 {
			return errors.New("node name must be between 1 and 11 characters")
		}
	}

	// Check name uniqueness if changed
	if updated.Name != name {
		for _, n := range cfg.Nodes {
			if n.Name == updated.Name {
				return ErrNodeAlreadyExist
			}
		}
	}

	updated.IP = strings.TrimSpace(updated.IP)
	updated.IP6 = strings.TrimSpace(updated.IP6)
	if idx := strings.Index(updated.IP6, "/"); idx != -1 {
		updated.IP6 = strings.TrimSpace(updated.IP6[:idx])
	}
	if updated.IP6 != "" && !updated.IsExternal {
		for _, existing := range cfg.Nodes {
			if existing.Name != name && !existing.IsExternal && existing.IP6 != "" && existing.IP6 == updated.IP6 {
				return fmt.Errorf("node with IPv6 %s already exists (%s)", updated.IP6, existing.Name)
			}
		}
	}

	if !updated.IsExternal {
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
	}

	if updated.Table <= 0 {
		updated.Table = 254
	}

	now := time.Now().UTC()
	updated.ModifiedAt = now

	// Update links and nodes if name changed
	oldNode := cfg.Nodes[idx]
	if updated.Name != name {
		m.applyNodeRenameLocked(cfg, name, updated.Name, updated.IsExternal, now)
	}

	if updated.IP != oldNode.IP {
		if newAddr, err := compiler.DeriveIPv6LinkLocal(updated.IP); err == nil {
			for i := range cfg.Links {
				if cfg.Links[i].From.Name == updated.Name {
					cfg.Links[i].From.Address = newAddr
					cfg.Links[i].ModifiedAt = now
				}
				if cfg.Links[i].To.Name == updated.Name {
					cfg.Links[i].To.Address = newAddr
					cfg.Links[i].ModifiedAt = now
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
				cfg.Links[i].ModifiedAt = now
			}
		}
	}

	return m.store.Save(cfg)
}

// applyNodeRenameLocked updates all references to a node name across config, links, interface names, and state.
// Caller must hold m.mu Lock.
func (m *Manager) applyNodeRenameLocked(cfg *config.Config, oldName, newName string, isExternal bool, now time.Time) {
	oldIfaceInternal := compiler.GetInterfaceName(oldName, false)
	oldIfaceExternal := compiler.GetInterfaceName(oldName, true)
	newIface := compiler.GetInterfaceName(newName, isExternal)
	newIfaceInternal := compiler.GetInterfaceName(newName, false)
	newIfaceExternal := compiler.GetInterfaceName(newName, true)

	replaceIface := func(iface string) string {
		if iface == oldIfaceInternal || iface == "wg42"+oldName {
			return newIfaceInternal
		}
		if iface == oldIfaceExternal || iface == "wg42-"+oldName {
			return newIfaceExternal
		}
		if strings.HasPrefix(iface, "wg42-") && strings.TrimPrefix(iface, "wg42-") == oldName {
			return newIfaceExternal
		}
		if strings.HasPrefix(iface, "wg42") && strings.TrimPrefix(iface, "wg42") == oldName {
			return newIfaceInternal
		}
		if strings.Contains(iface, oldName) {
			return strings.ReplaceAll(iface, oldName, newName)
		}
		return iface
	}

	// Update node name and interface in Nodes list
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == oldName {
			cfg.Nodes[i].Name = newName
			cfg.Nodes[i].ModifiedAt = now
		}
		if cfg.Nodes[i].Interface != "" {
			replaced := replaceIface(cfg.Nodes[i].Interface)
			if replaced != cfg.Nodes[i].Interface {
				cfg.Nodes[i].Interface = replaced
				cfg.Nodes[i].ModifiedAt = now
			}
		}
		for tIdx := range cfg.Nodes[i].Tags {
			if cfg.Nodes[i].Tags[tIdx] == oldName {
				cfg.Nodes[i].Tags[tIdx] = newName
				cfg.Nodes[i].ModifiedAt = now
			}
		}
	}

	// Update all links: From.Name, To.Name, From.Interface, To.Interface, and Tags
	for i := range cfg.Links {
		// Update endpoint names
		if cfg.Links[i].From.Name == oldName {
			cfg.Links[i].From.Name = newName
			cfg.Links[i].ModifiedAt = now
		}
		if cfg.Links[i].To.Name == oldName {
			cfg.Links[i].To.Name = newName
			cfg.Links[i].ModifiedAt = now
		}

		// Update From.Interface: if To is the renamed node, use new peer interface name
		if cfg.Links[i].To.Name == newName {
			cfg.Links[i].From.Interface = newIface
			cfg.Links[i].ModifiedAt = now
		} else {
			newFromIface := replaceIface(cfg.Links[i].From.Interface)
			if newFromIface != cfg.Links[i].From.Interface {
				cfg.Links[i].From.Interface = newFromIface
				cfg.Links[i].ModifiedAt = now
			}
		}

		// Update To.Interface: if From is the renamed node, use new peer interface name
		if cfg.Links[i].From.Name == newName {
			cfg.Links[i].To.Interface = newIface
			cfg.Links[i].ModifiedAt = now
		} else {
			newToIface := replaceIface(cfg.Links[i].To.Interface)
			if newToIface != cfg.Links[i].To.Interface {
				cfg.Links[i].To.Interface = newToIface
				cfg.Links[i].ModifiedAt = now
			}
		}

		// Update link tags if matching oldName
		for tIdx := range cfg.Links[i].Tags {
			if cfg.Links[i].Tags[tIdx] == oldName {
				cfg.Links[i].Tags[tIdx] = newName
				cfg.Links[i].ModifiedAt = now
			}
		}
	}

	// Update runtime statuses map
	if st, ok := m.statuses[oldName]; ok {
		st.Name = newName
		m.statuses[newName] = st
		delete(m.statuses, oldName)
	}

	// Update state store if present
	if m.stateStore != nil {
		oldIfaces := []string{
			oldIfaceInternal,
			oldIfaceExternal,
			"wg42" + oldName,
			"wg42-" + oldName,
		}
		_ = m.stateStore.RenameNode(oldName, newName, oldIfaces, newIface)
	}
}

// RenameNode renames an existing node across configuration, links, interfaces, and state
func (m *Manager) RenameNode(oldName, newName string) (*config.Node, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)

	if oldName == "" {
		return nil, errors.New("current node name cannot be empty")
	}
	if newName == "" {
		return nil, errors.New("new node name cannot be empty")
	}

	cfg := m.store.Get()
	if cfg == nil {
		return nil, ErrNodeNotFound
	}

	idx := -1
	for i, n := range cfg.Nodes {
		if n.Name == oldName {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, ErrNodeNotFound
	}

	targetNode := cfg.Nodes[idx]

	if newName == oldName {
		cp := targetNode
		return &cp, nil
	}

	if targetNode.IsExternal {
		if len(newName) == 0 || len(newName) > 10 {
			return nil, errors.New("external peer name must be between 1 and 10 characters")
		}
	} else {
		if len(newName) == 0 || len(newName) > 11 {
			return nil, errors.New("node name must be between 1 and 11 characters")
		}
	}

	// Validate allowed characters (letters, digits, hyphen, underscore)
	for _, ch := range newName {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return nil, errors.New("node name may only contain alphanumeric characters, hyphens, and underscores")
		}
	}

	// Check if new name already exists
	for _, n := range cfg.Nodes {
		if strings.EqualFold(n.Name, newName) {
			return nil, fmt.Errorf("node with name %s already exists", newName)
		}
	}

	now := time.Now().UTC()
	m.applyNodeRenameLocked(cfg, oldName, newName, targetNode.IsExternal, now)

	// Re-resolve endpoints for connected links
	for i := range cfg.Links {
		if cfg.Links[i].From.Name == newName || cfg.Links[i].To.Name == newName {
			var fromN, toN *config.Node
			for j := range cfg.Nodes {
				if cfg.Nodes[j].Name == cfg.Links[i].From.Name {
					fromN = &cfg.Nodes[j]
				}
				if cfg.Nodes[j].Name == cfg.Links[i].To.Name {
					toN = &cfg.Nodes[j]
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
				cfg.Links[i].ModifiedAt = now
			}
		}
	}

	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}

	for _, n := range cfg.Nodes {
		if n.Name == newName {
			cp := n
			return &cp, nil
		}
	}
	return &cfg.Nodes[idx], nil
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
	cfg.Nodes[idx].ModifiedAt = time.Now().UTC()

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

	_ = m.stateStore.RemoveNode(name)

	return m.store.Save(cfg)
}

// GetLinks returns all links with resolved endpoints populated
func (m *Manager) GetLinks() []config.Link {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil
	}
	nodeMap := make(map[string]*config.Node, len(cfg.Nodes))
	for i := range cfg.Nodes {
		nodeMap[cfg.Nodes[i].Name] = &cfg.Nodes[i]
	}

	res := make([]config.Link, len(cfg.Links))
	for i := range cfg.Links {
		res[i] = cfg.Links[i]
		fromNode := nodeMap[res[i].From.Name]
		toNode := nodeMap[res[i].To.Name]
		res[i].From.ResolvedEndpoint = compiler.ResolveLinkEndpoint(fromNode, toNode, &res[i].From, &res[i].To)
		res[i].To.ResolvedEndpoint = compiler.ResolveLinkEndpoint(toNode, fromNode, &res[i].To, &res[i].From)
		if toNode != nil && toNode.IsExternal {
			res[i].To.Endpoint = res[i].To.ResolvedEndpoint
		}
		if fromNode != nil && fromNode.IsExternal {
			res[i].From.Endpoint = res[i].From.ResolvedEndpoint
		}
	}
	return res
}

// AddLink creates a new WireGuard link between two nodes with keypairs and parameters.
// buildLink creates and configures a Link between two nodes with keypairs, addresses, ports, endpoints, and MTU.
// Must be called with m.mu held and vault unlocked.
func (m *Manager) buildLink(cfg *config.Config, n1, n2 *config.Node, listenPort1, listenPort2 int, tags []string, customMTU1, customMTU2 int, linkParams ...*config.Link) (*config.Link, error) {
	// Lexicographical ordering: from.Name < to.Name
	fromNode, toNode := n1, n2
	fromPort, toPort := listenPort1, listenPort2
	fromCustomMTU, toCustomMTU := customMTU1, customMTU2
	if strings.Compare(fromNode.Name, toNode.Name) > 0 {
		fromNode, toNode = toNode, fromNode
		fromPort, toPort = toPort, fromPort
		fromCustomMTU, toCustomMTU = toCustomMTU, fromCustomMTU
	}

	if fromNode.IsExternal && toNode.IsExternal {
		return nil, errors.New("cannot link two external nodes")
	}

	var customFromEnd, customToEnd *config.LinkEnd
	if len(linkParams) > 0 && linkParams[0] != nil {
		lp := linkParams[0]
		if lp.From.Name == fromNode.Name {
			customFromEnd = &lp.From
			customToEnd = &lp.To
		} else if lp.To.Name == fromNode.Name {
			customFromEnd = &lp.To
			customToEnd = &lp.From
		}
	}

	var encPrivFrom, pubKeyFrom string
	var encPrivTo, pubKeyTo string

	if fromNode.IsExternal {
		if customFromEnd != nil && customFromEnd.PublicKey != "" {
			pubKeyFrom = customFromEnd.PublicKey
		}
	} else {
		kpFrom, err := crypto.GenerateWgKeyPair()
		if err != nil {
			return nil, fmt.Errorf("failed to generate keypair for %s: %w", fromNode.Name, err)
		}
		encPriv, err := m.vault.EncryptField(kpFrom.PrivateKey)
		if err != nil {
			return nil, err
		}
		encPrivFrom = encPriv
		pubKeyFrom = kpFrom.PublicKey
	}

	if toNode.IsExternal {
		if customToEnd != nil && customToEnd.PublicKey != "" {
			pubKeyTo = customToEnd.PublicKey
		}
	} else {
		kpTo, err := crypto.GenerateWgKeyPair()
		if err != nil {
			return nil, fmt.Errorf("failed to generate keypair for %s: %w", toNode.Name, err)
		}
		encPriv, err := m.vault.EncryptField(kpTo.PrivateKey)
		if err != nil {
			return nil, err
		}
		encPrivTo = encPriv
		pubKeyTo = kpTo.PublicKey
	}

	fromAddr := ""
	if customFromEnd != nil && customFromEnd.Address != "" {
		fromAddr = customFromEnd.Address
	} else if fromNode.IP != "" {
		fromAddr, _ = compiler.DeriveIPv6LinkLocal(fromNode.IP)
	}
	if fromAddr == "" {
		fromAddr = "fe80::1/64"
	}

	toAddr := ""
	if customToEnd != nil && customToEnd.Address != "" {
		toAddr = customToEnd.Address
	} else if toNode.IP != "" {
		toAddr, _ = compiler.DeriveIPv6LinkLocal(toNode.IP)
	}
	if toAddr == "" {
		toAddr = "fe80::2/64"
	}

	if customFromEnd != nil && customFromEnd.ListenPort > 0 {
		fromPort = customFromEnd.ListenPort
	}
	if customToEnd != nil && customToEnd.ListenPort > 0 {
		toPort = customToEnd.ListenPort
	}
	if fromPort == 0 && !fromNode.IsExternal {
		if toNode.IP != "" {
			fromPort = compiler.DerivePortFromIP(toNode.IP)
		} else {
			fromPort = 51820
		}
	}
	if toPort == 0 && !toNode.IsExternal {
		if fromNode.IP != "" {
			toPort = compiler.DerivePortFromIP(fromNode.IP)
		} else {
			toPort = 51820
		}
	}

	fromEP := ""
	toEP := ""
	var epTo, epFrom *config.Entrypoint
	if customFromEnd != nil && customFromEnd.Endpoint != "" {
		fromEP = customFromEnd.Endpoint
	}
	if customToEnd != nil && customToEnd.Endpoint != "" {
		toEP = customToEnd.Endpoint
	}

	if fromNode.IsExternal {
		// fromNode is external, connecting to toNode (managed internal)
		fromEP, _, epTo = compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, toPort)
		if toEP == "" && customFromEnd != nil && customFromEnd.Endpoint != "" {
			toEP = customFromEnd.Endpoint
		}
	} else if toNode.IsExternal {
		// toNode is external, connecting to fromNode (managed internal)
		toEP, _, epFrom = compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, fromPort)
		if fromEP == "" && customToEnd != nil && customToEnd.Endpoint != "" {
			fromEP = customToEnd.Endpoint
		}
	} else {
		if fromEP == "" {
			fromEP, _, epTo = compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, toPort)
		}
		if toEP == "" {
			toEP, _, epFrom = compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, fromPort)
		}
	}

	fromKeepalive := 0
	if toEP != "" || (toNode.IsExternal && fromEP != "") {
		fromKeepalive = 25
	}
	toKeepalive := 0
	if fromEP != "" || (fromNode.IsExternal && toEP != "") {
		toKeepalive = 25
	}
	if customFromEnd != nil && customFromEnd.PersistentKeepalive > 0 {
		fromKeepalive = customFromEnd.PersistentKeepalive
	}
	if customToEnd != nil && customToEnd.PersistentKeepalive > 0 {
		toKeepalive = customToEnd.PersistentKeepalive
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
	if customFromEnd != nil && customFromEnd.MTU > 0 {
		fromMTU = customFromEnd.MTU
	}
	if customToEnd != nil && customToEnd.MTU > 0 {
		toMTU = customToEnd.MTU
	}

	fromUseIP := false
	if customFromEnd != nil {
		fromUseIP = customFromEnd.UseIp
	}
	toUseIP := false
	if customToEnd != nil {
		toUseIP = customToEnd.UseIp
	}

	fromIface := compiler.GetInterfaceName(toNode.Name, toNode.IsExternal)
	if customFromEnd != nil && customFromEnd.Interface != "" {
		fromIface = customFromEnd.Interface
	}
	toIface := compiler.GetInterfaceName(fromNode.Name, fromNode.IsExternal)
	if customToEnd != nil && customToEnd.Interface != "" {
		toIface = customToEnd.Interface
	}

	fromPolicy := ""
	if customFromEnd != nil && customFromEnd.Policy != "" {
		fromPolicy = customFromEnd.Policy
	} else if toNode.IsExternal {
		fromPolicy = config.PolicyDN42
	} else {
		fromPolicy = config.PolicyDefault
	}

	toPolicy := ""
	if customToEnd != nil && customToEnd.Policy != "" {
		toPolicy = customToEnd.Policy
	} else if fromNode.IsExternal {
		toPolicy = config.PolicyDN42
	} else {
		toPolicy = config.PolicyDefault
	}

	fromCost := 0
	if customFromEnd != nil && customFromEnd.Cost != 0 {
		fromCost = customFromEnd.Cost
	}
	toCost := 0
	if customToEnd != nil && customToEnd.Cost != 0 {
		toCost = customToEnd.Cost
	}

	link := &config.Link{
		From: config.LinkEnd{
			Name:                fromNode.Name,
			Interface:           fromIface,
			Address:             fromAddr,
			ListenPort:          fromPort,
			Endpoint:            fromEP,
			PrivateKey:          encPrivFrom,
			PublicKey:           pubKeyFrom,
			PersistentKeepalive: fromKeepalive,
			MTU:                 fromMTU,
			UseIp:               fromUseIP,
			Policy:              fromPolicy,
			Cost:                fromCost,
		},
		To: config.LinkEnd{
			Name:                toNode.Name,
			Interface:           toIface,
			Address:             toAddr,
			ListenPort:          toPort,
			Endpoint:            toEP,
			PrivateKey:          encPrivTo,
			PublicKey:           pubKeyTo,
			PersistentKeepalive: toKeepalive,
			MTU:                 toMTU,
			UseIp:               toUseIP,
			Policy:              toPolicy,
			Cost:                toCost,
		},
		Tags:       tags,
		ModifiedAt: time.Now().UTC(),
	}

	link.From.ResolvedEndpoint = compiler.ResolveLinkEndpoint(fromNode, toNode, &link.From, &link.To)
	link.To.ResolvedEndpoint = compiler.ResolveLinkEndpoint(toNode, fromNode, &link.To, &link.From)

	return link, nil
}

// AddLink adds a new WireGuard link between two nodes.
// Optional customMTU can specify [fromMTU, toMTU] (relative to node1Name, node2Name).
func (m *Manager) AddLink(node1Name, node2Name string, listenPort1, listenPort2 int, tags []string, customMTU ...int) (*config.Link, error) {
	return m.AddLinkAdvanced(node1Name, node2Name, nil, nil, tags, customMTU...)
}

// AddLinkAdvanced creates a link with full custom LinkEnd properties (useful for external peering)
func (m *Manager) AddLinkAdvanced(node1Name, node2Name string, fromEnd, toEnd *config.LinkEnd, tags []string, customMTU ...int) (*config.Link, error) {
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
	fromName, toName := n1.Name, n2.Name
	if strings.Compare(fromName, toName) > 0 {
		fromName, toName = toName, fromName
	}

	// Check duplicate link
	for _, l := range cfg.Links {
		if l.From.Name == fromName && l.To.Name == toName {
			return nil, ErrLinkAlreadyExist
		}
	}

	var lp *config.Link
	if fromEnd != nil || toEnd != nil {
		lp = &config.Link{}
		if fromEnd != nil {
			lp.From = *fromEnd
			if lp.From.Name == "" {
				lp.From.Name = node1Name
			}
		}
		if toEnd != nil {
			lp.To = *toEnd
			if lp.To.Name == "" {
				lp.To.Name = node2Name
			}
		}
	}

	listenPort1 := 0
	listenPort2 := 0
	if fromEnd != nil {
		listenPort1 = fromEnd.ListenPort
	}
	if toEnd != nil {
		listenPort2 = toEnd.ListenPort
	}

	link, err := m.buildLink(cfg, n1, n2, listenPort1, listenPort2, tags, customMTU1, customMTU2, lp)
	if err != nil {
		return nil, err
	}

	cfg.Links = append(cfg.Links, *link)
	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}

	return link, nil
}

// CreateFullMesh creates missing links between specified nodes (or all nodes if nil/empty)
// using default ports and MTU.
func (m *Manager) CreateFullMesh(nodeNames []string) ([]*config.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return nil, crypto.ErrVaultLocked
	}

	cfg := m.store.Get()
	var targetNodes []*config.Node

	if len(nodeNames) == 0 {
		for i := range cfg.Nodes {
			if !cfg.Nodes[i].IsExternal {
				targetNodes = append(targetNodes, &cfg.Nodes[i])
			}
		}
	} else {
		nodeMap := make(map[string]*config.Node)
		for i := range cfg.Nodes {
			nodeMap[cfg.Nodes[i].Name] = &cfg.Nodes[i]
		}
		for _, name := range nodeNames {
			if n, ok := nodeMap[name]; ok {
				if !n.IsExternal {
					targetNodes = append(targetNodes, n)
				}
			}
		}
	}

	if len(targetNodes) < 2 {
		return nil, errors.New("at least two nodes are required to create a mesh")
	}

	var addedLinks []*config.Link
	now := time.Now().UTC()

	for i := 0; i < len(targetNodes); i++ {
		for j := i + 1; j < len(targetNodes); j++ {
			n1 := targetNodes[i]
			n2 := targetNodes[j]

			// Lexicographical ordering
			fromName, toName := n1.Name, n2.Name
			if strings.Compare(fromName, toName) > 0 {
				fromName, toName = toName, fromName
			}

			// Check if link already exists
			exists := false
			for _, l := range cfg.Links {
				if l.From.Name == fromName && l.To.Name == toName {
					exists = true
					break
				}
			}
			if exists {
				continue
			}

			// Generate default ports
			fromPort := compiler.DerivePortFromIP(n2.IP)
			toPort := compiler.DerivePortFromIP(n1.IP)

			link, err := m.buildLink(cfg, n1, n2, fromPort, toPort, nil, 0, 0)
			if err != nil {
				return nil, err
			}
			link.ModifiedAt = now
			cfg.Links = append(cfg.Links, *link)
			addedLinks = append(addedLinks, link)
		}
	}

	if len(addedLinks) > 0 {
		if err := m.store.Save(cfg); err != nil {
			return nil, err
		}
	}

	return addedLinks, nil
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

	isExternalLink := fromNode.IsExternal || toNode.IsExternal
	if !isExternalLink {
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
	} else if toNode.IsExternal {
		toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, link.From.ListenPort)
		if toEP != "" {
			link.To.Endpoint = toEP
		}
	} else if fromNode.IsExternal {
		fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, link.To.ListenPort)
		if fromEP != "" {
			link.From.Endpoint = fromEP
		}
	}
	link.ModifiedAt = time.Now().UTC()
	link.From.ResolvedEndpoint = compiler.ResolveLinkEndpoint(fromNode, toNode, &link.From, &link.To)
	link.To.ResolvedEndpoint = compiler.ResolveLinkEndpoint(toNode, fromNode, &link.To, &link.From)

	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}

	return link, nil
}

// UpdateLinkAdvanced updates any parameters of an existing link, including addresses, endpoints, and public keys
func (m *Manager) UpdateLinkAdvanced(node1Name, node2Name string, customFrom, customTo *config.LinkEnd, tags []string) (*config.Link, error) {
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

	fromNode, toNode := n1, n2
	fromEnd, toEnd := customFrom, customTo
	if strings.Compare(fromNode.Name, toNode.Name) > 0 {
		fromNode, toNode = toNode, fromNode
		fromEnd, toEnd = toEnd, fromEnd
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

	if fromEnd != nil {
		if fromEnd.ListenPort > 0 {
			link.From.ListenPort = fromEnd.ListenPort
		}
		if fromEnd.Address != "" {
			link.From.Address = fromEnd.Address
		}
		if fromEnd.Endpoint != "" {
			link.From.Endpoint = fromEnd.Endpoint
		}
		if fromEnd.MTU > 0 {
			link.From.MTU = fromEnd.MTU
		}
		if fromEnd.PersistentKeepalive >= 0 {
			link.From.PersistentKeepalive = fromEnd.PersistentKeepalive
		}
		if fromNode.IsExternal && fromEnd.PublicKey != "" {
			link.From.PublicKey = fromEnd.PublicKey
		}
		link.From.UseIp = fromEnd.UseIp
		if fromEnd.Policy != "" {
			link.From.Policy = fromEnd.Policy
		}
		if fromEnd.Cost != 0 {
			link.From.Cost = fromEnd.Cost
		}
	}

	if toEnd != nil {
		if toEnd.ListenPort > 0 {
			link.To.ListenPort = toEnd.ListenPort
		}
		if toEnd.Address != "" {
			link.To.Address = toEnd.Address
		}
		if toEnd.Endpoint != "" {
			link.To.Endpoint = toEnd.Endpoint
		}
		if toEnd.MTU > 0 {
			link.To.MTU = toEnd.MTU
		}
		if toEnd.PersistentKeepalive >= 0 {
			link.To.PersistentKeepalive = toEnd.PersistentKeepalive
		}
		if toNode.IsExternal && toEnd.PublicKey != "" {
			link.To.PublicKey = toEnd.PublicKey
		}
		link.To.UseIp = toEnd.UseIp
		if toEnd.Policy != "" {
			link.To.Policy = toEnd.Policy
		}
		if toEnd.Cost != 0 {
			link.To.Cost = toEnd.Cost
		}
	}

	if tags != nil {
		link.Tags = tags
	}

	isExternalLink := fromNode.IsExternal || toNode.IsExternal
	if !isExternalLink {
		if (fromEnd == nil || fromEnd.Endpoint == "") && (toEnd == nil || toEnd.Endpoint == "") {
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
		}
	} else if toNode.IsExternal {
		if link.From.Endpoint == "" && toEnd != nil && toEnd.Endpoint != "" {
			link.From.Endpoint = toEnd.Endpoint
		}
		if link.From.Endpoint != "" && link.From.PersistentKeepalive == 0 {
			link.From.PersistentKeepalive = 25
		}
		toEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(toNode, fromNode, nil, link.From.ListenPort)
		if toEP != "" {
			link.To.Endpoint = toEP
		}
	} else if fromNode.IsExternal {
		if link.To.Endpoint == "" && fromEnd != nil && fromEnd.Endpoint != "" {
			link.To.Endpoint = fromEnd.Endpoint
		}
		if link.To.Endpoint != "" && link.To.PersistentKeepalive == 0 {
			link.To.PersistentKeepalive = 25
		}
		fromEP, _, _ := compiler.ResolvePeerEndpointWithEntrypoint(fromNode, toNode, nil, link.To.ListenPort)
		if fromEP != "" {
			link.From.Endpoint = fromEP
		}
	}

	link.ModifiedAt = time.Now().UTC()
	link.From.ResolvedEndpoint = compiler.ResolveLinkEndpoint(fromNode, toNode, &link.From, &link.To)
	link.To.ResolvedEndpoint = compiler.ResolveLinkEndpoint(toNode, fromNode, &link.To, &link.From)
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

	if node.IsExternal {
		status := &config.NodeStatus{
			Name:      node.Name,
			Host:      "external",
			LastSeen:  time.Now(),
			Connected: true,
			Hostname:  node.Name,
		}
		m.mu.Lock()
		m.statuses[nodeName] = status
		m.mu.Unlock()
		return status, nil
	}

	status := &config.NodeStatus{
		Name:      node.Name,
		Host:      node.Host,
		LastSeen:  time.Now(),
		Connected: false,
	}

	sshClient, _, err := m.pool.GetClientWithTimeout(node.Host, 4*time.Second)
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

// PlanSync computes the sync actions required to bring remote devices up to date.
// If nodeNames are provided, only changes for those target nodes are computed.
func (m *Manager) PlanSync(nodeNames ...string) ([]config.SyncAction, error) {
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

	targetMap := make(map[string]bool)
	for _, name := range nodeNames {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			targetMap[trimmed] = true
		}
	}
	isPartial := len(targetMap) > 0

	currentState := m.stateStore.Get()
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
		if n.IsExternal {
			continue
		}
		if isPartial && !targetMap[n.Name] {
			continue
		}
		node := n
		wg.Add(1)
		go func(targetNode config.Node) {
			defer wg.Done()
			sshClient, _, err := m.pool.GetClientWithTimeout(targetNode.Host, 3*time.Second)
			if err != nil {
				m.mu.Lock()
				m.statuses[targetNode.Name] = &config.NodeStatus{
					Name:      targetNode.Name,
					Host:      targetNode.Host,
					LastSeen:  time.Now(),
					Connected: false,
					Error:     err.Error(),
				}
				m.mu.Unlock()
				return
			}
			runningIfaces, err := ssh.GetRunningWgInterfaces(sshClient)
			if err != nil {
				return
			}
			expected := expectedIfacesPerNode[targetNode.Name]
			for _, iface := range runningIfaces {
				// We only manage wg42* prefix wireguard interfaces
				if strings.HasPrefix(iface, "wg42") {
					if !expected[iface] {
						targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", iface)
						cleanActionsMu.Lock()
						cleanActions = append(cleanActions, config.SyncAction{
							NodeName:    targetNode.Name,
							Host:        targetNode.Host,
							Type:        config.ActionDeleteConfig,
							Interface:   iface,
							TargetFile:  targetFile,
							Command:     fmt.Sprintf("wg-quick down %s && rm -f %s", iface, targetFile),
							Description: fmt.Sprintf("Remove deleted interface %s on %s", iface, targetNode.Name),
							FileContent: fmt.Sprintf("# Deleted interface %s is no longer in graph.\n# Action: wg-quick down %s && rm -f %s\n", iface, iface, targetFile),
							NeedsApply:  true,
							Status:      "pending",
							DiffStatus:  "delete",
						})
						cleanActionsMu.Unlock()
					}
				}
			}
		}(node)
	}
	wg.Wait()

	// Also check stateStore for recorded interfaces that are no longer expected
	for _, n := range nodes {
		if n.IsExternal {
			continue
		}
		if isPartial && !targetMap[n.Name] {
			continue
		}
		expected := expectedIfacesPerNode[n.Name]
		if stNode, ok := currentState.Nodes[n.Name]; ok {
			for ifaceName := range stNode.Interfaces {
				if strings.HasPrefix(ifaceName, "wg42") && !expected[ifaceName] {
					alreadyAdded := false
					for _, ca := range cleanActions {
						if ca.NodeName == n.Name && ca.Interface == ifaceName {
							alreadyAdded = true
							break
						}
					}
					if !alreadyAdded {
						targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", ifaceName)
						cleanActions = append(cleanActions, config.SyncAction{
							NodeName:    n.Name,
							Host:        n.Host,
							Type:        config.ActionDeleteConfig,
							Interface:   ifaceName,
							TargetFile:  targetFile,
							Command:     fmt.Sprintf("wg-quick down %s && rm -f %s", ifaceName, targetFile),
							Description: fmt.Sprintf("Remove deleted interface %s on %s", ifaceName, n.Name),
							FileContent: fmt.Sprintf("# Deleted interface %s is no longer in graph.\n# Action: wg-quick down %s && rm -f %s\n", ifaceName, ifaceName, targetFile),
							NeedsApply:  true,
							Status:      "pending",
							DiffStatus:  "delete",
						})
					}
				}
			}
		}
	}

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

		// 1. From node end (only if fromNode is managed)
		if !fromNode.IsExternal && (!isPartial || targetMap[fromNode.Name]) {
			fromConf, err := compiler.GenerateWgConfigContent(fromNode, toNode, &link.From, &link.To, m.vault)
			if err == nil {
				targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", link.From.Interface)
				desiredHash := config.HashConfig(compiler.NormalizeConfig(fromConf))

				needsApply := true
				status := "pending"
				diffStatus := "create"

				if stNode, ok := currentState.Nodes[fromNode.Name]; ok {
					if stIface, ok := stNode.Interfaces[link.From.Interface]; ok {
						if stIface.ConfigHash == desiredHash {
							needsApply = false
							status = "synced"
							diffStatus = "synced"
						} else {
							diffStatus = "update"
						}
					}
				}

				actions = append(actions, config.SyncAction{
					NodeName:    fromNode.Name,
					Host:        fromNode.Host,
					Type:        config.ActionSyncConfig,
					Interface:   link.From.Interface,
					TargetFile:  targetFile,
					FileContent: fromConf,
					Description: fmt.Sprintf("Configure %s on %s (peer %s)", link.From.Interface, fromNode.Name, toNode.Name),
					NeedsApply:  needsApply,
					Status:      status,
					DiffStatus:  diffStatus,
				})
			}
		}

		// 2. To node end (only if toNode is managed)
		if !toNode.IsExternal && (!isPartial || targetMap[toNode.Name]) {
			toConf, err := compiler.GenerateWgConfigContent(toNode, fromNode, &link.To, &link.From, m.vault)
			if err == nil {
				targetFile := fmt.Sprintf("/etc/wireguard/%s.conf", link.To.Interface)
				desiredHash := config.HashConfig(compiler.NormalizeConfig(toConf))

				needsApply := true
				status := "pending"
				diffStatus := "create"

				if stNode, ok := currentState.Nodes[toNode.Name]; ok {
					if stIface, ok := stNode.Interfaces[link.To.Interface]; ok {
						if stIface.ConfigHash == desiredHash {
							needsApply = false
							status = "synced"
							diffStatus = "synced"
						} else {
							diffStatus = "update"
						}
					}
				}

				actions = append(actions, config.SyncAction{
					NodeName:    toNode.Name,
					Host:        toNode.Host,
					Type:        config.ActionSyncConfig,
					Interface:   link.To.Interface,
					TargetFile:  targetFile,
					FileContent: toConf,
					Description: fmt.Sprintf("Configure %s on %s (peer %s)", link.To.Interface, toNode.Name, fromNode.Name),
					NeedsApply:  needsApply,
					Status:      status,
					DiffStatus:  diffStatus,
				})
			}
		}
	}

	// Build map of all policies and nodes for policy/ROA planning
	allPolicies := cfg.GetAllPolicies()
	policyMap := make(map[string]config.NetworkPolicy)
	for _, p := range allPolicies {
		policyMap[p.ID] = p
	}

	nodeByName := make(map[string]*config.Node)
	for i := range nodes {
		nodeByName[nodes[i].Name] = &nodes[i]
	}

	// 3. Node ROA & BIRD configurations
	for _, n := range nodes {
		if n.IsExternal {
			continue
		}
		if isPartial && !targetMap[n.Name] {
			continue
		}
		node := n

		// Determine policies used by links on this node
		usedPolicies := make(map[string]config.NetworkPolicy)
		for _, l := range links {
			var localEnd *config.LinkEnd
			var remoteNode *config.Node
			if l.From.Name == node.Name {
				localEnd = &l.From
				remoteNode = nodeByName[l.To.Name]
			} else if l.To.Name == node.Name {
				localEnd = &l.To
				remoteNode = nodeByName[l.From.Name]
			} else {
				continue
			}
			isRemoteExternal := remoteNode != nil && remoteNode.IsExternal
			pID := localEnd.EffectivePolicy(isRemoteExternal)
			if pol, ok := policyMap[pID]; ok {
				usedPolicies[pID] = pol
			} else if isRemoteExternal {
				usedPolicies[config.PolicyDN42] = policyMap[config.PolicyDN42]
			} else {
				usedPolicies[config.PolicyDefault] = policyMap[config.PolicyDefault]
			}
		}

		var usedPolicyIDs []string
		for pID := range usedPolicies {
			usedPolicyIDs = append(usedPolicyIDs, pID)
		}
		sort.Strings(usedPolicyIDs)

		nodeHasRoaUpdates := false
		for _, pID := range usedPolicyIDs {
			pol := usedPolicies[pID]
			cleanID := compiler.SanitizeIdentifier(pol.ID)

			if strings.TrimSpace(pol.ROA4) != "" {
				targetFile := fmt.Sprintf("/etc/easy42_%s_roa4.conf", cleanID)
				content, _ := m.roaManager.GetROAContent(context.Background(), pol.ROA4, false)
				desiredHash := config.HashConfig(compiler.NormalizeConfig(content))

				needsApply := true
				status := "pending"
				diffStatus := "create"

				if stNode, ok := currentState.Nodes[node.Name]; ok && stNode.RoaConfigHashes != nil {
					if h, exists := stNode.RoaConfigHashes[targetFile]; exists && h != "" {
						if h == desiredHash {
							needsApply = false
							status = "synced"
							diffStatus = "synced"
						} else {
							diffStatus = "update"
						}
					}
				}

				if needsApply {
					nodeHasRoaUpdates = true
				}

				actions = append(actions, config.SyncAction{
					NodeName:    node.Name,
					Host:        node.Host,
					Type:        config.ActionSyncRoaConfig,
					Interface:   cleanID + "_roa4",
					TargetFile:  targetFile,
					FileContent: content,
					Description: fmt.Sprintf("Deploy ROA4 table (%s) for policy '%s' on %s", targetFile, pol.Name, node.Name),
					NeedsApply:  needsApply,
					Status:      status,
					DiffStatus:  diffStatus,
				})
			}

			if strings.TrimSpace(pol.ROA6) != "" {
				targetFile := fmt.Sprintf("/etc/easy42_%s_roa6.conf", cleanID)
				content, _ := m.roaManager.GetROAContent(context.Background(), pol.ROA6, false)
				desiredHash := config.HashConfig(compiler.NormalizeConfig(content))

				needsApply := true
				status := "pending"
				diffStatus := "create"

				if stNode, ok := currentState.Nodes[node.Name]; ok && stNode.RoaConfigHashes != nil {
					if h, exists := stNode.RoaConfigHashes[targetFile]; exists && h != "" {
						if h == desiredHash {
							needsApply = false
							status = "synced"
							diffStatus = "synced"
						} else {
							diffStatus = "update"
						}
					}
				}

				if needsApply {
					nodeHasRoaUpdates = true
				}

				actions = append(actions, config.SyncAction{
					NodeName:    node.Name,
					Host:        node.Host,
					Type:        config.ActionSyncRoaConfig,
					Interface:   cleanID + "_roa6",
					TargetFile:  targetFile,
					FileContent: content,
					Description: fmt.Sprintf("Deploy ROA6 table (%s) for policy '%s' on %s", targetFile, pol.Name, node.Name),
					NeedsApply:  needsApply,
					Status:      status,
					DiffStatus:  diffStatus,
				})
			}
		}

		birdConf, err := compiler.GenerateBirdConfig(&node, nodes, links, &cfg.NetworkSettings, cfg.NetworkPolicies)
		if err != nil {
			continue
		}

		targetFile := compiler.DefaultBirdConfigPath
		desiredHash := config.HashConfig(compiler.NormalizeConfig(birdConf))

		needsApply := true
		status := "pending"
		diffStatus := "create"

		if stNode, ok := currentState.Nodes[node.Name]; ok {
			if stNode.BirdConfigHash != "" {
				if stNode.BirdConfigHash == desiredHash {
					needsApply = false
					status = "synced"
					diffStatus = "synced"
				} else {
					diffStatus = "update"
				}
			}
		}

		if nodeHasRoaUpdates {
			needsApply = true
			if diffStatus == "synced" {
				diffStatus = "update"
				status = "pending"
			}
		}

		actions = append(actions, config.SyncAction{
			NodeName:    node.Name,
			Host:        node.Host,
			Type:        config.ActionSyncBirdConfig,
			Interface:   "bird",
			TargetFile:  targetFile,
			FileContent: birdConf,
			Command:     "birdc configure",
			Description: fmt.Sprintf("Configure BIRD routing (%s) on %s", targetFile, node.Name),
			NeedsApply:  needsApply,
			Status:      status,
			DiffStatus:  diffStatus,
		})
	}

	// 4. Node nftables configurations
	for _, n := range nodes {
		if n.IsExternal {
			continue
		}
		if isPartial && !targetMap[n.Name] {
			continue
		}
		node := n
		nftConf, err := compiler.GenerateNftablesConfig(&node, nodes, links, &cfg.NetworkSettings, cfg.NetworkPolicies)
		if err != nil {
			continue
		}

		targetFile := compiler.DefaultNftablesConfigPath
		desiredHash := config.HashConfig(compiler.NormalizeConfig(nftConf))

		needsApply := true
		status := "pending"
		diffStatus := "create"

		if stNode, ok := currentState.Nodes[node.Name]; ok {
			if stNode.NftablesConfigHash != "" {
				if stNode.NftablesConfigHash == desiredHash {
					needsApply = false
					status = "synced"
					diffStatus = "synced"
				} else {
					diffStatus = "update"
				}
			}
		}

		actions = append(actions, config.SyncAction{
			NodeName:    node.Name,
			Host:        node.Host,
			Type:        config.ActionSyncNftablesConfig,
			Interface:   "nftables",
			TargetFile:  targetFile,
			FileContent: nftConf,
			Command:     targetFile,
			Description: fmt.Sprintf("Configure nftables firewall (%s) on %s", targetFile, node.Name),
			NeedsApply:  needsApply,
			Status:      status,
			DiffStatus:  diffStatus,
		})
	}

	// Sort actions: pending (NeedsApply == true) first, then synced.
	// Within each group, order by actionPriority (clean/delete -> wireguard -> bird), then NodeName and Interface
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].NeedsApply != actions[j].NeedsApply {
			return actions[i].NeedsApply // true comes before false
		}
		pI := actionPriority(actions[i].Type)
		pJ := actionPriority(actions[j].Type)
		if pI != pJ {
			return pI < pJ
		}
		if actions[i].NodeName != actions[j].NodeName {
			return actions[i].NodeName < actions[j].NodeName
		}
		return actions[i].Interface < actions[j].Interface
	})

	if actions == nil {
		actions = []config.SyncAction{}
	}
	return actions, nil
}

// ExecuteSync executes planned actions across nodes
func (m *Manager) ExecuteSync(force ...bool) ([]config.SyncResult, error) {
	isForce := len(force) > 0 && force[0]
	return m.ExecuteSyncNodes(isForce)
}

// ExecuteSyncNodes executes planned actions across specified nodes (or all nodes if none specified)
func (m *Manager) ExecuteSyncNodes(isForce bool, nodeNames ...string) ([]config.SyncResult, error) {
	actions, err := m.PlanSync(nodeNames...)
	if err != nil {
		return nil, err
	}

	var actionsToRun []config.SyncAction
	for _, act := range actions {
		if isForce || act.NeedsApply {
			actionsToRun = append(actionsToRun, act)
		}
	}

	if len(actionsToRun) == 0 {
		return []config.SyncResult{}, nil
	}

	results := make([]config.SyncResult, 0)
	for _, act := range actionsToRun {
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
				_ = m.stateStore.RemoveInterface(act.NodeName, act.Interface)
			}
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)
			continue
		}

		// Check current remote file
		currentContent, _ := ssh.ReadRemoteFile(sftpClient, act.TargetFile)
		needsUpdate := isForce || compiler.NeedsUpdate(currentContent, act.FileContent)

		// Handle ROA configuration update
		if act.Type == config.ActionSyncRoaConfig {
			if needsUpdate {
				if err := ssh.AtomicWriteFile(sftpClient, act.TargetFile, []byte(act.FileContent), 0644); err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to write ROA config (%s): %v", act.TargetFile, err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}
			}

			res.Success = true
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)

			// Record successful application in stateStore
			hash := config.HashConfig(compiler.NormalizeConfig(act.FileContent))
			_ = m.stateStore.UpdateRoaState(act.NodeName, act.Host, act.TargetFile, hash, time.Now())
			continue
		}

		// Handle BIRD configuration update
		if act.Type == config.ActionSyncBirdConfig {
			if needsUpdate {
				if err := ssh.AtomicWriteFile(sftpClient, act.TargetFile, []byte(act.FileContent), 0644); err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to write BIRD config: %v", err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}

				// Apply BIRD configuration on remote device
				out, err := ssh.RunCommand(sshClient, "birdc configure")
				res.Output = strings.TrimSpace(out)
				if err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to apply BIRD config (birdc configure): %v", err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}
			}

			res.Success = true
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)

			// Record successful application in stateStore
			hash := config.HashConfig(compiler.NormalizeConfig(act.FileContent))
			_ = m.stateStore.UpdateBirdState(act.NodeName, act.Host, hash, time.Now())
			continue
		}

		// Handle nftables configuration update
		if act.Type == config.ActionSyncNftablesConfig {
			if needsUpdate {
				if err := ssh.AtomicWriteFile(sftpClient, act.TargetFile, []byte(act.FileContent), 0755); err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to write nftables config: %v", err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}

				// Apply nftables rules on remote device
				cmd := act.Command
				if cmd == "" {
					cmd = "/etc/easy42.nft"
				}
				out, err := ssh.RunCommand(sshClient, fmt.Sprintf("chmod +x %s 2>/dev/null; %s", act.TargetFile, cmd))
				res.Output = strings.TrimSpace(out)
				if err != nil {
					res.Success = false
					res.Error = fmt.Sprintf("Failed to apply nftables rules (%s): %v", cmd, err)
					res.Duration = float64(time.Since(start).Milliseconds())
					results = append(results, res)
					continue
				}
			}

			res.Success = true
			res.Duration = float64(time.Since(start).Milliseconds())
			results = append(results, res)

			// Record successful application in stateStore
			hash := config.HashConfig(compiler.NormalizeConfig(act.FileContent))
			_ = m.stateStore.UpdateNftablesState(act.NodeName, act.Host, hash, time.Now())
			continue
		}

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

		// Record successful application in stateStore
		hash := config.HashConfig(compiler.NormalizeConfig(act.FileContent))
		existingState := m.stateStore.Get()
		var existingHandshake *time.Time
		workingState := config.WorkingStateUnknown
		var existingRx, existingTx int64
		if stNode, ok := existingState.Nodes[act.NodeName]; ok {
			if stIface, ok := stNode.Interfaces[act.Interface]; ok {
				existingHandshake = stIface.LatestHandshake
				workingState = stIface.WorkingState
				existingRx = stIface.TransferRxBytes
				existingTx = stIface.TransferTxBytes
			}
		}
		_ = m.stateStore.UpdateInterface(act.NodeName, act.Host, config.StateInterface{
			Name:            act.Interface,
			TargetFile:      act.TargetFile,
			ConfigHash:      hash,
			Status:          "active",
			LatestHandshake: existingHandshake,
			WorkingState:    workingState,
			TransferRxBytes: existingRx,
			TransferTxBytes: existingTx,
			AppliedAt:       time.Now(),
		})
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

// UpdateState connects to devices via SSH/SFTP to fetch their live state and update state.json.
// If nodeNames are provided, only those specific nodes are refreshed and their links/interfaces updated.
func (m *Manager) UpdateState(nodeNames ...string) (*config.NetworkState, []string, error) {
	m.mu.RLock()
	cfg := m.store.Get()
	nodes := make([]config.Node, len(cfg.Nodes))
	copy(nodes, cfg.Nodes)
	links := make([]config.Link, len(cfg.Links))
	copy(links, cfg.Links)
	m.mu.RUnlock()

	targetMap := make(map[string]bool)
	for _, name := range nodeNames {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			targetMap[trimmed] = true
		}
	}
	isPartialUpdate := len(targetMap) > 0

	type linkMetaInfo struct {
		peerNode            string
		peerPubKey          string
		persistentKeepalive int
	}
	linkMeta := make(map[string]map[string]linkMetaInfo)
	for _, link := range links {
		// End From
		if linkMeta[link.From.Name] == nil {
			linkMeta[link.From.Name] = make(map[string]linkMetaInfo)
		}
		ka := 0
		if link.From.PersistentKeepalive > 0 || link.To.PersistentKeepalive > 0 {
			ka = 25
		}
		linkMeta[link.From.Name][link.From.Interface] = linkMetaInfo{
			peerNode:            link.To.Name,
			peerPubKey:          link.To.PublicKey,
			persistentKeepalive: ka,
		}

		// End To
		if linkMeta[link.To.Name] == nil {
			linkMeta[link.To.Name] = make(map[string]linkMetaInfo)
		}
		linkMeta[link.To.Name][link.To.Interface] = linkMetaInfo{
			peerNode:            link.From.Name,
			peerPubKey:          link.From.PublicKey,
			persistentKeepalive: ka,
		}
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var warnings []string

	type nodeIfaceResult struct {
		nodeName string
		host     string
		ifaces   map[string]config.StateInterface
	}
	results := make([]nodeIfaceResult, 0, len(nodes))

	for _, n := range nodes {
		node := n
		if isPartialUpdate && !targetMap[node.Name] {
			continue
		}

		if node.IsExternal {
			m.mu.Lock()
			m.statuses[node.Name] = &config.NodeStatus{
				Name:      node.Name,
				Host:      "external",
				LastSeen:  time.Now(),
				Connected: true,
				Hostname:  node.Name,
			}
			m.mu.Unlock()
			continue
		}

		wg.Add(1)
		go func(targetNode config.Node) {
			defer wg.Done()

			type probeOutput struct {
				ifaces   map[string]config.StateInterface
				hostname string
				wgStatus []config.WgInterfaceStatus
				err      error
			}
			outChan := make(chan probeOutput, 1)

			go func() {
				sshClient, sftpClient, err := m.pool.GetClientWithTimeout(targetNode.Host, 4*time.Second)
				if err != nil {
					outChan <- probeOutput{err: err}
					return
				}

				// Query WireGuard interface and peer statuses
				wgStatus, _ := ssh.QueryWireGuardStatus(sshClient)
				wgMap := make(map[string]*config.WgInterfaceStatus)
				for i := range wgStatus {
					wgMap[wgStatus[i].Name] = &wgStatus[i]
				}

				// 1. Find all wg42*.conf files in /etc/wireguard
				filesOut, _ := ssh.RunCommand(sshClient, "ls -1 /etc/wireguard/wg42*.conf 2>/dev/null")
				lines := strings.Split(strings.TrimSpace(filesOut), "\n")

				nodeIfaces := make(map[string]config.StateInterface)
				for _, line := range lines {
					filePath := strings.TrimSpace(line)
					if filePath == "" {
						continue
					}
					base := filepath.Base(filePath)
					ifaceName := strings.TrimSuffix(base, ".conf")
					if !strings.HasPrefix(ifaceName, "wg42") {
						continue
					}

					// Read file content
					content, readErr := ssh.ReadRemoteFile(sftpClient, filePath)
					if readErr != nil {
						catOut, catErr := ssh.RunCommand(sshClient, fmt.Sprintf("cat %s", filePath))
						if catErr == nil {
							content = catOut
						}
					}

					hash := config.HashConfig(compiler.NormalizeConfig(content))
					status := "down"
					isStarted := ssh.IsInterfaceStarted(sshClient, ifaceName)
					if isStarted {
						status = "active"
					}

					meta := linkMeta[targetNode.Name][ifaceName]
					var latestHandshake time.Time
					var rxBytes, txBytes int64
					keepalive := meta.persistentKeepalive

					if wgInfo, ok := wgMap[ifaceName]; ok && len(wgInfo.Peers) > 0 {
						var matchedPeer *config.WgPeerStatus
						for pIdx := range wgInfo.Peers {
							if meta.peerPubKey != "" && wgInfo.Peers[pIdx].PublicKey == meta.peerPubKey {
								matchedPeer = &wgInfo.Peers[pIdx]
								break
							}
						}
						if matchedPeer == nil {
							matchedPeer = &wgInfo.Peers[0]
						}
						latestHandshake = matchedPeer.LatestHandshake
						rxBytes = matchedPeer.TransferRxBytes
						txBytes = matchedPeer.TransferTxBytes
						if matchedPeer.PersistentKeepalive > 0 {
							keepalive = matchedPeer.PersistentKeepalive
						}
					}

					now := time.Now()
					var handshakePtr *time.Time
					workingState := config.WorkingStateUnknown

					if !isStarted {
						workingState = config.WorkingStateNotWorking
					} else if !latestHandshake.IsZero() {
						handshakePtr = &latestHandshake
						if now.Sub(latestHandshake) <= 3*time.Minute {
							workingState = config.WorkingStateWorking
						} else {
							if keepalive > 0 {
								workingState = config.WorkingStateNotWorking
							} else {
								workingState = config.WorkingStateUnknown
							}
						}
					} else {
						if keepalive > 0 {
							workingState = config.WorkingStateNotWorking
						} else {
							workingState = config.WorkingStateUnknown
						}
					}

					nodeIfaces[ifaceName] = config.StateInterface{
						Name:            ifaceName,
						TargetFile:      filePath,
						ConfigHash:      hash,
						PeerNode:        meta.peerNode,
						PeerPubKey:      meta.peerPubKey,
						Status:          status,
						LatestHandshake: handshakePtr,
						WorkingState:    workingState,
						TransferRxBytes: rxBytes,
						TransferTxBytes: txBytes,
						AppliedAt:       time.Now(),
					}
				}

				hostname, _ := ssh.RunCommand(sshClient, "hostname")
				outChan <- probeOutput{
					ifaces:   nodeIfaces,
					hostname: strings.TrimSpace(hostname),
					wgStatus: wgStatus,
				}
			}()

			select {
			case out := <-outChan:
				if out.err != nil {
					mu.Lock()
					warnings = append(warnings, fmt.Sprintf("%s: failed to connect: %v", targetNode.Name, out.err))
					mu.Unlock()
					m.mu.Lock()
					m.statuses[targetNode.Name] = &config.NodeStatus{
						Name:      targetNode.Name,
						Host:      targetNode.Host,
						LastSeen:  time.Now(),
						Connected: false,
						Error:     out.err.Error(),
					}
					m.mu.Unlock()
					return
				}

				m.mu.Lock()
				m.statuses[targetNode.Name] = &config.NodeStatus{
					Name:         targetNode.Name,
					Host:         targetNode.Host,
					LastSeen:     time.Now(),
					Connected:    true,
					Hostname:     out.hostname,
					WgInterfaces: out.wgStatus,
				}
				m.mu.Unlock()

				mu.Lock()
				results = append(results, nodeIfaceResult{
					nodeName: targetNode.Name,
					host:     targetNode.Host,
					ifaces:   out.ifaces,
				})
				mu.Unlock()

			case <-time.After(15 * time.Second):
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("%s: probe timed out", targetNode.Name))
				mu.Unlock()
				m.mu.Lock()
				m.statuses[targetNode.Name] = &config.NodeStatus{
					Name:      targetNode.Name,
					Host:      targetNode.Host,
					LastSeen:  time.Now(),
					Connected: false,
					Error:     "probe timed out after 7s",
				}
				m.mu.Unlock()
				return
			}
		}(node)
	}
	wg.Wait()

	// Update state store
	currentState := m.stateStore.Get()

	// Remove deleted or external nodes from state ONLY on full cluster updates
	if !isPartialUpdate {
		activeNodeNames := make(map[string]bool)
		for _, n := range nodes {
			if !n.IsExternal {
				activeNodeNames[n.Name] = true
			}
		}

		for stName := range currentState.Nodes {
			if !activeNodeNames[stName] {
				delete(currentState.Nodes, stName)
			}
		}
	}

	// Merge probed node interfaces
	for _, res := range results {
		existingNode := currentState.Nodes[res.nodeName]
		currentState.Nodes[res.nodeName] = config.StateNode{
			Name:               res.nodeName,
			Host:               res.host,
			LastSeen:           time.Now(),
			BirdConfigHash:     existingNode.BirdConfigHash,
			BirdAppliedAt:      existingNode.BirdAppliedAt,
			NftablesConfigHash: existingNode.NftablesConfigHash,
			NftablesAppliedAt:  existingNode.NftablesAppliedAt,
			Interfaces:         res.ifaces,
		}
	}

	if err := m.stateStore.Save(currentState); err != nil {
		return nil, warnings, fmt.Errorf("failed to save updated state: %w", err)
	}

	return currentState, warnings, nil
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

// GenerateBirdConfig generates the BIRD configuration for a given node
func (m *Manager) GenerateBirdConfig(nodeName string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.Get()
	var targetNode *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == nodeName {
			targetNode = &cfg.Nodes[i]
			break
		}
	}
	if targetNode == nil {
		return "", fmt.Errorf("node %s not found", nodeName)
	}

	return compiler.GenerateBirdConfig(targetNode, cfg.Nodes, cfg.Links, &cfg.NetworkSettings, cfg.NetworkPolicies)
}

// GenerateBirdConfigWithTemplate generates BIRD config using a custom template
func (m *Manager) GenerateBirdConfigWithTemplate(nodeName string, tmplContent string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.Get()
	var targetNode *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == nodeName {
			targetNode = &cfg.Nodes[i]
			break
		}
	}
	if targetNode == nil {
		return "", fmt.Errorf("node %s not found", nodeName)
	}

	return compiler.GenerateBirdConfigWithTemplate(tmplContent, targetNode, cfg.Nodes, cfg.Links, &cfg.NetworkSettings, cfg.NetworkPolicies)
}

// GenerateNftablesConfig generates the nftables configuration for a given node
func (m *Manager) GenerateNftablesConfig(nodeName string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.Get()
	var targetNode *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == nodeName {
			targetNode = &cfg.Nodes[i]
			break
		}
	}
	if targetNode == nil {
		return "", fmt.Errorf("node %s not found", nodeName)
	}

	return compiler.GenerateNftablesConfig(targetNode, cfg.Nodes, cfg.Links, &cfg.NetworkSettings, cfg.NetworkPolicies)
}

// GenerateNftablesConfigWithTemplate generates nftables config using a custom template
func (m *Manager) GenerateNftablesConfigWithTemplate(nodeName string, tmplContent string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.store.Get()
	var targetNode *config.Node
	for i := range cfg.Nodes {
		if cfg.Nodes[i].Name == nodeName {
			targetNode = &cfg.Nodes[i]
			break
		}
	}
	if targetNode == nil {
		return "", fmt.Errorf("node %s not found", nodeName)
	}

	return compiler.GenerateNftablesConfigWithTemplate(tmplContent, targetNode, cfg.Nodes, cfg.Links, &cfg.NetworkSettings, cfg.NetworkPolicies)
}

// GetNetworkPolicies returns all policies (built-in virtual policies + custom policies)
func (m *Manager) GetNetworkPolicies() []config.NetworkPolicy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return config.GetBuiltinPolicies(nil)
	}
	return cfg.GetAllPolicies()
}

// GetNetworkPolicy finds a policy by its ID
func (m *Manager) GetNetworkPolicy(id string) (*config.NetworkPolicy, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return nil, errors.New("config not found")
	}
	p := cfg.FindPolicy(id)
	if p == nil {
		return nil, errors.New("network policy not found")
	}
	return p, nil
}

// CreateNetworkPolicy adds a new custom network policy
func (m *Manager) CreateNetworkPolicy(p config.NetworkPolicy) (*config.NetworkPolicy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return nil, crypto.ErrVaultLocked
	}

	id := strings.TrimSpace(p.ID)
	name := strings.TrimSpace(p.Name)
	if id == "" {
		return nil, errors.New("policy id is required")
	}
	if name == "" {
		return nil, errors.New("policy name is required")
	}
	if id == config.PolicyDefault || id == config.PolicyDN42 || id == config.PolicyNone {
		return nil, fmt.Errorf("cannot create policy with reserved ID %q", id)
	}

	cfg := m.store.Get()
	for _, existing := range cfg.NetworkPolicies {
		if existing.ID == id {
			return nil, fmt.Errorf("network policy with ID %q already exists", id)
		}
	}

	p.ID = id
	p.Name = name
	p.IsInternal = false
	if p.Cost <= 0 {
		p.Cost = 100
	}
	p.AllowedDstCIDRs = config.CleanPrefixes(p.AllowedDstCIDRs)
	p.AllowedSrcCIDRs = config.CleanPrefixes(p.AllowedSrcCIDRs)
	p.AllowedImportCIDRs = config.CleanPrefixes(p.AllowedImportCIDRs)
	p.InputTCPPorts = config.CleanPortList(p.InputTCPPorts)
	p.InputUDPPorts = config.CleanPortList(p.InputUDPPorts)

	cfg.NetworkPolicies = append(cfg.NetworkPolicies, p)
	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateNetworkPolicy updates an existing custom network policy
func (m *Manager) UpdateNetworkPolicy(id string, p config.NetworkPolicy) (*config.NetworkPolicy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return nil, crypto.ErrVaultLocked
	}

	if id == config.PolicyDefault || id == config.PolicyDN42 || id == config.PolicyNone {
		return nil, fmt.Errorf("cannot modify built-in policy %q", id)
	}

	name := strings.TrimSpace(p.Name)
	if name == "" {
		return nil, errors.New("policy name is required")
	}

	cfg := m.store.Get()
	idx := -1
	for i, existing := range cfg.NetworkPolicies {
		if existing.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, fmt.Errorf("network policy %q not found", id)
	}

	if p.Cost <= 0 {
		p.Cost = 100
	}

	cfg.NetworkPolicies[idx].Name = name
	cfg.NetworkPolicies[idx].Description = strings.TrimSpace(p.Description)
	cfg.NetworkPolicies[idx].Cost = p.Cost
	cfg.NetworkPolicies[idx].AllowedDstCIDRs = config.CleanPrefixes(p.AllowedDstCIDRs)
	cfg.NetworkPolicies[idx].AllowedSrcCIDRs = config.CleanPrefixes(p.AllowedSrcCIDRs)
	cfg.NetworkPolicies[idx].AllowedImportCIDRs = config.CleanPrefixes(p.AllowedImportCIDRs)
	cfg.NetworkPolicies[idx].RejectInternet = p.RejectInternet
	cfg.NetworkPolicies[idx].FilterForward = p.FilterForward
	cfg.NetworkPolicies[idx].FilterInput = p.FilterInput
	cfg.NetworkPolicies[idx].InputAllowICMP = p.InputAllowICMP
	cfg.NetworkPolicies[idx].InputAllowICMP6 = p.InputAllowICMP6
	cfg.NetworkPolicies[idx].InputTCPPorts = config.CleanPortList(p.InputTCPPorts)
	cfg.NetworkPolicies[idx].InputUDPPorts = config.CleanPortList(p.InputUDPPorts)
	cfg.NetworkPolicies[idx].SNAT = p.SNAT

	if err := m.store.Save(cfg); err != nil {
		return nil, err
	}
	res := cfg.NetworkPolicies[idx]
	return &res, nil
}

// DeleteNetworkPolicy deletes a custom network policy if it's not currently used by any link
func (m *Manager) DeleteNetworkPolicy(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.vault.IsUnlocked() {
		return crypto.ErrVaultLocked
	}

	if id == config.PolicyDefault || id == config.PolicyDN42 || id == config.PolicyNone {
		return fmt.Errorf("cannot delete built-in policy %q", id)
	}

	cfg := m.store.Get()
	// Check if any link uses this policy
	for _, l := range cfg.Links {
		if l.From.Policy == id || l.To.Policy == id {
			return fmt.Errorf("policy %q is currently in use by link %s <-> %s", id, l.From.Name, l.To.Name)
		}
	}

	idx := -1
	for i, existing := range cfg.NetworkPolicies {
		if existing.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("network policy %q not found", id)
	}

	cfg.NetworkPolicies = append(cfg.NetworkPolicies[:idx], cfg.NetworkPolicies[idx+1:]...)
	return m.store.Save(cfg)
}

// GetNetworkSettings returns current network settings from config
func (m *Manager) GetNetworkSettings() config.NetworkSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cfg := m.store.Get()
	if cfg == nil {
		return config.NetworkSettings{}
	}
	ns := cfg.NetworkSettings
	ns.Prefixes = config.CleanPrefixes(ns.Prefixes)
	return ns
}

// UpdateNetworkSettings updates the global network settings in config.json
func (m *Manager) UpdateNetworkSettings(settings config.NetworkSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg := m.store.Get()
	if cfg == nil {
		return config.ErrConfigNotFound
	}
	settings.Prefixes = config.CleanPrefixes(settings.Prefixes)
	cfg.NetworkSettings = settings
	return m.store.Save(cfg)
}

// RefreshAllROA forces a refresh of all ROA URLs configured across all policies
func (m *Manager) RefreshAllROA(ctx context.Context, force bool) error {
	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return err
		}
	}
	allPolicies := cfg.GetAllPolicies()
	for _, p := range allPolicies {
		if strings.TrimSpace(p.ROA4) != "" {
			_, _ = m.roaManager.GetROAContent(ctx, p.ROA4, force)
		}
		if strings.TrimSpace(p.ROA6) != "" {
			_, _ = m.roaManager.GetROAContent(ctx, p.ROA6, force)
		}
	}
	return nil
}

// RefreshPolicyROA refreshes the ROA cache for a specific policy ID
func (m *Manager) RefreshPolicyROA(ctx context.Context, policyID string) error {
	cfg := m.store.Get()
	if cfg == nil {
		var err error
		cfg, err = m.store.Load()
		if err != nil {
			return err
		}
	}
	p := cfg.FindPolicy(policyID)
	if p == nil {
		return fmt.Errorf("policy '%s' not found", policyID)
	}
	if strings.TrimSpace(p.ROA4) != "" {
		if _, err := m.roaManager.GetROAContent(ctx, p.ROA4, true); err != nil {
			return err
		}
	}
	if strings.TrimSpace(p.ROA6) != "" {
		if _, err := m.roaManager.GetROAContent(ctx, p.ROA6, true); err != nil {
			return err
		}
	}
	return nil
}

// StartROABackgroundRefresher runs a periodic background task to refresh ROA URL caches
func (m *Manager) StartROABackgroundRefresher(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 1 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = m.RefreshAllROA(ctx, true)
			}
		}
	}()
}

func actionPriority(t config.ActionType) int {
	switch t {
	case config.ActionDeleteConfig, config.ActionDownInterface:
		return 1
	case config.ActionCreateConfig, config.ActionUpdateConfig, config.ActionSyncConfig, config.ActionUpInterface:
		return 2
	case config.ActionSyncRoaConfig:
		return 3
	case config.ActionSyncBirdConfig:
		return 4
	case config.ActionSyncNftablesConfig:
		return 5
	default:
		return 6
	}
}
